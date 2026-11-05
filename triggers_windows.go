//go:build windows

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

type KeyBinding struct {
	Key       uint32
	Modifiers uint32
	Label     string
}
type MIDIBinding struct {
	Kind    string
	Channel uint32
	Number  uint32
}

func midiKey(m MIDIBinding) string { return fmt.Sprintf("%s:%d:%d", m.Kind, m.Channel, m.Number) }
func validateBindings(l libraryState) error {
	keys := map[string]bool{}
	midi := map[string]bool{}
	for _, b := range l.Boards {
		for _, s := range b.Sounds {
			for _, k := range soundKeys(s) {
				if k.Key < 1 || k.Key > 255 || k.Modifiers&^uint32(7) != 0 || k.Key == 16 || k.Key == 17 || k.Key == 18 || k.Key == 91 || k.Key == 92 {
					return errors.New("Choose a supported key combination")
				}
				id := fmt.Sprintf("%d:%d", k.Modifiers, k.Key)
				if keys[id] {
					return errors.New("That keyboard shortcut is already assigned")
				}
				keys[id] = true
			}
			for _, m := range soundMIDIs(s) {
				if (m.Kind != "note" && m.Kind != "cc") || m.Channel < 1 || m.Channel > 16 || m.Number > 127 {
					return errors.New("Invalid MIDI binding")
				}
				id := midiKey(m)
				if midi[id] {
					return errors.New("That MIDI trigger is already assigned")
				}
				midi[id] = true
			}
		}
	}
	return nil
}
func (s *appStore) SetBindings(id string, key *KeyBinding, midi *MIDIBinding) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i := range l.Boards {
		for j := range l.Boards[i].Sounds {
			if l.Boards[i].Sounds[j].ID == id {
				l.Boards[i].Sounds[j].Hotkeys = nil
				l.Boards[i].Sounds[j].MIDIs = nil
				l.Boards[i].Sounds[j].Hotkey = key
				l.Boards[i].Sounds[j].MIDI = midi
				if err = validateBindings(l); err != nil {
					return l, err
				}
				return l, s.write("soundboards.json", l)
			}
		}
	}
	return l, errors.New("Sound not found")
}

var user32 = windows.NewLazySystemDLL("user32.dll")
var registerHotKey = user32.NewProc("RegisterHotKey")
var unregisterHotKey = user32.NewProc("UnregisterHotKey")
var postThread = user32.NewProc("PostThreadMessageW")

type winMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type hotkeyCommand struct {
	Library libraryState
	Enabled bool
	Stop    bool
	Result  chan error
}
type globalKeys struct {
	thread   uint32
	commands chan hotkeyCommand
	done     chan struct{}
	trigger  func(string)
}

