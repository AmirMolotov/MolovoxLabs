//go:build windows

package main

import (
	"fmt"
	"math"
	"testing"
)

func TestLargeEffectGraphUsesPreparedAudioState(t *testing.T) {
	const count = 128
	options := AudioOptions{Volume: 1, BoardVolume: 1, MonitorMic: true, Patch: PatchBay{Enabled: true}}
	options.Patch.Nodes = append(options.Patch.Nodes, PatchNode{ID: "mic", Kind: "mic", X: -20000, Y: -5000})
	previous := "mic"
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("gain-%d", i)
		options.Filters = append(options.Filters, MicFilter{ID: id, Kind: "gain", Enabled: true})
		options.Patch.Nodes = append(options.Patch.Nodes, PatchNode{ID: id, Kind: "effect", FilterID: id, X: float64(i) * 300, Y: -5000})
		options.Patch.Cables = append(options.Patch.Cables, PatchCable{From: previous, To: id})
		previous = id
	}
	options.Filters[count-1].GainDB = 6
	options.Patch.Nodes = append(options.Patch.Nodes, PatchNode{ID: "out", Kind: "output", X: 50000, Y: -5000})
	options.Patch.Cables = append(options.Patch.Cables, PatchCable{From: previous, To: "out"})
	engine := &nativeEngine{}
	defer engine.vst.close()
	if err := engine.Configure(options); err != nil {
		t.Fatal(err)
	}
	live := engine.options.Load()
	render := &renderEndpoint{filters: *live.filterChains[0]}
	var clips [8]float32
	expected := float32(.01 * math.Pow(10, 6.0/20))
	for i := 0; i < 10; i++ {
		got := live.patchProgram.sample(render, .01, nil, &clips, live)
		if math.Abs(float64(got-expected)) > 1e-7 {
			t.Fatalf("Large graph output %g, want %g", got, expected)
		}
	}
	if count := testing.AllocsPerRun(100, func() { live.patchProgram.sample(render, .01, nil, &clips, live) }); count != 0 {
		t.Fatalf("Audio rendering allocated %g times", count)
	}
	first := live.filterChains[0].slots[0]
	options.Filters[0], options.Filters[1] = options.Filters[1], options.Filters[0]
	if err := engine.Configure(options); err != nil {
		t.Fatal(err)
	}
	if engine.options.Load().filterChains[0].slots[1] != first {
		t.Fatal("Reordering discarded an existing effect's state")
	}
}
