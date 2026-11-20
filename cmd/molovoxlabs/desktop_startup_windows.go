//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
