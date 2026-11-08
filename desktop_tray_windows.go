//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	trayNotifyMessage = 0x8000 + 714
	trayOpenMessage   = 0x8000 + 715
)

// Two completed, unmodified Alt taps. Neither Alt nor any other key is swallowed.
// Message timestamps use uint32 subtraction so Windows' tick counter can wrap.
type altTapDetector struct {
	down             [256]bool
	altDown          bool
	started, lastTap uint32
	used, pending    bool
}

func (d *altTapDetector) key(vk uint32, down bool, at uint32, blocked bool) bool {
	if vk > 255 {
		return false
	}
	alt := vk == 18 || vk == 164 || vk == 165
	if !alt {
		d.down[vk] = down
		if down {
			d.used = true
			d.pending = false
		}
		return false
	}
	if down {
		if d.down[vk] {
			return false
		}
		d.down[vk] = true
		if d.altDown {
			d.used = true
			d.pending = false
			return false
		}
		d.altDown = true
		d.started = at
		d.used = blocked
		for key, held := range d.down {
			if held && key != 18 && key != 164 && key != 165 {
				d.used = true
			}
		}
		if d.used {
			d.pending = false
		}
		return false
	}
	if !d.down[vk] {
		d.pending = false
		return false
	}
	d.down[vk] = false
	for _, key := range []int{18, 164, 165} {
		if d.down[key] {
			return false
		}
	}
	d.altDown = false
	if d.used || blocked || at-d.started > 250 {
		d.pending = false
		return false
	}
	if d.pending && at-d.lastTap <= 400 {
		d.pending = false
		return true
	}
	d.lastTap = at
	d.pending = true
	return false
}

var trayKeyState = user32.NewProc("GetAsyncKeyState")
var trayPostMessage = user32.NewProc("PostMessageW")
var trayNextHook = user32.NewProc("CallNextHookEx")

type altTapListener struct {
	thread uint32
	done   chan struct{}
	once   sync.Once
}

func newAltTapListener(hwnd uintptr) (*altTapListener, error) {
	listener := &altTapListener{done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(listener.done)
		listener.thread = windows.GetCurrentThreadId()
		var msg winMessage
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
		detector := altTapDetector{}
		for key := 8; key < 256; key++ {
			if key == 16 || key == 17 || key == 18 {
				continue
			}
			v, _, _ := trayKeyState.Call(uintptr(key))
			detector.down[key] = v&0x8000 != 0
		}
		detector.altDown = detector.down[18] || detector.down[164] || detector.down[165]
		detector.used = detector.altDown
		callback := windows.NewCallback(func(code, wp, lp uintptr) uintptr {
			if int32(code) >= 0 {
				event := (*struct {
					Key, Scan, Flags, Time uint32
					Extra                  uintptr
				})(unsafe.Pointer(lp))
				if event.Flags&0x10 != 0 {
					detector.pending = false
					if detector.altDown {
						detector.used = true
					}
				}
				if event.Flags&0x10 == 0 && (wp == 0x100 || wp == 0x101 || wp == 0x104 || wp == 0x105) {
					blocked := false
					if event.Key == 18 || event.Key == 164 || event.Key == 165 {
						for _, key := range []uintptr{1, 2, 4, 5, 6, 16, 17, 91, 92} {
							state, _, _ := trayKeyState.Call(key)
							if state&0x8000 != 0 {
								blocked = true
								break
							}
						}
					}
					if detector.key(event.Key, wp == 0x100 || wp == 0x104, event.Time, blocked) {
						trayPostMessage.Call(hwnd, trayOpenMessage, 0, 0)
					}
				}
			}
			result, _, _ := trayNextHook.Call(0, code, wp, lp)
			return result
		})
		module, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
		hook, _, err := user32.NewProc("SetWindowsHookExW").Call(13, callback, module, 0)
		if hook == 0 {
			ready <- fmt.Errorf("Double-Alt shortcut: %w", err)
			return
		}
		defer user32.NewProc("UnhookWindowsHookEx").Call(hook)
		ready <- nil
		for {
			result, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(result) <= 0 {
				return
			}
			user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
			user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	if err := <-ready; err != nil {
		<-listener.done
		return nil, err
	}
	return listener, nil
}
func (l *altTapListener) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() { user32.NewProc("PostThreadMessageW").Call(uintptr(l.thread), 0x12, 0, 0); <-l.done })
}

type trayIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

var trayComctl = windows.NewLazySystemDLL("comctl32.dll")
var trayShell = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")

