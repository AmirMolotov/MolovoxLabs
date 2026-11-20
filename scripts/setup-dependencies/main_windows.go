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
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
