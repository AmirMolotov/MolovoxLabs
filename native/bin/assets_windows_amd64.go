//go:build windows && amd64

// Package nativebinary embeds the native VST3 bridge built by native/vst3/build.cmd.
package nativebinary

import _ "embed"

// VSTBridge is the bridge DLL shipped inside the application executable.
//go:embed MolovoxVST3.dll
var VSTBridge []byte
