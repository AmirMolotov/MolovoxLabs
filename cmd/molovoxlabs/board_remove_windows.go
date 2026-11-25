//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const removalHistoryLimit = 20

type libraryRemoval struct {
	Board      SoundBoard
	FallbackID string `json:",omitempty"`
	PadID      string `json:",omitempty"`
	Index      int
}

type BoardRemoval struct {
	Library  libraryState
	Warnings []string
	BoardID  string
	removed  []SoundEntry
}

func (s *appStore) RemoveBoard(id string) (BoardRemoval, error) {
	return s.removeLibraryItem(id, "", false)
}

func takeLibraryItem(l *libraryState, boardID, padID string) (libraryRemoval, error) {
	for i, board := range l.Boards {
		if padID != "" {
			for j, sound := range board.Sounds {
				if sound.ID != padID {
					continue
				}
				l.Boards[i].Sounds = slices.Delete(board.Sounds, j, j+1)
				return libraryRemoval{Board: SoundBoard{ID: board.ID, Name: board.Name, Kind: board.Kind, Sounds: []SoundEntry{sound}}, PadID: padID, Index: j}, nil
			}
			continue
		}
		if board.ID != boardID {
			continue
		}
		l.Boards = slices.Delete(l.Boards, i, i+1)
		item := libraryRemoval{Board: board, Index: i}
		if !slices.ContainsFunc(l.Boards, func(b SoundBoard) bool { return (b.Kind == "video") == (board.Kind == "video") }) {
			name := "My soundboard"
			if board.Kind == "video" {
				name = "Video sampler"
			}
			item.FallbackID = newID()
			l.Boards = append(l.Boards, SoundBoard{ID: item.FallbackID, Kind: board.Kind, Name: name, Sounds: []SoundEntry{}})
		}
		return item, nil
	}
	if padID != "" {
		return libraryRemoval{}, errors.New("Pad no longer exists")
	}
	return libraryRemoval{}, errors.New("Soundboard not found")
}

func (s *appStore) removeLibraryItem(boardID, padID string, reversible bool) (BoardRemoval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return BoardRemoval{}, err
	}
	item, err := takeLibraryItem(&l, boardID, padID)
	if err != nil {
		if padID != "" {
			// A stale UI can retry an already completed pad removal.
			return BoardRemoval{Library: l}, nil
		}
		return BoardRemoval{}, err
	}
	discard := []libraryRemoval{item}
	if reversible {
		discard = nil
		l.RedoRemovals = nil
		l.UndoRemovals = append(l.UndoRemovals, item)
		if count := len(l.UndoRemovals) - removalHistoryLimit; count > 0 {
			discard, l.UndoRemovals = l.UndoRemovals[:count], l.UndoRemovals[count:]
		}
	}
	if err := s.write("soundboards.json", l); err != nil {
		return BoardRemoval{}, err
	}
	return BoardRemoval{Library: l, Warnings: s.discardRemovedAudio(l, discard), BoardID: item.Board.ID, removed: item.Board.Sounds}, nil
}

func (s *appStore) discardRemovedAudio(l libraryState, removals []libraryRemoval) []string {
	warnings := []string{}
	if len(removals) == 0 {
		return warnings
	}
	used := map[string]bool{}
	for _, board := range l.Boards {
		for _, clip := range board.Sounds {
			used[strings.ToLower(clip.File)] = true
		}
	}
	for _, item := range l.UndoRemovals {
		for _, clip := range item.Board.Sounds {
			used[strings.ToLower(clip.File)] = true
		}
	}
	for _, item := range removals {
		for _, clip := range item.Board.Sounds {
			name := clip.File
			if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || filepath.Base(name) != name || used[strings.ToLower(name)] {
				continue
			}
			if err := os.Remove(filepath.Join(s.root, "sounds", name)); err != nil && !os.IsNotExist(err) {
				warnings = append(warnings, "Could not remove copied audio: "+name)
			}
		}
	}
	return warnings
}

func (s *appStore) changeRemovalHistory(redo bool) (BoardRemoval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return BoardRemoval{}, err
	}
	var item libraryRemoval
	var removed []SoundEntry
	if redo {
		if len(l.RedoRemovals) == 0 {
			return BoardRemoval{}, errors.New("No removal to redo")
		}
		i := len(l.RedoRemovals) - 1
		previous := l.RedoRemovals[i]
		item, err = takeLibraryItem(&l, previous.Board.ID, previous.PadID)
		if err != nil {
			return BoardRemoval{}, err
		}
		removed = item.Board.Sounds
		l.RedoRemovals = l.RedoRemovals[:i]
		l.UndoRemovals = append(l.UndoRemovals, item)
	} else {
		if len(l.UndoRemovals) == 0 {
			return BoardRemoval{}, errors.New("No removal to undo")
		}
		i := len(l.UndoRemovals) - 1
		item = l.UndoRemovals[i]
		if item.PadID == "" {
			if slices.ContainsFunc(l.Boards, func(b SoundBoard) bool { return b.ID == item.Board.ID }) {
				return BoardRemoval{}, errors.New("This board is already in the library")
			}
			l.Boards = slices.DeleteFunc(l.Boards, func(b SoundBoard) bool {
				return b.ID == item.FallbackID && len(b.Sounds) == 0 && len(b.Groups) == 0
			})
			l.Boards = slices.Insert(l.Boards, max(0, min(item.Index, len(l.Boards))), item.Board)
		} else {
			board := slices.IndexFunc(l.Boards, func(b SoundBoard) bool { return b.ID == item.Board.ID })
			if board < 0 || len(item.Board.Sounds) != 1 {
				return BoardRemoval{}, errors.New("The pad's original board is missing")
			}
			clip := item.Board.Sounds[0]
			if slices.ContainsFunc(l.Boards[board].Sounds, func(s SoundEntry) bool { return s.ID == clip.ID }) {
				return BoardRemoval{}, errors.New("This pad is already in the library")
			}
			if !slices.ContainsFunc(l.Boards[board].Groups, func(g PadGroup) bool { return g.ID == clip.GroupID }) {
				clip.GroupID = ""
			}
			sounds := l.Boards[board].Sounds
			l.Boards[board].Sounds = slices.Insert(sounds, max(0, min(item.Index, len(sounds))), clip)
		}
		var preferences Preferences
		if err = s.read("preferences.json", &preferences); err != nil && !os.IsNotExist(err) {
			return BoardRemoval{}, err
		}
		if err = validateBindings(triggerLibrary(l, preferences)); err != nil {
			return BoardRemoval{}, err
		}
		l.UndoRemovals = l.UndoRemovals[:i]
		l.RedoRemovals = append(l.RedoRemovals, item)
	}
	if err = s.write("soundboards.json", l); err != nil {
		return BoardRemoval{}, err
	}
	return BoardRemoval{Library: l, Warnings: []string{}, BoardID: item.Board.ID, removed: removed}, nil
}
