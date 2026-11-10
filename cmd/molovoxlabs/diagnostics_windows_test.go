//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticLogCapturesBackgroundPanic(t *testing.T) {
	if root := os.Getenv("MOLOVOX_CRASH_TEST_ROOT"); root != "" {
		if _, err := openDiagnosticLog(root); err != nil {
			panic(err)
		}
		go func() { panic("diagnostic-test-background-panic") }()
		time.Sleep(5 * time.Second)
		os.Exit(2)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDiagnosticLogCapturesBackgroundPanic$")
	cmd.Env = append(os.Environ(), "MOLOVOX_CRASH_TEST_ROOT="+root)
	if err := cmd.Run(); err == nil {
		t.Fatal("Expected child process panic")
	}
	data, err := os.ReadFile(filepath.Join(root, "logs", "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "diagnostic-test-background-panic") || !strings.Contains(string(data), "goroutine") {
		t.Fatalf("Missing panic trace: %s", data)
	}
}

func TestDiagnosticLogRetainsPreviousSession(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"app.log": "last session", "app.previous.log": "older session"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	file, err := openDiagnosticLog(root)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	defer debug.SetCrashOutput(nil, debug.CrashOptions{})
	data, err := os.ReadFile(filepath.Join(dir, "app.previous.log"))
	if err != nil || string(data) != "last session" {
		t.Fatalf("Previous log lost: %s, %v", data, err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatalf("New log was not empty: %v", err)
	}
	badRoot := filepath.Join(root, "file")
	if err := os.WriteFile(badRoot, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := openDiagnosticLog(badRoot); err == nil {
		f.Close()
		t.Fatal("Expected invalid log directory to fail")
	}
}
