//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"unsafe"
)

func TestDoubleAltOnlyActivatesForTwoCompletedQuickTaps(t *testing.T) {
	for _, key := range []uint32{18, 164, 165} {
		d := altTapDetector{}
		if d.key(key, true, 100, false) || d.key(key, false, 150, false) || d.key(key, true, 230, false) {
			t.Fatal("activated before second release")
		}
		if !d.key(key, false, 280, false) {
			t.Fatal("quick taps not detected", key)
		}
		if d.key(key, true, 330, false) || d.key(key, false, 380, false) {
			t.Fatal("third tap retriggered")
		}
		if !(!d.key(key, true, 430, false) && d.key(key, false, 480, false)) {
			t.Fatal("next pair not detected")
		}
	}
}
func TestDoubleAltRejectsHoldsCombinationsAndInterveningKeys(t *testing.T) {
	type e struct {
		key     uint32
		down    bool
		at      uint32
		blocked bool
	}
	cases := map[string][]e{
		"hold":                   {{164, true, 100, false}, {164, true, 200, false}, {164, true, 300, false}, {164, false, 450, false}, {164, true, 500, false}, {164, false, 550, false}},
		"slow":                   {{164, true, 100, false}, {164, false, 150, false}, {164, true, 600, false}, {164, false, 650, false}},
		"alt tab":                {{164, true, 100, false}, {9, true, 120, false}, {9, false, 130, false}, {164, false, 150, false}, {164, true, 200, false}, {164, false, 250, false}},
		"ctrl alt":               {{17, true, 80, false}, {164, true, 100, false}, {164, false, 150, false}, {17, false, 160, false}, {164, true, 200, false}, {164, false, 250, false}},
		"intervening key":        {{164, true, 100, false}, {164, false, 150, false}, {65, true, 170, false}, {65, false, 180, false}, {164, true, 200, false}, {164, false, 250, false}},
		"mouse or modifier held": {{164, true, 100, true}, {164, false, 150, true}, {164, true, 200, false}, {164, false, 250, false}},
		"both alts held":         {{164, true, 100, false}, {165, true, 120, false}, {164, false, 130, false}, {165, false, 150, false}, {164, true, 200, false}, {164, false, 250, false}},
	}
	for name, events := range cases {
		t.Run(name, func(t *testing.T) {
			d := altTapDetector{}
			for _, event := range events {
				if d.key(event.key, event.down, event.at, event.blocked) {
					t.Fatal("unwanted activation", event)
				}
			}
		})
	}
}
func TestDoubleAltHandlesTickWrapAndMixedAltSides(t *testing.T) {
	d := altTapDetector{}
	d.key(164, true, 0xffffff80, false)
	d.key(164, false, 0xffffffb0, false)
	d.key(165, true, 20, false)
	if !d.key(165, false, 60, false) {
		t.Fatal("tick wrap or left/right mix failed")
	}
}
func TestTrayIconStructUsesWindowsX64Layout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("x64 desktop app")
	}
	data := trayIconData{}
	if unsafe.Sizeof(data) != 976 || unsafe.Offsetof(data.Icon) != 32 || unsafe.Offsetof(data.Tip) != 40 || unsafe.Offsetof(data.GUID) != 952 {
		t.Fatalf("incorrect NOTIFYICONDATA layout: %d", unsafe.Sizeof(data))
	}
}
func trayTestWindow(t *testing.T) uintptr {
	t.Helper()
	class, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("Molovox native tray test (hidden)")
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 100, 100, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	return hwnd
}
func TestTrayCloseKeepsHiddenWindowAliveAndQuitCanDestroyIt(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd := trayTestWindow(t)
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	tray, err := installDesktopTray(hwnd, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer tray.Close()
	user32.NewProc("SendMessageW").Call(hwnd, 0x10, 0, 0)
	alive, _, _ := user32.NewProc("IsWindow").Call(hwnd)
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
	if alive == 0 || visible != 0 || !tray.added {
		t.Fatal("close destroyed the window or removed tray icon")
	}
	tray.exiting = true
	user32.NewProc("SendMessageW").Call(hwnd, 0x10, 0, 0)
	alive, _, _ = user32.NewProc("IsWindow").Call(hwnd)
	if alive != 0 || tray.added {
		t.Fatal("explicit quit did not destroy the window and remove its icon")
	}
}
func TestGlobalAltListenerStartsAndReleasesItsHookThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd := trayTestWindow(t)
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	listener, err := newAltTapListener(hwnd)
	if err != nil {
		t.Fatal(err)
	}
	if listener.thread == 0 || listener.thread == windows.GetCurrentThreadId() {
		t.Fatal("hook must run on its own message thread")
	}
	listener.Close()
	listener.Close()
	select {
	case <-listener.done:
	default:
		t.Fatal("hook thread was not released")
	}
}
