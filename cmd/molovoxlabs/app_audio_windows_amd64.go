//go:build windows && amd64

package main

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type AppAudioSource struct{ Path, Name, Status string }
type AppAudioList struct {
	Supported bool
	Apps      []AppAudioSource
}
type nativeAppInfo struct {
	PID  uint32
	Path [1024]uint16
}
type appAudioBridge struct{ list, open, read, close, mixerList, mixerSet, mixerAdjust *windows.Proc }

var appBridgeOnce sync.Once
var appBridge *appAudioBridge
var appBridgeError error

func loadAppAudioBridge() (*appAudioBridge, error) {
	appBridgeOnce.Do(func() {
		b, err := loadVSTBridge()
		if err != nil {
			appBridgeError = err
			return
		}
		next := &appAudioBridge{}
		for name, target := range map[string]**windows.Proc{
			"ql_app_list": &next.list, "ql_app_open": &next.open, "ql_app_read": &next.read, "ql_app_close": &next.close,
			"ql_mixer_list": &next.mixerList, "ql_mixer_set": &next.mixerSet, "ql_mixer_adjust": &next.mixerAdjust,
		} {
			*target, err = b.dll.FindProc(name)
			if err != nil {
				appBridgeError = err
				return
			}
		}
		appBridge = next
	})
	return appBridge, appBridgeError
}
func appAudioSupported() bool { v := windows.RtlGetVersion(); return v.BuildNumber >= 20348 }
func appAudioResult(value uintptr) error {
	if int32(value) < 0 {
		return fmt.Errorf("Windows app audio error 0x%08X", uint32(value))
	}
	return nil
}
func listAppAudioProcesses() ([]nativeAppInfo, error) {
	if !appAudioSupported() {
		return nil, errors.New("App audio requires Windows build 20348 or later")
	}
	b, err := loadAppAudioBridge()
	if err != nil {
		return nil, err
	}
	entries := make([]nativeAppInfo, 256)
	var count uint32
	result, _, _ := b.list.Call(uintptr(unsafe.Pointer(&entries[0])), uintptr(len(entries)), uintptr(unsafe.Pointer(&count)))
	if err = appAudioResult(result); err != nil {
		return nil, err
	}
	if count > uint32(len(entries)) {
		return nil, errors.New("Invalid app audio process count")
	}
	return entries[:count], nil
}
func appAudioPath(path string) string { return strings.ToLower(path) }
func (e *nativeEngine) AppAudioSources() (AppAudioList, error) {
	result := AppAudioList{Supported: appAudioSupported(), Apps: []AppAudioSource{}}
	if !result.Supported {
		return result, nil
	}
	entries, err := listAppAudioProcesses()
	if err != nil {
		return result, err
	}
	apps := map[string]AppAudioSource{}
	for _, entry := range entries {
		path := windows.UTF16ToString(entry.Path[:])
		key := appAudioPath(path)
		apps[key] = AppAudioSource{Path: path, Name: filepath.Base(path), Status: "Playing"}
	}
	if o := e.options.Load(); o != nil {
		for _, node := range o.Patch.Nodes {
			if node.Kind == "app" {
				key := appAudioPath(node.AppPath)
				if _, ok := apps[key]; !ok {
					apps[key] = AppAudioSource{Path: node.AppPath, Name: filepath.Base(node.AppPath), Status: "Waiting for audio"}
				}
			}
		}
	}
	if snapshot := e.appAudio.current.Load(); snapshot != nil {
		for key, status := range snapshot.status {
			if app, ok := apps[key]; ok {
				app.Status = status
				apps[key] = app
			}
		}
	}
	for _, app := range apps {
		result.Apps = append(result.Apps, app)
	}
	sort.Slice(result.Apps, func(i, j int) bool { return result.Apps[i].Path < result.Apps[j].Path })
	return result, nil
}

type appAudioStream struct {
	path   string
	pid    uint32
	handle uintptr
	ring   sampleRing
}
type appAudioSnapshot struct {
	streams []*appAudioStream
	status  map[string]string
}
type appAudioCapture struct {
	once      sync.Once
	closeOnce sync.Once
	done      chan struct{}
	stopped   chan struct{}
	current   atomic.Pointer[appAudioSnapshot]
}

