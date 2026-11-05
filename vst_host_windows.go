//go:build windows && amd64

package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed native/bin/QuietLabVST3.dll
var vstBridgeBinary []byte

type vstBridge struct {
	dll                                                                 *windows.DLL
	scan, open, process, parameters, setParameter, editor, state, close *windows.Proc
	mu                                                                  sync.Mutex // Serializes control calls and their result buffers, never audio.
}

var bridgeOnce sync.Once
var bridgeInstance *vstBridge
var bridgeError error

func loadVSTBridge() (*vstBridge, error) {
	bridgeOnce.Do(func() {
		root, err := os.UserCacheDir()
		if err != nil {
			bridgeError = err
			return
		}
		digest := sha256.Sum256(vstBridgeBinary)
		root = filepath.Join(root, "QuietLab", "native")
		if err = os.MkdirAll(root, 0700); err != nil {
			bridgeError = err
			return
		}
		path := filepath.Join(root, fmt.Sprintf("QuietLabVST3-%x.dll", digest[:8]))
		if existing, err := os.ReadFile(path); err != nil || sha256.Sum256(existing) != digest {
			if err = os.WriteFile(path, vstBridgeBinary, 0600); err != nil {
				bridgeError = err
				return
			}
		}
		dll, err := windows.LoadDLL(path)
		if err != nil {
			bridgeError = err
			return
		}
		b := &vstBridge{dll: dll}
		for name, target := range map[string]**windows.Proc{"ql_scan": &b.scan, "ql_open": &b.open, "ql_process": &b.process, "ql_parameters": &b.parameters, "ql_set_parameter": &b.setParameter, "ql_editor": &b.editor, "ql_state": &b.state, "ql_close": &b.close} {
			*target, err = dll.FindProc(name)
			if err != nil {
				bridgeError = err
				return
			}
		}
		bridgeInstance = b
	})
	return bridgeInstance, bridgeError
}
func vstCString(s string) (*byte, error) { return windows.BytePtrFromString(s) }
func vstText(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

type VSTPlugin struct{ Path, ClassID, Name, Vendor, Category string }
type VSTScanResult struct {
	Plugins []VSTPlugin
	Errors  []string
	Folders []string
}
type VSTScanStatus struct {
	Running bool
	Result  VSTScanResult
}
type vstScanner struct {
	mu     sync.Mutex
	status VSTScanStatus
}

func (s *vstScanner) start(store *appStore, folders []string) error {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return errors.New("A plugin scan is already running")
	}
	s.status = VSTScanStatus{Running: true}
	s.mu.Unlock()
	go func() {
		result := store.ScanVST(folders)
		s.mu.Lock()
		s.status = VSTScanStatus{Result: result}
		s.mu.Unlock()
	}()
	return nil
}
func (s *vstScanner) snapshot() VSTScanStatus { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

type VSTParameter struct {
	ID               uint32
	Name, Display    string
	Value            float64
	Steps            int
	ReadOnly, Hidden bool
}
type VSTInfo struct {
	Latency      uint32
	NeedsRestart bool
	Parameters   []VSTParameter
	Error        string
}

func scanVSTModule(path string) ([]VSTPlugin, error) {
	b, err := loadVSTBridge()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p, err := vstCString(path)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1024*1024)
	ok, _, _ := b.scan.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)))
	if ok == 0 {
		return nil, errors.New(vstText(out))
	}
	var plugins []VSTPlugin
	if err = json.Unmarshal([]byte(vstText(out)), &plugins); err != nil {
		return nil, err
	}
	for i := range plugins {
		plugins[i].Path = path
	}
	return plugins, nil
}
func defaultVSTFolders() []string {
	exe, _ := os.Executable()
	return []string{filepath.Join(os.Getenv("ProgramFiles"), "Common Files", "VST3"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Common", "VST3"), filepath.Join(filepath.Dir(exe), "VST3"), filepath.Join(filepath.Dir(filepath.Dir(exe)), "VST3")}
}
func findVSTModules(folders []string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, folder := range folders {
		if folder == "" {
			continue
		}
		filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".vst3") {
				absolute, _ := filepath.Abs(path)
				key := strings.ToLower(absolute)
				if !seen[key] {
					seen[key] = true
					paths = append(paths, absolute)
				}
				if d.IsDir() {
					return filepath.SkipDir
				}
			}
			return nil
		})
	}
	return paths
}
func scanVSTPlugins(folders []string) VSTScanResult {
	folders = append(defaultVSTFolders(), folders...)
	result := VSTScanResult{Plugins: []VSTPlugin{}, Errors: []string{}, Folders: folders}
	exe, err := os.Executable()
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	for _, path := range findVSTModules(folders) {
		func() {
			file, err := os.CreateTemp("", "quietlab-vst-scan-*.json")
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				return
			}
			output := file.Name()
			file.Close()
			defer os.Remove(output)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "--vst-scan", path, "--vst-result", output)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if err = cmd.Run(); err != nil {
				result.Errors = append(result.Errors, filepath.Base(path)+": scan failed or timed out")
				return
			}
			data, err := os.ReadFile(output)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				return
			}
			var item VSTScanResult
			if err = json.Unmarshal(data, &item); err != nil {
				result.Errors = append(result.Errors, err.Error())
				return
			}
			result.Plugins = append(result.Plugins, item.Plugins...)
			result.Errors = append(result.Errors, item.Errors...)
		}()
	}
	return result
}

