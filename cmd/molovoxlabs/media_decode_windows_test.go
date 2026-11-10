//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func generateMedia(t *testing.T, name string, args ...string) string {
	t.Helper()
	if _, err := mediaTool("ffmpeg"); err != nil {
		t.Skip("Installed FFmpeg is unavailable")
	}
	if _, err := mediaTool("ffprobe"); err != nil {
		t.Skip("Installed ffprobe is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	path := filepath.Join(t.TempDir(), name)
	options := append([]string{"-v", "error", "-nostdin", "-y"}, args...)
	options = append(options, path)
	cmd, err := mediaCommand(ctx, "ffmpeg", options...)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	return path
}
func TestNativeMediaConvertsVideoAndSavesEditedPad(t *testing.T) {
	path := generateMedia(t, "test-codec.avi", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=660:sample_rate=44100", "-t", "1.5", "-c:v", "mpeg4", "-q:v", "3", "-c:a", "pcm_s16le")
	s := &appStore{root: filepath.Join(t.TempDir(), "native-media-test-profile")}
	source, err := s.ImportVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.PrepareMedia(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio || !info.HasVideo || info.Frames < 48000 || info.Seconds < 1 {
		t.Fatal(info)
	}
	proxy, err := s.SourcePath(source.ID)
	if err != nil || !strings.HasSuffix(proxy, ".mp4") {
		t.Fatal("No seekable compatible proxy", proxy, err)
	}
	detail, err := s.MediaWave(source.ID, .2, .7, 1000)
	if err != nil || len(detail) != 1000 {
		t.Fatal(err)
	}
	var peak float32
	for _, p := range detail {
		peak = max(peak, p)
	}
	if peak < .05 {
		t.Fatal("Native audio waveform is empty")
	}
	coarse, err := s.MediaWave(source.ID, 0, info.Seconds, 8)
	if err != nil || len(coarse) != 8 {
		t.Fatal(err)
	}
	wav, err := s.MediaWAV(source.ID, .2, .7, false)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := decodeSound(wav, ".wav")
	if err != nil || len(pcm) != 24000 {
		t.Fatal("Selection decode failed", len(pcm), err)
	}
	state, err := s.SamplerLibrary()
	if err != nil {
		t.Fatal(err)
	}
	meta := CreateVideoSample{BoardID: state.Library.Boards[1].ID, Name: "Edited AVI sample", Video: VideoSample{SourceID: source.ID, In: .2, Out: .7, Crop: VideoCrop{X: .1, Y: .1, Width: .6, Height: .7}}}
	body, _ := json.Marshal(meta)
	request := httptest.NewRequest("POST", "http://local/session/sampler/create", bytes.NewReader(body))
	request.Header.Set("Origin", "http://local")
	request.Header.Set("Content-Type", "application/json")
	changed := 0
	handler := (&samplerHTTP{prefix: "/session/", store: s, onLibraryChanged: func() error { changed++; return nil }}).handler(http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var saved libraryState
	if err = json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	pad := saved.Boards[1].Sounds[0]
	if pad.Name != meta.Name || pad.Video == nil || pad.Video.In != .2 || pad.Video.Crop.Width != .6 || !strings.HasPrefix(pad.Video.Thumbnail, "data:image/jpeg;base64,") {
		t.Fatal("Edited video pad was not returned with its crop and thumbnail")
	}
	if changed != 1 {
		t.Fatal("New video pad was not configured for native playback")
	}
	state, err = s.SamplerLibrary()
	if err != nil || len(state.Library.Boards[1].Sounds) != 1 {
		t.Fatal("Saved video pad disappeared on reload", err)
	}
	if pcm, err = s.Sound(pad.ID); err != nil || len(pcm) != 24000 {
		t.Fatal("Edited pad audio cannot play", err)
	}
}
func TestNativeAudioImportBeyondOldThirtyMinuteLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Long-source streaming integration")
	}
	path := generateMedia(t, "long-source.wav", "-f", "lavfi", "-i", "anullsrc=r=1000:cl=mono", "-t", "1801", "-c:a", "pcm_u8")
	s := &appStore{root: filepath.Join(t.TempDir(), "long-audio-test-profile")}
	info, err := s.ImportAudioMedia(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Seconds <= 1800 || info.Frames < 1800*sampleRate {
		t.Fatal("Long audio was rejected or truncated", info)
	}
	wav, err := s.MediaWAV(info.ID, 1800, 1800.1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) != 44+4800*2 {
		t.Fatal("Late selection was not extracted independently")
	}
	peaks, err := s.MediaWave(info.ID, 1800, 1801, 1000)
	if err != nil || len(peaks) != 1000 {
		t.Fatal(err)
	}
	state, err := s.SamplerLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if state.Sources[0].Kind != "audio" {
		t.Fatal("Soundboard source was not classified separately")
	}
}
func TestNativeWaveformUsesOriginalPCMAtDeepZoom(t *testing.T) {
	s, _, source := samplerFixture(t)
	dir := filepath.Dir(s.mediaPath(source.ID, ".json"))
	os.MkdirAll(dir, 0700)
	info := MediaInfo{ID: source.ID, Seconds: 1, HasAudio: true, Frames: 48000}
	data, _ := json.Marshal(info)
	os.WriteFile(s.mediaPath(source.ID, ".json"), data, 0600)
	pcm := make([]byte, 48000*4)
	binary.LittleEndian.PutUint32(pcm[24001*4:], math.Float32bits(.87))
	os.WriteFile(s.mediaPath(source.ID, ".f32"), pcm, 0600)
	peaks, err := s.MediaWave(source.ID, .5, .51, 480)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(peaks[1]-.87)) > 1e-6 {
		t.Fatal("Individual sample detail was lost at high zoom", peaks[:3])
	}
	if _, err = s.MediaWAV("../preferences", 0, .1, false); err == nil {
		t.Fatal("Unregistered source path accepted")
	}
	if _, err = s.MediaWave(source.ID, math.NaN(), .5, 100); err == nil {
		t.Fatal("Invalid range accepted")
	}
}
