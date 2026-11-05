//go:build windows

package main

import (
	"math"
	"testing"
)

func effectFixtures() []MicFilter {
	return []MicFilter{
		{ID: "r", Kind: "reverb", Enabled: true, Mix: 25, RoomSize: 55, Damping: 45},
		{ID: "e", Kind: "echo", Enabled: true, Mix: 50, DelayMS: 100, Feedback: 50},
		{ID: "c", Kind: "chorus", Enabled: true, Mix: 40, RateHz: 1.2, DepthMS: 5},
		{ID: "d", Kind: "distortion", Enabled: true, Mix: 65, DriveDB: 18, OutputDB: -12},
		{ID: "b", Kind: "robot", Enabled: true, Mix: 100, Frequency: 90},
	}
}

func TestEchoTimingAndFeedback(t *testing.T) {
	var e voiceEffect
	f := effectFixtures()[1]
	for i := 0; i <= 9600; i++ {
		x := 0.
		if i == 0 {
			x = 1
		}
		y := e.sample(x, f)
		want := 0.
		switch i {
		case 0:
			want = .5
		case 4800:
			want = .5
		case 9600:
			want = .25
		}
		if math.Abs(y-want) > 1e-7 {
			t.Fatalf("Echo sample %d: got %g want %g", i, y, want)
		}
	}
}

func TestReverbProducesDecayingTail(t *testing.T) {
	var e voiceEffect
	f := effectFixtures()[0]
	f.Mix = 100
	first, last := 0., 0.
	for i := 0; i < 240000; i++ {
		x := 0.
		if i == 0 {
			x = 1
		}
		y := e.sample(x, f)
		if i < 48000 {
			first += y * y
		}
		if i >= 192000 {
			last += y * y
		}
	}
	if first < .001 || last > first*.001 {
		t.Fatal("Reverb missing or does not decay", first, last)
	}
}

func TestChorusDistortionAndRobot(t *testing.T) {
	for _, f := range effectFixtures()[2:] {
		var e voiceEffect
		difference := 0.
		for i := 0; i < 48000; i++ {
			x := .2 * math.Sin(2*math.Pi*440*float64(i)/48000)
			y := e.sample(x, f)
			if math.IsNaN(y) || math.IsInf(y, 0) || math.Abs(y) > 1 {
				t.Fatal("Invalid effect output", f.Kind, y)
			}
			if i > 4096 {
				difference += (y - x) * (y - x)
			}
			if f.Kind == "robot" {
				want := x * math.Sin(2*math.Pi*90*float64(i)/48000)
				if math.Abs(y-want) > 1e-9 {
					t.Fatal("Robot modulation frequency incorrect")
				}
			}
		}
		if difference < 1 {
			t.Fatal("Effect did not change signal", f.Kind)
		}
	}
}

func TestVoiceEffectsValidationBypassAndAllocation(t *testing.T) {
	for _, f := range effectFixtures() {
		if err := validateFilters([]MicFilter{f}); err != nil {
			t.Fatal(err)
		}
		var chain micFilterChain
		f.Enabled = false
		if chain.sample(.125, []MicFilter{f}) != .125 {
			t.Fatal("Disabled filter changed audio", f.Kind)
		}
		f.Enabled = true
		f.Mix = 0
		if chain.sample(.125, []MicFilter{f}) != .125 {
			t.Fatal("Dry mix changed audio", f.Kind)
		}
		f.Mix = 101
		if validateFilters([]MicFilter{f}) == nil {
			t.Fatal("Invalid mix accepted", f.Kind)
		}
	}
	invalid := []MicFilter{
		{Kind: "echo", DelayMS: 0}, {Kind: "echo", DelayMS: 100, Feedback: 100},
		{Kind: "reverb", RoomSize: 101}, {Kind: "reverb", Damping: -1},
		{Kind: "chorus", RateHz: 0}, {Kind: "chorus", RateHz: 1, DepthMS: 16},
		{Kind: "distortion", DriveDB: 37}, {Kind: "distortion", OutputDB: 1},
		{Kind: "robot", Frequency: 0}, {Kind: "robot", Frequency: math.NaN()},
	}
	for _, f := range invalid {
		if validateFilters([]MicFilter{f}) == nil {
			t.Fatal("Invalid control accepted", f)
		}
	}
	var chain micFilterChain
	filters := effectFixtures()
	if n := testing.AllocsPerRun(20, func() {
		for i := 0; i < 480; i++ {
			chain.sample(.1, filters)
		}
	}); n != 0 {
		t.Fatal("Audio callback allocated", n)
	}
}