type vstVoice struct {
	input, output [128]float32
	at            int
	failed        atomic.Bool
}
type vstUnit struct {
	handle        uintptr
	bridge        *vstBridge
	path, classID string
	voices        []vstVoice
}

func openVSTUnit(f MicFilter, outputs int) (*vstUnit, error) {
	b, err := loadVSTBridge()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p, err := vstCString(f.PluginPath)
	if err != nil {
		return nil, err
	}
	class, err := vstCString(f.ClassID)
	if err != nil {
		return nil, err
	}
	var state []byte
	if f.VSTState != "" {
		state, err = base64.StdEncoding.DecodeString(f.VSTState)
		if err != nil {
			return nil, errors.New("Invalid plugin state")
		}
	}
	var statePtr uintptr
	if len(state) > 0 {
		statePtr = uintptr(unsafe.Pointer(&state[0]))
	}
	out := make([]byte, 2048)
	h, _, _ := b.open.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(class)), statePtr, uintptr(len(state)), uintptr(outputs), uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)))
	if h == 0 {
		return nil, errors.New(vstText(out))
	}
	return &vstUnit{handle: h, bridge: b, path: f.PluginPath, classID: f.ClassID, voices: make([]vstVoice, outputs)}, nil
}
func (u *vstUnit) sample(voice int, x float32) float32 {
	if voice < 0 || voice >= len(u.voices) {
		return x
	}
	v := &u.voices[voice]
	if v.failed.Load() {
		return x
	}
	y := v.output[v.at]
	v.input[v.at] = x
	v.at++
	if v.at == len(v.input) {
		v.at = 0
		ok, _, _ := syscall.SyscallN(u.bridge.process.Addr(), u.handle, uintptr(voice), uintptr(unsafe.Pointer(&v.input[0])), uintptr(unsafe.Pointer(&v.output[0])), uintptr(len(v.input)))
		if ok == 0 {
			v.failed.Store(true)
			return x
		}
	}
	return y
}
func (u *vstUnit) info() (VSTInfo, error) {
	b := u.bridge
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, 8*1024*1024)
	ok, _, _ := b.parameters.Call(u.handle, uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)))
	if ok == 0 {
		return VSTInfo{}, errors.New(vstText(out))
	}
	var info VSTInfo
	err := json.Unmarshal([]byte(vstText(out)), &info)
	for i := range u.voices {
		if u.voices[i].failed.Load() {
			info.Error = "Plugin processing failed; this effect is bypassed. Remove and add it again."
		}
	}
	return info, err
}
func (u *vstUnit) setParameter(id uint32, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return errors.New("Invalid parameter value")
	}
	b := u.bridge
	b.mu.Lock()
	defer b.mu.Unlock()
	ok, _, _ := b.setParameter.Call(u.handle, uintptr(id), uintptr(unsafe.Pointer(&value)))
	if ok == 0 {
		return errors.New("Plugin rejected the parameter change")
	}
	return nil
}
func (u *vstUnit) openEditor() error {
	b := u.bridge
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, 2048)
	ok, _, _ := b.editor.Call(u.handle, uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)))
	if ok == 0 {
		return errors.New(vstText(out))
	}
	return nil
}
func (u *vstUnit) state() (string, error) {
	b := u.bridge
	b.mu.Lock()
	defer b.mu.Unlock()
	n, _, _ := b.state.Call(u.handle, 0, 0)
	if int32(n) < 0 || n > 16*1024*1024 {
		return "", errors.New("Plugin state could not be saved (maximum 16 MB)")
	}
	if n == 0 {
		return "", nil
	}
	data := make([]byte, n)
	result, _, _ := b.state.Call(u.handle, uintptr(unsafe.Pointer(&data[0])), n)
	if result != n {
		return "", errors.New("Plugin state could not be saved")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
func (u *vstUnit) close() {
	u.bridge.mu.Lock()
	defer u.bridge.mu.Unlock()
	u.bridge.close.Call(u.handle)
}

type vstRackSnapshot struct {
	units  map[string]*vstUnit
	errors map[string]string
}
type vstRack struct {
	mu      sync.Mutex
	current atomic.Pointer[vstRackSnapshot]
	retired []*vstUnit
}

func (r *vstRack) configure(filters []MicFilter, outputs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if outputs < 1 {
		outputs = 1
	}
	old := r.current.Load()
	next := &vstRackSnapshot{units: map[string]*vstUnit{}, errors: map[string]string{}}
	for _, f := range filters {
		if f.Kind != "vst3" {
			continue
		}
		var unit *vstUnit
		if old != nil {
			unit = old.units[f.ID]
		}
		if unit != nil && (unit.path != f.PluginPath || unit.classID != f.ClassID || len(unit.voices) != outputs) {
			if unit.path == f.PluginPath && unit.classID == f.ClassID {
				if state, err := unit.state(); err == nil {
					f.VSTState = state
				}
			}
			unit = nil
		}
		if unit == nil && f.Enabled {
			var err error
			unit, err = openVSTUnit(f, outputs)
			if err != nil {
				next.errors[f.ID] = "Plugin bypassed: " + err.Error()
			}
		}
		if unit != nil {
			next.units[f.ID] = unit
		}
	}
	if old != nil {
		for id, u := range old.units {
			if next.units[id] != u {
				r.retired = append(r.retired, u)
			}
		}
	}
	r.current.Store(next)
}
func (r *vstRack) unit(id string) (*vstUnit, error) {
	s := r.current.Load()
	if s == nil {
		return nil, errors.New("Plugin is not loaded")
	}
	if err := s.errors[id]; err != "" {
		return nil, errors.New(err)
	}
	u := s.units[id]
	if u == nil {
		return nil, errors.New("Enable this plugin to load it")
	}
	return u, nil
}
func (r *vstRack) capture(filters []MicFilter) ([]MicFilter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := append([]MicFilter(nil), filters...)
	for i, f := range result {
		if f.Kind != "vst3" {
			continue
		}
		u, err := r.unit(f.ID)
		if err != nil {
			continue
		}
		state, err := u.state()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.PluginName, err)
		}
		result[i].VSTState = state
	}
	return result, nil
}
func (r *vstRack) collectRetired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.retired {
		u.close()
	}
	r.retired = nil
}
func (r *vstRack) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.current.Swap(nil); old != nil {
		for _, u := range old.units {
			u.close()
		}
	}
	for _, u := range r.retired {
		u.close()
	}
	r.retired = nil
}

