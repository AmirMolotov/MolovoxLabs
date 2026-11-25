//go:build windows && amd64

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAppMixerIcon(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	icon := readAppMixerIcon(exe)
	le := binary.LittleEndian
	if len(icon) <= 22 || le.Uint16(icon[2:4]) != 1 || le.Uint16(icon[4:6]) != 1 ||
		le.Uint32(icon[14:18]) != uint32(len(icon)-22) || le.Uint32(icon[18:22]) != 22 {
		t.Fatal("Expected a single-image ICO from the executable's embedded icon")
	}
	if icon := readAppMixerIcon(filepath.Join(t.TempDir(), "missing.exe")); icon != nil {
		t.Fatal("Expected no icon for a missing executable")
	}
}

func TestAppMixerBindingsJoinGlobalShortcutAndMIDIValidation(t *testing.T) {
	p := Preferences{}
	target := `@app:30:C:\Audio\Player.exe`
	key := KeyBinding{Key: 80, Modifiers: 2}
	midi := MIDIBinding{Kind: "note", Channel: 1, Number: 42}
	if err := updateAppMixerBindings(&p, target, []KeyBinding{key}, []MIDIBinding{midi}); err != nil {
		t.Fatal(err)
	}
	path, percent, err := appMixerTarget(target)
	if err != nil || path != `C:\Audio\Player.exe` || percent != 30 {
		t.Fatalf("incorrect mixer target: %q %d %v", path, percent, err)
	}
	board := activeTriggerLibrary(libraryState{}, p).Boards[0]
	binding := board.Sounds[len(board.Sounds)-1]
	if binding.ID != target || soundKeys(binding)[0] != key || soundMIDIs(binding)[0] != midi {
		t.Fatalf("missing mixer shortcut: %+v", binding)
	}
	p.AllFilterMIDIs = []MIDIBinding{midi}
	if err := validateBindings(triggerLibrary(libraryState{}, p)); err == nil {
		t.Fatal("duplicate MIDI trigger should be rejected")
	}
	if err := updateAppMixerBindings(&p, target, nil, nil); err != nil || len(p.AppMixerShortcuts) != 0 {
		t.Fatalf("clear assignment: %+v, %v", p.AppMixerShortcuts, err)
	}
	if _, _, err := appMixerTarget("@app:17:relative.exe"); err == nil {
		t.Fatal("invalid path and volume accepted")
	}
}

func TestMixerKnobsAndGroupBindings(t *testing.T) {
	group := appMixerGroup{ID: "music", Name: "Music", Paths: []string{`C:\Music.exe`, `C:\Radio.exe`}}
	p := Preferences{AppMixerGroups: []appMixerGroup{group}}
	binding := MIDIBinding{Kind: "cc", Channel: 1, Number: 12}
	if err := updateAppMixerBindings(&p, "@app:group-knob:music", nil, []MIDIBinding{binding}); err != nil {
		t.Fatal(err)
	}
	if err := validateAppMixerConfig(p); err != nil {
		t.Fatal(err)
	}
	if err := updateAppMixerBindings(&p, "@app:group-up:missing", nil, []MIDIBinding{binding}); err == nil {
		t.Fatal("missing group accepted")
	}
	if err := updateAppMixerBindings(&p, "@app:group-encoder:music", nil, []MIDIBinding{{Kind: "note", Channel: 1, Number: 12}}); err == nil {
		t.Fatal("a note should not be assigned to a knob")
	}
	for _, check := range []struct {
		mode                   string
		previous, value, delta int
	}{
		{"knob", -1, 100, 0}, {"knob", 57, 64, 10}, {"knob", 64, 57, -10},
		{"encoder", 1, 1, 10}, {"encoder", 127, 127, -10},
		{"centered", 64, 65, 10}, {"centered", 64, 63, -10},
		{"up", 0, 0, 10}, {"down", 0, 0, -10},
	} {
		if got := appMixerDelta(check.mode, midiTriggerEvent{Binding: binding, Previous: check.previous, Value: check.value}); got != check.delta {
			t.Errorf("%+v: got %d", check, got)
		}
	}
	p.AppMixerGroups = append(p.AppMixerGroups, appMixerGroup{ID: "duplicate", Name: "Duplicate", Paths: []string{`c:\music.exe`}})
	if err := validateAppMixerConfig(p); err == nil {
		t.Fatal("an app should not belong to more than one group")
	}
}
