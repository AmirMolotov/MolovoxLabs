//go:build windows

package main

import (
	"math"
	"testing"
)

func boardEffectPatch() PatchBay {
	return PatchBay{Enabled: true, Nodes: []PatchNode{
		{ID: "mic", Kind: "mic"}, {ID: "board", Kind: "board", BoardID: "music"}, {ID: "fx", Kind: "effect", FilterID: "gain"}, {ID: "out", Kind: "output"},
	}, Cables: []PatchCable{{From: "board", To: "fx"}, {From: "fx", To: "out"}, {From: "mic", To: "out"}}}
}

func TestPatchRoutesBoardThroughEffectsAndKeepsMicrophoneDry(t *testing.T) {
	filters := []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 20 * math.Log10(2)}}
	p, err := compilePatch(boardEffectPatch(), filters)
	if err != nil {
		t.Fatal(err)
	}
	r := &renderEndpoint{}
	sounds := &boardVoices{Slots: [8]boardVoice{{BoardID: "music"}, {BoardID: "other"}}}
	clips := [8]float32{.1, .7}
	o := &AudioOptions{Filters: filters, MonitorMic: true, BoardVolume: 1, Volume: 1}
	if got := p.sample(r, .2, sounds, &clips, o); math.Abs(float64(got-.4)) > 1e-6 {
		t.Fatal("Soundboard effect or dry microphone routing incorrect", got)
	}
	o.BypassFilters = true
	if got := p.sample(r, .2, sounds, &clips, o); math.Abs(float64(got-.3)) > 1e-6 {
		t.Fatal("Global bypass did not preserve source routing", got)
	}
	o.MonitorMic = false
	if got := p.sample(r, .2, sounds, &clips, o); math.Abs(float64(got-.1)) > 1e-6 {
		t.Fatal("Microphone mute ignored", got)
	}
	if n := testing.AllocsPerRun(50, func() { p.sample(r, .2, sounds, &clips, o) }); n != 0 {
		t.Fatal("Graph rendering allocated", n)
	}
}
func TestPatchOutputSelectionFanInAndFanOut(t *testing.T) {
	graph := boardEffectPatch()
	graph.Version = 1
	graph.Preset = "board"
	graph.Nodes[1].BoardID = ""
	graph.Nodes[3].DeviceID = "cable"
	graph.Nodes = append(graph.Nodes, PatchNode{ID: "phones", Kind: "output", DeviceID: "phones"})
	graph.Cables = append(graph.Cables, PatchCable{From: "fx", To: "phones"}, PatchCable{From: "mic", To: "fx"})
	filters := []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 0}}
	p, err := compilePatch(graph, filters)
	if err != nil {
		t.Fatal(err)
	}
	o := &AudioOptions{Filters: filters, MonitorMic: true, Volume: 1, BoardVolume: 1}
	sounds := &boardVoices{Slots: [8]boardVoice{{BoardID: "a"}, {BoardID: "b"}}}
	clips := [8]float32{.1, .2}
	for _, pair := range []struct {
		id   string
		want float32
	}{{"cable", .7}, {"phones", .5}, {"unconnected", 0}} {
		r := &renderEndpoint{ID: pair.id}
		got := p.sample(r, .2, sounds, &clips, o)
		if math.Abs(float64(got-pair.want)) > 1e-6 {
			t.Fatal("Output/fan-in/fan-out routing incorrect", pair.id, got)
		}
	}
}
func TestPatchRejectsFeedbackAndDanglingCables(t *testing.T) {
	f := []MicFilter{{ID: "gain", Kind: "gain"}}
	for _, mutate := range []func(*PatchBay){
		func(p *PatchBay) { p.Cables = append(p.Cables, PatchCable{From: "fx", To: "fx"}) },
		func(p *PatchBay) { p.Cables = append(p.Cables, PatchCable{From: "out", To: "fx"}) },
		func(p *PatchBay) { p.Cables = append(p.Cables, PatchCable{From: "missing", To: "out"}) },
		func(p *PatchBay) { p.Cables = append(p.Cables, p.Cables[0]) },
		func(p *PatchBay) { p.Nodes[2].FilterID = "missing" },
		func(p *PatchBay) {
			p.Nodes = append(p.Nodes, PatchNode{ID: "duplicate", Kind: "effect", FilterID: "gain"})
		},
	} {
		p := boardEffectPatch()
		mutate(&p)
		if _, err := compilePatch(p, f); err == nil {
			t.Fatal("Invalid graph accepted", p)
		}
	}
	p := boardEffectPatch()
	p.Nodes = append(p.Nodes, PatchNode{ID: "second", Kind: "effect", FilterID: "two"})
	p.Cables = append(p.Cables, PatchCable{From: "fx", To: "second"}, PatchCable{From: "second", To: "fx"})
	f = append(f, MicFilter{ID: "two", Kind: "gain"})
	if _, err := compilePatch(p, f); err == nil {
		t.Fatal("Feedback loop accepted")
	}
}

func TestNativeRenderAppliesSoundboardGraph(t *testing.T) {
	e := &nativeEngine{captures: []*captureEndpoint{{ID: "mic", gain: 1}}}
	defer e.vst.close()
	e.running.Store(true)
	f := MicFilter{ID: "gain", Kind: "gain", Enabled: true, GainDB: -6.020599913}
	o := AudioOptions{Filters: []MicFilter{f}, Volume: 1, BoardVolume: 1, Patch: boardEffectPatch()}
	if err := e.Configure(o); err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 2400)
	for i := range samples {
		samples[i] = .4
	}
	e.voices.Store(&boardVoices{Slots: [8]boardVoice{{ID: "clip", BoardID: "music", Sequence: 1, Samples: samples}}})
	r := &renderEndpoint{readers: make([]ringReader, 1)}
	buffer := make([]byte, 480*8)
	e.render(r, buffer, 480, true)
	if math.Abs(e.Snapshot().OutputDB-(-16.255)) > 1 {
		t.Fatal("Rendered board did not pass through effect", e.Snapshot().OutputDB)
	}
	if r.soundAt[0] != 480 {
		t.Fatal("Playback advanced more than once for graph fan-out")
	}
}

func TestPatchLayoutPersistsInPreferences(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	graph := boardEffectPatch()
	graph.Nodes[1].X = 320
	graph.Nodes[1].Y = 180
	if err := s.SavePreferences(Preferences{Options: AudioOptions{Patch: graph}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if !p.Options.Patch.Enabled || p.Options.Patch.Version != 1 || p.Options.Patch.Preset != "board" || len(p.Options.Patch.Cables) != 3 || p.Options.Patch.Nodes[1].X != 320 {
		t.Fatal("Patch layout not remembered", p.Options.Patch)
	}
}
