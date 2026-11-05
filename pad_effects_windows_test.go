//go:build windows

package main

import (
	"math"
	"testing"
)

func TestIndividualPadEffectsExcludeDryBoardCopyAndOtherPads(t *testing.T) {
	filters := []MicFilter{{ID: "gain", ClipID: "one", Kind: "gain", Enabled: true, GainDB: 20 * math.Log10(2)}}
	graph := PatchBay{Enabled: true, Nodes: []PatchNode{{ID: "board", Kind: "board"}, {ID: "clip", Kind: "clip", ClipID: "one"}, {ID: "effect", Kind: "effect", FilterID: "gain"}, {ID: "out", Kind: "output"}}, Cables: []PatchCable{{From: "board", To: "out"}, {From: "clip", To: "effect"}, {From: "effect", To: "out"}}}
	p, err := compilePatch(graph, filters)
	if err != nil {
		t.Fatal(err)
	}
	sounds := &boardVoices{Slots: [8]boardVoice{{ID: "one", BoardID: "same-board"}, {ID: "two", BoardID: "same-board"}, {ID: "one", BoardID: "same-board"}}}
	clips := [8]float32{.1, .3, .2}
	options := &AudioOptions{Volume: 1, BoardVolume: 1, Filters: filters}
	endpoint := &renderEndpoint{}
	if got := p.sample(endpoint, 0, sounds, &clips, options); math.Abs(float64(got-.9)) > 1e-6 {
		t.Fatal("Pad effects included another pad or duplicated the dry mix", got)
	}
	if n := testing.AllocsPerRun(50, func() { p.sample(endpoint, 0, sounds, &clips, options) }); n != 0 {
		t.Fatal("Per-pad routing allocated in audio callback", n)
	}
	graph.Cables = []PatchCable{{From: "board", To: "out"}}
	p, err = compilePatch(graph, filters)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.sample(endpoint, 0, sounds, &clips, options); math.Abs(float64(got-.6)) > 1e-6 {
		t.Fatal("An unconnected individual source muted the normal board mix", got)
	}
	graph.Cables = append(graph.Cables, PatchCable{From: "board", To: "clip"})
	if _, err = compilePatch(graph, filters); err == nil {
		t.Fatal("A clip source accepted an input cable")
	}
}
func TestPadOwnedEffectsDoNotBecomeMicrophoneEffects(t *testing.T) {
	filters := []MicFilter{{ID: "pad-gain", ClipID: "one", Kind: "gain", Enabled: true, GainDB: 20}, {ID: "mic-gain", Kind: "gain", Enabled: true, GainDB: -6.020599913}}
	chain := &micFilterChain{}
	if got := chain.sample(.2, filters); math.Abs(float64(got-.1)) > 1e-6 {
		t.Fatal("Pad effect was applied to the default microphone path", got)
	}
	if got := chain.sampleAt(.2, filters, 0); math.Abs(float64(got-2)) > 1e-6 {
		t.Fatal("Dedicated pad effect could not be applied explicitly", got)
	}
}
func TestPlaybackModeChangesBeforeRecordingAndAllowsRepeatedIDs(t *testing.T) {
	e := recordingModeEngine(t)
	if err := e.SetPlaybackMode("single"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 1 {
		t.Fatal("Preview mode did not immediately collapse existing clips")
	}
	if err := e.PlaySound("same-video", make([]float32, sampleRate), "video"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 1 {
		t.Fatal("Single preview mode stacked clips")
	}
	if err := e.SetPlaybackMode("layered"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.PlaySound("same-video", make([]float32, sampleRate), "video"); err != nil {
			t.Fatal(err)
		}
	}
	if activeClipCount(e) != 3 {
		t.Fatal("Layered preview did not allow simultaneous copies of one video")
	}
	seen := map[uint64]bool{}
	for _, voice := range e.voices.Load().Slots {
		if voice.Sequence != 0 {
			if seen[voice.Sequence] {
				t.Fatal("Copies reused a playback sequence")
			}
			seen[voice.Sequence] = true
		}
	}
}
