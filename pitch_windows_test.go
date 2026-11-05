//go:build windows

package main

import (
	"math"
	"testing"
)

func TestPitchFrequencyAndBypass(t *testing.T) {
	for _, semitones := range []float64{-12, -7, 7, 12} {
		var chain micFilterChain
		filters := []MicFilter{{ID: "pitch-test", Kind: "pitch", Enabled: true, Semitones: semitones}}
		crossings := 0
		previous, energy := float64(0), float64(0)
		for i := 0; i < 96000; i++ {
			x := float32(.25 * math.Sin(2*math.Pi*440*float64(i)/48000))
			y := float64(chain.sample(x, filters))
			if math.IsNaN(y) || math.IsInf(y, 0) || math.Abs(y) > .251 {
				t.Fatal("Invalid pitch output", y)
			}
			if i >= 48000 {
				if previous <= 0 && y > 0 {
					crossings++
				}
				energy += y * y
			}
			previous = y
		}
		want := 440 * math.Exp2(semitones/12)
		if math.Abs(float64(crossings)-want) > want*.035 {
			t.Fatalf("%g semitones: got %d Hz, want %.1f", semitones, crossings, want)
		}
		if energy/48000 < .001 {
			t.Fatal("Pitch output inaudible")
		}
		filters[0].Enabled = false
		if chain.sample(.125, filters) != .125 {
			t.Fatal("Disabled pitch did not bypass")
		}
		filters[0].Enabled = true
		filters[0].Semitones = 0
		if chain.sample(.125, filters) != .125 {
			t.Fatal("Zero pitch did not preserve original")
		}
	}
}

func TestPitchValidation(t *testing.T) {
	for _, value := range []float64{-12, 0, 12} {
		if err := validateFilters([]MicFilter{{Kind: "pitch", Semitones: value}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []float64{-13, 13, math.NaN(), math.Inf(1)} {
		if validateFilters([]MicFilter{{Kind: "pitch", Semitones: value}}) == nil {
			t.Fatal("Accepted invalid pitch", value)
		}
	}
}
