//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoardExportContainsOriginalAudioAndMultipleTriggers(t *testing.T) {
	root := t.TempDir()
	s := &appStore{root: filepath.Join(root, "source")}
	original := wavFixture()
	path := filepath.Join(root, "clip.wav")
	os.WriteFile(path, original, 0600)
	l, err := s.ImportFile("default", path)
	if err != nil {
		t.Fatal(err)
	}
	id := l.Boards[0].Sounds[0].ID
	keys := []KeyBinding{{Key: 49, Modifiers: 3, Label: "Ctrl+Alt+1"}, {Key: 50, Modifiers: 3, Label: "Ctrl+Alt+2"}}
	midis := []MIDIBinding{{Kind: "note", Channel: 1, Number: 36}, {Kind: "note", Channel: 1, Number: 37}}
	if _, err = s.SetMultipleBindings(id, keys, midis); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "export.qlboard")
	if err = s.ExportBoard("default", archive); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	embedded := false
	for _, f := range z.File {
		if filepath.Ext(f.Name) == ".wav" {
			r, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			data, e := io.ReadAll(r)
			r.Close()
			if e != nil || !bytes.Equal(data, original) {
				t.Fatal("Export did not include original audio bytes")
			}
			embedded = true
		}
	}
	z.Close()
	if !embedded {
		t.Fatal("No audio file embedded in board package")
	}
	standalone := filepath.Join(root, "standalone.wav")
	if err = s.ExportAudio(id, standalone); err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(standalone)
	if !bytes.Equal(copied, original) {
		t.Fatal("Standalone audio export changed the file")
	}
	os.Remove(path)
	destination := &appStore{root: filepath.Join(root, "destination")}
	imported, err := destination.ImportBoard(archive)
	if err != nil {
		t.Fatal(err)
	}
	sound := imported.Boards[len(imported.Boards)-1].Sounds[0]
	if len(soundKeys(sound)) != 2 || len(soundMIDIs(sound)) != 2 {
		t.Fatal("Multiple trigger assignments not preserved")
	}
	if _, err = destination.Sound(sound.ID); err != nil {
		t.Fatal("Imported board depended on source audio", err)
	}
	if len(sound.Waveform) != 40 {
		t.Fatal("Missing waveform preview")
	}
}

