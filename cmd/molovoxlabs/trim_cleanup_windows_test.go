//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrimCleanupRemovesCopiedSourceAndCachesButKeepsOriginalAndPads(t *testing.T) {
	root := t.TempDir()
	s := &appStore{root: filepath.Join(root, "trim-cleanup-test-profile")}
	original := filepath.Join(root, "original.wav")
	os.WriteFile(original, wavFixture(), 0600)
	source := VideoSource{ID: "temporary", Name: "Track", Kind: "audio", File: "temporary.wav"}
	s.write("sampler-sources.json", []VideoSource{source})
	raw := filepath.Join(s.root, "sampler", "videos", source.File)
	os.MkdirAll(filepath.Dir(raw), 0700)
	os.WriteFile(raw, wavFixture(), 0600)
	os.MkdirAll(filepath.Dir(s.mediaPath(source.ID, ".f32")), 0700)
	for _, ext := range []string{".f32", ".peaks", ".json", ".mp4", ".part.f32"} {
		os.WriteFile(s.mediaPath(source.ID, ext), []byte("cache"), 0600)
	}
	pad := filepath.Join(s.root, "sounds", "bite.wav")
	os.MkdirAll(filepath.Dir(pad), 0700)
	os.WriteFile(pad, wavFixture(), 0600)
	s.write("soundboards.json", libraryState{Boards: []SoundBoard{{ID: "board", Sounds: []SoundEntry{{ID: "bite", File: "bite.wav"}}}}})
	if err := s.ReleaseAudioTrimSource(source.ID); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{raw, s.mediaPath(source.ID, ".f32"), s.mediaPath(source.ID, ".peaks"), s.mediaPath(source.ID, ".json"), s.mediaPath(source.ID, ".mp4"), s.mediaPath(source.ID, ".part.f32")} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("temporary file retained", file, err)
		}
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatal("original removed", err)
	}
	if _, err := s.Sound("bite"); err != nil {
		t.Fatal("saved bite lost", err)
	}
	state, err := s.SamplerLibrary()
	if err != nil || len(state.Sources) != 0 {
		t.Fatal("temporary source metadata retained", err)
	}
	if err = s.ReleaseAudioTrimSource(source.ID); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
}
func TestTrimCleanupProtectsVideoSourcesAndReferencedMedia(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		referenced bool
	}{{"", false}, {"audio", true}} {
		s := &appStore{root: t.TempDir()}
		s.write("sampler-sources.json", []VideoSource{{ID: "keep", Kind: tc.kind, File: "keep.wav"}})
		l := libraryState{Boards: []SoundBoard{{ID: "board"}}}
		if tc.referenced {
			l.Boards[0].Sounds = []SoundEntry{{ID: "video", Video: &VideoSample{SourceID: "keep"}}}
		}
		s.write("soundboards.json", l)
		if err := s.ReleaseAudioTrimSource("keep"); err == nil {
			t.Fatal("protected media accepted")
		}
		state, err := s.SamplerLibrary()
		if err != nil || len(state.Sources) != 1 {
			t.Fatal("protected source removed", err)
		}
	}
}
