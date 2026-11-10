//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDroppedAudioUsesNativeMediaAndCleansStagedUpload(t *testing.T) {
	if _, err := mediaTool("ffmpeg"); err != nil {
		t.Skip("FFmpeg unavailable")
	}
	if _, err := mediaTool("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	s := &appStore{root: filepath.Join(t.TempDir(), "dropped-audio-test-profile")}
	handler := (&samplerHTTP{prefix: "/token/", store: s}).handler(http.NotFoundHandler())
	request := httptest.NewRequest("POST", "http://local/token/sampler/audio-import?name=my%20source.wav", bytes.NewReader(samplerWavFixture(48000)))
	request.Header.Set("Origin", "http://local")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var info MediaInfo
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.ID == "" || info.Name != "my source" || !info.HasAudio || info.Seconds < .99 {
		t.Fatal("incorrect imported source", info)
	}
	source, err := s.rawSourcePath(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatal("source lost after upload cleanup", err)
	}
	wav, err := s.MediaWAV(info.ID, .2, .4, false)
	if err != nil || len(wav) != 44+9600*2 {
		t.Fatal("source cannot be trimmed", err, len(wav))
	}
	staged, _ := filepath.Glob(filepath.Join(s.root, ".trim-upload-*"))
	if len(staged) != 0 {
		t.Fatal("staged upload not cleaned", staged)
	}
	state, err := s.SamplerLibrary()
	if err != nil || state.Sources[0].Kind != "audio" {
		t.Fatal("incorrect source classification", err)
	}
}
func TestDroppedAudioRejectsUnsupportedFilesAndForeignOrigins(t *testing.T) {
	s := &appStore{root: filepath.Join(t.TempDir(), "rejected-upload-test-profile")}
	handler := (&samplerHTTP{prefix: "/token/", store: s}).handler(http.NotFoundHandler())
	for _, tc := range []struct {
		name, origin string
		code         int
	}{{"app.exe", "http://local", 400}, {"clip.wav", "http://elsewhere", 403}, {"../app.exe", "", 400}} {
		request := httptest.NewRequest("POST", "http://local/token/sampler/audio-import?name="+tc.name, bytes.NewReader([]byte("not media")))
		if tc.origin != "" {
			request.Header.Set("Origin", tc.origin)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.code {
			t.Fatal(tc, response.Code)
		}
	}
	if _, err := os.Stat(s.root); !os.IsNotExist(err) {
		t.Fatal("rejected upload created profile files")
	}
}