func TestTwoGlobalShortcutsToggleSameFilterAndPersist(t *testing.T) {
	store := &appStore{root: t.TempDir()}
	p := Preferences{GlobalKeys: true, Options: AudioOptions{Volume: 1, BoardVolume: 1, Filters: []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, Hotkeys: []KeyBinding{{Key: 0x84, Modifiers: 3, Label: "Ctrl+Alt+F21"}, {Key: 0x85, Modifiers: 3, Label: "Ctrl+Alt+F22"}}}}}}
	if err := store.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	engine := &nativeEngine{}
	engine.Configure(p.Options)
	service := &triggerService{store: store, engine: engine}
	received := make(chan string, 2)
	keys := newGlobalKeys(func(target string) { received <- target })
	defer keys.Close()
	if err := keys.Apply(triggerLibrary(libraryState{}, p), true); err != nil {
		t.Skip("Test shortcut unavailable:", err)
	}
	for _, id := range []uintptr{1, 2} {
		postThread.Call(uintptr(keys.thread), 0x0312, id, 0)
		select {
		case target := <-received:
			if target != "@filter:gain" {
				t.Fatal("Wrong filter target", target)
			}
			if err := service.Run(target); err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("Shortcut not dispatched")
		}
	}
	restored, err := store.LoadPreferences()
	if err != nil || !restored.Options.Filters[0].Enabled {
		t.Fatal("Two toggles did not persist")
	}
	if !engine.options.Load().Filters[0].Enabled || service.filterRevision.Load() != 2 {
		t.Fatal("Shortcut failed to update live native DSP")
	}
}
func TestIndividualGroupAndGlobalFilterActions(t *testing.T) {
	p := Preferences{Options: AudioOptions{Filters: []MicFilter{{ID: "a", Kind: "gain", Enabled: true}, {ID: "b", Kind: "gain", Enabled: true}, {ID: "c", Kind: "gain", Enabled: true}}}, FilterGroups: []FilterGroup{{ID: "ab", FilterIDs: []string{"a", "b"}}}}
	if err := toggleFilterState(&p, "@filter:a"); err != nil {
		t.Fatal(err)
	}
	if p.Options.Filters[0].Enabled || !p.Options.Filters[1].Enabled {
		t.Fatal("Individual toggle changed another filter")
	}
	if err := toggleFilterState(&p, "@group:ab"); err != nil {
		t.Fatal(err)
	}
	if p.Options.Filters[0].Enabled || p.Options.Filters[1].Enabled || !p.Options.Filters[2].Enabled {
		t.Fatal("Group did not affect only its selected filters")
	}
	if err := toggleFilterState(&p, "@all"); err != nil {
		t.Fatal(err)
	}
	if !p.Options.BypassFilters || !p.Options.Filters[2].Enabled {
		t.Fatal("Global bypass discarded individual states")
	}
	toggleFilterState(&p, "@all")
	if p.Options.BypassFilters || p.Options.Filters[0].Enabled {
		t.Fatal("Unbypass failed to preserve states")
	}
}
func TestFilterShortcutsShareCollisionValidation(t *testing.T) {
	key := KeyBinding{Key: 49, Modifiers: 3, Label: "Ctrl+Alt+1"}
	l := libraryState{Boards: []SoundBoard{{Sounds: []SoundEntry{{ID: "clip", Hotkeys: []KeyBinding{key}}}}}}
	p := Preferences{Options: AudioOptions{Filters: []MicFilter{{ID: "a", Kind: "gain", Hotkeys: []KeyBinding{key}}}}}
	if validateBindings(triggerLibrary(l, p)) == nil {
		t.Fatal("Filter shortcut collided silently with a clip shortcut")
	}
}
func TestNativeClipProgressUsesRenderedSamples(t *testing.T) {
	e := &nativeEngine{captures: []*captureEndpoint{{gain: 1}}}
	e.options.Store(&AudioOptions{Volume: 1, BoardVolume: 1})
	e.running.Store(true)
	r := &renderEndpoint{readers: make([]ringReader, 1)}
	e.renders = []*renderEndpoint{r}
	samples := make([]float32, 2400)
	for i := range samples {
		samples[i] = .1
	}
	e.voices.Store(&boardVoices{Slots: [8]boardVoice{{ID: "clip", Sequence: 1, Samples: samples}}})
	e.render(r, make([]byte, 480*8), 480, true)
	snapshot := e.Snapshot()
	if len(snapshot.Playback) != 1 {
		t.Fatal("Playing clip not reported")
	}
	state := snapshot.Playback[0]
	if math.Abs(state.Progress-.2) > 1e-8 || math.Abs(state.Remaining-.04) > 1e-8 {
		t.Fatalf("Progress is not sample-based: %+v", state)
	}
	e.StopSound("clip")
	if len(e.Snapshot().Playback) != 0 {
		t.Fatal("Stopped clip still highlighted")
	}
}
func TestLiveMicGainAndFilterBypass(t *testing.T) {
	e := &nativeEngine{captures: []*captureEndpoint{{ID: "mic", gain: 1}}}
	data := make([]byte, 2400*4)
	for i := 0; i < 2400; i++ {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(.1))
	}
	e.captures[0].ring.push(data)
	o := AudioOptions{Volume: 1, MonitorMic: true, InputGains: map[string]float32{"mic": .5}, Filters: []MicFilter{{Kind: "gain", Enabled: true, GainDB: 6}}}
	e.options.Store(&o)
	out := make([]byte, 240*8)
	e.render(&renderEndpoint{readers: make([]ringReader, 1)}, out, 240, true)
	got := math.Float32frombits(binary.LittleEndian.Uint32(out[100*8:]))
	if math.Abs(float64(got)-.099763) > .0001 {
		t.Fatal("Live gain/filter not applied", got)
	}
	o.BypassFilters = true
	e.options.Store(&o)
	e.render(&renderEndpoint{readers: make([]ringReader, 1)}, out, 240, true)
	got = math.Float32frombits(binary.LittleEndian.Uint32(out[100*8:]))
	if math.Abs(float64(got)-.05) > .0001 {
		t.Fatal("Filter bypass failed", got)
	}
}
