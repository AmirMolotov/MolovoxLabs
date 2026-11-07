//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	webview "github.com/jchv/go-webview2"
)

//go:embed dist/*
var assets embed.FS

func assetHandler(prefix string) http.Handler {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix(prefix, http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Permissions-Policy", "microphone=(self), camera=()")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if len(r.URL.Path) < len(prefix) || r.URL.Path[:len(prefix)] != prefix {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func main() {
	runtime.LockOSThread()
	verifyPath := flag.String("verify", "", "Run offline inversion check, write JSON result, and exit")
	smokePath := flag.String("native-smoke", "", "Run silent native WASAPI playback check and write JSON")
	capturePath := flag.String("native-capture-check", "", "Check native capture and render with muted output")
	dataRoot := flag.String("data-dir", "", "Use an alternate local profile directory")
	vstScan := flag.String("vst-scan", "", "Scan one VST3 module in an isolated process")
	vstScanAll := flag.Bool("vst-scan-all", false, "Scan installed VST3 effects and exit")
	vstResult := flag.String("vst-result", "", "Write VST3 scan results to this file")
	flag.Parse()
	if *vstScan != "" || *vstScanAll {
		result := VSTScanResult{}
		if *vstScanAll {
			result = scanVSTPlugins(nil)
		} else {
			plugins, err := scanVSTModule(*vstScan)
			result.Plugins = plugins
			if err != nil {
				result.Errors = []string{filepath.Base(*vstScan) + ": " + err.Error()}
			}
		}
		data, _ := json.Marshal(result)
		if *vstResult != "" {
			os.WriteFile(*vstResult, data, 0600)
		} else {
			fmt.Println(string(data))
		}
		return
	}
	if *verifyPath != "" || *smokePath != "" || *capturePath != "" {
		result := nativeSelfTest()
		path := *verifyPath
		if *smokePath != "" {
			result = nativeSilentSmoke()
			path = *smokePath
		}
		if *capturePath != "" {
			result = nativeMutedCaptureSmoke()
			path = *capturePath
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			log.Fatal(err)
		}
		if result["passed"] != true {
			os.Exit(1)
		}
		return
	}
	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	appRoot := filepath.Join(config, "QuietLab")
	if *dataRoot != "" {
		appRoot = *dataRoot
	}
	profile := filepath.Join(appRoot, "WebView2")
	store := &appStore{root: appRoot}
	engine := &nativeEngine{}
	recorder := &samplerRecorder{store: store}
	arranger := &arrangementRenderer{store: store}
	defer arranger.stop()
	if err := recorder.recoverPending(); err != nil {
		log.Print("Sampler recording recovery: ", err)
	}
	defer recorder.close()
	if err = os.MkdirAll(profile, 0700); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		log.Fatal(err)
	}
	prefix := "/" + hex.EncodeToString(token) + "/"
	media := &samplerHTTP{prefix: prefix, store: store, recorder: recorder, engine: engine, arranger: arranger}
	server := &http.Server{Handler: media.handler(assetHandler(prefix)), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	windowTitle := "Molovox Labs — Soundboard Studio"
	if *dataRoot != "" {
		windowTitle += " — Test profile"
	}
	window := webview.NewWithOptions(webview.WebViewOptions{
		DataPath: profile, AutoFocus: true,
		WindowOptions: webview.WindowOptions{Title: windowTitle, Width: 1240, Height: 950, Center: true},
	})
	if window == nil {
		log.Fatal("Could not start WebView2. Install Microsoft Edge WebView2 Runtime.")
	}
	defer window.Destroy()
	window.SetSize(760, 600, webview.HintMin)
	setMolovoxIcon(uintptr(window.Window()))
	window.Init("window.QUIET_DESKTOP=true;window.QUIET_NATIVE=true;")
	scanner := &vstScanner{}
	defer engine.Close()
	persistVST := func() error { return store.SaveVSTStates(&engine.vst) }
	defer persistVST()
	triggers := newTriggerService(store, engine)
	defer triggers.Close()
	triggers.SoundPlayed = func(id string) {
		data, _ := json.Marshal(id)
		window.Dispatch(func() {
			window.Eval("window.dispatchEvent(new CustomEvent('sound-play',{detail:" + string(data) + "}));")
		})
	}
	triggers.FilterChanged = func(p Preferences) {
		data, _ := json.Marshal(p.Options)
		window.Dispatch(func() {
			window.Eval("window.dispatchEvent(new CustomEvent('filter-state',{detail:" + string(data) + "}));")
		})
	}

	reloadTriggers := func() error {
		p, err := store.LoadPreferences()
		if err != nil {
			return err
		}
		return triggers.Configure(p)
	}
	if err = reloadTriggers(); err != nil {
		triggers.mu.Lock()
		triggers.status.Message = err.Error()
		triggers.mu.Unlock()
	}
	media.onLibraryChanged = reloadTriggers
	bindings := map[string]interface{}{
		"nativeLoadArrangement": store.LoadArrangement,
		"nativeRemoveArrangementAsset": func(id string) (ArrangementLibrary, error) {
			if arranger.snapshot().State == "rendering" {
				return ArrangementLibrary{}, errors.New("Wait for the render or cancel it before removing media")
			}
			return store.RemoveArrangementAsset(id)
		},
		"nativeSaveArrangement":    store.SaveArrangement,
		"nativeArrangementFromPad": store.ArrangementFromPad,
		"nativeImportArrangementAsset": func() (ArrangementAsset, error) {
			path, err := chooseMediaFile(uintptr(window.Window()), "Import arrangement media", "*.wav;*.mp3;*.flac;*.ogg;*.opus;*.aac;*.m4a;*.aiff;*.wma;*.mp4;*.mkv;*.webm;*.mov;*.avi;*.m4v;*.wmv;*.png;*.jpg;*.jpeg;*.webp;*.gif;*.bmp;*.tif;*.tiff")
			if err != nil || path == "" {
				return ArrangementAsset{}, err
			}
			return store.ImportArrangementAsset(path, "")
		},
		"nativeBeginArrangementRender": func(project ArrangementProject, kind string) (ArrangementRenderStatus, error) {
			target := ""
			if kind != "preview" {
				extension := ".mp4"
				if kind == "audio" {
					extension = ".wav"
				}
				var err error
				target, err = chooseAudioExportFile(uintptr(window.Window()), project.Name, extension)
				if err != nil || target == "" {
					return ArrangementRenderStatus{}, err
				}
			}
			return arranger.begin(project, kind, target)
		},
		"nativeArrangementRenderStatus": arranger.snapshot,
		"nativeCancelArrangementRender": arranger.stop,
		"nativeStartArrangementPreview": func(id string, at float64) error {
			path, err := arranger.previewPath(id, ".f32")
			if err != nil {
				return err
			}
			return engine.playArrangementPreview(path, at)
		},
		"nativeStopArrangementPreview":   engine.StopArrangementPreview,
		"nativeArrangementPreviewStatus": engine.ArrangementPreviewStatus,
		"nativeSamplerLibrary":           store.SamplerLibrary,
		"nativeBeginMicRecording":        engine.beginMicRecording,
		"nativeAbortMicRecording":        engine.abortMicRecording,
		"nativeCreateSamplerBoard":       store.CreateSamplerBoard,
		"nativePrepareMedia":             store.PrepareMedia,
		"nativeSetPlaybackMode":          engine.SetPlaybackMode,
		"nativeImportAudioMedia": func() (MediaInfo, error) {
			path, err := chooseMediaFile(uintptr(window.Window()), "Add audio or video", "*.wav;*.mp3;*.flac;*.ogg;*.opus;*.aac;*.m4a;*.aiff;*.aif;*.wma;*.mp4;*.mkv;*.webm;*.mov;*.avi;*.m4v;*.mpeg;*.mpg;*.ts;*.m2ts;*.wmv;*.flv")
			if err != nil || path == "" {
				return MediaInfo{}, err
			}
			return store.ImportAudioMedia(path)
		},
		"nativeImportVideo": func() (VideoSource, error) {
			path, err := chooseVideoFile(uintptr(window.Window()))
			if err != nil || path == "" {
				return VideoSource{}, err
			}
			return store.ImportVideo(path)
		},
		"nativeBeginSamplerRecording": func(name, output, mode string) (string, error) {
			id, err := recorder.begin(name)
			if err != nil {
				return "", err
			}
			if err = engine.beginSamplerTap(id, output, mode); err != nil {
				recorder.abort(id)
				return "", err
			}
			return id, nil
		},
		"nativeFinishSamplerRecording": func(id string, seconds float64) (VideoRecording, error) {
			engine.endSamplerTap(id)
			return recorder.finish(id, seconds)
		},
		"nativeAbortSamplerRecording": func(id string) { engine.endSamplerTap(id); recorder.abort(id) },
		"nativeExportSamplerRecording": func(id string) (string, error) {
			path, err := chooseAudioExportFile(uintptr(window.Window()), "Molovox Labs performance", ".webm")
			if err != nil || path == "" {
				return "", err
			}
			return path, store.ExportRecording(id, path)
		},
		"nativeRecordingAsSource":      store.RecordingAsSource,
		"nativeRemoveSamplerRecording": store.RemoveRecording,
		"nativeLoadPreferences":        store.LoadPreferences,
		"nativeScanVST":                func(folders []string) error { return scanner.start(store, folders) },
		"nativeVSTScanStatus":          scanner.snapshot,
		"nativeLoadVSTPlugins":         store.VSTLibrary,
		"nativeVSTInfo":                engine.vst.info,
		"nativeVSTEditor":              engine.vst.editor,
		"nativeVSTParameter": func(id string, param uint32, value float64) error {
			if err := engine.vst.parameter(id, param, value); err != nil {
				return err
			}
			return persistVST()
		},
		"nativeSaveVSTState": persistVST,
		"nativeSavePreferences": func(p Preferences) error {
			l, err := store.Library()
			if err != nil {
				return err
			}
			if err = validateFilters(p.Options.Filters); err != nil {
				return err
			}
			if _, err = compilePatch(p.Options.Patch, p.Options.Filters); err != nil {
				return err
			}
			if err = validateBindings(triggerLibrary(l, p)); err != nil {
				return err
			}
			p.Options.Filters, err = engine.vst.capture(p.Options.Filters)
			if err != nil {
				return err
			}
			if err = store.SavePreferences(p); err != nil {
				return err
			}
			if err = engine.Configure(p.Options); err != nil {
				return err
			}
			if err := triggers.Configure(p); err != nil {
				triggers.mu.Lock()
				triggers.status.Message = "Settings saved; trigger setup needs attention: " + err.Error()
				triggers.mu.Unlock()
			}
			return nil
		},
		"nativeSetTriggerTarget": func(target string, keys []KeyBinding, midis []MIDIBinding) (triggerEditResult, error) {
			old, pErr := store.LoadPreferences()
			l, lErr := store.Library()
			original := l
			result := triggerEditResult{Library: l, Preferences: old}
			if pErr != nil {
				return result, pErr
			}
			if lErr != nil {
				return result, lErr
			}
			p, err := store.LoadPreferences()
			if err != nil {
				return result, err
			}
			if strings.HasPrefix(target, "@") {
				if err = updateFilterBindings(&p, target, keys, midis); err != nil {
					return result, err
				}
			} else {
				l, err = store.SetMultipleBindings(target, keys, midis)
				if err != nil {
					return result, err
				}
			}
			if err = triggers.Configure(p); err == nil {
				err = store.SavePreferences(p)
			}
			if err != nil {
				store.replaceLibrary(original)
				triggers.Configure(old)
				return result, err
			}
			engine.Configure(p.Options)
			return triggerEditResult{Library: l, Preferences: p}, nil
		},
		"nativeToggleFilters": func(target string) (Preferences, error) {
			if err := triggers.Run(target); err != nil {
				return Preferences{}, err
			}
			return store.LoadPreferences()
		},
		"nativeExportAudio": func(id string) (string, error) {
			l, err := store.Library()
			if err != nil {
				return "", err
			}
			for _, b := range l.Boards {
				for _, sound := range b.Sounds {
					if sound.ID == id {
						path, err := chooseAudioExportFile(uintptr(window.Window()), sound.Name, filepath.Ext(sound.File))
						if err != nil || path == "" {
							return "", err
						}
						return path, store.ExportAudio(id, path)
					}
				}
			}
			return "", fmt.Errorf("Sound not found")
		},
		"nativeStopSound":   engine.StopSound,
		"nativeLibrary":     store.Library,
		"nativeCreateBoard": store.CreateBoard,
		"nativeRemoveBoard": func(id string) (BoardRemoval, error) {
			old, err := store.Library()
			if err != nil {
				return BoardRemoval{}, err
			}
			result, err := store.RemoveBoard(id)
			if err != nil {
				return result, err
			}
			if err = reloadTriggers(); err != nil {
				result.Warnings = append(result.Warnings, "Board deleted; shortcut setup needs attention: "+err.Error())
			}
			triggers.mu.Lock()
			for _, board := range old.Boards {
				if board.ID == id {
					for _, sound := range board.Sounds {
						delete(triggers.cache, sound.ID)
						engine.StopSound(sound.ID)
					}
				}
			}
			triggers.mu.Unlock()
			return result, nil
		},
		"nativeRemoveSound": func(id string) (libraryState, error) {
			l, err := store.RemoveSound(id)
			if err != nil {
				return l, err
			}
			triggers.mu.Lock()
			delete(triggers.cache, id)
			triggers.mu.Unlock()
			engine.StopSound(id)
			if err = reloadTriggers(); err != nil {
				triggers.mu.Lock()
				triggers.status.Message = "Clip removed; shortcut setup needs attention: " + err.Error()
				triggers.mu.Unlock()
			}
			return l, nil
		},
		"nativeMIDIDevices":      midiDevices,
		"nativeSelectMIDIInput":  triggers.SelectMIDIInput,
		"nativeTriggerStatus":    triggers.Status,
		"nativeLearnMIDI":        triggers.Learn,
		"nativeSuspendShortcuts": triggers.SuspendShortcuts,
		"nativeSetBindings": func(id string, key *KeyBinding, midi *MIDIBinding) (libraryState, error) {
			old, err := store.Library()
			if err != nil {
				return old, err
			}
			l, err := store.SetBindings(id, key, midi)
			if err != nil {
				return old, err
			}
			if err = reloadTriggers(); err != nil {
				for _, b := range old.Boards {
					for _, sound := range b.Sounds {
						if sound.ID == id {
							store.SetBindings(id, sound.Hotkey, sound.MIDI)
						}
					}
				}
				reloadTriggers()
				return old, err
			}
			return l, nil
		},
		"nativeExportBoard": func(id string) (string, error) {
			path, err := chooseBoardFile(uintptr(window.Window()), true)
			if err != nil || path == "" {
				return "", err
			}
			return path, store.ExportBoard(id, path)
		},
		"nativeImportBoard": func() (libraryState, error) {
			path, err := chooseBoardFile(uintptr(window.Window()), false)
			if err != nil {
				return libraryState{}, err
			}
			if path == "" {
				return store.Library()
			}
			l, err := store.ImportBoard(path)
			if err != nil {
				return l, err
			}
			if err = reloadTriggers(); err != nil {
				triggers.mu.Lock()
				triggers.status.Message = "Board imported; resolve shortcut/MIDI conflict: " + err.Error()
				triggers.mu.Unlock()
			}
			return l, nil
		},
		"nativeImportSound": func(board string) (libraryState, error) {
			path, err := chooseSoundFile(uintptr(window.Window()))
			if err != nil {
				return libraryState{}, err
			}
			if path == "" {
				return store.Library()
			}
			return store.ImportFile(board, path)
		},
		"nativePlaySound":  triggers.Play,
		"nativeStopSounds": engine.StopSounds,
		"nativeDevices":    engine.Devices,
		"nativeStart":      engine.Start,
		"nativeStop":       engine.Stop,
		"nativeConfigure":  engine.Configure,
		"nativeSnapshot":   engine.Snapshot,
		"nativeTone":       engine.TestTone,
		"nativeSelfTest":   nativeSelfTest,
	}
	for name, fn := range bindings {
		if err = window.Bind(name, fn); err != nil {
			log.Fatal(err)
		}
	}

	window.Navigate(fmt.Sprintf("http://%s%s", listener.Addr(), prefix+"studio.html"))
	window.Run()
}
