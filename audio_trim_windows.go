//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CreateAudioBite always creates a copy; existing pads and original files stay intact.
func (s *appStore) CreateAudioBite(board, name string, data []byte) (libraryState, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return libraryState{}, errors.New("Name your sound bite (up to 80 characters)")
	}
	samples, err := decodeSound(data, ".wav")
	if err != nil {
		return libraryState{}, err
	}
	if len(samples) < sampleRate/20 {
		return libraryState{}, errors.New("Select at least 0.05 seconds")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i, b := range l.Boards {
		if b.ID != board || b.Kind == "video" {
			continue
		}
		if len(b.Sounds) >= 64 {
			return l, errors.New("Maximum 64 clips per board")
		}
		id := newID()
		file := id + ".wav"
		dir := filepath.Join(s.root, "sounds")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return l, err
		}
		target := filepath.Join(dir, file)
		if err = os.WriteFile(target, data, 0600); err != nil {
			return l, err
		}
		l.Boards[i].Sounds = append(l.Boards[i].Sounds, SoundEntry{ID: id, Name: name, File: file, Seconds: float64(len(samples)) / sampleRate, Waveform: soundWaveform(samples)})
		if err = s.write("soundboards.json", l); err != nil {
			os.Remove(target)
		}
		return l, err
	}
	return l, errors.New("Choose a soundboard")
}

func (s *appStore) AudioClipPath(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return "", err
	}
	for _, b := range l.Boards {
		for _, sound := range b.Sounds {
			if sound.ID == id && sound.File != "" && filepath.Base(sound.File) == sound.File {
				return filepath.Join(s.root, "sounds", sound.File), nil
			}
		}
	}
	return "", errors.New("Sound not found")
}
