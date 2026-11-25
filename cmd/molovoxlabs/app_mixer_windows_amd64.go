//go:build windows && amd64

package main

import (
	"errors"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const appMixerTargetPrefix = "@app:"

type appMixerShortcut struct {
	ID      string
	Hotkeys []KeyBinding
	MIDIs   []MIDIBinding
}

type appMixerGroup struct {
	ID, Name string
	Paths    []string
}

func splitAppMixerTarget(id string) (string, string) {
	value, _ := strings.CutPrefix(id, appMixerTargetPrefix)
	action, target, _ := strings.Cut(value, ":")
	return action, target
}

func appMixerKnob(action string) bool {
	return action == "knob" || action == "encoder" || action == "centered"
}

func appMixerTarget(id string) (string, int, error) {
	value, ok := strings.CutPrefix(id, appMixerTargetPrefix)
	level, path, found := strings.Cut(value, ":")
	percent, err := strconv.Atoi(level)
	if !ok || !found || err != nil || strconv.Itoa(percent) != level || percent < 0 || percent > 100 || percent%10 != 0 || !validAppMixerPath(path) {
		return "", 0, errors.New("Choose an app and a mixer volume from 0 to 100 in steps of ten")
	}
	return path, percent, nil
}

func validAppMixerPath(path string) bool {
	name, err := windows.UTF16FromString(path)
	return err == nil && filepath.IsAbs(path) && len(name) <= 1024
}

func validateAppMixerConfig(p Preferences) error {
	groups := make(map[string]bool, len(p.AppMixerGroups))
	paths := map[string]bool{}
	for _, group := range p.AppMixerGroups {
		if group.ID == "" || len(group.ID) > 64 || strings.Contains(group.ID, ":") || groups[group.ID] || strings.TrimSpace(group.Name) == "" || len(group.Name) > 160 || len(group.Paths) == 0 || len(group.Paths) > 256 {
			return errors.New("Invalid app mixer group")
		}
		groups[group.ID] = true
		for _, path := range group.Paths {
			key := appAudioPath(path)
			if !validAppMixerPath(path) || paths[key] {
				return errors.New("An app can only belong to one mixer group")
			}
			paths[key] = true
		}
	}
	seen := make(map[string]bool, len(p.AppMixerShortcuts))
	for _, shortcut := range p.AppMixerShortcuts {
		if err := validateAppMixerBinding(shortcut.ID, shortcut.Hotkeys, shortcut.MIDIs, groups); err != nil {
			return err
		}
		key := strings.ToLower(shortcut.ID)
		if seen[key] {
			return errors.New("Duplicate app mixer shortcut target")
		}
		seen[key] = true
	}
	return nil
}

func updateAppMixerBindings(p *Preferences, target string, keys []KeyBinding, midis []MIDIBinding) error {
	groups := make(map[string]bool, len(p.AppMixerGroups))
	for _, group := range p.AppMixerGroups {
		groups[group.ID] = true
	}
	if err := validateAppMixerBinding(target, keys, midis, groups); err != nil {
		return err
	}
	p.AppMixerShortcuts = slices.DeleteFunc(p.AppMixerShortcuts, func(shortcut appMixerShortcut) bool {
		return strings.EqualFold(shortcut.ID, target)
	})
	if len(keys) != 0 || len(midis) != 0 {
		p.AppMixerShortcuts = append(p.AppMixerShortcuts, appMixerShortcut{ID: target, Hotkeys: keys, MIDIs: midis})
	}
	return nil
}

func validateAppMixerBinding(id string, keys []KeyBinding, midis []MIDIBinding, groups map[string]bool) error {
	action, target := splitAppMixerTarget(id)
	mode, group := strings.CutPrefix(action, "group-")
	if mode != "up" && mode != "down" && !appMixerKnob(mode) {
		_, _, err := appMixerTarget(id)
		return err
	}
	if group && !groups[target] || !group && !validAppMixerPath(target) {
		return errors.New("Choose an app or mixer group")
	}
	if appMixerKnob(mode) {
		if len(keys) != 0 {
			return errors.New("Learn a MIDI control for this knob assignment")
		}
		for _, midi := range midis {
			if midi.Kind != "cc" {
				return errors.New("Turn the knob to learn its MIDI CC")
			}
		}
	}
	return nil
}

func appMixerDelta(mode string, event midiTriggerEvent) int {
	switch mode {
	case "up":
		return 10
	case "down":
		return -10
	case "knob":
		if event.Previous < 0 {
			return 0
		}
		// Divide a full absolute sweep into ten volume steps.
		return 10 * ((event.Value*10+63)/127 - (event.Previous*10+63)/127)
	case "encoder":
		if event.Value > 64 {
			return -10
		}
		if event.Value > 0 && event.Value < 64 {
			return 10
		}
	case "centered":
		if event.Value > 64 {
			return 10
		}
		if event.Value > 0 && event.Value < 64 {
			return -10
		}
	}
	return 0
}

func adjustAppMixerVolume(paths []string, delta int) error {
	if delta == 0 || len(paths) == 0 {
		return nil
	}
	names := make([][1024]uint16, len(paths))
	for i, path := range paths {
		name, err := windows.UTF16FromString(path)
		if err != nil || !filepath.IsAbs(path) || len(name) > len(names[i]) {
			return errors.New("Invalid app path")
		}
		copy(names[i][:], name)
	}
	b, err := loadAppAudioBridge()
	if err != nil {
		return err
	}
	result, _, _ := b.mixerAdjust.Call(uintptr(unsafe.Pointer(&names[0])), uintptr(len(names)), uintptr(int32(delta)))
	if uint32(result) == uint32(windows.ERROR_NOT_FOUND)|0x80070000 {
		return errors.New("No matching Windows audio session")
	}
	return appAudioResult(result)
}

type appMixerEntry struct {
	Path, Name string
	Volume     int
	Playing    bool
	Icon       []byte `json:",omitempty"`
}

func listAppMixerWithIcons(visiblePaths []string) ([]appMixerEntry, error) {
	apps, err := listAppMixer()
	for i := range apps {
		app := &apps[i]
		if app.Playing && !slices.Contains(visiblePaths, app.Path) {
			app.Icon = readAppMixerIcon(app.Path)
		}
	}
	return apps, err
}

func listAppMixer() ([]appMixerEntry, error) {
	b, err := loadAppAudioBridge()
	if err != nil {
		return nil, err
	}
	// shortcut: up to 256 distinct app paths per scan, grow the bridge buffer if the mixer reaches that limit.
	entries := make([]struct {
		path         [1024]uint16
		volume, peak float32
	}, 256)
	var count uint32
	result, _, _ := b.mixerList.Call(uintptr(unsafe.Pointer(&entries[0])), uintptr(len(entries)), uintptr(unsafe.Pointer(&count)))
	if err := appAudioResult(result); err != nil {
		return nil, err
	}
	if count > uint32(len(entries)) {
		return nil, errors.New("Invalid app mixer count")
	}
	apps := make([]appMixerEntry, 0, count)
	for _, entry := range entries[:count] {
		path := windows.UTF16ToString(entry.path[:])
		apps = append(apps, appMixerEntry{
			Path: path, Name: filepath.Base(path),
			Volume: int(math.Round(float64(entry.volume) * 100)), Playing: entry.peak > 0,
		})
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Path < apps[j].Path })
	return apps, nil
}

func setAppMixerVolume(path string, percent int) error {
	if percent < 0 || percent > 100 {
		return errors.New("App volume must be between 0 and 100")
	}
	name, err := windows.UTF16FromString(path)
	if err != nil || !filepath.IsAbs(path) || len(name) > 1024 {
		return errors.New("Invalid app path")
	}
	b, err := loadAppAudioBridge()
	if err != nil {
		return err
	}
	result, _, _ := b.mixerSet.Call(uintptr(unsafe.Pointer(&name[0])), uintptr(percent))
	if uint32(result) == uint32(windows.ERROR_NOT_FOUND)|0x80070000 {
		return errors.New("This app has no Windows audio session")
	}
	return appAudioResult(result)
}
