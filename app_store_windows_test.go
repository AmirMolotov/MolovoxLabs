//go:build windows

package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func wavFixture() []byte {
	data := make([]byte, 44+480*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 48000)
	binary.LittleEndian.PutUint32(data[28:], 96000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 960)
	for i := 0; i < 480; i++ {
		binary.LittleEndian.PutUint16(data[44+i*2:], 8192)
	}
	return data
}
func TestPreferencesSurviveNewStore(t *testing.T) {
	root := t.TempDir()
	store := &appStore{root: root}
	defaults, err := store.LoadPreferences()
	if err != nil || !defaults.AutoListen || defaults.Saved {
		t.Fatal("Incorrect startup defaults")
	}
	p := Preferences{AutoListen: true, Inputs: []InputSelection{{ID: "mic-id", Gain: .75}}, Outputs: []string{"speaker-id"}, Options: AudioOptions{Volume: .8, BoardVolume: .6, Inverted: true}}
	if err = store.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	restored, err := (&appStore{root: root}).LoadPreferences()
	if err != nil || !restored.Saved || restored.Inputs[0].ID != "mic-id" || restored.Outputs[0] != "speaker-id" || restored.Options.BoardVolume != .6 {
		t.Fatalf("Preferences lost: %+v %v", restored, err)
	}
}
func TestSoundboardImportAndPersistence(t *testing.T) {
	root := t.TempDir()
	store := &appStore{root: filepath.Join(root, "store")}
	source := filepath.Join(root, "clip.wav")
	os.WriteFile(source, wavFixture(), 0600)
	library, err := store.ImportFile("default", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(library.Boards[0].Sounds) != 1 {
		t.Fatal("Clip not saved")
	}
	clip := library.Boards[0].Sounds[0]
	os.Remove(source)
	samples, err := (&appStore{root: store.root}).Sound(clip.ID)
	if err != nil || len(samples) != 480 || math.Abs(float64(samples[300]-.25)) > .0001 {
		t.Fatalf("Copied clip not decoded correctly: %d %v", len(samples), err)
	}
	library, err = store.RemoveSound(clip.ID)
	if err != nil || len(library.Boards[0].Sounds) != 0 {
		t.Fatal("Remove failed")
	}
	if _, err = store.Sound(clip.ID); err == nil {
		t.Fatal("Removed sound still accessible")
	}
}
func TestSoundboardOutputIsNotInverted(t *testing.T) {
	samples := make([]float32, 1000)
	for i := range samples {
		samples[i] = .25
	}
	voices := &boardVoices{}
	voices.Slots[0] = boardVoice{Sequence: 1, Samples: samples}
	a, b := renderEndpoint{}, renderEndpoint{}
	for i := 0; i < 300; i++ {
		a.boardSample(voices)
	}
	if a.boardSample(voices) != .25 {
		t.Fatal("Wrong soundboard sample")
	}
	if b.boardSample(voices) != 0 {
		t.Fatal("Independent output cursor was consumed")
	}
	e := nativeEngine{captures: []*captureEndpoint{{gain: 1}}}
	e.options.Store(&AudioOptions{Volume: 1, BoardVolume: 1, Inverted: true})
	e.voices.Store(voices)
	out := make([]byte, 512*8)
	e.render(&renderEndpoint{readers: make([]ringReader, 1)}, out, 512, true)
	got := math.Float32frombits(binary.LittleEndian.Uint32(out[300*8:]))
	if got != .25 {
		t.Fatalf("Board was incorrectly muted or inverted: %v", got)
	}
	e.StopSounds()
	clear(out)
	e.render(&renderEndpoint{readers: make([]ringReader, 1)}, out, 512, true)
	for _, v := range out {
		if v != 0 {
			t.Fatal("Stop all sounds failed")
		}
	}
}
func TestBadAudioDoesNotImport(t *testing.T) {
	if _, err := decodeSound([]byte("not audio"), ".wav"); err == nil {
		t.Fatal("Invalid WAV accepted")
	}
	if _, err := decodeSound([]byte("not audio"), ".mp3"); err == nil {
		t.Fatal("Invalid MP3 accepted")
	}
}
