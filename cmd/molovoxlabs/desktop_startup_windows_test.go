//go:build windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	webview "github.com/jchv/go-webview2"
)

func TestTrayStartupCreatesWebViewWithoutShowingWindow(t *testing.T) {
	profile := os.Getenv("MOLOVOX_TRAY_TEST_PROFILE")
	if profile == "" {
		// WebView owns a message loop and browser process that must exit before profile cleanup.
		profile, err := os.MkdirTemp("", "molovox-tray-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			var err error
			for range 40 {
				if err = os.RemoveAll(profile); err == nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Error(err)
		})
		cmd := exec.Command(os.Args[0], "-test.run=^TestTrayStartupCreatesWebViewWithoutShowingWindow$")
		cmd.Env = append(os.Environ(), "MOLOVOX_TRAY_TEST_PROFILE="+profile)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("tray startup process: %v\n%s", err, output)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	finish, err := hideInitialDesktopWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	window := webview.NewWithOptions(webview.WebViewOptions{DataPath: profile})
	if window == nil {
		t.Fatal("could not create startup WebView")
	}
	hwnd := uintptr(window.Window())
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
	if visible != 0 {
		t.Fatal("startup window became visible")
	}
	finish()
	// The normal tray open path must work after the startup guard is removed.
	tray := &desktopTray{hwnd: hwnd}
	tray.show()
	visible, _, _ = user32.NewProc("IsWindowVisible").Call(hwnd)
	user32.NewProc("ShowWindow").Call(hwnd, 0)
	if visible == 0 {
		t.Fatal("tray could not open the startup window")
	}
}
