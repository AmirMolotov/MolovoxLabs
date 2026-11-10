//go:build windows

package main

import (
	"math"
	"testing"
)

func TestAddedNoiseLevelAndBypass(t *testing.T) {
	for _, level := range []float64{-60, -36, -6} {
		var chain micFilterChain
		f := []MicFilter{{Kind: "addnoise", Enabled: true, NoiseLevelDB: level}}
		mean, energy := 0., 0.
		for i := 0; i < 96000; i++ {
			y := float64(chain.sample(.125, f)) - .125
			if math.IsNaN(y) || math.IsInf(y, 0) {
				t.Fatal("Invalid noise")
			}
			if i >= 48000 {
				mean += y
				energy += y * y
			}
		}
		want := math.Pow(10, level/20)
		if math.Abs(math.Sqrt(energy/48000)/want-1) > .025 || math.Abs(mean/48000) > want*.02 {
			t.Fatal("Incorrect noise level or DC offset", level, mean, energy)
		}
		f[0].Enabled = false
		if chain.sample(.125, f) != .125 {
			t.Fatal("Disabled generator changed microphone")
		}
	}
}

func TestAddedNoiseValidation(t *testing.T) {
	for _, level := range []float64{-81, -5, math.NaN(), math.Inf(1)} {
		if validateFilters([]MicFilter{{Kind: "addnoise", NoiseLevelDB: level}}) == nil {
			t.Fatal("Accepted invalid noise level")
		}
	}
	if err := validateFilters([]MicFilter{{Kind: "addnoise", NoiseLevelDB: -36}}); err != nil {
		t.Fatal(err)
	}
}
