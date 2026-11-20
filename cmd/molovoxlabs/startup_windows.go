//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const startupRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValue = "MolovoxLabs"
const startupTrayArg = "--start-in-tray"

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
		return key.SetStringValue(startupValue, command+" "+startupTrayArg)
	}
	err := key.DeleteValue(startupValue)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func upgradeWindowsStartup() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return upgradeWindowsStartupCommand(key, executable)
}

func upgradeWindowsStartupCommand(key registry.Key, executable string) error {
	command, _, err := key.GetStringValue(startupValue)
	if errors.Is(err, registry.ErrNotExist) || (err == nil && command == "") {
		return nil
	}
	if err != nil {
		return err
	}
	args, err := windows.DecomposeCommandLine(command)
	if err != nil {
		return err
	}
	if len(args) == 0 || !strings.EqualFold(args[0], executable) {
		return nil
	}
	if len(args) == 1 || (len(args) == 3 && args[1] == "--data-dir") {
		return writeWindowsStartup(key, true, command)
	}
	return nil
}
