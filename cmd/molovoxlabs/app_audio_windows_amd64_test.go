//go:build windows && amd64

package main

import (
	"encoding/binary"
	audio "github.com/intermernet/gominiaudio"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestAppAudioRouting(t *testing.T) {
	const path = `C:\Music\player.exe`
	graph := PatchBay{Enabled: true, Nodes: []PatchNode{{ID: "app", Kind: "app", AppPath: path, Volume: new(float32(.5))}, {ID: "gain", Kind: "effect", FilterID: "gain"}, {ID: "out", Kind: "output", DeviceID: "virtual"}}, Cables: []PatchCable{{From: "app", To: "gain"}, {From: "gain", To: "out"}}}
	filters := []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 6.0206}}
	program, err := compilePatch(graph, filters)
	if err != nil {
		t.Fatal(err)
	}
	stream := &appAudioStream{path: appAudioPath(path)}
	for i := uint64(0); i < bridgeFrames*2; i++ {
		stream.ring.data[i%ringSize].Store(math.Float32bits(.2))
	}
	stream.ring.write.Store(bridgeFrames * 2)
	snapshot := &appAudioSnapshot{streams: []*appAudioStream{stream}}
	options := &AudioOptions{Filters: filters, Volume: .5}
	for _, id := range []string{"virtual", "speakers"} {
		r := &renderEndpoint{ID: id}
		r.prepareAppAudio(snapshot)
		r.nextAppAudio()
		got := program.sample(r, 0, nil, &[8]float32{}, options)
		want := float32(0)
		if id == "virtual" {
			want = .1
		}
		if math.Abs(float64(got-want)) > .00001 {
			t.Fatalf("%s: got %v want %v", id, got, want)
		}
		if n := testing.AllocsPerRun(20, func() { r.nextAppAudio(); program.sample(r, 0, nil, &[8]float32{}, options) }); n != 0 {
			t.Fatalf("Audio callback allocated %v", n)
		}
	}
	graph.Cables = append(graph.Cables, PatchCable{From: "gain", To: "app"})
	if _, err = compilePatch(graph, filters); err == nil {
		t.Fatal("App audio accepted an incoming cable")
	}
}
func TestAppAudioValidation(t *testing.T) {
	for _, path := range []string{"", `relative.exe`, `C:\Music\file.wav`, "C:\\bad\x00.exe"} {
		if _, err := compilePatch(PatchBay{Enabled: true, Nodes: []PatchNode{{ID: "a", Kind: "app", AppPath: path}}}, nil); err == nil {
			t.Fatalf("Accepted %q", path)
		}
	}
}

func TestAppMixerVolumeValidation(t *testing.T) {
	for _, percent := range []int{-1, 101} {
		if err := setAppMixerVolume(`C:\Music\player.exe`, percent); err == nil {
			t.Fatalf("Accepted volume %d", percent)
		}
	}
	if err := setAppMixerVolume("relative.exe", 50); err == nil {
		t.Fatal("Accepted a relative path")
	}
}

