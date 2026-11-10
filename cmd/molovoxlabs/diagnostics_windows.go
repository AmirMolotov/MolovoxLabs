//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

func openDiagnosticLog(root string) (*os.File, error) {
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "app.log")
	if err := os.Rename(path, filepath.Join(dir, "app.previous.log")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	// A GUI build has no console to retain fatal Go errors or background panics.
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
