//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func setMolovoxIcon(hwnd uintptr) {
	appID, _ := windows.UTF16PtrFromString("MolovoxLabs.AudioWorkspace")
	windows.NewLazySystemDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID").Call(uintptr(unsafe.Pointer(appID)))
	module, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	load := user32.NewProc("LoadImageW")
	send := user32.NewProc("SendMessageW")
	for _, icon := range []struct {
		size int
		kind uintptr
	}{{32, 1}, {16, 0}} {
		h, _, _ := load.Call(module, 1, 1, uintptr(icon.size), uintptr(icon.size), 0x8000)
		if h != 0 {
			send.Call(hwnd, 0x80, icon.kind, h)
		}
	}
}