func newGlobalKeys(trigger func(string)) *globalKeys {
	g := &globalKeys{commands: make(chan hotkeyCommand, 8), done: make(chan struct{}), trigger: trigger}
	ready := make(chan struct{})
	go g.loop(ready)
	<-ready
	return g
}
func (g *globalKeys) loop(ready chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(g.done)
	g.thread = windows.GetCurrentThreadId()
	var msg winMessage
	user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
	close(ready)
	current := map[int]string{}
	bindings := map[int]KeyBinding{}
	clearKeys := func() {
		for id := range current {
			unregisterHotKey.Call(0, uintptr(id))
		}
		current = map[int]string{}
	}
	defer clearKeys()
	for {
		result, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if result == 0 || int32(result) == -1 {
			return
		}
		if msg.Message == 0x0312 {
			if id := current[int(msg.WParam)]; id != "" {
				g.trigger(id)
			}
		}
		if msg.Message == 0x8001 {
			for {
				select {
				case cmd := <-g.commands:
					if cmd.Stop {
						cmd.Result <- nil
						return
					}
					previous, previousKeys := current, bindings
					clearKeys()
					bindings = map[int]KeyBinding{}
					var err error
					nextID := 1
					if cmd.Enabled {
					registerLoop:
						for _, b := range cmd.Library.Boards {
							for _, sound := range b.Sounds {
								for _, key := range soundKeys(sound) {
									ok, _, _ := registerHotKey.Call(0, uintptr(nextID), uintptr(key.Modifiers|0x4000), uintptr(key.Key))
									if ok == 0 {
										err = fmt.Errorf("Shortcut %s is reserved or already used by another app", key.Label)
										break registerLoop
									}
									current[nextID] = sound.ID
									bindings[nextID] = key
									nextID++
								}
							}
						}
					}
					if err != nil {
						clearKeys()
						bindings = previousKeys
						for id, key := range previousKeys {
							ok, _, _ := registerHotKey.Call(0, uintptr(id), uintptr(key.Modifiers|0x4000), uintptr(key.Key))
							if ok != 0 {
								current[id] = previous[id]
							}
						}
					}
					cmd.Result <- err
				default:
					goto drained
				}
			}
		drained:
		}
	}
}
func (g *globalKeys) Apply(l libraryState, enabled bool) error {
	reply := make(chan error, 1)
	select {
	case <-g.done:
		return errors.New("Keyboard service stopped")
	case g.commands <- hotkeyCommand{Library: l, Enabled: enabled, Result: reply}:
	}
	ok, _, _ := postThread.Call(uintptr(g.thread), 0x8001, 0, 0)
	if ok == 0 {
		return errors.New("Could not update global keyboard shortcuts")
	}
	select {
	case err := <-reply:
		return err
	case <-g.done:
		return errors.New("Keyboard service stopped")
	}
}
func (g *globalKeys) Close() {
	reply := make(chan error, 1)
	select {
	case <-g.done:
		return
	case g.commands <- hotkeyCommand{Stop: true, Result: reply}:
	}
	postThread.Call(uintptr(g.thread), 0x8001, 0, 0)
	<-g.done
}

type MIDIDevice struct{ Name string }
type midiCaps struct {
	Manufacturer, Product uint16
	Version               uint32
	Name                  [32]uint16
	Support               uint32
}

var winmm = windows.NewLazySystemDLL("winmm.dll")

func midiDevices() []MIDIDevice {
	count, _, _ := winmm.NewProc("midiInGetNumDevs").Call()
	result := []MIDIDevice{}
	for i := uintptr(0); i < count; i++ {
		var caps midiCaps
		code, _, _ := winmm.NewProc("midiInGetDevCapsW").Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
		if code == 0 {
			result = append(result, MIDIDevice{Name: windows.UTF16ToString(caps.Name[:])})
		}
	}
	return result
}
func decodeMIDI(packet uint32) (MIDIBinding, uint32, bool) {
	status := packet & 255
	number := (packet >> 8) & 127
	value := (packet >> 16) & 127
	kind := ""
	switch status & 0xf0 {
	case 0x90:
		if value > 0 {
			kind = "note"
		}
	case 0xb0:
		kind = "cc"
	}
	return MIDIBinding{Kind: kind, Channel: (status & 15) + 1, Number: number}, value, kind != ""
}

type midiInput struct {
	mu               sync.Mutex
	handle, callback uintptr
	name             string
	events           chan MIDIBinding
	cc               [2048]atomic.Uint32
}

