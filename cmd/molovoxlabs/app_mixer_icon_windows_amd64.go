//go:build windows && amd64

package main

import (
	"encoding/binary"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	mixerIconKernel        = windows.NewLazySystemDLL("kernel32.dll")
	mixerEnumResourceNames = mixerIconKernel.NewProc("EnumResourceNamesW")
	mixerFindResource      = mixerIconKernel.NewProc("FindResourceW")
	mixerLookupIcon        = user32.NewProc("LookupIconIdFromDirectoryEx")
	mixerFirstIconGroup    = windows.NewCallback(func(module, kind, name uintptr, resource *windows.Handle) uintptr {
		handle, _, _ := mixerFindResource.Call(module, name, kind)
		*resource = windows.Handle(handle)
		return 0
	})
)

func readAppMixerIcon(path string) []byte {
	if !filepath.IsAbs(path) {
		return nil
	}
	// shortcut: use the executable's embedded icon, resolve package assets if packaged apps need custom icons.
	module, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_AS_DATAFILE|windows.LOAD_LIBRARY_AS_IMAGE_RESOURCE)
	if err != nil {
		return nil
	}
	defer windows.FreeLibrary(module)

	var resource windows.Handle
	mixerEnumResourceNames.Call(uintptr(module), uintptr(windows.RT_GROUP_ICON), mixerFirstIconGroup, uintptr(unsafe.Pointer(&resource)))
	if resource == 0 {
		return nil
	}
	const groupHeader, groupEntry = 6, 14
	group, err := windows.LoadResourceData(module, resource)
	if err != nil || len(group) < groupHeader {
		return nil
	}
	le := binary.LittleEndian
	count := int(le.Uint16(group[4:6]))
	if le.Uint16(group[:2]) != 0 || le.Uint16(group[2:4]) != 1 || count == 0 || len(group) < groupHeader+count*groupEntry {
		return nil
	}
	id, _, _ := mixerLookupIcon.Call(uintptr(unsafe.Pointer(&group[0])), 1, 32, 32, 0)
	if id == 0 {
		return nil
	}
	for i := range count {
		entry := group[groupHeader+i*groupEntry:][:groupEntry]
		if le.Uint16(entry[12:14]) != uint16(id) {
			continue
		}
		resource, err := windows.FindResource(module, windows.ResourceID(id), windows.RT_ICON)
		if err != nil {
			return nil
		}
		image, err := windows.LoadResourceData(module, resource)
		if err != nil || len(image) == 0 || len(image) > 128*1024 {
			return nil
		}
		// An ICO directory stores an image offset in place of the PE resource ID.
		const imageOffset = 22
		icon := make([]byte, imageOffset+len(image))
		copy(icon, group[:4])
		le.PutUint16(icon[4:6], 1)
		copy(icon[6:14], entry[:8])
		le.PutUint32(icon[14:18], uint32(len(image)))
		le.PutUint32(icon[18:22], imageOffset)
		copy(icon[imageOffset:], image)
		return icon
	}
	return nil
}
