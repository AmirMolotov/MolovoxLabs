//go:build windows

package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

type boardPackage struct {
	Version int
	Board   SoundBoard
}

func chooseBoardFile(owner uintptr, save bool) (string, error) {
	description, _ := windows.UTF16FromString("Molovox Labs soundboard (*.qlboard)")
	pattern, _ := windows.UTF16FromString("*.qlboard")
	filter := append(description, pattern...)
	filter = append(filter, 0)
	title := "Import soundboard"
	method := "GetOpenFileNameW"
	flags := uint32(0x1000 | 0x800 | 0x80000 | 8)
	file := make([]uint16, 32768)
	if save {
		title = "Export soundboard"
		method = "GetSaveFileNameW"
		flags = 0x800 | 0x80000 | 8 | 2
		name, _ := windows.UTF16FromString("soundboard.qlboard")
		copy(file, name)
	}
	titlePtr, _ := windows.UTF16PtrFromString(title)
	extension, _ := windows.UTF16PtrFromString("qlboard")
	args := openFileName{Owner: owner, Filter: &filter[0], File: &file[0], MaxFile: uint32(len(file)), Title: titlePtr, Flags: flags, DefaultExtension: extension}
	args.Size = uint32(unsafe.Sizeof(args))
	result, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc(method).Call(uintptr(unsafe.Pointer(&args)))
	if result == 0 {
		code, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return "", fmt.Errorf("File picker failed: %x", code)
		}
		return "", nil
	}
	return windows.UTF16ToString(file), nil
}
func (s *appStore) ExportBoard(id, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return err
	}
	var board *SoundBoard
	for i := range l.Boards {
		if l.Boards[i].ID == id {
			board = &l.Boards[i]
			break
		}
	}
	if board == nil {
		return errors.New("Board not found")
	}
	if board.Kind == "video" {
		return errors.New("Use the sampler recorder to export video performances; sampler board packages are not supported")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".quiet-board-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	archive := zip.NewWriter(f)
	fail := func(err error) error { archive.Close(); f.Close(); return err }
	manifest, err := json.Marshal(boardPackage{Version: 1, Board: *board})
	if err != nil {
		return fail(err)
	}
	w, err := archive.Create("manifest.json")
	if err != nil {
		return fail(err)
	}
	if _, err = w.Write(manifest); err != nil {
		return fail(err)
	}
	for _, sound := range board.Sounds {
		if filepath.Base(sound.File) != sound.File {
			return fail(errors.New("Invalid stored sound path"))
		}
		input, err := os.Open(filepath.Join(s.root, "sounds", sound.File))
		if err != nil {
			return fail(err)
		}
		w, err = archive.Create("sounds/" + sound.File)
		if err == nil {
			_, err = io.Copy(w, input)
		}
		input.Close()
		if err != nil {
			return fail(err)
		}
	}
	if err = archive.Close(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func readZipFile(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, errors.New("Soundboard package is too large")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if int64(len(data)) > max {
		return nil, errors.New("Soundboard entry is too large")
	}
	return data, err
}
func (s *appStore) ImportBoard(path string, importShortcuts ...bool) (libraryState, error) {
	includeShortcuts := len(importShortcuts) == 0 || importShortcuts[0]
	z, err := zip.OpenReader(path)
	if err != nil {
		return libraryState{}, err
	}
	defer z.Close()
	entries := map[string]*zip.File{}
	for _, f := range z.File {
		if strings.Contains(f.Name, "\\") || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "..") {
			return libraryState{}, errors.New("Invalid path in soundboard package")
		}
		if entries[f.Name] != nil {
			return libraryState{}, errors.New("Duplicate file in soundboard package")
		}
		entries[f.Name] = f
	}
	manifest := entries["manifest.json"]
	if manifest == nil {
		return libraryState{}, errors.New("Not a Molovox Labs soundboard package")
	}
	data, err := readZipFile(manifest, 1024*1024)
	if err != nil {
		return libraryState{}, err
	}
	var pkg boardPackage
	if err = json.Unmarshal(data, &pkg); err != nil {
		return libraryState{}, err
	}
	if pkg.Version != 1 {
		return libraryState{}, errors.New("Unsupported soundboard package version")
	}
	if pkg.Board.Kind == "video" {
		return libraryState{}, errors.New("Video sampler board packages are not supported")
	}
	for _, sound := range pkg.Board.Sounds {
		if sound.Video != nil {
			return libraryState{}, errors.New("Video samples require their source video library")
		}
	}
	if len(pkg.Board.Sounds) > 64 || strings.TrimSpace(pkg.Board.Name) == "" || len(pkg.Board.Name) > 80 {
		return libraryState{}, errors.New("Invalid soundboard manifest")
	}
	audioFiles := make([][]byte, len(pkg.Board.Sounds))
	var total int64
	for i := range pkg.Board.Sounds {
		sound := &pkg.Board.Sounds[i]
		if sound.File == "" || filepath.Base(sound.File) != sound.File {
			return libraryState{}, errors.New("Invalid audio path in package")
		}
		entry := entries["sounds/"+sound.File]
		if entry == nil {
			return libraryState{}, errors.New("Soundboard package is missing an audio file")
		}
		total += int64(entry.UncompressedSize64)
		if total > 200*1024*1024 {
			return libraryState{}, errors.New("Maximum imported board size is 200 MB")
		}
		audioFiles[i], err = readZipFile(entry, 50*1024*1024)
		if err != nil {
			return libraryState{}, err
		}
		samples, err := decodeSound(audioFiles[i], filepath.Ext(sound.File))
		if err != nil {
			return libraryState{}, fmt.Errorf("%s: %w", sound.Name, err)
		}
		sound.Seconds = float64(len(samples)) / sampleRate
		sound.Waveform = soundWaveform(samples)
		sound.ID = newID()
		sound.File = sound.ID + strings.ToLower(filepath.Ext(sound.File))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
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
	// Keep free imported assignments, including multiple triggers per clip.
	existingKeys := map[string]bool{}
	existingMIDI := map[string]bool{}
	var prefs Preferences
	s.read("preferences.json", &prefs)
	for _, board := range triggerLibrary(l, prefs).Boards {
		for _, sound := range board.Sounds {
			for _, k := range soundKeys(sound) {
				existingKeys[fmt.Sprintf("%d:%d", k.Modifiers, k.Key)] = true
			}
			for _, m := range soundMIDIs(sound) {
				existingMIDI[midiKey(m)] = true
			}
		}
	}
	for i := range pkg.Board.Sounds {
		sound := &pkg.Board.Sounds[i]
		if !includeShortcuts {
			sound.Hotkey, sound.MIDI = nil, nil
			sound.Hotkeys, sound.MIDIs = nil, nil
		}
		keys := []KeyBinding{}
		midis := []MIDIBinding{}
		for _, key := range soundKeys(*sound) {
			code := fmt.Sprintf("%d:%d", key.Modifiers, key.Key)
			if !existingKeys[code] {
				keys = append(keys, key)
				existingKeys[code] = true
			}
		}
		for _, m := range soundMIDIs(*sound) {
			code := midiKey(m)
			if !existingMIDI[code] {
				midis = append(midis, m)
				existingMIDI[code] = true
			}
		}
		sound.Hotkey = nil
		sound.MIDI = nil
		sound.Hotkeys = keys
		sound.MIDIs = midis
	}
	pkg.Board.ID = newID()
	l.Boards = append(l.Boards, pkg.Board)
	if err = validateBindings(l); err != nil {
		return l, err
	}
	if err = os.MkdirAll(filepath.Join(s.root, "sounds"), 0700); err != nil {
		return l, err
	}
	written := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, p := range written {
				os.Remove(p)
			}
		}
	}()
	for i, sound := range pkg.Board.Sounds {
		path := filepath.Join(s.root, "sounds", sound.File)
		if err = os.WriteFile(path, audioFiles[i], 0600); err != nil {
			return l, err
		}
		written = append(written, path)
	}
	if err = s.write("soundboards.json", l); err != nil {
		return l, err
	}
	committed = true
	return l, nil
}
