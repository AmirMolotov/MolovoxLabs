//go:build windows

package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

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
