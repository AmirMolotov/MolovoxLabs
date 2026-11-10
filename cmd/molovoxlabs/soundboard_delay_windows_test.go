//go:build windows

package main

import (
	"math"
	"testing"
)

func delayedSoundboardEngine() *nativeEngine {
	e := &nativeEngine{renders: []*renderEndpoint{{}}}
	e.running.Store(true)
	e.options.Store(&AudioOptions{SoundboardDelay: true})
	return e
}
func TestSoundboardDelayWaitsHalfASecondWithoutConsumingClipSamples(t *testing.T) {
	e := delayedSoundboardEngine()
	samples := make([]float32, 960)
	for i := range samples {
		samples[i] = .5
	}
	if err := e.PlaySoundboard("clip", samples, "board"); err != nil {
		t.Fatal(err)
	}
	r := e.renders[0]
	voice := e.voices.Load()
	if voice.Slots[0].DelayFrames != 24000 {
		t.Fatal("incorrect delay")
	}
	for i := 0; i < 12000; i++ {
		if got := r.boardSample(voice); got != 0 {
			t.Fatal("audio escaped during delay", i, got)
		}
	}
	status := e.Snapshot().Playback
	if len(status) != 1 || !status[0].Queued || math.Abs(status[0].DelayRemaining-.25) > 1e-6 || status[0].Position != 0 || status[0].Progress != 0 {
		t.Fatal("queued tracker advanced audio", status)
	}
	for i := 0; i < 12000; i++ {
		if r.boardSample(voice) != 0 {
			t.Fatal("audio before half a second")
		}
	}
	if r.soundAt[0] != 0 {
		t.Fatal("clip consumed while waiting", r.soundAt[0])
	}
	for i := 0; i < 241; i++ {
		r.boardSample(voice)
	}
	status = e.Snapshot().Playback
	if len(status) != 1 || status[0].Queued || status[0].Position <= 0 {
		t.Fatal("playback did not start", status)
	}
}
func TestSoundboardDelayCanBeDisabledAndQueuedClipsCanBeCancelled(t *testing.T) {
	e := delayedSoundboardEngine()
	samples := make([]float32, 960)
	e.PlaySoundboard("delayed", samples, "board")
	e.options.Store(&AudioOptions{SoundboardDelay: false})
	e.PlaySoundboard("immediate", samples, "board")
	voices := e.voices.Load()
	if voices.Slots[0].DelayFrames != 24000 || voices.Slots[1].DelayFrames != 0 {
		t.Fatal("toggle did not affect new triggers independently")
	}
	e.StopSound("delayed")
	if e.voices.Load().Slots[0].Sequence != 0 {
		t.Fatal("queued clip was not cancelled")
	}
	e.StopSounds()
	if len(e.Snapshot().Playback) != 0 {
		t.Fatal("stop all retained pending playback")
	}
}
func TestSoundboardDelayDoesNotAffectVideoPadsOrTrimPreviews(t *testing.T) {
	for _, kind := range []string{"", "video"} {
		e := delayedSoundboardEngine()
		service := &triggerService{engine: e, cache: map[string][]float32{"pad": make([]float32, 960)}}
		service.maps.Store(&triggerMap{Sounds: map[string]bool{"pad": true}, Media: map[string]triggerMedia{"pad": {BoardID: "board", Kind: kind}}})
		if err := service.Run("pad"); err != nil {
			t.Fatal(err)
		}
		want := 0
		if kind != "video" {
			want = 24000
		}
		if got := e.voices.Load().Slots[0].DelayFrames; got != want {
			t.Fatal("incorrect delay for board kind", kind, got)
		}
	}
	e := delayedSoundboardEngine()
	if err := e.PlayPreview("@audio-preview", make([]float32, 960), "board", true); err != nil {
		t.Fatal(err)
	}
	if e.voices.Load().Slots[0].DelayFrames != 0 {
		t.Fatal("trimmer preview was delayed")
	}
}
func TestSoundboardDelayPreferenceSurvivesRestart(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	if err := s.SavePreferences(Preferences{Options: AudioOptions{SoundboardDelay: true}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadPreferences()
	if err != nil || !p.Options.SoundboardDelay {
		t.Fatal("delay setting was lost", err)
	}
}
