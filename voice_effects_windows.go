//go:build windows

package main

import "math"

// Each filter slot owns its buffers. No memory allocation or shared state is
// needed while rendering; disabling a filter discards its tail on next use.
type voiceEffect struct {
	delay    [48001]float32
	combs    [4][4096]float32
	allpass  [2][1024]float32
	combPos  [4]int
	combLow  [4]float64
	allPos   [2]int
	write    int
	phase    float64
	id, kind string
	active   bool
}

func (e *voiceEffect) sample(x float64, f MicFilter) float64 {
	if f.Mix == 0 {
		e.active = false
		return x
	}
	if !e.active || e.id != f.ID || e.kind != f.Kind {
		*e = voiceEffect{active: true, id: f.ID, kind: f.Kind}
	}
	wet := x
	switch f.Kind {
	case "echo":
		length := int(f.DelayMS * sampleRate / 1000)
		read := (e.write - length + len(e.delay)) % len(e.delay)
		wet = float64(e.delay[read])
		e.delay[e.write] = float32(x + wet*f.Feedback/100)
		e.write = (e.write + 1) % len(e.delay)
	case "chorus":
		e.delay[e.write] = float32(x)
		delay := (18 + f.DepthMS*math.Sin(e.phase)) * sampleRate / 1000
		position := float64(e.write) - delay
		if position < 0 {
			position += float64(len(e.delay))
		}
		i := int(position)
		fraction := position - float64(i)
		wet = float64(e.delay[i])*(1-fraction) + float64(e.delay[(i+1)%len(e.delay)])*fraction
		e.write = (e.write + 1) % len(e.delay)
		e.phase += 2 * math.Pi * f.RateHz / sampleRate
		if e.phase >= 2*math.Pi {
			e.phase -= 2 * math.Pi
		}
	case "reverb":
		wet = 0
		for j, base := range [4]int{1695, 1760, 1623, 1548} {
			length := int(float64(base) * (.65 + f.RoomSize*.0065))
			pos := e.combPos[j] % length
			v := float64(e.combs[j][pos])
			e.combLow[j] += (1 - .85*f.Damping/100) * (v - e.combLow[j])
			e.combs[j][pos] = float32(x + e.combLow[j]*(.65+.0028*f.RoomSize))
			e.combPos[j] = (pos + 1) % length
			wet += v * .25
		}
		for j, length := range [2]int{241, 379} {
			pos := e.allPos[j]
			v := float64(e.allpass[j][pos])
			e.allpass[j][pos] = float32(wet + .5*v)
			wet = v - .5*wet
			e.allPos[j] = (pos + 1) % length
		}
	case "distortion":
		wet = math.Tanh(x*math.Pow(10, f.DriveDB/20)) * math.Pow(10, f.OutputDB/20)
	case "robot":
		wet = x * math.Sin(e.phase)
		e.phase += 2 * math.Pi * f.Frequency / sampleRate
		if e.phase >= 2*math.Pi {
			e.phase -= 2 * math.Pi
		}
	}
	mix := f.Mix / 100
	return x*(1-mix) + wet*mix
}
