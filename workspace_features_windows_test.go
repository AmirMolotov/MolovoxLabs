//go:build windows

package main

import (
	"archive/zip"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMicFilters(t *testing.T) {
	var chain micFilterChain
	if y := chain.sample(.1, []MicFilter{{Kind: "gain", Enabled: true, GainDB: 6}}); math.Abs(float64(y)-.199526) > .0001 {
		t.Fatal("Mic gain failed", y)
	}
	chain = micFilterChain{}
	var y float32
	for i := 0; i < 48000; i++ {
		y = chain.sample(.3, []MicFilter{{Kind: "highpass", Enabled: true, Frequency: 80}})
	}
	if math.Abs(float64(y)) > .0001 {
		t.Fatal("High-pass failed to reject DC", y)
	}
	chain = micFilterChain{}
	for i := 0; i < 12000; i++ {
		y = chain.sample(.0001, []MicFilter{{Kind: "gate", Enabled: true, Threshold: -40}})
	}
	if y != 0 {
		t.Fatal("Gate did not suppress quiet input", y)
	}
	for i := 0; i < 12000; i++ {
		y = chain.sample(.1, []MicFilter{{Kind: "gate", Enabled: true, Threshold: -40}})
	}
	if y < .09 {
		t.Fatal("Gate failed to open", y)
	}
	chain = micFilterChain{}
	for i := 0; i < 48000; i++ {
		y = chain.sample(1, []MicFilter{{Kind: "compressor", Enabled: true, Threshold: -20, Ratio: 4}})
	}
	if math.Abs(float64(y)-.177828) > .001 {
		t.Fatal("Compression ratio failed", y)
	}
	if validateFilters([]MicFilter{{Kind: "invalid"}}) == nil {
		t.Fatal("Unknown filter accepted")
	}
}
func TestEqualizerBandGains(t *testing.T) {
	filters := []MicFilter{{Kind: "eq", Enabled: true}}
	var chain micFilterChain
	if got := chain.sampleAt(.2, filters, 0); got != .2 {
		t.Fatalf("Flat equalizer changed signal: %g", got)
	}
	for _, tc := range []struct{ frequency, gain float64 }{{120, 6}, {1000, -9}, {8000, 12}} {
		filters[0].BassDB, filters[0].MidDB, filters[0].TrebleDB = 0, 0, 0
		switch tc.frequency {
		case 120:
			filters[0].BassDB = tc.gain
		case 1000:
			filters[0].MidDB = tc.gain
		case 8000:
			filters[0].TrebleDB = tc.gain
		}
		if err := validateFilters(filters); err != nil {
			t.Fatal(err)
		}
		var inputEnergy, outputEnergy float64
		for i := 0; i < sampleRate; i++ {
			x := .1 * math.Sin(2*math.Pi*tc.frequency*float64(i)/sampleRate)
			y := float64(chain.sampleAt(float32(x), filters, 0))
			if i >= sampleRate/2 {
				inputEnergy += x * x
				outputEnergy += y * y
			}
		}
		if got := 10 * math.Log10(outputEnergy/inputEnergy); math.Abs(got-tc.gain) > .01 {
			t.Errorf("%g Hz: got %g dB, want %g dB", tc.frequency, got, tc.gain)
		}
	}
	filters[0].TrebleDB = 13
	if validateFilters(filters) == nil {
		t.Fatal("Accepted equalizer gain above 12 dB")
	}
}
func TestParametricEqualizer(t *testing.T) {
	for _, tc := range []struct{ frequency, gain, q float64 }{
		{350, 9, .4}, {3400, -18, 7}, {17000, 12, 2},
	} {
		filters := []MicFilter{{Kind: "eq", Enabled: true, EQBands: []EQBand{{tc.frequency, tc.gain, tc.q}}}}
		if err := validateFilters(filters); err != nil {
			t.Fatal(err)
		}
		var chain micFilterChain
		var inputEnergy, outputEnergy float64
		for i := 0; i < sampleRate; i++ {
			x := .05 * math.Sin(2*math.Pi*tc.frequency*float64(i)/sampleRate)
			y := float64(chain.sampleAt(float32(x), filters, 0))
			if i >= sampleRate/2 {
				inputEnergy += x * x
				outputEnergy += y * y
			}
		}
		if got := 10 * math.Log10(outputEnergy/inputEnergy); math.Abs(got-tc.gain) > .01 {
			t.Errorf("%g Hz Q %g: got %g dB, want %g", tc.frequency, tc.q, got, tc.gain)
		}
		filters[0].EQBands = []EQBand{}
		if got := chain.sample(.123, filters); got != .123 {
			t.Errorf("Empty equalizer changed signal: %g", got)
		}
	}
	for _, band := range []EQBand{{19, 0, 1}, {20001, 0, 1}, {1000, 25, 1}, {1000, 0, 0}, {1000, 0, 13}, {math.NaN(), 0, 1}, {1000, math.Inf(1), 1}} {
		if validateFilters([]MicFilter{{Kind: "eq", EQBands: []EQBand{band}}}) == nil {
			t.Errorf("Accepted invalid EQ band: %+v", band)
		}
	}
}
func TestDownwardExpander(t *testing.T) {
	filters := []MicFilter{{Kind: "expander", Enabled: true, Threshold: -40, Ratio: 2}}
	if err := validateFilters(filters); err != nil {
		t.Fatal(err)
	}
	var chain micFilterChain
	for _, tc := range []struct{ input, want float32 }{
		{.1, .1}, {-.01, -.01}, {.001, .0001}, {.1, .1},
	} {
		var got float32
		for i := 0; i < sampleRate; i++ {
			got = chain.sample(tc.input, filters)
		}
		if math.Abs(float64(got-tc.want)) > 1e-6 {
			t.Errorf("expanding %g: got %g, want %g", tc.input, got, tc.want)
		}
	}
	filters[0].Ratio = 1
	if got := chain.sampleAt(.001, filters, 0); got != .001 {
		t.Errorf("1:1 ratio must pass through, got %g", got)
	}
	filters[0].Ratio = .9
	if validateFilters(filters) == nil {
		t.Fatal("Accepted ratio below 1:1")
	}
}
func TestBoardPackageRoundTripAndDuplicateBindings(t *testing.T) {
	root := t.TempDir()
	a := &appStore{root: filepath.Join(root, "a")}
	b := &appStore{root: filepath.Join(root, "b")}
	source := filepath.Join(root, "test.wav")
	os.WriteFile(source, wavFixture(), 0600)
	l, err := a.ImportFile("default", source)
	if err != nil {
		t.Fatal(err)
	}
	id := l.Boards[0].Sounds[0].ID
	_, err = a.SetBindings(id, &KeyBinding{Key: 49, Modifiers: 3, Label: "Ctrl+Alt+1"}, &MIDIBinding{Kind: "note", Channel: 1, Number: 36})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "board.qlboard")
	if err = a.ExportBoard("default", path); err != nil {
		t.Fatal(err)
	}
	imported, err := b.ImportBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	board := imported.Boards[len(imported.Boards)-1]
	sound := board.Sounds[0]
	if len(soundKeys(sound)) == 0 || len(soundMIDIs(sound)) == 0 || sound.ID == id {
		t.Fatal("Exported audio/assignments not restored with new identity")
	}
	if _, err = b.Sound(sound.ID); err != nil {
		t.Fatal(err)
	}
	imported, err = b.ImportBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := imported.Boards[len(imported.Boards)-1].Sounds[0]
	if len(soundKeys(duplicate)) > 0 || len(soundMIDIs(duplicate)) > 0 {
		t.Fatal("Duplicate shortcuts should be unassigned")
	}
}
func TestArchiveTraversalRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.qlboard")
	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	w, _ := z.Create("../outside.wav")
	w.Write([]byte("bad"))
	z.Close()
	f.Close()
	s := &appStore{root: t.TempDir()}
	if _, err := s.ImportBoard(path); err == nil {
		t.Fatal("Unsafe archive accepted")
	}
}
func TestMIDIDecoding(t *testing.T) {
	binding, velocity, ok := decodeMIDI(0x90 | (36 << 8) | (100 << 16))
	if !ok || velocity != 100 || binding.Kind != "note" || binding.Channel != 1 || binding.Number != 36 {
		t.Fatal("Note-on decoding failed")
	}
	if _, _, ok = decodeMIDI(0x90 | (36 << 8)); ok {
		t.Fatal("Note-on with zero velocity triggered")
	}
	if _, _, ok = decodeMIDI(0x80 | (36 << 8) | (100 << 16)); ok {
		t.Fatal("Note-off triggered")
	}
	binding, _, ok = decodeMIDI(0xb2 | (12 << 8) | (127 << 16))
	if !ok || binding.Kind != "cc" || binding.Channel != 3 {
		t.Fatal("CC decoding failed")
	}
}
func TestGlobalHotkeyMessageDispatch(t *testing.T) {
	played := make(chan string, 1)
	keys := newGlobalKeys(func(id string) { played <- id })
	defer keys.Close()
	l := libraryState{Boards: []SoundBoard{{Sounds: []SoundEntry{{ID: "sound", Hotkey: &KeyBinding{Key: 0x86, Modifiers: 3, Label: "Ctrl+Alt+F23"}}}}}}
	if err := keys.Apply(l, true); err != nil {
		t.Skip("Test shortcut unavailable:", err)
	}
	postThread.Call(uintptr(keys.thread), 0x0312, 1, 0)
	select {
	case id := <-played:
		if id != "sound" {
			t.Fatal("Wrong sound triggered")
		}
	case <-time.After(time.Second):
		t.Fatal("Global hotkey message was not dispatched")
	}
	if err := keys.Apply(l, false); err != nil {
		t.Fatal(err)
	}
}
