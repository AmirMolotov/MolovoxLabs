//go:build windows

package main

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestWindowsStartup(t *testing.T) {
	// Use an isolated key so tests never register an application at sign-in.
	path := fmt.Sprintf(`Software\MolovoxLabsTests\Startup-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { key.Close(); registry.DeleteKey(registry.CURRENT_USER, path) })
	executable := `C:\Apps with spaces\MolovoxLabs.exe`
	command := `"` + executable + `" --data-dir "C:\Test profile"`
	for _, enabled := range []bool{false, true, false, false} {
		if err := writeWindowsStartup(key, enabled, command); err != nil {
			t.Fatal(err)
		}
		got, err := readWindowsStartup(key)
		if err != nil || got != enabled {
			t.Fatalf("startup = %v, %v; want %v", got, err, enabled)
		}
		if enabled {
			value, _, err := key.GetStringValue(startupValue)
			if err != nil || value != command+" --start-in-tray" {
				t.Fatalf("command = %q, %v", value, err)
			}
		}
	}
	if err := key.SetStringValue(startupValue, command); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upgradeWindowsStartupCommand(key, executable); err != nil {
			t.Fatal(err)
		}
	}
	if value, _, err := key.GetStringValue(startupValue); err != nil || value != command+" --start-in-tray" {
		t.Fatalf("upgraded command = %q, %v", value, err)
	}
}
