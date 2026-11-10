//go:build windows

package main

import "testing"

func TestTrayForegroundRetriesDeniedActivationAndRestoresKeyboardFocus(t *testing.T) {
	foreground := uintptr(10)
	attached := false
	activations := 0
	focused := uintptr(0)
	joins, leaves := 0, 0
	api := foregroundAccess{
		foreground: func() uintptr { return foreground }, windowThread: func(uintptr) uint32 { return 2 }, currentThread: func() uint32 { return 1 },
		attach: func(a, b uint32, on bool) bool {
			if a != 1 || b != 2 {
				t.Fatal("wrong input queues")
			}
			attached = on
			if on {
				joins++
			} else {
				leaves++
			}
			return true
		},
		activate: func(w uintptr) bool {
			activations++
			if !attached {
				return false
			}
			foreground = w
			return true
		},
		focus: func(w uintptr) {
			if foreground != w || !attached {
				t.Fatal("focus before foreground access")
			}
			focused = w
		},
	}
	if !activateTrayWindow(20, api) || focused != 20 || activations != 2 || joins != 1 || leaves != 1 || attached {
		t.Fatal("foreground recovery or cleanup failed")
	}
}
func TestTrayForegroundRefocusesAlreadyActiveAppWithoutJoiningOtherThreads(t *testing.T) {
	focused := false
	api := foregroundAccess{foreground: func() uintptr { return 20 }, focus: func(w uintptr) { focused = w == 20 }}
	if !activateTrayWindow(20, api) || !focused {
		t.Fatal("already-foreground app did not regain content focus")
	}
}
func TestTrayForegroundDoesNotAttachWhenNormalActivationSucceeds(t *testing.T) {
	foreground := uintptr(10)
	api := foregroundAccess{foreground: func() uintptr { return foreground }, activate: func(w uintptr) bool { foreground = w; return true }, focus: func(uintptr) {}}
	if !activateTrayWindow(20, api) {
		t.Fatal("normal activation failed")
	}
}
func TestTrayForegroundAlwaysDetachesAfterFailedRetry(t *testing.T) {
	attached := false
	api := foregroundAccess{foreground: func() uintptr { return 10 }, windowThread: func(uintptr) uint32 { return 2 }, currentThread: func() uint32 { return 1 }, attach: func(_, _ uint32, on bool) bool { attached = on; return true }, activate: func(uintptr) bool { return false }, focus: func(uintptr) { t.Fatal("must not focus while another application remains foreground") }}
	if activateTrayWindow(20, api) || attached {
		t.Fatal("input queue left attached after failure")
	}
}
func TestTrayForegroundSkipsInvalidOrOwnForegroundThread(t *testing.T) {
	for _, thread := range []uint32{0, 1} {
		api := foregroundAccess{foreground: func() uintptr { return 10 }, windowThread: func(uintptr) uint32 { return thread }, currentThread: func() uint32 { return 1 }, attach: func(uint32, uint32, bool) bool { t.Fatal("invalid input queue attachment"); return false }, activate: func(uintptr) bool { return false }}
		if activateTrayWindow(20, api) {
			t.Fatal("unexpected activation")
		}
	}
}