type desktopTray struct {
	hwnd, callback uintptr
	data           trayIconData
	taskbarCreated uint32
	added, exiting bool
	open           func()
	keys           *altTapListener
}

func installDesktopTray(hwnd uintptr, open func()) (*desktopTray, error) {
	t := &desktopTray{hwnd: hwnd, open: open}
	text, _ := windows.UTF16PtrFromString("TaskbarCreated")
	message, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(text)))
	t.taskbarCreated = uint32(message)
	module, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	icon, _, err := user32.NewProc("LoadImageW").Call(module, 1, 1, 32, 32, 0x8000)
	if icon == 0 {
		return nil, fmt.Errorf("Tray icon: %w", err)
	}
	t.data = trayIconData{Window: hwnd, ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: trayNotifyMessage, Icon: icon}
	t.data.Size = uint32(unsafe.Sizeof(t.data))
	tip, _ := windows.UTF16FromString("Molovox Labs · Alt, Alt to open Soundboard")
	copy(t.data.Tip[:], tip)
	t.callback = windows.NewCallback(func(hwnd, msg, wp, lp, id, ref uintptr) uintptr {
		switch uint32(msg) {
		case 0x10: // Closing the window keeps audio, MIDI and shortcuts running.
			if !t.exiting && t.added {
				user32.NewProc("ShowWindow").Call(hwnd, 0)
				return 0
			}
		case 0x5:
			if wp == 1 && t.added {
				user32.NewProc("ShowWindow").Call(hwnd, 0)
			}
		case trayOpenMessage:
			t.show()
			return 0
		case trayNotifyMessage:
			switch uint32(lp) & 0xffff {
			case 0x400, 0x401, 0x203, 0x202:
				t.show()
			case 0x7b, 0x205:
				t.menu()
			}
			return 0
		case 0x82:
			t.remove()
			trayComctl.NewProc("RemoveWindowSubclass").Call(hwnd, t.callback, 714)
		}
		if t.taskbarCreated != 0 && uint32(msg) == t.taskbarCreated {
			if err := t.add(); err != nil {
				t.show()
			}
			return 0
		}
		result, _, _ := trayComctl.NewProc("DefSubclassProc").Call(hwnd, msg, wp, lp)
		return result
	})
	ok, _, err := trayComctl.NewProc("SetWindowSubclass").Call(hwnd, t.callback, 714, 0)
	if ok == 0 {
		return nil, fmt.Errorf("Tray window integration: %w", err)
	}
	if err := t.add(); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}
func (t *desktopTray) add() error {
	ok, _, err := trayShell.Call(0, uintptr(unsafe.Pointer(&t.data)))
	if ok == 0 {
		t.added = false
		return fmt.Errorf("Could not add Molovox Labs to the tray: %w", err)
	}
	t.added = true
	t.data.Version = 4
	trayShell.Call(4, uintptr(unsafe.Pointer(&t.data)))
	return nil
}
func (t *desktopTray) remove() {
	if t.added {
		trayShell.Call(2, uintptr(unsafe.Pointer(&t.data)))
		t.added = false
	}
}
func (t *desktopTray) show() {
	minimized, _, _ := user32.NewProc("IsIconic").Call(t.hwnd)
	if minimized != 0 {
		user32.NewProc("ShowWindow").Call(t.hwnd, 9)
	} else {
		user32.NewProc("ShowWindow").Call(t.hwnd, 5)
	}
	focusTrayWindow(t.hwnd)
	if t.open != nil {
		t.open()
	}
}
func (t *desktopTray) menu() {
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	for _, item := range []struct {
		id   uintptr
		text string
	}{{1, "Open Soundboard\tAlt, Alt"}, {2, "Quit Molovox Labs"}} {
		label, _ := windows.UTF16PtrFromString(item.text)
		user32.NewProc("AppendMenuW").Call(menu, 0, item.id, uintptr(unsafe.Pointer(label)))
	}
	var point struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	user32.NewProc("SetForegroundWindow").Call(t.hwnd)
	command, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x100|0x80|2, uintptr(point.X), uintptr(point.Y), 0, t.hwnd, 0)
	trayPostMessage.Call(t.hwnd, 0, 0, 0)
	if command == 1 {
		t.show()
	} else if command == 2 {
		t.exiting = true
		trayPostMessage.Call(t.hwnd, 0x10, 0, 0)
	}
}
func (t *desktopTray) Close() {
	if t == nil {
		return
	}
	t.keys.Close()
	t.remove()
	trayComctl.NewProc("RemoveWindowSubclass").Call(t.hwnd, t.callback, 714)
}
