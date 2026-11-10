//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestApplicationHasEmbeddedIcon(t *testing.T) {
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	module, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	resource, _, _ := kernel.NewProc("FindResourceW").Call(module, 1, 14)
	if resource == 0 {
		t.Fatal("Executable is missing its application icon resource")
	}
	icon, _, _ := user32.NewProc("LoadImageW").Call(module, 1, 1, 32, 32, 0x8000)
	if icon == 0 {
		t.Fatal("Embedded application icon could not be loaded")
	}
}
