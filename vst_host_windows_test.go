//go:build windows && amd64

package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func testVSTFilter(t *testing.T) MicFilter {
	t.Helper()
	path, err := filepath.Abs("native/test/MolovoxTestGain.vst3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("Build the native VST test fixture first: native/vst3/build.cmd", err)
	}
	plugins, err := scanVSTModule(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Name != "Molovox Labs Test Gain" {
		t.Fatal("VST scan failed", plugins)
	}
	return MicFilter{ID: "test-vst", Kind: "vst3", Enabled: true, PluginName: plugins[0].Name, PluginPath: path, ClassID: plugins[0].ClassID}
}
func TestVSTRealProcessingParametersStateAndMultipleOutputs(t *testing.T) {
	f := testVSTFilter(t)
	u, err := openVSTUnit(f, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer u.close()
	info, err := u.info()
	if err != nil || len(info.Parameters) != 1 || info.Parameters[0].ID != 100 {
		t.Fatal("Plugin parameters unavailable", info, err)
	}
	for voice := 0; voice < 2; voice++ {
		for i := 0; i < 512; i++ {
			y := u.sample(voice, .25)
			want := float32(.125)
			if i < 128 {
				want = 0
			}
			if math.Abs(float64(y-want)) > 1e-6 {
				t.Fatal("VST audio or block latency incorrect", voice, i, y)
			}
		}
	}
	if err = u.setParameter(100, .8); err != nil {
		t.Fatal(err)
	}
	for voice := 0; voice < 2; voice++ {
		var y float32
		for i := 0; i < 512; i++ {
			y = u.sample(voice, .25)
		}
		if math.Abs(float64(y-.2)) > 1e-6 {
			t.Fatal("Parameter not sent to every output", voice, y)
		}
	}
	state, err := u.state()
	if err != nil || state == "" {
		t.Fatal("State could not be saved", err)
	}
	f.VSTState = state
	restored, err := openVSTUnit(f, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.close()
	var y float32
	for i := 0; i < 512; i++ {
		y = restored.sample(0, .25)
	}
	if math.Abs(float64(y-.2)) > 1e-6 {
		t.Fatal("Saved VST state not restored", y)
	}
	if err = u.openEditor(); err == nil {
		t.Fatal("Fixture has no native editor; fallback should be offered")
	}
	if err = u.setParameter(100, math.NaN()); err == nil {
		t.Fatal("Invalid parameter accepted")
	}
	if n := testing.AllocsPerRun(10, func() {
		for i := 0; i < 128; i++ {
			u.sample(0, .1)
		}
	}); n != 0 {
		t.Fatal("VST audio callback allocated", n)
	}
}
func TestVSTRackPersistenceRemovalAndMissingPlugin(t *testing.T) {
	f := testVSTFilter(t)
	var rack vstRack
	defer rack.close()
	rack.configure([]MicFilter{f}, 1)
	unit, err := rack.unit(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = rack.parameter(f.ID, 100, .3); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		unit.sample(0, .1)
	}
	store := &appStore{root: t.TempDir()}
	p := Preferences{Options: AudioOptions{Filters: []MicFilter{f}}}
	if err = store.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveVSTStates(&rack); err != nil {
		t.Fatal(err)
	}
	saved, err := store.LoadPreferences()
	if err != nil || saved.Options.Filters[0].VSTState == "" {
		t.Fatal("VST settings not persisted", err)
	}
	rack.configure(nil, 1)
	if _, err = rack.unit(f.ID); err == nil {
		t.Fatal("Deleted filter remained active")
	}
	// Retired units remain valid until all audio endpoints have stopped.
	unit.sample(0, .1)
	rack.collectRetired()
	f.PluginPath = filepath.Join(t.TempDir(), "Missing.vst3")
	rack.configure([]MicFilter{f}, 1)
	info, err := rack.info(f.ID)
	if err != nil || info.Error == "" {
		t.Fatal("Missing plugin was silently accepted", info, err)
	}
	f.Enabled = false
	rack.configure([]MicFilter{f}, 1)
}
func TestVSTScannerFindsBundlesOnce(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Test.vst3")
	inside := filepath.Join(bundle, "Contents", "x86_64-win")
	os.MkdirAll(inside, 0700)
	os.WriteFile(filepath.Join(inside, "Test.vst3"), []byte("test"), 0600)
	paths := findVSTModules([]string{root, root})
	if len(paths) != 1 || paths[0] != bundle {
		t.Fatal("Scanner recursed into bundle or duplicated it", paths)
	}
}

func TestInstalledVSTSmoke(t *testing.T) {
	path := os.Getenv("QUIETLAB_TEST_VST")
	if path == "" {
		t.Skip("Optional installed-plugin smoke test")
	}
	plugins, err := scanVSTModule(path)
	if err != nil || len(plugins) == 0 {
		t.Fatal("No audio effect found", plugins, err)
	}
	f := MicFilter{ID: "installed", Kind: "vst3", Enabled: true, PluginPath: path, PluginName: plugins[0].Name, ClassID: plugins[0].ClassID}
	u, err := openVSTUnit(f, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer u.close()
	info, err := u.info()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Plugin", f.PluginName, "parameters", len(info.Parameters), "latency", info.Latency)
	energy := 0.
	for i := 0; i < 48000; i++ {
		y := u.sample(0, float32(.01*math.Sin(2*math.Pi*440*float64(i)/48000)))
		if math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
			t.Fatal("Invalid plugin output")
		}
		energy += float64(y * y)
	}
	if u.voices[0].failed.Load() || energy < 1e-8 {
		t.Fatal("Installed plugin failed to process audio", energy)
	}
	if _, err = u.state(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("QUIETLAB_TEST_VST_EDITOR") == "1" {
		if err = u.openEditor(); err != nil {
			t.Fatal(err)
		}
	}
}
