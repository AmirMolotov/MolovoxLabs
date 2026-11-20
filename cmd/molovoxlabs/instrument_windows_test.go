//go:build windows && amd64

package main

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestVSTEditorCloseKeepsPendingPluginRequests(t *testing.T) {
	const child = "MOLOVOX_TEST_VST_EDITOR_CHILD"
	if os.Getenv(child) == "" {
		// Isolate the bridge so a hung native call cannot block subsequent tests.
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVSTEditorCloseKeepsPendingPluginRequests$")
		cmd.Env = append(os.Environ(), child+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("editor lifecycle: %v\n%s", err, output)
		}
		return
	}
	path, err := filepath.Abs("../../native/test/MolovoxTestInstrument.vst3")
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := scanVSTModule(path)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("instrument scan: %v %v", plugins, err)
	}
	var e nativeEngine
	if err := e.setInstruments([]string{path + "|" + plugins[0].ClassID}, plugins); err != nil {
		t.Fatal(err)
	}
	rack := &e.instruments.rack
	id := path + "|" + plugins[0].ClassID
	findWindow := user32.NewProc("FindWindowW")
	isWindow := user32.NewProc("IsWindow")
	postMessage := user32.NewProc("PostMessageW")
	find := func(title string) uintptr {
		name, err := windows.UTF16PtrFromString(title)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(name))); hwnd != 0 {
				return hwnd
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("window %q did not open", title)
		return 0
	}
	for range 2 {
		if err := rack.editor(id); err != nil {
			t.Fatal(err)
		}
		editor := find("Molovox Test Instrument Editor")
		postMessage.Call(editor, 0x0010, 0, 0) // WM_CLOSE
		dialog := find("Molovox Test Editor Close")
		done := make(chan error, 1)
		go func() {
			_, err := rack.info(id)
			done <- err
		}()
		time.Sleep(100 * time.Millisecond)                                 // Let the modal loop receive the parameter request.
		if ok, _, err := postMessage.Call(dialog, 0x0010, 0, 0); ok == 0 { // WM_CLOSE
			t.Fatal("could not dismiss the fixture dialog", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			if ok, _, _ := isWindow.Call(dialog); ok != 0 {
				t.Fatal("fixture dialog stayed open")
			}
			t.Fatal("VST parameter request hung after the editor closed")
		}
	}
	rack.close()
}

func TestVSTInstrumentRoutesKeyboardAndMIDIThroughPatch(t *testing.T) {
	path, err := filepath.Abs("../../native/test/MolovoxTestInstrument.vst3")
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := scanVSTModule(path)
	if err != nil || len(plugins) != 1 || !isVSTInstrument(plugins[0]) {
		t.Fatalf("instrument scan: %v %v", plugins, err)
	}
	e := &nativeEngine{}
	defer e.Close()
	e.outputCount.Store(2)
	if err := e.setInstruments([]string{path + "|" + plugins[0].ClassID}, plugins); err != nil {
		t.Fatal(err)
	}
	patch := PatchBay{Enabled: true, Nodes: []PatchNode{
		{ID: "instrument", Kind: "instrument"}, {ID: "output", Kind: "output", DeviceID: "connected"},
	}, Cables: []PatchCable{{From: "instrument", To: "output"}}}
	if err := e.Configure(AudioOptions{Patch: patch, Volume: 1, BoardVolume: 1}); err != nil {
		t.Fatal(err)
	}
	connected, unconnected := &renderEndpoint{ID: "connected"}, &renderEndpoint{ID: "other", index: 1}
	block := make([]byte, 256*8)
	render := func(endpoint *renderEndpoint) float32 {
		e.render(endpoint, block, 256, endpoint.index == 0)
		return math.Float32frombits(binary.LittleEndian.Uint32(block[len(block)-8:]))
	}
	assertLevel := func(got, want float32) {
		t.Helper()
		if math.Abs(float64(got-want)) > 1e-6 {
			t.Fatalf("got audio sample %v, want %v", got, want)
		}
	}
	assertLevel(render(connected), 0)
	e.instruments.setInput(true, true)
	if err := e.instruments.keyNote(60, 127); err != nil {
		t.Fatal(err)
	}
	assertLevel(render(connected), .25)
	assertLevel(render(unconnected), 0)
	if !e.instruments.midiPacket(0x91 | 64<<8 | 63<<16) {
		t.Fatal("MIDI note was not captured")
	}
	assertLevel(render(connected), .25+.25*63/127)
	e.instruments.midiPacket(0x81 | 64<<8)
	assertLevel(render(connected), .25)
	e.instruments.setInput(false, false)
	assertLevel(render(connected), 0)
	if e.instruments.midiPacket(0x90 | 60<<8 | 100<<16) {
		t.Fatal("MIDI should reach regular triggers when instrument MIDI mode is disabled")
	}
	patch.Nodes = append(patch.Nodes, PatchNode{ID: "mic", Kind: "mic"})
	patch.Cables = []PatchCable{{From: "mic", To: "instrument"}}
	if _, err := compilePatch(patch, nil); err == nil {
		t.Fatal("Instruments must be source-only")
	}
}
