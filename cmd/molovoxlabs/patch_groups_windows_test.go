//go:build windows

package main

import (
	"math"
	"reflect"
	"testing"
)

func TestPatchGroupsAndReusableTemplatesPersistWithParameters(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	graph := boardEffectPatch()
	graph.Groups = []PatchGroup{{ID: "g", Name: "Voice rack", NodeIDs: []string{"board", "fx"}, X: -120, Y: 300, Collapsed: true}}
	graph.Templates = []PatchTemplate{{ID: "saved", Name: "Voice rack", Nodes: graph.Nodes[1:3], Cables: []PatchCable{{From: "board", To: "fx"}}, Filters: []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 6}}}}
	if err := s.SavePreferences(Preferences{Options: AudioOptions{Patch: graph, Filters: graph.Templates[0].Filters}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Options.Patch, graph) {
		t.Fatal("group layout or saved effects changed after restart", loaded.Options.Patch)
	}
}
func TestGroupedSoundBitesMixThroughOneSharedEffectRegardlessOfCollapse(t *testing.T) {
	filters := []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 20 * math.Log10(2)}}
	graph := PatchBay{Enabled: true, Nodes: []PatchNode{{ID: "one", Kind: "clip", ClipID: "one"}, {ID: "two", Kind: "clip", ClipID: "two"}, {ID: "fx", Kind: "effect", FilterID: "gain"}, {ID: "out", Kind: "output"}}, Cables: []PatchCable{{From: "one", To: "fx"}, {From: "two", To: "fx"}, {From: "fx", To: "out"}}, Groups: []PatchGroup{{ID: "g", Name: "Reactions", NodeIDs: []string{"one", "two", "fx"}, Collapsed: true}}}
	voices := &boardVoices{Slots: [8]boardVoice{{ID: "one"}, {ID: "two"}, {ID: "unrelated"}}}
	samples := [8]float32{.1, .2, .7}
	options := &AudioOptions{Filters: filters, Volume: 1, BoardVolume: 1}
	for _, collapsed := range []bool{true, false} {
		graph.Groups[0].Collapsed = collapsed
		program, err := compilePatch(graph, filters)
		if err != nil {
			t.Fatal(err)
		}
		output := program.sample(&renderEndpoint{}, 0, voices, &samples, options)
		if math.Abs(float64(output-.6)) > 1e-6 {
			t.Fatal("group mix changed or included an unrelated sound", collapsed, output)
		}
	}
}
