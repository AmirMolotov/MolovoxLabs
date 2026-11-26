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
func watchStudioStartup(window webview.WebView, url string, timeout time.Duration, showWhenReady func()) (start, ready, stop func()) {
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
				showWhenReady()
			})
		})
		timer = next
	}
	start = func() { stop(); load(false) }
	ready = func() {
		stop()
		log.Print("Studio page opened")
		showWhenReady()
	}
	return start, ready, stop
}

const studioStartupError = `<!doctype html><meta charset="utf-8"><title>Molovox Labs startup</title>
<body style="background:#101916;color:#e7eeec;font:16px Segoe UI,sans-serif;padding:2rem">
<h1>Studio didn't load</h1><p>Retry opening Studio.</p>
<button onclick="nativeStudioRetry()">Retry</button></body>`

// WebView2 needs a visible host while its controller is created. Cloak the
// window so it can initialize without appearing on the desktop or taking focus.
func hideInitialDesktopWindow() (func(), error) {
	const dwmwaCloak = 13
	setAttribute := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	setCloak := func(window uintptr, enabled int32) uintptr {
		result, _, _ := setAttribute.Call(window, dwmwaCloak, uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled))
		return result
	}
	var hwnd uintptr
	cloaked := false
	subclass := windows.NewCallback(func(window, msg, wp, lp, id, ref uintptr) uintptr {
		if msg == 0x01 { // WM_CREATE
			// Consume the launcher's show state before WebView2 shows its host.
			user32.NewProc("ShowWindow").Call(window, 0)
		}
		if msg == 0x18 && wp != 0 && !cloaked { // WM_SHOWWINDOW
			result := setCloak(window, 1)
			cloaked = result == 0
			if !cloaked {
				log.Printf("Could not cloak Studio window: %#x", result)
			}
		}
		result, _, _ := trayComctl.NewProc("DefSubclassProc").Call(window, msg, wp, lp)
		return result
	})
	callback := windows.NewCallback(func(code, wp, lp uintptr) uintptr {
		if code == 3 && hwnd == 0 { // HCBT_CREATEWND
			var class [64]uint16
			user32.NewProc("GetClassNameW").Call(wp, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
			if windows.UTF16ToString(class[:]) == "webview" {
				if ok, _, failure := trayComctl.NewProc("SetWindowSubclass").Call(wp, subclass, 716, 0); ok == 0 {
					log.Printf("Could not prepare Studio window: %v", failure)
				}
				hwnd = wp
			}
		}
		if code == 5 && wp == hwnd { // HCBT_ACTIVATE
			return 1
		}
		result, _, _ := trayNextHook.Call(0, code, wp, lp)
		return result
	})
	hook, _, err := user32.NewProc("SetWindowsHookExW").Call(5, callback, 0, uintptr(windows.GetCurrentThreadId()))
	if hook == 0 {
		return nil, fmt.Errorf("Could not prepare Studio window: %w", err)
	}
	return func() {
		if hook == 0 {
			return
		}
		user32.NewProc("UnhookWindowsHookEx").Call(hook)
		hook = 0
		if hwnd != 0 {
			user32.NewProc("ShowWindow").Call(hwnd, 0)
			trayComctl.NewProc("RemoveWindowSubclass").Call(hwnd, subclass, 716)
			if cloaked {
				if result := setCloak(hwnd, 0); result != 0 {
					log.Printf("Could not uncloak Studio window: %#x", result)
				}
			}
		}
	}, nil
}
