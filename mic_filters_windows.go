//go:build windows

package main

import (
	"errors"
	"math"
)

type MicFilter struct {
	ClipID       string `json:",omitempty"`
	PluginPath   string
	ClassID      string
	PluginName   string
	VSTState     string
	ID           string
	Hotkeys      []KeyBinding
	MIDIs        []MIDIBinding
	Kind         string
	Enabled      bool
	Frequency    float64
	Threshold    float64
	Ratio        float64
	GainDB       float64
	BassDB       float64
	MidDB        float64
	TrebleDB     float64
	EQBands      []EQBand
	Semitones    float64
	Strength     float64
	NoiseDB      float64
	NoiseLevelDB float64
	Mix          float64
	DelayMS      float64
	Feedback     float64
	RoomSize     float64
	Damping      float64
	RateHz       float64
	DepthMS      float64
	DriveDB      float64
	OutputDB     float64
}

const maxEQBands = 8

type EQBand struct {
	Frequency float64
	GainDB    float64
	Q         float64
}

type micFilterState struct {
	kind                           string
	previous, high, low, env, gain float64
	random                         uint32
	noiseGain                      float64
	eq                             [maxEQBands]eqBand
	eqCount                        int
}

// Slots retain their identity across configuration changes. Large DSP buffers
// are allocated only for effects that need them, before publishing a live graph.
type micFilterSlot struct {
	id, kind string
	state    micFilterState
	pitch    *pitchShifter
	noise    *noiseReducer
	effect   *voiceEffect
}
type micFilterChain struct {
	slots    []*micFilterSlot
	vstSlots []*vstUnit
	vstVoice int
	values   []float32
}

func newMicFilterSlot(f MicFilter) *micFilterSlot {
	slot := &micFilterSlot{id: f.ID, kind: f.Kind}
	switch f.Kind {
	case "pitch":
		slot.pitch = &pitchShifter{}
	case "noise":
		slot.noise = &noiseReducer{}
	case "reverb", "echo", "chorus", "distortion", "robot":
		slot.effect = &voiceEffect{}
	}
	return slot
}
func prepareFilterChain(filters []MicFilter, previous *micFilterChain, nodes int) *micFilterChain {
	chain := &micFilterChain{slots: make([]*micFilterSlot, len(filters)), vstSlots: make([]*vstUnit, len(filters)), values: make([]float32, nodes)}
	existing := map[string]*micFilterSlot{}
	if previous != nil {
		for _, slot := range previous.slots {
			if slot.id != "" {
				existing[slot.id] = slot
			}
		}
	}
	for i, filter := range filters {
		slot := existing[filter.ID]
		if slot == nil && filter.ID == "" && previous != nil && i < len(previous.slots) {
			slot = previous.slots[i]
		}
		if slot == nil || slot.kind != filter.Kind {
			slot = newMicFilterSlot(filter)
		}
		chain.slots[i] = slot
	}
	return chain
}

