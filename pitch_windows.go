//go:build windows

package main

import "math"

// Two overlapping delay heads change pitch while preserving the live stream's
// duration. Complementary Hann windows hide each head's delay wraparound.
// All storage is fixed; the audio callback performs no allocation.
type pitchShifter struct {
	buffer                 [8192]float32
	write                  int
	phase, semitones, step float64
	id                     string
	active                 bool
}

func (p *pitchShifter) sample(x, semitones float64, id string) float64 {
	if semitones == 0 {
		p.active = false
		return x
	}
	if !p.active || p.id != id {
		clear(p.buffer[:])
		p.write, p.phase = 0, 0
		p.active, p.id = true, id
		p.step = (1 - math.Exp2(semitones/12)) / 4096
		p.semitones = semitones
	} else if p.semitones != semitones {
		p.step = (1 - math.Exp2(semitones/12)) / 4096
		p.semitones = semitones
	}
	p.buffer[p.write] = float32(x)
	read := func(phase float64) float64 {
		position := float64(p.write) - 64 - phase*4096
		if position < 0 {
			position += float64(len(p.buffer))
		}
		i := int(position)
		fraction := position - float64(i)
		return float64(p.buffer[i])*(1-fraction) + float64(p.buffer[(i+1)%len(p.buffer)])*fraction
	}
	other := p.phase + .5
	if other >= 1 {
		other--
	}
	weight := .5 - .5*math.Cos(2*math.Pi*p.phase)
	y := weight*read(p.phase) + (1-weight)*read(other)
	p.write = (p.write + 1) % len(p.buffer)
	p.phase += p.step
	p.phase -= math.Floor(p.phase)
	return y
}
