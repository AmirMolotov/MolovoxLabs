//go:build windows

package main

import (
	"errors"
	"strings"
)

type PadGroup struct {
	ID        string
	Name      string
	Collapsed bool
}

type PadPlacement struct {
	ID      string
	GroupID string
}

// SaveBoardLayout only changes presentation metadata, preserving media and triggers.
func (s *appStore) SaveBoardLayout(id string, placements []PadPlacement, groups []PadGroup) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i := range l.Boards {
		b := &l.Boards[i]
		if b.ID != id {
			continue
		}
		if b.Kind == "video" {
			return l, errors.New("Layout is only available for soundboards")
		}
		groupIDs := map[string]bool{}
		for j := range groups {
			g := &groups[j]
			g.Name = strings.TrimSpace(g.Name)
			if g.ID == "" || groupIDs[g.ID] || g.Name == "" || len(g.Name) > 240 {
				return l, errors.New("Invalid pad group")
			}
			groupIDs[g.ID] = true
		}
		if len(placements) != len(b.Sounds) {
			return l, errors.New("Board changed; refresh and try again")
		}
		sounds := map[string]SoundEntry{}
		for _, sound := range b.Sounds {
			sounds[sound.ID] = sound
		}
		ordered := make([]SoundEntry, 0, len(placements))
		for _, p := range placements {
			sound, ok := sounds[p.ID]
			if !ok || (p.GroupID != "" && !groupIDs[p.GroupID]) {
				return l, errors.New("Invalid pad placement")
			}
			delete(sounds, p.ID)
			sound.GroupID = p.GroupID
			ordered = append(ordered, sound)
		}
		b.Sounds, b.Groups = ordered, groups
		return l, s.write("soundboards.json", l)
	}
	return l, errors.New("Board not found")
}