func newMIDIInput(events chan MIDIBinding) *midiInput {
	m := &midiInput{events: events}
	m.callback = windows.NewCallback(func(_, message, _, packet, _ uintptr) uintptr {
		if message != 0x3c3 && message != 0x3cc {
			return 0
		}
		binding, value, ok := decodeMIDI(uint32(packet))
		if !ok {
			return 0
		}
		if binding.Kind == "cc" {
			old := m.cc[(binding.Channel-1)*128+binding.Number].Swap(value)
			if value == 0 || old != 0 {
				return 0
			}
		}
		select {
		case m.events <- binding:
		default:
		}
		return 0
	})
	return m
}
func (m *midiInput) Open(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == m.name {
		return nil
	}
	m.closeLocked()
	if name == "" {
		return nil
	}
	count, _, _ := winmm.NewProc("midiInGetNumDevs").Call()
	for i := uintptr(0); i < count; i++ {
		var caps midiCaps
		code, _, _ := winmm.NewProc("midiInGetDevCapsW").Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
		if code != 0 || windows.UTF16ToString(caps.Name[:]) != name {
			continue
		}
		code, _, _ = winmm.NewProc("midiInOpen").Call(uintptr(unsafe.Pointer(&m.handle)), i, m.callback, 0, 0x30000)
		if code != 0 {
			return fmt.Errorf("Cannot open MIDI device (code %d); it may be in use", code)
		}
		code, _, _ = winmm.NewProc("midiInStart").Call(m.handle)
		if code != 0 {
			m.closeLocked()
			return fmt.Errorf("Cannot start MIDI input (%d)", code)
		}
		for i := range m.cc {
			m.cc[i].Store(0)
		}
		m.name = name
		return nil
	}
	return errors.New("Saved MIDI device is not connected")
}
func (m *midiInput) closeLocked() {
	if m.handle != 0 {
		winmm.NewProc("midiInStop").Call(m.handle)
		winmm.NewProc("midiInReset").Call(m.handle)
		winmm.NewProc("midiInClose").Call(m.handle)
		m.handle = 0
	}
	m.name = ""
}
func (m *midiInput) Close() { m.mu.Lock(); defer m.mu.Unlock(); m.closeLocked() }

type triggerStatus struct {
	FilterRevision uint64
	FilterOptions  *AudioOptions
	Sequence       uint64
	MIDI           *MIDIBinding
	Message        string
}
type triggerMap struct {
	MIDI   map[string]string
	Sounds map[string]bool
	Media  map[string]triggerMedia
}
type triggerMedia struct{ BoardID, File string }

func activeTriggerLibrary(l libraryState, p Preferences) libraryState {
	id := p.BoardID
	found := false
	for _, b := range l.Boards {
		if b.ID == id && b.Kind != "video" {
			found = true
			break
		}
	}
	if !found {
		for _, b := range l.Boards {
			if b.Kind != "video" {
				id = b.ID
				break
			}
		}
	}
	selected := libraryState{}
	for _, b := range l.Boards {
		if b.ID == id {
			selected.Boards = append(selected.Boards, b)
			break
		}
	}
	videoID := p.SamplerBoardID
	for _, b := range l.Boards {
		if b.Kind == "video" && b.ID == videoID {
			selected.Boards = append(selected.Boards, b)
			return triggerLibrary(selected, p)
		}
	}
	for _, b := range l.Boards {
		if b.Kind == "video" {
			selected.Boards = append(selected.Boards, b)
			break
		}
	}
	return triggerLibrary(selected, p)
}

type triggerService struct {
	filterRevision atomic.Uint64
	FilterChanged  func(Preferences)
	SoundPlayed    func(string)
	suspended      atomic.Bool
	store          *appStore
	engine         *nativeEngine
	keys           *globalKeys
	midi           *midiInput
	maps           atomic.Pointer[triggerMap]
	learn          atomic.Bool
	events         chan MIDIBinding
	plays          chan string
	stop           chan struct{}
	done           chan struct{}
	mu             sync.Mutex
	status         triggerStatus
	cache          map[string][]float32
}