func validVSTFilter(f MicFilter) error {
	if f.ID == "" || f.PluginName == "" || len(f.PluginName) > 256 || !filepath.IsAbs(f.PluginPath) || !strings.EqualFold(filepath.Ext(f.PluginPath), ".vst3") {
		return errors.New("Invalid VST3 plugin selection")
	}
	id, err := hex.DecodeString(f.ClassID)
	if err != nil || len(id) != 16 {
		return errors.New("Invalid VST3 class ID")
	}
	if len(f.VSTState) > 24*1024*1024 {
		return errors.New("Plugin state is too large")
	}
	return nil
}

func (r *vstRack) info(id string) (VSTInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, err := r.unit(id)
	if err != nil {
		return VSTInfo{Error: err.Error()}, nil
	}
	return u.info()
}
func (r *vstRack) editor(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, err := r.unit(id)
	if err != nil {
		return err
	}
	return u.openEditor()
}
func (r *vstRack) parameter(id string, param uint32, value float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, err := r.unit(id)
	if err != nil {
		return err
	}
	return u.setParameter(param, value)
}
func (s *appStore) SaveVSTStates(r *vstRack) error {
	if snapshot := r.current.Load(); snapshot == nil || len(snapshot.units) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var p Preferences
	if err := s.read("preferences.json", &p); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	filters, err := r.capture(p.Options.Filters)
	if err != nil {
		return err
	}
	p.Options.Filters = filters
	return s.write("preferences.json", p)
}
func (s *appStore) VSTLibrary() VSTScanResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result VSTScanResult
	if s.read("vst-plugins.json", &result) != nil {
		result.Plugins = []VSTPlugin{}
	}
	return result
}
func (s *appStore) ScanVST(folders []string) VSTScanResult {
	result := scanVSTPlugins(folders)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write("vst-plugins.json", result); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}
