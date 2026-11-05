//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	mp3 "github.com/hajimehoshi/go-mp3"
	audio "github.com/intermernet/gominiaudio"
	"golang.org/x/sys/windows"
)

type Preferences struct {
	VSTFolders       []string
	FilterGroups     []FilterGroup
	AllFilterHotkeys []KeyBinding
	AllFilterMIDIs   []MIDIBinding
	GlobalKeys       bool
	MIDIInput        string
	BoardID          string
	SamplerBoardID   string
	Saved            bool
	AutoListen       bool
	Inputs           []InputSelection
	Outputs          []string
	Options          AudioOptions
}
type SoundEntry struct {
	Video    *VideoSample `json:",omitempty"`
	Hotkeys  []KeyBinding
	MIDIs    []MIDIBinding
	Waveform []float32
	Hotkey   *KeyBinding
	MIDI     *MIDIBinding
	ID       string
	Name     string
	File     string
	Seconds  float64
}
type SoundBoard struct {
	Kind   string `json:",omitempty"`
	ID     string
	Name   string
	Sounds []SoundEntry
}
type libraryState struct{ Boards []SoundBoard }
type appStore struct {
	mu      sync.Mutex
	mediaMu sync.Mutex
	root    string
}

func (s *appStore) read(name string, v interface{}) error {
	data, err := os.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func (s *appStore) write(name string, v interface{}) error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.root, ".settings-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.root, name))
}
func (s *appStore) LoadPreferences() (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Preferences{AutoListen: true, GlobalKeys: true, Options: AudioOptions{Volume: 1, Inverted: true, BoardVolume: 1}}
	err := s.read("preferences.json", &p)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err == nil && normalizePreferences(&p) {
		err = s.write("preferences.json", p)
	}
	return p, err
}
func (s *appStore) SavePreferences(p Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(p.Inputs) > 8 || len(p.Outputs) > 8 {
		return errors.New("Select at most eight inputs and outputs")
	}
	normalizePreferences(&p)
	p.Saved = true
	return s.write("preferences.json", p)
}
func (s *appStore) loadLibrary() (libraryState, error) {
	var l libraryState
	err := s.read("soundboards.json", &l)
	if os.IsNotExist(err) {
		return libraryState{Boards: []SoundBoard{{ID: "default", Name: "My soundboard", Sounds: []SoundEntry{}}}}, nil
	}
	return l, err
}
func (s *appStore) Library() (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	changed := false
	for i := range l.Boards {
		for j := range l.Boards[i].Sounds {
			sound := &l.Boards[i].Sounds[j]
			if len(sound.Waveform) == 0 && filepath.Base(sound.File) == sound.File {
				data, e := os.ReadFile(filepath.Join(s.root, "sounds", sound.File))
				if e != nil {
					continue
				}
				samples, e := decodeSound(data, filepath.Ext(sound.File))
				if e == nil {
					sound.Waveform = soundWaveform(samples)
					changed = true
				}
			}
		}
	}
	if changed {
		err = s.write("soundboards.json", l)
	}
	return l, err
}
func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *appStore) CreateBoard(name string) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return l, errors.New("Enter a board name (up to 80 characters)")
	}
	count := 0
	for _, b := range l.Boards {
		if b.Kind != "video" {
			count++
		}
	}
	if count >= 20 {
		return l, errors.New("Maximum 20 soundboards")
	}
	l.Boards = append(l.Boards, SoundBoard{ID: newID(), Name: name, Sounds: []SoundEntry{}})
	return l, s.write("soundboards.json", l)
}
func decodeSound(data []byte, extension string) ([]float32, error) {
	if len(data) > 50*1024*1024 {
		return nil, errors.New("Audio file is larger than 50 MB")
	}
	const maxFrames = sampleRate * 120
	result := make([]float32, 0)
	if strings.EqualFold(extension, ".mp3") {
		d, err := mp3.NewDecoder(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(d, int64(d.SampleRate())*4*121))
		if err != nil {
			return nil, err
		}
		frames := len(raw) / 4
		if frames > d.SampleRate()*120 {
			return nil, errors.New("Please use a clip up to two minutes long")
		}
		n := int(float64(frames) * sampleRate / float64(d.SampleRate()))
		result = make([]float32, n)
		mono := func(i int) float32 {
			if i >= frames {
				i = frames - 1
			}
			if i < 0 {
				return 0
			}
			return (float32(int16(binary.LittleEndian.Uint16(raw[i*4:]))) + float32(int16(binary.LittleEndian.Uint16(raw[i*4+2:])))) / 65536
		}
		for i := range result {
			p := float64(i) * float64(d.SampleRate()) / sampleRate
			j := int(p)
			f := float32(p - float64(j))
			result[i] = mono(j)*(1-f) + mono(j+1)*f
		}
	} else if strings.EqualFold(extension, ".wav") {
		cfg := audio.DecoderConfigInit(audio.FormatF32, 1, sampleRate)
		d, err := audio.NewDecoderMemory(data, &cfg)
		if err != nil {
			return nil, fmt.Errorf("Unsupported WAV encoding: %w", err)
		}
		defer d.Close()
		buf := make([]byte, 4096*4)
		for {
			n, err := d.Read(buf, 4096)
			if len(result)+int(n) > maxFrames {
				return nil, errors.New("Please use a clip up to two minutes long")
			}
			for i := 0; i < int(n); i++ {
				v := math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					v = 0
				}
				result = append(result, v)
			}
			if err != nil && err != io.EOF && err != audio.ErrAtEnd {
				return nil, err
			}
			if n == 0 || err == io.EOF || err == audio.ErrAtEnd {
				break
			}
		}
	} else {
		return nil, errors.New("Choose a WAV or MP3 file")
	}
	if len(result) == 0 {
		return nil, errors.New("Audio file contains no samples")
	}
	return result, nil
}
func (s *appStore) ImportFile(boardID, path string) (libraryState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return libraryState{}, err
	}
	if info.Size() > 50*1024*1024 {
		return libraryState{}, errors.New("Audio file is larger than 50 MB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return libraryState{}, err
	}
	samples, err := decodeSound(data, filepath.Ext(path))
	if err != nil {
		return libraryState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i := range l.Boards {
		if l.Boards[i].ID == boardID {
			if len(l.Boards[i].Sounds) >= 64 {
				return l, errors.New("Maximum 64 clips per board")
			}
			id := newID()
			file := id + strings.ToLower(filepath.Ext(path))
			if err = os.MkdirAll(filepath.Join(s.root, "sounds"), 0700); err != nil {
				return l, err
			}
			target := filepath.Join(s.root, "sounds", file)
			if err = os.WriteFile(target, data, 0600); err != nil {
				return l, err
			}
			l.Boards[i].Sounds = append(l.Boards[i].Sounds, SoundEntry{ID: id, Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), File: file, Seconds: float64(len(samples)) / sampleRate, Waveform: soundWaveform(samples)})
			if err = s.write("soundboards.json", l); err != nil {
				os.Remove(target)
				return l, err
			}
			return l, nil
		}
	}
	return l, errors.New("Soundboard not found")
}
func (s *appStore) Sound(id string) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return nil, err
	}
	for _, b := range l.Boards {
		for _, sound := range b.Sounds {
			if sound.ID == id {
				if filepath.Base(sound.File) != sound.File {
					return nil, errors.New("Invalid saved sound file")
				}
				data, err := os.ReadFile(filepath.Join(s.root, "sounds", sound.File))
				if err != nil {
					return nil, err
				}
				return decodeSound(data, filepath.Ext(sound.File))
			}
		}
	}
	return nil, errors.New("Sound not found")
}
func (s *appStore) SoundFile(file string) ([]float32, error) {
	if file == "" || file == "." || file == ".." || filepath.IsAbs(file) || filepath.Base(file) != file {
		return nil, errors.New("Invalid sound file")
	}
	data, err := os.ReadFile(filepath.Join(s.root, "sounds", file))
	if err != nil {
		return nil, err
	}
	return decodeSound(data, filepath.Ext(file))
}
func (s *appStore) RemoveSound(id string) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i := range l.Boards {
		for j, sound := range l.Boards[i].Sounds {
			if sound.ID == id {
				l.Boards[i].Sounds = append(l.Boards[i].Sounds[:j], l.Boards[i].Sounds[j+1:]...)
				if err = s.write("soundboards.json", l); err != nil {
					return l, err
				}
				shared := false
				for _, board := range l.Boards {
					for _, other := range board.Sounds {
						if strings.EqualFold(other.File, sound.File) {
							shared = true
						}
					}
				}
				if !shared && sound.File != "" && sound.File != "." && sound.File != ".." && filepath.Base(sound.File) == sound.File {
					os.Remove(filepath.Join(s.root, "sounds", sound.File))
				}
				return l, nil
			}
		}
	}
	// A stale UI may retry removal after the clip was already removed on disk.
	return l, nil
}

// Windows' standard file picker. It returns only the explicitly chosen local file.
type openFileName struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefaultExtension             *uint16
	CustomData, Hook             uintptr
	Template                     *uint16
	Reserved                     uintptr
	Reserved2, FlagsEx           uint32
}

func chooseSoundFile(owner uintptr) (string, error) {
	filter, _ := windows.UTF16FromString("Audio files (*.wav;*.mp3)")
	// Embedded NUL strings are supplied directly as UTF-16.
	filter = append(filter[:len(filter)-1], 0)
	pattern, _ := windows.UTF16FromString("*.wav;*.mp3")
	filter = append(filter, pattern...)
	filter = append(filter, 0)
	title, _ := windows.UTF16PtrFromString("Add a sound to Quiet Lab")
	file := make([]uint16, 32768)
	args := openFileName{Owner: owner, Filter: &filter[0], File: &file[0], MaxFile: uint32(len(file)), Title: title, Flags: 0x00001000 | 0x00000800 | 0x00080000 | 0x00000008}
	args.Size = uint32(unsafe.Sizeof(args))
	result, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&args)))
	if result == 0 {
		code, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return "", fmt.Errorf("File picker failed: %x", code)
		}
		return "", nil
	}
	return windows.UTF16ToString(file), nil
}
