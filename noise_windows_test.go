//go:build windows

package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestNoiseReductionPreservesVoiceAndReducesNoise(t *testing.T) {
	f := MicFilter{ID: "noise", Kind: "noise", Enabled: true, Strength: 100, NoiseDB: -40}
	for _, voice := range []bool{false, true} {
		var reducer noiseReducer
		rng := rand.New(rand.NewSource(1))
		inputEnergy, outputEnergy, correlation := 0., 0., 0.
		for i := 0; i < 48000; i++ {
			x := rng.NormFloat64() * .01
			if voice {
				x += .2 * math.Sin(2*math.Pi*440*float64(i)/48000)
			}
			y := reducer.sample(x, f)
			if math.IsNaN(y) || math.IsInf(y, 0) {
				t.Fatal("Invalid output")
			}
			if i > 4096 {
				inputEnergy += x * x
				outputEnergy += y * y
				correlation += y * math.Sin(2*math.Pi*440*float64(i-noiseSize)/48000)
			}
		}
		if !voice && outputEnergy/inputEnergy > .55 {
			t.Fatalf("Background reduction too weak: %.2f", outputEnergy/inputEnergy)
		}
		if voice && (outputEnergy/inputEnergy < .8 || 2*correlation/float64(48000-4097) < .18) {
			t.Fatal("Voice was suppressed or distorted", outputEnergy/inputEnergy, correlation)
		}
	}
	var chain micFilterChain
	f.Strength = 0
	if chain.sample(.125, []MicFilter{f}) != .125 {
		t.Fatal("Zero reduction should bypass")
	}
	f.Strength = 75
	f.Enabled = false
	if chain.sample(.125, []MicFilter{f}) != .125 {
		t.Fatal("Disabled reduction should bypass")
	}
}

func TestNoiseReductionValidation(t *testing.T) {
	for _, f := range []MicFilter{
		{Kind: "noise", Strength: -1, NoiseDB: -45},
		{Kind: "noise", Strength: 101, NoiseDB: -45},
		{Kind: "noise", Strength: 75, NoiseDB: -81},
		{Kind: "noise", Strength: 75, NoiseDB: -14},
		{Kind: "noise", Strength: math.NaN(), NoiseDB: -45},
	} {
		if validateFilters([]MicFilter{f}) == nil {
			t.Fatal("Accepted invalid reduction", f)
		}
	}
	if err := validateFilters([]MicFilter{{Kind: "noise", Strength: 75, NoiseDB: -45}}); err != nil {
		t.Fatal(err)
	}
}
