//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type BoardRemoval struct {
	Library  libraryState
	Warnings []string
}

func (s *appStore) RemoveBoard(id string) (BoardRemoval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return BoardRemoval{}, err
	}
	index := -1
	for i, b := range l.Boards {
		if b.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return BoardRemoval{}, errors.New("Soundboard not found")
	}
	removed := l.Boards[index]
	l.Boards = append(l.Boards[:index:index], l.Boards[index+1:]...)
	hasAudioBoard := false
	for _, b := range l.Boards {
		if b.Kind != "video" {
			hasAudioBoard = true
		}
	}
	if !hasAudioBoard {
		l.Boards = append(l.Boards, SoundBoard{ID: newID(), Name: "My soundboard", Sounds: []SoundEntry{}})
	}
	if removed.Kind == "video" {
		hasVideo := false
		for _, b := range l.Boards {
			if b.Kind == "video" {
				hasVideo = true
			}
		}
		if !hasVideo {
			l.Boards = append(l.Boards, SoundBoard{ID: newID(), Kind: "video", Name: "Video sampler", Sounds: []SoundEntry{}})
		}
	}
	if err = s.write("soundboards.json", l); err != nil {
		return BoardRemoval{}, err
	}
	result := BoardRemoval{Library: l, Warnings: []string{}}
	used := map[string]bool{}
	for _, b := range l.Boards {
		for _, clip := range b.Sounds {
			used[strings.ToLower(clip.File)] = true
		}
	}
	for _, clip := range removed.Sounds {
		name := clip.File
		if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || filepath.Base(name) != name || used[strings.ToLower(name)] {
			continue
		}
		if err = os.Remove(filepath.Join(s.root, "sounds", name)); err != nil && !os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, "Could not remove copied audio: "+name)
		}
	}
	return result, nil
}
