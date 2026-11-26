package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

//go:embed install-dependencies.ps1
var script string

func main() {
	if len(os.Args) != 2 {
		os.Exit(1)
	}
	switch os.Args[1] {
	case "ffmpeg", "webview2", "vbcable":
	default:
		os.Exit(1)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		os.Exit(1)
	}
	var command []byte
	for _, unit := range utf16.Encode([]rune(script)) {
		command = binary.LittleEndian.AppendUint16(command, unit)
	}
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(command))
	// The elevated setup uses Windows PowerShell's own modules even when launched from PowerShell 7.
	cmd.Env = append(cmd.Environ(), "PSModulePath="+filepath.Join(filepath.Dir(powershell), "Modules"), "MOLOVOX_PREREQUISITE="+os.Args[1])
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
