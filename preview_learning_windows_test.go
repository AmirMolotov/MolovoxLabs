//go:build windows

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativePreviewLoopsAndCanBeStopped(t *testing.T) {
	e := &nativeEngine{}
	e.running.Store(true)
	samples := make([]float32, 960)
	for i := range samples {
		samples[i] = .5
	}
	if err := e.PlayPreview("@audio-preview", samples, "board", true); err != nil {
		t.Fatal(err)
	}
	r := &renderEndpoint{}
	e.renders = []*renderEndpoint{r}
	voices := e.voices.Load()
	for i := 0; i < 960*3+480; i++ {
		r.boardSample(voices)
	}
	if r.soundAt[0] != 480 {
		t.Fatal("Native loop did not wrap", r.soundAt[0])
	}
	snapshot := e.Snapshot()
	if len(snapshot.Playback) != 1 || snapshot.Playback[0].Position != .01 {
		t.Fatal("Loop tracker did not follow the current cycle", snapshot.Playback)
	}
	voices.Slots[0].Started = time.Now().Add(-time.Hour)
	if err := e.PlaySound("another", samples); err != nil {
		t.Fatal(err)
	}
	if e.voices.Load().Slots[0].ID != "@audio-preview" {
		t.Fatal("An old active loop was reclaimed as an expired clip")
	}
	e.StopSound("@audio-preview")
	if e.voices.Load().Slots[0].Sequence != 0 {
		t.Fatal("Loop did not stop")
	}
}
func TestPreviewEndpointAcceptsNativeLoopFlag(t *testing.T) {
	e := &nativeEngine{}
	e.running.Store(true)
	handler := (&samplerHTTP{prefix: "/session/", engine: e}).handler(http.NotFoundHandler())
	r := httptest.NewRequest("POST", "http://local/session/sampler/preview", bytes.NewReader(samplerWavFixture(4800)))
	r.Header.Set("Origin", "http://local")
	r.Header.Set("X-Preview-ID", "@audio-preview")
	r.Header.Set("X-Preview-Loop", "true")
	r.Header.Set("X-Sampler-Board", "test-board")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	voice := e.voices.Load().Slots[0]
	if !voice.Loop || voice.ID != "@audio-preview" || voice.BoardID != "test-board" {
		t.Fatal("Preview loop settings were lost")
	}
}
func TestMissingClipDeletionIsPersistentAndIdempotent(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	l := libraryState{Boards: []SoundBoard{{ID: "audio", Sounds: []SoundEntry{{ID: "missing", File: "gone.wav"}}}, {ID: "video", Kind: "video", Sounds: []SoundEntry{{ID: "missing-video", File: "gone-too.wav", Video: &VideoSample{SourceID: "source"}}}}}}
	if err := s.write("soundboards.json", l); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"missing", "missing-video", "missing"} {
		updated, err := s.RemoveSound(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range updated.Boards {
			for _, clip := range b.Sounds {
				if clip.ID == id {
					t.Fatal("Missing-file pad remained in returned library")
				}
			}
		}
	}
	saved, err := s.Library()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range saved.Boards {
		if len(b.Sounds) != 0 {
			t.Fatal("Deleted stale pads returned after reload")
		}
	}
}
func TestShortcutSuspensionDoesNotReadBrokenLibrary(t *testing.T) {
	keys := newGlobalKeys(func(string) {})
	defer keys.Close()
	s := &appStore{root: t.TempDir()}
	os.WriteFile(filepath.Join(s.root, "soundboards.json"), []byte("invalid-json"), 0600)
	service := &triggerService{keys: keys, store: s}
	if err := service.SuspendShortcuts(true); err != nil {
		t.Fatal(err)
	}
	if !service.suspended.Load() {
		t.Fatal("Shortcuts not suspended")
	}
	if err := service.SuspendShortcuts(false); err != nil {
		t.Fatal(err)
	}
}
func TestUnavailableMIDIIsReportedWithoutBlockingKeyboardConfiguration(t *testing.T) {
	service := newTriggerService(&appStore{root: t.TempDir()}, &nativeEngine{})
	defer service.Close()
	if err := service.Configure(Preferences{MIDIInput: "QuietLab test disconnected MIDI device"}); err != nil {
		t.Fatal("Disconnected MIDI blocked keyboard setup", err)
	}
	if !strings.Contains(service.Status().Message, "MIDI unavailable") {
		t.Fatal("Missing MIDI was not reported")
	}
	if err := service.Learn(true); err == nil {
		t.Fatal("MIDI learning started without a connected controller")
	}
	if err := service.Learn(false); err != nil {
		t.Fatal(err)
	}
}
