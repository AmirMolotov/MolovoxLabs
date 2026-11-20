//go:build windows

package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestInstanceActivationPersistsUntilTheWindowIsReady(t *testing.T) {
	name := fmt.Sprintf(`Local\MolovoxLabsActivationTest-%d`, time.Now().UnixNano())
	first, err := openInstanceActivation(name)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(first)
	second, err := openInstanceActivation(name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatal(err)
	}
	if err := windows.SetEvent(second); err != nil {
		t.Fatal(err)
	}
	windows.CloseHandle(second)
	if state, err := windows.WaitForSingleObject(first, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("pending activation = %v, %v", state, err)
	}
	if state, err := windows.WaitForSingleObject(first, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("already handled activation = %v, %v", state, err)
	}
}

func TestInstanceMutex(t *testing.T) {
	name := fmt.Sprintf(`Local\MolovoxLabsTest-%d`, time.Now().UnixNano())
	first, err := openInstanceMutex(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if first != 0 {
			windows.CloseHandle(first)
		}
	}()
	second, err := openInstanceMutex(name)
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) || second != 0 {
		t.Fatalf("Duplicate instance = %v, %v", second, err)
	}
	windows.CloseHandle(first)
	first = 0
	third, err := openInstanceMutex(name)
	if err != nil {
		t.Fatal("Could not restart after closing first instance:", err)
	}
	windows.CloseHandle(third)
}
