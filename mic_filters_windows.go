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
type micFilterState struct {
	kind                           string
	previous, high, low, env, gain float64
	random                         uint32
	noiseGain                      float64
}
type micFilterChain struct {
	vstSlots  [8]*vstUnit
	vstVoice  int
	states    [8]micFilterState
	pitches   [8]pitchShifter
	denoisers [8]noiseReducer
	effects   [8]voiceEffect
}

func validateFilters(filters []MicFilter) error {
	if len(filters) > 8 {
		return errors.New("Maximum eight microphone filters")
	}
	for _, f := range filters {
		for _, v := range []float64{f.Frequency, f.Threshold, f.Ratio, f.GainDB, f.Semitones, f.Strength, f.NoiseDB, f.NoiseLevelDB, f.Mix, f.DelayMS, f.Feedback, f.RoomSize, f.Damping, f.RateHz, f.DepthMS, f.DriveDB, f.OutputDB} {
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
	x := float64(input)
	for i := start; i < end; i++ {
		f := filters[i]
		if microphone && f.ClipID != "" {
			continue
		}
		s := &c.states[i]
		if s.kind != f.Kind {
			*s = micFilterState{kind: f.Kind}
			if f.Kind == "expander" {
				s.gain = 1
			}
			c.pitches[i] = pitchShifter{}
			c.denoisers[i] = noiseReducer{}
		}
		if !f.Enabled {
			c.effects[i].active = false
			if f.Kind == "expander" {
				s.env, s.gain = 0, 1
			}
			if f.Kind == "addnoise" {
				s.noiseGain = 0
			}
			if f.Kind == "noise" {
				c.denoisers[i].active = false
			}
			if f.Kind == "pitch" {
				c.pitches[i].active = false
			}
			continue
		}
		switch f.Kind {
		case "vst3":
			if unit := c.vstSlots[i]; unit != nil {
				x = float64(unit.sample(c.vstVoice, float32(x)))
			}
		case "reverb", "echo", "chorus", "distortion", "robot":
			x = c.effects[i].sample(x, f)
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
			x = c.denoisers[i].sample(x, f)
		case "pitch":
			x = c.pitches[i].sample(x, f.Semitones, f.ID)
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
		case "gain":
			x *= math.Pow(10, f.GainDB/20)
		}
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return float32(x)
}
