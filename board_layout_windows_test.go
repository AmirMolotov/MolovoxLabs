//go:build windows

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBoardLayoutPersistsOrderGroupsAndLeavesSoundDataIntact(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	original := libraryState{Boards: []SoundBoard{{ID: "board", Sounds: []SoundEntry{{ID: "a", Name: "Alpha", File: "a.wav", Hotkeys: []KeyBinding{{Key: 65, Modifiers: 2}}}, {ID: "b", Name: "Beta", File: "b.wav"}}}}}
	if err := s.write("soundboards.json", original); err != nil {
		t.Fatal(err)
	}
	groups := []PadGroup{{ID: "g", Name: "  Reactions  ", Collapsed: true}}
	_, err := s.SaveBoardLayout("board", []PadPlacement{{ID: "b", GroupID: "g"}, {ID: "a"}}, groups)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Library()
	if err != nil {
		t.Fatal(err)
	}
	b := loaded.Boards[0]
	if b.Sounds[0].ID != "b" || b.Sounds[0].GroupID != "g" || b.Groups[0].Name != "Reactions" || !b.Groups[0].Collapsed {
		t.Fatal("layout was not persisted", b)
	}
	if !reflect.DeepEqual(b.Sounds[1], original.Boards[0].Sounds[0]) {
		t.Fatal("media or triggers changed")
	}
	// Dissolving a group keeps its clips and their order.
	_, err = s.SaveBoardLayout("board", []PadPlacement{{ID: "b"}, {ID: "a"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _ = s.Library()
	if len(loaded.Boards[0].Sounds) != 2 || loaded.Boards[0].Sounds[0].GroupID != "" {
		t.Fatal("ungroup lost audio")
	}
}
func TestBoardLayoutRejectsStaleDuplicateAndForeignPlacementsAtomically(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    []PadPlacement
		g    []PadGroup
	}{
		{"missing", nil, nil}, {"duplicate", []PadPlacement{{ID: "a"}, {ID: "a"}}, nil},
		{"foreign", []PadPlacement{{ID: "a"}, {ID: "elsewhere"}}, nil},
		{"unknown group", []PadPlacement{{ID: "a", GroupID: "g"}, {ID: "b"}}, nil},
		{"duplicate group", []PadPlacement{{ID: "a"}, {ID: "b"}}, []PadGroup{{ID: "g", Name: "One"}, {ID: "g", Name: "Two"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &appStore{root: t.TempDir()}
			original := libraryState{Boards: []SoundBoard{{ID: "board", Sounds: []SoundEntry{{ID: "a"}, {ID: "b"}}}}}
			s.write("soundboards.json", original)
			before, _ := os.ReadFile(filepath.Join(s.root, "soundboards.json"))
			if _, err := s.SaveBoardLayout("board", tc.p, tc.g); err == nil {
				t.Fatal("invalid layout accepted")
			}
			after, _ := os.ReadFile(filepath.Join(s.root, "soundboards.json"))
			if string(before) != string(after) {
				t.Fatal("failed save changed library")
			}
		})
	}
}
func TestBoardLayoutTravelsWithEmbeddedAudioExport(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	os.MkdirAll(filepath.Join(s.root, "sounds"), 0700)
	os.WriteFile(filepath.Join(s.root, "sounds", "a.wav"), wavFixture(), 0600)
	board := SoundBoard{ID: "board", Name: "Reactions", Groups: []PadGroup{{ID: "g", Name: "Wins", Collapsed: true}}, Sounds: []SoundEntry{{ID: "a", Name: "Win", File: "a.wav", GroupID: "g"}}}
	s.write("soundboards.json", libraryState{Boards: []SoundBoard{board}})
	archive := filepath.Join(t.TempDir(), "board.qlboard")
	if err := s.ExportBoard("board", archive); err != nil {
		t.Fatal(err)
	}
	dest := &appStore{root: t.TempDir()}
	loaded, err := dest.ImportBoard(archive)
	if err != nil {
		t.Fatal(err)
	}
	imported := loaded.Boards[len(loaded.Boards)-1]
	if !reflect.DeepEqual(imported.Groups, board.Groups) || imported.Sounds[0].GroupID != "g" || imported.Sounds[0].ID == "a" {
		t.Fatal("import lost grouping or reused audio ID", imported)
	}
}
