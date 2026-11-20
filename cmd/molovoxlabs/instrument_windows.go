//go:build windows && amd64

package main

import (
	"errors"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const instrumentNoteCount = 16 * 128

type instrumentBank struct {
	rack           vstRack
	plugins        []MicFilter // Protected by nativeEngine.configMu.
	keyboard, midi atomic.Bool
	computerNotes  [128]atomic.Uint32
	midiNotes      [instrumentNoteCount]atomic.Uint32
}

type instrumentVoice struct {
	previous [instrumentNoteCount]uint8
	events   [instrumentNoteCount]uint32
}

func isVSTInstrument(plugin VSTPlugin) bool {
	for _, category := range strings.Split(plugin.Category, "|") {
		if strings.EqualFold(strings.TrimSpace(category), "Instrument") {
			return true
		}
	}
	return false
}

func (e *nativeEngine) setInstruments(ids []string, catalog []VSTPlugin) error {
	if len(ids) > 4 {
		return errors.New("Load up to four VST3 instruments")
	}
	plugins := make([]MicFilter, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		matched := false
		for _, plugin := range catalog {
			if id != plugin.Path+"|"+plugin.ClassID || !isVSTInstrument(plugin) {
				continue
			}
			if seen[id] {
				return errors.New("This instrument is already loaded")
			}
			f := MicFilter{ID: id, Kind: "vst3", Enabled: true, PluginPath: plugin.Path, ClassID: plugin.ClassID, PluginName: plugin.Name}
			if err := validVSTFilter(f); err != nil {
				return err
			}
			plugins = append(plugins, f)
			seen[id], matched = true, true
			break
		}
		if !matched {
			return errors.New("Scan for the installed VST3 instrument before loading it")
		}
	}
	e.configMu.Lock()
	defer e.configMu.Unlock()
	if err := e.instruments.rack.configurePlugins(plugins, max(1, int(e.outputCount.Load())), true); err != nil {
		return err
	}
	e.instruments.plugins = plugins
	if !e.running.Load() && !e.closing.Load() && e.outputCount.Load() == 0 {
		e.instruments.rack.collectRetired()
	}
	return nil
}

func (b *instrumentBank) loaded() bool {
	snapshot := b.rack.current.Load()
	return snapshot != nil && len(snapshot.playback) > 0
}

func (b *instrumentBank) configure(outputs int) error {
	if len(b.plugins) == 0 {
		return nil
	}
	return b.rack.configurePlugins(b.plugins, outputs, true)
}

func (b *instrumentBank) setInput(keyboard, midi bool) {
	if b.keyboard.Swap(keyboard) && !keyboard {
		for i := range b.computerNotes {
			b.computerNotes[i].Store(0)
		}
	}
	if b.midi.Swap(midi) && !midi {
		b.clearMIDINotes()
	}
}

func (b *instrumentBank) clearMIDINotes() {
	for i := range b.midiNotes {
		b.midiNotes[i].Store(0)
	}
}

func (b *instrumentBank) keyNote(note, velocity uint32) error {
	if note > 127 || velocity > 127 {
		return errors.New("Invalid MIDI note or velocity")
	}
	if b.keyboard.Load() {
		b.computerNotes[note].Store(velocity)
	}
	return nil
}

func (b *instrumentBank) midiPacket(packet uint32) bool {
	if !b.midi.Load() {
		return false
	}
	status := packet & 255
	channel := status & 15
	switch status & 0xf0 {
	case 0x80, 0x90:
		note := &b.midiNotes[channel*128+((packet>>8)&127)]
		var velocity uint32
		if status&0xf0 == 0x90 {
			velocity = (packet >> 16) & 127
		}
		note.Store(velocity)
		if !b.midi.Load() {
			note.Store(0)
		}
	case 0xb0:
		if cc := (packet >> 8) & 127; cc == 120 || cc == 123 {
			for i := channel * 128; i < (channel+1)*128; i++ {
				b.midiNotes[i].Store(0)
			}
		}
	}
	return true
}

func (n *instrumentVoice) collect(b *instrumentBank) int {
	count := 0
	// shortcut: states sampled once per 128-frame block, queue events if sub-block retriggers are needed.
	for i := range n.previous {
		velocity := b.midiNotes[i].Load()
		if i < 128 {
			velocity = max(velocity, b.computerNotes[i].Load())
		}
		previous := n.previous[i]
		n.previous[i] = uint8(velocity)
		if (previous == 0) == (velocity == 0) {
			continue
		}
		n.events[count] = uint32(i) | (velocity << 11)
		count++
	}
	return count
}

func (u *vstUnit) instrumentSample(index int, b *instrumentBank) float32 {
	if index < 0 || index >= len(u.voices) {
		return 0
	}
	v := &u.voices[index]
	if v.failed.Load() {
		return 0
	}
	y := v.output[v.at]
	v.at++
	if v.at == len(v.output) {
		v.at = 0
		count := v.notes.collect(b)
		ok, _, _ := syscall.SyscallN(u.bridge.instrument.Addr(), u.handle, uintptr(index), uintptr(unsafe.Pointer(&v.output[0])), uintptr(len(v.output)), uintptr(unsafe.Pointer(&v.notes.events[0])), uintptr(count))
		if ok == 0 {
			v.failed.Store(true)
			return 0
		}
	}
	return y
}