func (a *appAudioCapture) start(e *nativeEngine) {
	a.once.Do(func() { a.done = make(chan struct{}); a.stopped = make(chan struct{}); go a.run(e) })
}
func (a *appAudioCapture) close() {
	if a.done != nil {
		a.closeOnce.Do(func() { close(a.done); <-a.stopped })
	}
}
func (a *appAudioCapture) run(e *nativeEngine) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(a.stopped)
	ole := windows.NewLazySystemDLL("ole32.dll")
	result, _, _ := ole.NewProc("CoInitializeEx").Call(0, 0)
	if appAudioResult(result) != nil {
		return
	}
	defer ole.NewProc("CoUninitialize").Call()
	b, err := loadAppAudioBridge()
	if err != nil {
		return
	}
	streams := map[uint32]*appAudioStream{}
	defer func() {
		for _, s := range streams {
			b.close.Call(s.handle)
		}
		a.current.Store(nil)
	}()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	nextScan := time.Time{}
	buffer := make([]float32, 8192)
	for {
		select {
		case <-a.done:
			return
		case now := <-ticker.C:
			if !now.Before(nextScan) {
				nextScan = now.Add(time.Second)
				wanted := map[string]bool{}
				if e.running.Load() {
					if o := e.options.Load(); o != nil && o.Patch.Enabled {
						for _, n := range o.Patch.Nodes {
							if n.Kind == "app" {
								wanted[appAudioPath(n.AppPath)] = true
							}
						}
					}
				}
				entries := []nativeAppInfo{}
				var scanErr error
				if len(wanted) > 0 {
					entries, scanErr = listAppAudioProcesses()
				}
				// shortcut: at most 32 process trees across 8 apps, raise together with the render reader arrays if needed.
				active := map[uint32]string{}
				status := map[string]string{}
				for path := range wanted {
					status[path] = "Waiting for audio"
					if scanErr != nil {
						status[path] = scanErr.Error()
					}
				}
				for _, entry := range entries {
					path := appAudioPath(windows.UTF16ToString(entry.Path[:]))
					if wanted[path] && len(active) < 32 {
						active[entry.PID] = path
					}
				}
				for pid, s := range streams {
					if active[pid] != s.path {
						b.close.Call(s.handle)
						delete(streams, pid)
					}
				}
				for pid, path := range active {
					select {
					case <-a.done:
						return
					default:
					}
					if streams[pid] == nil {
						var handle uintptr
						result, _, _ := b.open.Call(uintptr(pid), uintptr(unsafe.Pointer(&handle)))
						if err := appAudioResult(result); err != nil {
							status[path] = err.Error()
							continue
						}
						streams[pid] = &appAudioStream{path: path, pid: pid, handle: handle}
					}
					status[path] = "Capturing"
				}
				snapshot := &appAudioSnapshot{status: status}
				for _, s := range streams {
					snapshot.streams = append(snapshot.streams, s)
				}
				sort.Slice(snapshot.streams, func(i, j int) bool { return snapshot.streams[i].pid < snapshot.streams[j].pid })
				a.current.Store(snapshot)
			}
			for pid, s := range streams {
				var count uint32
				result, _, _ := b.read.Call(s.handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&count)))
				if err := appAudioResult(result); err != nil {
					b.close.Call(s.handle)
					delete(streams, pid)
					nextScan = time.Time{}
					continue
				}
				at := s.ring.write.Load()
				for _, value := range buffer[:min(int(count), len(buffer))] {
					s.ring.data[at%ringSize].Store(math.Float32bits(value))
					at++
				}
				s.ring.write.Store(at)
			}
		}
	}
}
func (r *renderEndpoint) prepareAppAudio(snapshot *appAudioSnapshot) {
	r.appSnapshot = snapshot
	for i := range r.appRings {
		var ring *sampleRing
		if snapshot != nil && i < len(snapshot.streams) {
			ring = &snapshot.streams[i].ring
		}
		if r.appRings[i] != ring {
			r.appRings[i] = ring
			r.appReaders[i] = ringReader{}
		}
		if ring != nil {
			r.appReaders[i].prepare(ring)
		}
	}
}
func (r *renderEndpoint) nextAppAudio() {
	if r.appSnapshot == nil {
		return
	}
	for i, stream := range r.appSnapshot.streams {
		r.appSamples[i], _ = r.appReaders[i].next(&stream.ring)
	}
}
func (r *renderEndpoint) appSample(path string) float32 {
	var value float32
	if r.appSnapshot != nil {
		for i, s := range r.appSnapshot.streams {
			if s.path == path {
				value += r.appSamples[i]
			}
		}
	}
	return value
}
