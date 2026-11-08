//go:build windows

package main

import "golang.org/x/sys/windows"

// Foreground activation and keyboard focus are separate Windows operations.
// Only attach input queues for this explicit user request, and always detach.
type foregroundAccess struct {
	foreground    func() uintptr
	windowThread  func(uintptr) uint32
	currentThread func() uint32
	attach        func(uint32, uint32, bool) bool
	activate      func(uintptr) bool
	focus         func(uintptr)
}

func activateTrayWindow(hwnd uintptr, api foregroundAccess) bool {
	if api.foreground() != hwnd {
		if !api.activate(hwnd) || api.foreground() != hwnd {
			current, owner := api.currentThread(), api.windowThread(api.foreground())
			if owner != 0 && owner != current && api.attach(current, owner, true) {
				defer api.attach(current, owner, false)
			}
			api.activate(hwnd)
		}
	}
	if api.foreground() != hwnd {
		return false
	}
	api.focus(hwnd)
	return true
}
func focusTrayWindow(hwnd uintptr) bool {
	return activateTrayWindow(hwnd, foregroundAccess{
		foreground: func() uintptr { w, _, _ := user32.NewProc("GetForegroundWindow").Call(); return w },
		windowThread: func(w uintptr) uint32 {
			thread, _, _ := user32.NewProc("GetWindowThreadProcessId").Call(w, 0)
			return uint32(thread)
		},
		currentThread: windows.GetCurrentThreadId,
		attach: func(current, owner uint32, join bool) bool {
			var flag uintptr
			if join {
				flag = 1
			}
			ok, _, _ := user32.NewProc("AttachThreadInput").Call(uintptr(current), uintptr(owner), flag)
			return ok != 0
		},
		activate: func(w uintptr) bool { ok, _, _ := user32.NewProc("SetForegroundWindow").Call(w); return ok != 0 },
		focus: func(w uintptr) {
			user32.NewProc("SetActiveWindow").Call(w)
			user32.NewProc("SetFocus").Call(w)
			// WebView2's host handles WM_ACTIVATE by moving native focus into its
			// browser controller, including when the host was already foreground.
			user32.NewProc("SendMessageW").Call(w, 0x6, 1, 0)
		},
	})
}
