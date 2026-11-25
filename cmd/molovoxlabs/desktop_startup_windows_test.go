//go:build windows

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
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

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/studio.html" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusNoContent) // Leave the WebView on its blank document.
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><script>nativeStudioReady()</script>")
	}))
	defer server.Close()
	open, ready, stop := watchStudioStartup(window, server.URL+"/studio.html", 250*time.Millisecond)
	defer stop()
	loaded := false
	if err := window.Bind("nativeStudioReady", func() {
		ready()
		loaded = true
		window.Dispatch(window.Terminate)
	}); err != nil {
		t.Fatal(err)
	}
	guard := time.AfterFunc(5*time.Second, func() { window.Dispatch(window.Terminate) })
	defer guard.Stop()
	window.Dispatch(open)
	window.Run()
	if !loaded || requests.Load() != 2 {
		t.Fatalf("Studio startup did not recover after the blank first response: loaded=%v, requests=%d", loaded, requests.Load())
	}
}