func newTriggerService(store *appStore, engine *nativeEngine) *triggerService {
	t := &triggerService{store: store, engine: engine, events: make(chan MIDIBinding, 64), plays: make(chan string, 32), stop: make(chan struct{}), done: make(chan struct{}), cache: map[string][]float32{}}
	t.keys = newGlobalKeys(func(id string) {
		select {
		case t.plays <- id:
		default:
		}
	})
	t.midi = newMIDIInput(t.events)
	go t.loop()
	return t
}
func (t *triggerService) Configure(p Preferences) error {
	l, err := t.store.Library()
	if err != nil {
		return err
	}
	if err = validateBindings(triggerLibrary(l, p)); err != nil {
		return err
	}
	l = activeTriggerLibrary(l, p)
	mapping := &triggerMap{MIDI: map[string]string{}, Sounds: map[string]bool{}, Media: map[string]triggerMedia{}}
	for _, b := range l.Boards {
		for _, s := range b.Sounds {
			if !strings.HasPrefix(s.ID, "@") {
				mapping.Sounds[s.ID] = true
				mapping.Media[s.ID] = triggerMedia{BoardID: b.ID, File: s.File}
			}
			for _, m := range soundMIDIs(s) {
				mapping.MIDI[midiKey(m)] = s.ID
			}
		}
	}
	t.maps.Store(mapping)
	if err = t.keys.Apply(l, p.GlobalKeys && !t.suspended.Load()); err != nil {
		return err
	}
	return t.midi.Open(p.MIDIInput)
}
func (t *triggerService) Play(id string) error {
	// Decode outside audio callbacks; cache up to eight recently used sounds.
	t.mu.Lock()
	defer t.mu.Unlock()
	if mapping := t.maps.Load(); mapping != nil {
		if media, ok := mapping.Media[id]; ok {
			samples := t.cache[id]
			if samples == nil {
				var err error
				samples, err = t.store.SoundFile(media.File)
				if err != nil {
					return err
				}
				if len(t.cache) >= 8 {
					t.cache = map[string][]float32{}
				}
				t.cache[id] = samples
			}
			err := t.engine.PlaySound(id, samples, media.BoardID)
			if err == nil && t.SoundPlayed != nil {
				t.SoundPlayed(id)
			}
			return err
		}
	}
	samples := t.cache[id]
	if samples == nil {
		var err error
		samples, err = t.store.Sound(id)
		if err != nil {
			return err
		}
		if len(t.cache) >= 8 {
			t.cache = map[string][]float32{}
		}
		t.cache[id] = samples
	}
	l, err := t.store.Library()
	if err != nil {
		return err
	}
	for _, board := range l.Boards {
		for _, sound := range board.Sounds {
			if sound.ID == id {
				err := t.engine.PlaySound(id, samples, board.ID)
				if err == nil && t.SoundPlayed != nil {
					t.SoundPlayed(id)
				}
				return err
			}
		}
	}
	return errors.New("Sound no longer exists")
}
func (t *triggerService) loop() {
	defer close(t.done)
	for {
		select {
		case <-t.stop:
			return
		case id := <-t.plays:
			err := t.Run(id)
			t.mu.Lock()
			if err != nil {
				t.status.Message = err.Error()
			} else {
				if strings.HasPrefix(id, "@") {
					t.status.Message = "Filter state changed by global shortcut"
				} else {
					t.status.Message = "Sound triggered by global shortcut"
				}
			}
			t.mu.Unlock()
		case m := <-t.events:
			t.mu.Lock()
			t.status.Sequence++
			copy := m
			t.status.MIDI = &copy
			t.mu.Unlock()
			if t.learn.Load() {
				continue
			}
			if mapping := t.maps.Load(); mapping != nil {
				if id := mapping.MIDI[midiKey(m)]; id != "" {
					err := t.Run(id)
					t.mu.Lock()
					if err != nil {
						t.status.Message = err.Error()
					} else {
						if strings.HasPrefix(id, "@") {
							t.status.Message = "Filter state changed by MIDI"
						} else {
							t.status.Message = "Sound triggered by MIDI"
						}
					}
					t.mu.Unlock()
				}
			}
		}
	}
}
func (t *triggerService) Status() triggerStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := t.status
	result.FilterRevision = t.filterRevision.Load()
	result.FilterOptions = t.engine.options.Load()
	return result
}
func (t *triggerService) Run(id string) error {
	if !strings.HasPrefix(id, "@") {
		// A queued key/MIDI event may belong to the board just left.
		if mapping := t.maps.Load(); mapping == nil || !mapping.Sounds[id] {
			return nil
		}
		return t.Play(id)
	}
	p, err := t.store.ToggleFilters(id, t.engine.options.Load())
	if err != nil {
		return err
	}
	if err = t.engine.Configure(p.Options); err != nil {
		return err
	}
	t.filterRevision.Add(1)
	if t.FilterChanged != nil {
		t.FilterChanged(p)
	}
	return nil
}

func (t *triggerService) Close()              { t.keys.Close(); t.midi.Close(); close(t.stop); <-t.done }
func (t *triggerService) Learn(on bool) error { t.learn.Store(on); return nil }
