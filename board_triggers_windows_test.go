//go:build windows

package main

import "testing"

func TestTriggersFollowSelectedBoard(t *testing.T) {
	key := KeyBinding{Key: 49, Modifiers: 3}
	midi := MIDIBinding{Kind: "note", Channel: 1, Number: 36}
	l := libraryState{Boards: []SoundBoard{
		{ID: "first", Sounds: []SoundEntry{{ID: "one", Hotkeys: []KeyBinding{key}, MIDIs: []MIDIBinding{midi}}}},
		{ID: "second", Sounds: []SoundEntry{{ID: "two", Hotkeys: []KeyBinding{key}, MIDIs: []MIDIBinding{midi}}}},
	}}
	for _, board := range []string{"first", "second", "missing"} {
		p := Preferences{BoardID: board, AllFilterHotkeys: []KeyBinding{{Key: 50, Modifiers: 3}}}
		active := activeTriggerLibrary(l, p)
		want := "one"
		if board == "second" {
			want = "two"
		}
		if len(active.Boards) != 2 || len(active.Boards[0].Sounds) != 1 || active.Boards[0].Sounds[0].ID != want {
			t.Fatalf("Wrong active board: %+v", active)
		}
		if len(soundKeys(active.Boards[0].Sounds[0])) != 1 || len(soundMIDIs(active.Boards[0].Sounds[0])) != 1 {
			t.Fatal("Selected triggers lost")
		}
		if active.Boards[1].Sounds[0].ID != "@all" {
			t.Fatal("Global filter shortcuts lost")
		}
	}
	service := &triggerService{}
	service.maps.Store(&triggerMap{Sounds: map[string]bool{"two": true}})
	// No store/engine: an old queued event must return before touching playback.
	if err := service.Run("one"); err != nil {
		t.Fatal(err)
	}
	service.maps.Store(&triggerMap{Sounds: map[string]bool{"one": true}})
	if err := service.Run("two"); err != nil {
		t.Fatal(err)
	}
}
