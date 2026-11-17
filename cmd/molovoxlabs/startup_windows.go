//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const startupRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValue = "MolovoxLabs"

func loadWindowsStartup() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	return readWindowsStartup(key)
}

func readWindowsStartup(key registry.Key) (bool, error) {
	value, _, err := key.GetStringValue(startupValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return value != "", err
}

func saveWindowsStartup(enabled bool, profile string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if profile != "" {
		profile, err = filepath.Abs(profile)
		if err != nil {
			return err
		}
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	command := `"` + executable + `"`
	if profile != "" {
		command += " --data-dir " + syscall.EscapeArg(profile)
	}
	return writeWindowsStartup(key, enabled, command)
}

func writeWindowsStartup(key registry.Key, enabled bool, command string) error {
	if enabled {
		return key.SetStringValue(startupValue, command)
	}
	err := key.DeleteValue(startupValue)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
