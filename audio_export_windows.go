//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func chooseAudioExportFile(owner uintptr, name, extension string) (string, error) {
	if extension != ".wav" && extension != ".mp3" && extension != ".webm" {
		return "", errors.New("Unsupported audio file type")
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune("<>:\"/\\|?*", r) || r < 32 {
			return '_'
		}
		return r
	}, name)
	kind, titleText := "Audio file", "Export original audio"
	if extension == ".webm" {
		kind, titleText = "Video recording", "Save sampler recording"
	}
	description, _ := windows.UTF16FromString(kind + " (*" + extension + ")")
	pattern, _ := windows.UTF16FromString("*" + extension)
	filter := append(description, pattern...)
	filter = append(filter, 0)
	file := make([]uint16, 32768)
	initial, _ := windows.UTF16FromString(name + extension)
	copy(file, initial)
	title, _ := windows.UTF16PtrFromString(titleText)
	ext, _ := windows.UTF16PtrFromString(strings.TrimPrefix(extension, "."))
	args := openFileName{Owner: owner, Filter: &filter[0], File: &file[0], MaxFile: uint32(len(file)), Title: title, DefaultExtension: ext, Flags: 0x800 | 0x80000 | 8 | 2}
	args.Size = uint32(unsafe.Sizeof(args))
	result, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetSaveFileNameW").Call(uintptr(unsafe.Pointer(&args)))
	if result == 0 {
		return "", nil
	}
	return windows.UTF16ToString(file), nil
}
func (s *appStore) ExportAudio(id, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return err
	}
	for _, b := range l.Boards {
		for _, sound := range b.Sounds {
			if sound.ID == id {
				if filepath.Base(sound.File) != sound.File {
					return errors.New("Invalid audio path")
				}
				data, err := os.ReadFile(filepath.Join(s.root, "sounds", sound.File))
				if err != nil {
					return err
				}
				f, err := os.CreateTemp(filepath.Dir(path), ".quiet-audio-")
				if err != nil {
					return err
				}
				temp := f.Name()
				defer os.Remove(temp)
				if _, err = f.Write(data); err != nil {
					f.Close()
					return err
				}
				if err = f.Close(); err != nil {
					return err
				}
				return os.Rename(temp, path)
			}
		}
	}
	return errors.New("Sound not found")
}
