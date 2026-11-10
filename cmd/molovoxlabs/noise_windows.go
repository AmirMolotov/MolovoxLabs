//go:build windows

package main

import "math"

const noiseSize = 512
const noiseHop = 128

var noiseWindow = func() (w [noiseSize]float64) {
	for i := range w {
		w[i] = math.Sqrt(.5 - .5*math.Cos(2*math.Pi*float64(i)/noiseSize))
	}
	return
}()
var noiseTwiddle = func() (w [noiseSize / 2]complex128) {
	for i := range w {
		angle := -2 * math.Pi * float64(i) / noiseSize
		w[i] = complex(math.Cos(angle), math.Sin(angle))
	}
	return
}()

// Fixed-size FFT and overlap-add buffers avoid allocating in audio callbacks.
type noiseReducer struct {
	input, output [noiseSize]float64
	frequency     [noiseSize]complex128
	gains         [noiseSize/2 + 1]float64
	cursor, hop   int
	active        bool
	id            string
}

func noiseFFT(a *[noiseSize]complex128, inverse bool) {
	j := 0
	for i := 1; i < noiseSize; i++ {
		bit := noiseSize >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= noiseSize; size *= 2 {
		for start := 0; start < noiseSize; start += size {
			for k := 0; k < size/2; k++ {
				w := noiseTwiddle[k*noiseSize/size]
				if inverse {
					w = complex(real(w), -imag(w))
				}
				u, v := a[start+k], a[start+k+size/2]*w
				a[start+k], a[start+k+size/2] = u+v, u-v
			}
		}
	}
	if inverse {
		for i := range a {
			a[i] /= noiseSize
		}
	}
}

func (n *noiseReducer) sample(x float64, f MicFilter) float64 {
	if f.Strength == 0 {
		n.active = false
		return x
	}
	if !n.active || n.id != f.ID {
		*n = noiseReducer{active: true, id: f.ID}
		for i := range n.gains {
			n.gains[i] = 1
		}
	}
	y := n.output[n.cursor]
	n.output[n.cursor] = 0
	n.input[n.cursor] = x
	n.cursor = (n.cursor + 1) % noiseSize
	n.hop++
	if n.hop < noiseHop {
		return y
	}
	n.hop = 0
	for i := range n.frequency {
		n.frequency[i] = complex(n.input[(n.cursor+i)%noiseSize]*noiseWindow[i], 0)
	}
	noiseFFT(&n.frequency, false)
	// NoiseDB is the estimated broadband RMS background level, converted to
	// expected power per FFT bin for this window. No calibration recording needed.
	strength := f.Strength / 100
	noisePower := float64(noiseSize) / 2 * math.Pow(10, f.NoiseDB/10)
	minimum := 1 - .9*strength
	for i := range n.gains {
		z := n.frequency[i]
		power := real(z)*real(z) + imag(z)*imag(z)
		target := math.Sqrt(math.Max(minimum*minimum, 1-strength*noisePower/math.Max(power, 1e-20)))
		speed := .2
		if target > n.gains[i] {
			speed = .7
		}
		n.gains[i] += speed * (target - n.gains[i])
		n.frequency[i] *= complex(n.gains[i], 0)
		if i > 0 && i < noiseSize/2 {
			n.frequency[noiseSize-i] *= complex(n.gains[i], 0)
		}
	}
	noiseFFT(&n.frequency, true)
	for i := range n.frequency {
		// Four overlapping squared sqrt-Hann windows sum to two.
		n.output[(n.cursor+i)%noiseSize] += real(n.frequency[i]) * noiseWindow[i] / 2
	}
	return y
}
