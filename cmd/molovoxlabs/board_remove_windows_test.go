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

func TestRemovalHistoryRestoresPadsAndBoardsAfterRestart(t *testing.T) {
	root := t.TempDir()
	s := &appStore{root: root}
	dir := filepath.Join(root, "sounds")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "clip.wav")
	if err := os.WriteFile(file, []byte("copied audio"), 0600); err != nil {
		t.Fatal(err)
	}
	board := SoundBoard{ID: "board", Name: "Original", Sounds: []SoundEntry{{ID: "pad", File: "clip.wav", Hotkeys: []KeyBinding{{Key: 80, Modifiers: 2}}}}}
	if err := s.write("soundboards.json", libraryState{Boards: []SoundBoard{board}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.removeLibraryItem("", "pad", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.removeLibraryItem("board", "", true); err != nil {
		t.Fatal(err)
	}
	s = &appStore{root: root}
	restored, err := s.changeRemovalHistory(false)
	if err != nil || len(restored.Library.Boards) != 1 || restored.Library.Boards[0].ID != board.ID {
		t.Fatalf("undo board: %+v, %v", restored, err)
	}
	restored, err = s.changeRemovalHistory(false)
	if err != nil || len(restored.Library.Boards[0].Sounds) != 1 || restored.Library.Boards[0].Sounds[0].Hotkeys[0].Key != 80 {
		t.Fatalf("undo pad: %+v, %v", restored, err)
	}
	redone, err := s.changeRemovalHistory(true)
	if err != nil || len(redone.Library.Boards[0].Sounds) != 0 {
		t.Fatalf("redo pad: %+v, %v", redone, err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("undo must retain copied audio:", err)
	}
}
