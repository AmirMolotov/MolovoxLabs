//go:build windows

package main

import "golang.org/x/sys/windows"

func openInstanceMutex(name string) (windows.Handle, error) {
	key, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateMutex(nil, false, key)
	if err != nil && handle != 0 {
		windows.CloseHandle(handle)
		return 0, err
	}
	return handle, err
}