func TestAppMixerLive(t *testing.T) {
	if os.Getenv("MOLOVOX_APP_AUDIO_LIVE") != "1" {
		t.Skip("Set MOLOVOX_APP_AUDIO_LIVE=1 to check the mixer with a quiet test tone")
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(path, "-test.run=^TestAppAudioCaptureChild$")
	child.Env = append(os.Environ(), "MOLOVOX_APP_AUDIO_CHILD=1", "MOLOVOX_APP_AUDIO_TONE=1")
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	waitFor := func(playing bool, volume int) appMixerEntry {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			apps, err := listAppMixer()
			if err != nil {
				t.Fatal(err)
			}
			for _, app := range apps {
				if strings.EqualFold(app.Path, path) && app.Playing == playing && (volume < 0 || app.Volume == volume) {
					return app
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("Mixer did not detect test app with playing=%v volume=%d", playing, volume)
		return appMixerEntry{}
	}
	initial := waitFor(true, -1)
	defer func() {
		if err := setAppMixerVolume(path, initial.Volume); err != nil {
			t.Error(err)
		}
	}()
	for _, volume := range []int{42, 0, 75} {
		if err := setAppMixerVolume(path, volume); err != nil {
			t.Fatal(err)
		}
		waitFor(true, volume)
	}
}
func TestAppAudioCaptureChild(t *testing.T) {
	if os.Getenv("MOLOVOX_APP_AUDIO_CHILD") == "1" {
		if os.Getenv("MOLOVOX_APP_AUDIO_TONE") == "1" {
			context, err := audio.InitContext([]audio.Backend{audio.BackendWASAPI}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer context.Uninit()
			var frame uint64
			device, err := audio.InitDevice(context, config(audio.DeviceTypePlayback, nil), audio.DeviceCallbacks{Data: func(_ *audio.Device, output, _ []byte, frames uint32) {
				for i := uint32(0); i < frames; i++ {
					value := float32(.001 * math.Sin(2*math.Pi*440*float64(frame)/sampleRate))
					frame++
					bits := math.Float32bits(value)
					binary.LittleEndian.PutUint32(output[i*8:], bits)
					binary.LittleEndian.PutUint32(output[i*8+4:], bits)
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer device.Uninit()
			if err = device.Start(); err != nil {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Second)
		} else {
			time.Sleep(20 * time.Second)
		}
	}
}
func TestNativeAppAudioCaptureLifecycle(t *testing.T) {
	if !appAudioSupported() {
		t.Skip("Process loopback is unavailable on this Windows build")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	result, _, _ := ole.NewProc("CoInitializeEx").Call(0, 0)
	if err := appAudioResult(result); err != nil {
		t.Fatal(err)
	}
	defer ole.NewProc("CoUninitialize").Call()
	b, err := loadAppAudioBridge()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = listAppAudioProcesses(); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestAppAudioCaptureChild$")
	child.Env = append(os.Environ(), "MOLOVOX_APP_AUDIO_CHILD=1")
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	var handle uintptr
	result, _, _ = b.open.Call(uintptr(child.Process.Pid), uintptr(unsafe.Pointer(&handle)))
	if err = appAudioResult(result); err != nil {
		t.Fatal(err)
	}
	defer b.close.Call(handle)
	var samples [8192]float32
	var count uint32
	result, _, _ = b.read.Call(handle, uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)), uintptr(unsafe.Pointer(&count)))
	if err = appAudioResult(result); err != nil {
		t.Fatal(err)
	}
	if count > uint32(len(samples)) {
		t.Fatal("Capture exceeded buffer")
	}
}

// Opt-in: emits a quiet test tone on the default playback device; never opens a microphone.
func TestAppAudioLiveCapture(t *testing.T) {
	if os.Getenv("MOLOVOX_APP_AUDIO_LIVE") != "1" {
		t.Skip("Set MOLOVOX_APP_AUDIO_LIVE=1 to check real playback capture")
	}
	if !appAudioSupported() {
		t.Skip("Process loopback unavailable")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	result, _, _ := ole.NewProc("CoInitializeEx").Call(0, 0)
	if err := appAudioResult(result); err != nil {
		t.Fatal(err)
	}
	defer ole.NewProc("CoUninitialize").Call()
	child := exec.Command(os.Args[0], "-test.run=^TestAppAudioCaptureChild$")
	child.Env = append(os.Environ(), "MOLOVOX_APP_AUDIO_CHILD=1", "MOLOVOX_APP_AUDIO_TONE=1")
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	b, err := loadAppAudioBridge()
	if err != nil {
		t.Fatal(err)
	}
	var handle uintptr
	result, _, _ = b.open.Call(uintptr(child.Process.Pid), uintptr(unsafe.Pointer(&handle)))
	if err = appAudioResult(result); err != nil {
		t.Fatal(err)
	}
	defer b.close.Call(handle)
	deadline := time.Now().Add(5 * time.Second)
	detected := false
	var peak float32
	var samples [8192]float32
	for time.Now().Before(deadline) {
		entries, err := listAppAudioProcesses()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.PID == uint32(child.Process.Pid) {
				detected = true
			}
		}
		var count uint32
		result, _, _ = b.read.Call(handle, uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)), uintptr(unsafe.Pointer(&count)))
		if err = appAudioResult(result); err != nil {
			t.Fatal(err)
		}
		for _, sample := range samples[:min(int(count), len(samples))] {
			if magnitude := float32(math.Abs(float64(sample))); magnitude > peak {
				peak = magnitude
			}
		}
		if detected && peak > .00001 {
			t.Logf("Detected playing process and captured audio, peak %.6f", peak)
			engine := &nativeEngine{}
			engine.running.Store(true)
			defer engine.Close()
			path, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if err = engine.Configure(AudioOptions{Volume: 1, Patch: PatchBay{Enabled: true, Nodes: []PatchNode{{ID: "app", Kind: "app", AppPath: path}}}}); err != nil {
				t.Fatal(err)
			}
			waitFor := func(playing bool) {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					snapshot := engine.appAudio.current.Load()
					if snapshot != nil {
						capturing := len(snapshot.streams) > 0 && snapshot.streams[0].ring.write.Load() > 0
						if playing && capturing || !playing && len(snapshot.streams) == 0 {
							return
						}
					}
					time.Sleep(20 * time.Millisecond)
				}
				t.Fatalf("Dynamic capture did not reach playing=%v", playing)
			}
			waitFor(true)
			child.Process.Kill()
			child.Wait()
			waitFor(false)
			restarted := exec.Command(os.Args[0], "-test.run=^TestAppAudioCaptureChild$")
			restarted.Env = child.Env
			restarted.SysProcAttr = child.SysProcAttr
			if err = restarted.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { restarted.Process.Kill(); restarted.Wait() }()
			waitFor(true)
			t.Log("App capture reconnected after the playing process restarted")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Live capture failed: process detected=%v peak=%v", detected, peak)
}