func validateFilters(filters []MicFilter) error {
	for _, f := range filters {
		for _, v := range []float64{f.Frequency, f.Threshold, f.Ratio, f.GainDB, f.Semitones, f.Strength, f.NoiseDB, f.NoiseLevelDB, f.Mix, f.DelayMS, f.Feedback, f.RoomSize, f.Damping, f.RateHz, f.DepthMS, f.DriveDB, f.OutputDB, f.BassDB, f.MidDB, f.TrebleDB} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return errors.New("Invalid microphone filter")
			}
		}
		switch f.Kind {
		case "vst3":
			if err := validVSTFilter(f); err != nil {
				return err
			}
		case "reverb", "echo", "chorus", "distortion", "robot":
			if f.Mix < 0 || f.Mix > 100 {
				return errors.New("Effect mix must be 0–100%")
			}
			switch f.Kind {
			case "reverb":
				if f.RoomSize < 0 || f.RoomSize > 100 || f.Damping < 0 || f.Damping > 100 {
					return errors.New("Reverb size and damping must be 0–100%")
				}
			case "echo":
				if f.DelayMS < 20 || f.DelayMS > 1000 || f.Feedback < 0 || f.Feedback > 85 {
					return errors.New("Echo delay must be 20–1000 ms and feedback 0–85%")
				}
			case "chorus":
				if f.RateHz < .1 || f.RateHz > 5 || f.DepthMS < 0 || f.DepthMS > 15 {
					return errors.New("Chorus rate must be 0.1–5 Hz and depth 0–15 ms")
				}
			case "distortion":
				if f.DriveDB < 0 || f.DriveDB > 36 || f.OutputDB < -24 || f.OutputDB > 0 {
					return errors.New("Distortion drive must be 0–36 dB and output −24–0 dB")
				}
			case "robot":
				if f.Frequency < 20 || f.Frequency > 2000 {
					return errors.New("Robot frequency must be 20–2000 Hz")
				}
			}
		case "addnoise":
			if f.NoiseLevelDB < -80 || f.NoiseLevelDB > -6 {
				return errors.New("Added noise level must be −80 to −6 dB")
			}
		case "noise":
			if f.Strength < 0 || f.Strength > 100 || f.NoiseDB < -80 || f.NoiseDB > -15 {
				return errors.New("Noise reduction strength must be 0–100% and noise level −80 to −15 dB")
			}
		case "pitch":
			if f.Semitones < -12 || f.Semitones > 12 {
				return errors.New("Pitch must be −12 to +12 semitones")
			}
		case "highpass":
			if f.Frequency < 20 || f.Frequency > 1000 {
				return errors.New("High-pass frequency must be 20–1000 Hz")
			}
		case "lowpass":
			if f.Frequency < 1000 || f.Frequency > 20000 {
				return errors.New("Low-pass frequency must be 1000–20000 Hz")
			}
		case "gate":
			if f.Threshold < -90 || f.Threshold > -5 {
				return errors.New("Gate threshold must be −90 to −5 dB")
			}
		case "eq":
			if len(f.EQBands) > maxEQBands {
				return errors.New("Equalizer supports up to eight bands")
			}
			for _, band := range f.EQBands {
				if !(band.Frequency >= 20 && band.Frequency <= 20000 &&
					band.GainDB >= -24 && band.GainDB <= 24 && band.Q >= .2 && band.Q <= 12) {
					return errors.New("Invalid equalizer band frequency, gain or width")
				}
			}
			if math.Abs(f.BassDB) > 12 || math.Abs(f.MidDB) > 12 || math.Abs(f.TrebleDB) > 12 {
				return errors.New("Equalizer band gains must be −12 to +12 dB")
			}
		case "expander":
			if f.Threshold < -90 || f.Threshold > 0 || f.Ratio < 1 || f.Ratio > 10 {
				return errors.New("Expander threshold must be −90 to 0 dB and ratio 1:1 to 10:1")
			}
		case "compressor":
			if f.Threshold < -60 || f.Threshold > 0 || f.Ratio < 1 || f.Ratio > 20 {
				return errors.New("Invalid compressor controls")
			}
		case "gain":
			if f.GainDB < -24 || f.GainDB > 24 {
				return errors.New("Mic gain must be −24 to +24 dB")
			}
		default:
			return errors.New("Unknown microphone filter")
		}
	}
	return nil
}
func (c *micFilterChain) sample(input float32, filters []MicFilter) float32 {
	return c.process(input, filters, 0, len(filters), true)
}
func (c *micFilterChain) sampleAt(input float32, filters []MicFilter, index int) float32 {
	return c.process(input, filters, index, index+1, false)
}
func (c *micFilterChain) process(input float32, filters []MicFilter, start, end int, microphone bool) float32 {
	// Standalone DSP callers can use a zero-value chain. Live rendering receives
	// a fully prepared chain from Configure and never enters this allocation path.
	if len(c.slots) != len(filters) {
		*c = *prepareFilterChain(filters, c, len(c.values))
	}
	x := float64(input)
	for i := start; i < end; i++ {
		f := filters[i]
		if microphone && f.ClipID != "" {
			continue
		}
		slot := c.slots[i]
		if slot.id != f.ID || slot.kind != f.Kind {
			slot = newMicFilterSlot(f)
			c.slots[i] = slot
		}
		s := &slot.state
		if s.kind != f.Kind {
			*s = micFilterState{kind: f.Kind}
			if f.Kind == "expander" {
				s.gain = 1
			}
		}
		if !f.Enabled {
			if slot.effect != nil {
				slot.effect.active = false
			}
			if f.Kind == "eq" {
				s.eq = [maxEQBands]eqBand{}
				s.eqCount = 0
			}
			if f.Kind == "expander" {
				s.env, s.gain = 0, 1
			}
			if f.Kind == "addnoise" {
				s.noiseGain = 0
			}
			if f.Kind == "noise" {
				slot.noise.active = false
			}
			if f.Kind == "pitch" {
				slot.pitch.active = false
			}
			continue
		}
		switch f.Kind {
		case "vst3":
			if unit := c.vstSlots[i]; unit != nil {
				x = float64(unit.sample(c.vstVoice, float32(x)))
			}
		case "reverb", "echo", "chorus", "distortion", "robot":
			x = slot.effect.sample(x, f)
		case "addnoise":
			// Per-filter PRNG: no locks, allocations or shared random state.
			if s.random == 0 {
				s.random = uint32(i+1) * 2654435761
			}
			s.random ^= s.random << 13
			s.random ^= s.random >> 17
			s.random ^= s.random << 5
			s.noiseGain += .002 * (math.Pow(10, f.NoiseLevelDB/20) - s.noiseGain)
			x += (float64(s.random>>8)/8388608 - 1) * math.Sqrt(3) * s.noiseGain
		case "noise":
			x = slot.noise.sample(x, f)
		case "pitch":
			x = slot.pitch.sample(x, f.Semitones, f.ID)
		case "highpass":
			a := math.Exp(-2 * math.Pi * f.Frequency / sampleRate)
			s.high = a * (s.high + x - s.previous)
			s.previous = x
			x = s.high
		case "lowpass":
			a := 1 - math.Exp(-2*math.Pi*f.Frequency/sampleRate)
			s.low += a * (x - s.low)
			x = s.low
		case "gate":
			level := math.Abs(x)
			a := .0005
			if level > s.env {
				a = .02
			}
			s.env += a * (level - s.env)
			target := float64(0)
			if s.env > math.Pow(10, f.Threshold/20) {
				target = 1
			}
			speed := .0004
			if target > s.gain {
				speed = .01
			}
			s.gain += speed * (target - s.gain)
			x *= s.gain
		case "expander":
			if f.Ratio == 1 {
				s.env, s.gain = 0, 1
				break
			}
			level := math.Abs(x)
			speed := .0005
			if level > s.env {
				speed = .02
			}
			s.env += speed * (level - s.env)
			threshold := math.Pow(10, f.Threshold/20)
			target := 1.0
			if s.env < threshold {
				// Below threshold, each dB of input drop becomes Ratio dB at the output.
				target = math.Pow(s.env/threshold, f.Ratio-1)
			}
			speed = .0004
			if target > s.gain {
				speed = .01
			}
			s.gain += speed * (target - s.gain)
			x *= s.gain
		case "compressor":
			level := math.Abs(x)
			a := .0002
			if level > s.env {
				a = .01
			}
			s.env += a * (level - s.env)
			db := 20 * math.Log10(math.Max(s.env, 1e-9))
			reduction := float64(0)
			if db > f.Threshold {
				reduction = (f.Threshold + (db-f.Threshold)/f.Ratio) - db
			}
			x *= math.Pow(10, reduction/20)
		case "eq":
			bands := f.EQBands
			if bands == nil {
				// Older settings stored three fixed-frequency bands.
				bands = []EQBand{{120, f.BassDB, 1}, {1000, f.MidDB, 1}, {8000, f.TrebleDB, 1}}
			}
			for i, band := range bands {
				x = s.eq[i].sample(x, band)
			}
			if len(bands) < s.eqCount {
				clear(s.eq[len(bands):s.eqCount])
			}
			s.eqCount = len(bands)
		case "gain":
			x *= math.Pow(10, f.GainDB/20)
		}
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return float32(x)
}

type eqBand struct {
	params     EQBand
	b0, b1, b2 float64
	a1, a2     float64
	z1, z2     float64
}

func (b *eqBand) sample(x float64, band EQBand) float64 {
	gainDB := band.GainDB
	if band != b.params {
		if gainDB == 0 {
			*b = eqBand{params: band}
		} else {
			// Cache the peaking biquad coefficients until the band changes.
			omega := 2 * math.Pi * band.Frequency / sampleRate
			alpha := math.Sin(omega) / (2 * band.Q)
			a := math.Pow(10, gainDB/40)
			a0 := 1 + alpha/a
			b.params = band
			b.b0 = (1 + alpha*a) / a0
			b.b1 = -2 * math.Cos(omega) / a0
			b.b2 = (1 - alpha*a) / a0
			b.a1 = b.b1
			b.a2 = (1 - alpha/a) / a0
		}
	}
	if gainDB == 0 {
		return x
	}
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}
