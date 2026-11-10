//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveBoardDeletesOnlyItsUnsharedCopiedAudio(t *testing.T) {
	root := t.TempDir()
	s := &appStore{root: filepath.Join(root, "profile")}
	sounds := filepath.Join(s.root, "sounds")
	os.MkdirAll(sounds, 0700)
	original := filepath.Join(root, "original.wav")
	outside := filepath.Join(s.root, "outside.wav")
	for _, path := range []string{original, outside, filepath.Join(sounds, "gone.wav"), filepath.Join(sounds, "shared.wav")} {
		os.WriteFile(path, []byte("audio"), 0600)
	}
	l := libraryState{Boards: []SoundBoard{
		{ID: "delete", Sounds: []SoundEntry{{ID: "gone", File: "gone.wav"}, {ID: "shared-one", File: "shared.wav"}, {ID: "unsafe", File: "../outside.wav"}}},
		{ID: "keep", Sounds: []SoundEntry{{ID: "shared-two", File: "shared.wav"}}},
	}}
	if err := s.write("soundboards.json", l); err != nil {
		t.Fatal(err)
	}
	result, err := s.RemoveBoard("delete")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Library.Boards) != 1 || result.Library.Boards[0].ID != "keep" || len(result.Warnings) != 0 {
		t.Fatal("Incorrect board removal", result)
	}
	if _, err = os.Stat(filepath.Join(sounds, "gone.wav")); !os.IsNotExist(err) {
		t.Fatal("Copied audio not removed")
	}
	for _, path := range []string{original, outside, filepath.Join(sounds, "shared.wav")} {
		if _, err = os.Stat(path); err != nil {
			t.Fatal("Unrelated or shared audio removed", path, err)
		}
	}
	loaded, err := s.Library()
	if err != nil || len(loaded.Boards) != 1 || loaded.Boards[0].ID != "keep" {
		t.Fatal("Deletion did not persist", err)
	}
	if _, err = s.RemoveBoard("missing"); err == nil {
		t.Fatal("Unknown board accepted")
	}
}

func TestRemoveLastBoardCreatesFreshEmptyBoardAndReportsCleanupFailure(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	locked := filepath.Join(s.root, "sounds", "blocked.wav")
	os.MkdirAll(locked, 0700)
	os.WriteFile(filepath.Join(locked, "file"), []byte("keep"), 0600)
	l := libraryState{Boards: []SoundBoard{{ID: "last", Sounds: []SoundEntry{{ID: "clip", File: "blocked.wav"}}}}}
	if err := s.write("soundboards.json", l); err != nil {
		t.Fatal(err)
	}
	result, err := s.RemoveBoard("last")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Library.Boards) != 1 || result.Library.Boards[0].ID == "last" || len(result.Library.Boards[0].Sounds) != 0 {
		t.Fatal("No fresh board after deleting last board")
	}
	if len(result.Warnings) != 1 {
		t.Fatal("Cleanup failure not reported", result.Warnings)
	}
}
