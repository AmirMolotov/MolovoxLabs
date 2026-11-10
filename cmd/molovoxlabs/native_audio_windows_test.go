//go:build windows

package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestNativePolarity(t *testing.T) {
	if nativeSelfTest()["passed"] != true {
		t.Fatal("Native inversion is not exact")
	}
}
func TestNativeDSPMuteDelayAndClipping(t *testing.T) {
	muted := processor{}
	for i := 0; i < 4096; i++ {
		y, _ := muted.sample(.5, &AudioOptions{})
		if y != 0 {
			t.Fatal("Muted output must be silent")
		}
	}
	delayed := processor{gain: 1}
	options := AudioOptions{Volume: 1, Enabled: true, DelayMS: 1}
	for i := 0; i < 60; i++ {
		x := float32(0)
		if i == 0 {
			x = .25
		}
		y, _ := delayed.sample(x, &options)
		if i == 48 && y != .25 {
			t.Fatalf("Expected impulse at 48, got %v", y)
		}
		if i != 48 && y != 0 {
			t.Fatalf("Unexpected delayed output at %d: %v", i, y)
		}
	}
	clip := processor{gain: 1}
	y, clipped := clip.sample(2, &AudioOptions{Volume: 1, Enabled: true})
	if !clipped || y > .9991 {
		t.Fatal("Peak clipping failed")
	}
}
func TestNativeRingReadersAreIndependent(t *testing.T) {
	var ring sampleRing
	data := make([]byte, 1200*4)
	for i := 0; i < 1200; i++ {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(float32(i)))
	}
	ring.push(data)
	a, b := ringReader{}, ringReader{}
	a.prepare(&ring)
	b.prepare(&ring)
	for i := 0; i < 10; i++ {
		v, ok := a.next(&ring)
		if !ok || v != float32(240+i) {
			t.Fatalf("reader a %v %v", v, ok)
		}
	}
	v, ok := b.next(&ring)
	if !ok || v != 240 {
		t.Fatal("One output consumed another output's input")
	}
	for i := 0; i < 1200; i++ {
		a.next(&ring)
	}
	a.prepare(&ring)
	if a.ready {
		t.Fatal("Underflow should re-prime the reader")
	}
}
func TestNativeControlsValidation(t *testing.T) {
	var engine nativeEngine
	if engine.Configure(AudioOptions{Volume: 2}) == nil {
		t.Fatal("Volume above 100% accepted")
	}
	if engine.Configure(AudioOptions{Volume: float32(math.NaN())}) == nil {
		t.Fatal("NaN accepted")
	}
	if engine.Configure(AudioOptions{DelayMS: 51}) == nil {
		t.Fatal("Invalid delay accepted")
	}
}
