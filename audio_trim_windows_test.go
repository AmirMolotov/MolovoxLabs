//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioBitePreservesSourceAndCreatesPlayableCopy(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	source := filepath.Join(t.TempDir(), "Original.wav")
	original := samplerWavFixture(9600)
	os.WriteFile(source, original, 0600)
	l, err := s.ImportFile("default", source)
	if err != nil {
		t.Fatal(err)
	}
	id := l.Boards[0].Sounds[0].ID
	l, err = s.CreateAudioBite("default", "Cut", samplerWavFixture(4800))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Boards[0].Sounds) != 2 {
		t.Fatal("Original pad was replaced")
	}
	bite := l.Boards[0].Sounds[1]
	samples, err := s.Sound(bite.ID)
	if err != nil || len(samples) != 4800 {
		t.Fatal("Native playback duration incorrect", err)
	}
	path, err := s.AudioClipPath(id)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, original) {
		t.Fatal("Original pad changed")
	}
	data, _ = os.ReadFile(source)
	if !bytes.Equal(data, original) {
		t.Fatal("Original file changed")
	}
	if _, err = s.CreateAudioBite("missing", "Cut", original); err == nil {
		t.Fatal("Missing board accepted")
	}
	if _, err = s.AudioClipPath("../soundboards.json"); err == nil {
		t.Fatal("Path accepted as ID")
	}
}
func TestAudioBiteHTTPRequiresOriginAndReloadsNativeTriggers(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	changed := 0
	h := (&samplerHTTP{prefix: "/session/", store: s, onLibraryChanged: func() error { changed++; return nil }}).handler(http.NotFoundHandler())
	request := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://local/session/sampler/bite", bytes.NewReader(samplerWavFixture(4800)))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Bite-Board", "default")
		r.Header.Set("X-Bite-Name", base64.StdEncoding.EncodeToString([]byte("A bite")))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("http://elsewhere"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if changed != 0 {
		t.Fatal("Cross-origin write")
	}
	if w := request("http://local"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if changed != 1 {
		t.Fatal("Native shortcut cache not reloaded")
	}
}
