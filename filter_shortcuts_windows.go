//go:build windows

package main

import (
	"errors"
	"strings"
)

type FilterGroup struct {
	ID        string
	Name      string
	FilterIDs []string
	Hotkeys   []KeyBinding
	MIDIs     []MIDIBinding
}

func soundKeys(s SoundEntry) []KeyBinding {
	if s.Hotkeys != nil {
		return s.Hotkeys
	}
	if s.Hotkey != nil {
		return []KeyBinding{*s.Hotkey}
	}
	return nil
}
func soundMIDIs(s SoundEntry) []MIDIBinding {
	if s.MIDIs != nil {
		return s.MIDIs
	}
	if s.MIDI != nil {
		return []MIDIBinding{*s.MIDI}
	}
	return nil
}
func triggerLibrary(l libraryState, p Preferences) libraryState {
	result := libraryState{Boards: append([]SoundBoard(nil), l.Boards...)}
	board := SoundBoard{ID: "@filters"}
	board.Sounds = append(board.Sounds, SoundEntry{ID: "@all", Hotkeys: p.AllFilterHotkeys, MIDIs: p.AllFilterMIDIs})
	for _, f := range p.Options.Filters {
		if f.ID != "" {
			board.Sounds = append(board.Sounds, SoundEntry{ID: "@filter:" + f.ID, Hotkeys: f.Hotkeys, MIDIs: f.MIDIs})
		}
	}
	for _, g := range p.FilterGroups {
		board.Sounds = append(board.Sounds, SoundEntry{ID: "@group:" + g.ID, Hotkeys: g.Hotkeys, MIDIs: g.MIDIs})
	}
	result.Boards = append(result.Boards, board)
	return result
}
func normalizePreferences(p *Preferences) bool {
	changed := false
	for i := range p.Options.Filters {
		if p.Options.Filters[i].ID == "" {
			p.Options.Filters[i].ID = newID()
			changed = true
		}
	}
	return changed
}
func (s *appStore) replaceLibrary(l libraryState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write("soundboards.json", l)
}
func (s *appStore) SetMultipleBindings(id string, keys []KeyBinding, midis []MIDIBinding) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i := range l.Boards {
		for j := range l.Boards[i].Sounds {
			if l.Boards[i].Sounds[j].ID == id {
				sound := &l.Boards[i].Sounds[j]
				sound.Hotkey = nil
				sound.MIDI = nil
				sound.Hotkeys = keys
				sound.MIDIs = midis
				if err = validateBindings(l); err != nil {
					return l, err
				}
				return l, s.write("soundboards.json", l)
			}
		}
	}
	return l, errors.New("Sound not found")
}

type triggerEditResult struct {
	Library     libraryState
	Preferences Preferences
}

func updateFilterBindings(p *Preferences, target string, keys []KeyBinding, midis []MIDIBinding) error {
	if target == "@all" {
		p.AllFilterHotkeys = keys
		p.AllFilterMIDIs = midis
		return nil
	}
	if strings.HasPrefix(target, "@filter:") {
		id := strings.TrimPrefix(target, "@filter:")
		for i := range p.Options.Filters {
			if p.Options.Filters[i].ID == id {
				p.Options.Filters[i].Hotkeys = keys
				p.Options.Filters[i].MIDIs = midis
				return nil
			}
		}
	}
	if strings.HasPrefix(target, "@group:") {
		id := strings.TrimPrefix(target, "@group:")
		for i := range p.FilterGroups {
			if p.FilterGroups[i].ID == id {
				p.FilterGroups[i].Hotkeys = keys
				p.FilterGroups[i].MIDIs = midis
				return nil
			}
		}
	}
	return errors.New("Filter shortcut target no longer exists")
}
func toggleFilterState(p *Preferences, target string) error {
	if target == "@all" {
		p.Options.BypassFilters = !p.Options.BypassFilters
		return nil
	}
	ids := map[string]bool{}
	if strings.HasPrefix(target, "@filter:") {
		ids[strings.TrimPrefix(target, "@filter:")] = true
	}
	if strings.HasPrefix(target, "@group:") {
		for _, g := range p.FilterGroups {
			if g.ID == strings.TrimPrefix(target, "@group:") {
				for _, id := range g.FilterIDs {
					ids[id] = true
				}
			}
		}
	}
	found, anyEnabled := false, false
	for _, f := range p.Options.Filters {
		if ids[f.ID] {
			found = true
			anyEnabled = anyEnabled || f.Enabled
		}
	}
	if !found {
		return errors.New("No matching filters")
	}
	for i := range p.Options.Filters {
		if ids[p.Options.Filters[i].ID] {
			p.Options.Filters[i].Enabled = !anyEnabled
		}
	}
	return nil
}
func (s *appStore) ToggleFilters(target string, live ...*AudioOptions) (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p Preferences
	if err := s.read("preferences.json", &p); err != nil {
		return p, err
	}
	if len(live) > 0 && live[0] != nil {
		p.Options = *live[0]
		p.Options.Filters = append([]MicFilter(nil), live[0].Filters...)
	}
	if err := toggleFilterState(&p, target); err != nil {
		return p, err
	}
	return p, s.write("preferences.json", p)
}
