//go:build windows

package main

import "testing"

func activeClipCount(e *nativeEngine) int {
	count := 0
	if voices := e.voices.Load(); voices != nil {
		for _, voice := range voices.Slots {
			if voice.Sequence != 0 {
				count++
			}
		}
	}
	return count
}
func recordingModeEngine(t *testing.T) *nativeEngine {
	t.Helper()
	e := &nativeEngine{renders: []*renderEndpoint{{ID: "record-output"}}}
	e.running.Store(true)
	samples := make([]float32, sampleRate)
	if err := e.PlaySound("first-video", samples, "video-board"); err != nil {
		t.Fatal(err)
	}
	if err := e.PlaySound("second-video", samples, "video-board"); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestSingleRecordingModeReplacesClipsAndRestoresLayeringAfterStop(t *testing.T) {
	e := recordingModeEngine(t)
	if activeClipCount(e) != 2 {
		t.Fatal("Fixture did not layer")
	}
	if err := e.beginSamplerTap("recording", "record-output", "single"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 1 || e.voices.Load().Slots[1].ID != "second-video" {
		t.Fatal("Single recording did not keep the newest existing clip")
	}
	if err := e.PlaySound("audio-pad", make([]float32, sampleRate), "audio-board"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 1 || e.voices.Load().Slots[0].ID != "audio-pad" {
		t.Fatal("Audio and video stacked in single mode")
	}
	if err := e.PlaySound("new-video", make([]float32, sampleRate), "video-board"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 1 || e.voices.Load().Slots[0].ID != "new-video" {
		t.Fatal("New video did not replace audio")
	}
	e.endSamplerTap("recording")
	if err := e.PlaySound("layered-again", make([]float32, sampleRate), "audio-board"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 2 {
		t.Fatal("Single recording policy remained after stop/abort")
	}
}
func TestLayeredRecordingModeKeepsSimultaneousClips(t *testing.T) {
	e := recordingModeEngine(t)
	if err := e.beginSamplerTap("recording", "record-output", "layered"); err != nil {
		t.Fatal(err)
	}
	if err := e.PlaySound("audio-pad", make([]float32, sampleRate), "audio-board"); err != nil {
		t.Fatal(err)
	}
	if activeClipCount(e) != 3 {
		t.Fatal("Layered mode unexpectedly replaced clips")
	}
}
func TestInvalidRecordingModeDoesNotInterruptClips(t *testing.T) {
	e := recordingModeEngine(t)
	if err := e.beginSamplerTap("recording", "record-output", "bad-mode"); err == nil {
		t.Fatal("Unknown mode accepted")
	}
	if activeClipCount(e) != 2 || e.samplerTap.Load() != nil {
		t.Fatal("Invalid mode changed playback")
	}
}
