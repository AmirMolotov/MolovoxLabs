//go:build windows

package main

import (
	"errors"
	"math"
)

type PatchNode struct {
	ID, Kind, FilterID, BoardID, DeviceID string
	X, Y                                  float64
}
type PatchCable struct{ From, To string }
type PatchBay struct {
	Enabled bool
	Nodes   []PatchNode
	Cables  []PatchCable
}
type patchStep struct {
	kind, boardID, deviceID string
	filter                  int
	inputs                  []int
}
type patchProgram struct{ steps []patchStep }

// Compile the cable graph once when settings change. Audio evaluation uses only
// fixed scratch buffers and the existing per-filter DSP state.
func compilePatch(p PatchBay, filters []MicFilter) (*patchProgram, error) {
	if len(p.Nodes) > 32 || len(p.Cables) > 96 {
		return nil, errors.New("Patch bay supports up to 32 nodes and 96 cables")
	}
	if !p.Enabled {
		return nil, nil
	}
	index := map[string]int{}
	effectIDs := map[string]int{}
	usedEffects := map[string]bool{}
	for i, f := range filters {
		effectIDs[f.ID] = i
	}
	for i, node := range p.Nodes {
		if node.ID == "" || len(node.ID) > 128 || math.IsNaN(node.X) || math.IsNaN(node.Y) || math.IsInf(node.X, 0) || math.IsInf(node.Y, 0) || node.X < 0 || node.X > 5000 || node.Y < 0 || node.Y > 5000 {
			return nil, errors.New("Invalid patch node")
		}
		if _, ok := index[node.ID]; ok {
			return nil, errors.New("Duplicate patch node")
		}
		index[node.ID] = i
		switch node.Kind {
		case "mic", "board", "output":
		case "effect":
			if _, ok := effectIDs[node.FilterID]; !ok {
				return nil, errors.New("A connected effect no longer exists")
			}
			if usedEffects[node.FilterID] {
				return nil, errors.New("Place each effect once; its output can feed multiple cables")
			}
			usedEffects[node.FilterID] = true
		default:
			return nil, errors.New("Unknown patch node type")
		}
	}
	inputs := make([][]int, len(p.Nodes))
	edges := map[[2]int]bool{}
	for _, cable := range p.Cables {
		from, ok1 := index[cable.From]
		to, ok2 := index[cable.To]
		if !ok1 || !ok2 || from == to {
			return nil, errors.New("Invalid cable endpoints")
		}
		if p.Nodes[from].Kind == "output" || p.Nodes[to].Kind == "mic" || p.Nodes[to].Kind == "board" {
			return nil, errors.New("Connect source/effect outputs to effect/output inputs")
		}
		key := [2]int{from, to}
		if edges[key] {
			return nil, errors.New("Duplicate patch cable")
		}
		edges[key] = true
		inputs[to] = append(inputs[to], from)
	}
	state := make([]uint8, len(p.Nodes))
	order := []int{}
	var visit func(int) error
	visit = func(i int) error {
		if state[i] == 1 {
			return errors.New("Feedback loops are not supported; remove the circular cable")
		}
		if state[i] == 2 {
			return nil
		}
		state[i] = 1
		for _, source := range inputs[i] {
			if err := visit(source); err != nil {
				return err
			}
		}
		state[i] = 2
		order = append(order, i)
		return nil
	}
	for i := range p.Nodes {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	positions := make([]int, len(order))
	for i, original := range order {
		positions[original] = i
	}
	program := &patchProgram{}
	for _, original := range order {
		node := p.Nodes[original]
		step := patchStep{kind: node.Kind, boardID: node.BoardID, deviceID: node.DeviceID, filter: effectIDs[node.FilterID]}
		for _, source := range inputs[original] {
			step.inputs = append(step.inputs, positions[source])
		}
		program.steps = append(program.steps, step)
	}
	return program, nil
}
func (p *patchProgram) sample(r *renderEndpoint, mic float32, sounds *boardVoices, clips *[8]float32, o *AudioOptions) float32 {
	var values [32]float32
	var output float32
	for i, step := range p.steps {
		var x float32
		for _, source := range step.inputs {
			x += values[source]
		}
		switch step.kind {
		case "mic":
			if o.MonitorMic {
				x = mic
			}
		case "board":
			if sounds != nil {
				for slot, voice := range sounds.Slots {
					if step.boardID == "" || voice.BoardID == step.boardID {
						x += clips[slot] * o.BoardVolume
					}
				}
			}
		case "effect":
			if !o.BypassFilters {
				x = r.filters.sampleAt(x, o.Filters, step.filter)
			}
		case "output":
			if step.deviceID == "" || step.deviceID == r.ID {
				output += x
			}
		}
		values[i] = x
	}
	return output * o.Volume
}
