//go:build windows

package main

import (
	"fmt"
	"log"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"

	"golang.org/x/sys/windows"
)

// Startup callbacks run on the WebView thread. Ignore a timeout that was
// already dispatched when the page reported that it had loaded.
func watchStudioStartup(window webview.WebView, url string, timeout time.Duration) (start, ready, stop func()) {
	var timer *time.Timer
	stop = func() {
		if timer != nil {
			timer.Stop()
			timer = nil
		}
	}
	var load func(retried bool)
	load = func(retried bool) {
		window.Navigate(url)
		var next *time.Timer
		next = time.AfterFunc(timeout, func() {
			window.Dispatch(func() {
				if timer != next {
					return
				}
				if !retried {
					log.Print("Studio did not report startup; retrying navigation")
					load(true)
					return
				}
				stop()
				log.Print("Studio did not report startup after retry")
				window.SetHtml(studioStartupError)
			})
		})
		timer = next
	}
	start = func() { stop(); load(false) }
	ready = func() {
		stop()
		log.Print("Studio startup completed")
	}
	return start, ready, stop
}

const studioStartupError = `<!doctype html><meta charset="utf-8"><title>Molovox Labs startup</title>
<body style="background:#101916;color:#e7eeec;font:16px Segoe UI,sans-serif;padding:2rem">
<h1>Studio didn't load</h1><p>Retry opening Studio.</p>
<button onclick="nativeStudioRetry()">Retry</button></body>`

type desktopWindowPosition struct {
	window, after uintptr
	x, y, cx, cy  int32
	flags         uint32
}

// WebView2's window constructor always calls ShowWindow; intercept that first
// show before it happens so sign-in never flashes or activates the window.
func hideInitialDesktopWindow() (func(), error) {
	var hwnd uintptr
	subclass := windows.NewCallback(func(window, msg, wp uintptr, lp unsafe.Pointer, id, ref uintptr) uintptr {
		if msg == 0x46 { // WM_WINDOWPOSCHANGING
			position := (*desktopWindowPosition)(lp)
			position.flags &^= 0x40 // SWP_SHOWWINDOW
			position.flags |= 0x10  // SWP_NOACTIVATE
		}
		result, _, _ := trayComctl.NewProc("DefSubclassProc").Call(window, msg, wp, uintptr(lp))
		return result
	})
	callback := windows.NewCallback(func(code, wp, lp uintptr) uintptr {
		if code == 3 && hwnd == 0 { // HCBT_CREATEWND
			var class [64]uint16
			user32.NewProc("GetClassNameW").Call(wp, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
			if windows.UTF16ToString(class[:]) == "webview" {
				ok, _, _ := trayComctl.NewProc("SetWindowSubclass").Call(wp, subclass, 716, 0)
				if ok == 0 {
					return 1 // Abort creation rather than show a startup window.
				}
				hwnd = wp
			}
		}
		result, _, _ := trayNextHook.Call(0, code, wp, lp)
		return result
	})
	hook, _, err := user32.NewProc("SetWindowsHookExW").Call(5, callback, 0, uintptr(windows.GetCurrentThreadId()))
	if hook == 0 {
		return nil, fmt.Errorf("Could not prepare tray startup: %w", err)
	}
	return func() {
		if hook == 0 {
			return
		}
		user32.NewProc("UnhookWindowsHookEx").Call(hook)
		hook = 0
		if hwnd != 0 {
			trayComctl.NewProc("RemoveWindowSubclass").Call(hwnd, subclass, 716)
		}
	}, nil
}
