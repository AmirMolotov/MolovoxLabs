//go:build windows

package main

import (
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSoundboardRecorderTapsMicrophonesWithoutPadsOrEffects(t *testing.T) {
	captures := []*captureEndpoint{{gain: .5}, {gain: 1}}
	for i, capture := range captures {
		pcm := make([]byte, bridgeFrames*4)
		level := []float32{.4, .1}[i]
		for j := 0; j < len(pcm); j += 4 {
			binary.LittleEndian.PutUint32(pcm[j:], math.Float32bits(level))
		}
		capture.ring.push(pcm)
	}
	render := &renderEndpoint{ID: "out", readers: make([]ringReader, len(captures))}
	e := &nativeEngine{captures: captures, renders: []*renderEndpoint{render}}
	e.running.Store(true)
	defer e.vst.close()
	if err := e.Configure(AudioOptions{Volume: .5, BoardVolume: 1, Filters: []MicFilter{{Kind: "gain", Enabled: true, GainDB: 20}}}); err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 1000)
	for i := range samples {
		samples[i] = .7
	}
	if err := e.PlaySound("pad", samples); err != nil {
		t.Fatal(err)
	}
	id, err := e.beginMicRecording()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.beginMicRecording(); err == nil {
		t.Fatal("Accepted a second microphone recording")
	}
	if err = e.beginSamplerTap("video", "out"); err != nil {
		t.Fatal(err)
	}
	output := make([]byte, 480*8)
	e.render(render, output, 480, true)
	data, cursor, dropped, err := e.micRecordingAudio(id, 0, false)
	if err != nil || cursor != 480 || dropped != 0 {
		t.Fatal(cursor, dropped, err)
	}
	got := math.Float32frombits(binary.LittleEndian.Uint32(data[300*4:]))
	if math.Abs(float64(got-.15)) > 1e-6 {
		t.Fatal("Wrong raw microphone mix", got)
	}
	mixed, _, _, err := e.samplerAudio("video", 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(mixed[300*4:]))-.35)) > 1e-6 {
		t.Fatal("Video output mix changed")
	}
	handler := (&samplerHTTP{prefix: "/local/", engine: e}).handler(http.NotFoundHandler())
	finish := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/local/sampler/mic-finish/"+id+"?cursor=300", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if response := finish("http://other"); response.Code != 403 {
		t.Fatal("Cross-origin finish succeeded", response.Code)
	}
	if response := finish("http://localhost"); response.Code != 200 || response.Body.Len() != 180*4 {
		t.Fatal("Final microphone frames were lost", response.Code, response.Body.Len())
	}
	if _, _, _, err = e.micRecordingAudio(id, 0, false); err == nil {
		t.Fatal("Finished microphone recording remained active")
	}
	if e.samplerTap.Load() == nil {
		t.Fatal("Stopping microphone recording stopped video capture")
	}
}
