//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoSampleMultipartSaveAndReload(t *testing.T) {
	s, state, source := samplerFixture(t)
	meta := CreateVideoSample{BoardID: state.Library.Boards[1].ID, Name: "Trimmed moment", Video: VideoSample{SourceID: source.ID, In: 1, Out: 1.1, Crop: VideoCrop{X: .1, Y: .1, Width: .8, Height: .8}, Thumbnail: "data:image/jpeg;base64," + strings.Repeat("a", 70000)}}
	var payload bytes.Buffer
	form := multipart.NewWriter(&payload)
	metadata, _ := json.Marshal(meta)
	form.WriteField("metadata", string(metadata))
	file, _ := form.CreateFormFile("audio", "sample.wav")
	file.Write(samplerWavFixture(4800))
	form.Close()
	r := httptest.NewRequest("POST", "http://local/session/sampler/sample", &payload)
	r.Header.Set("Origin", "http://local")
	r.Header.Set("Content-Type", form.FormDataContentType())
	changed := 0
	h := (&samplerHTTP{prefix: "/session/", store: s, onLibraryChanged: func() error { changed++; return nil }}).handler(http.NotFoundHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if changed != 1 {
		t.Fatal("Native trigger library was not refreshed")
	}
	saved, err := s.SamplerLibrary()
	if err != nil {
		t.Fatal(err)
	}
	board := saved.Library.Boards[1]
	if len(board.Sounds) != 1 || board.Sounds[0].Name != meta.Name || board.Sounds[0].Video.In != 1 {
		t.Fatal("Sample did not persist on selected board", board)
	}
	pcm, err := s.Sound(board.Sounds[0].ID)
	if err != nil || len(pcm) != 4800 {
		t.Fatal("Saved sample is not playable", err)
	}
}

func samplerWavFixture(frames int) []byte {
	data := make([]byte, 44+frames*2)
	copy(data, wavFixture()[:44])
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*2))
	for i := 0; i < frames; i++ {
		binary.LittleEndian.PutUint16(data[44+i*2:], 1000)
	}
	return data
}
func samplerFixture(t *testing.T) (*appStore, SamplerLibrary, VideoSource) {
	t.Helper()
	s := &appStore{root: t.TempDir()}
	path := filepath.Join(t.TempDir(), "source.mp4")
	os.WriteFile(path, []byte("video source fixture"), 0600)
	source, err := s.ImportVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.SamplerLibrary()
	if err != nil {
		t.Fatal(err)
	}
	return s, state, source
}
func TestVideoSampleCreationPreservesTrimCropAudioAndOriginal(t *testing.T) {
	s, state, source := samplerFixture(t)
	board := state.Library.Boards[1]
	meta := CreateVideoSample{BoardID: board.ID, Name: "Moment", Video: VideoSample{SourceID: source.ID, In: 2, Out: 2.1, Crop: VideoCrop{X: .1, Y: .2, Width: .5, Height: .5}}}
	l, err := s.CreateVideoSample(meta, samplerWavFixture(4800))
	if err != nil {
		t.Fatal(err)
	}
	clip := l.Boards[1].Sounds[0]
	if clip.Video == nil || clip.Video.In != 2 || clip.Video.Crop.Width != .5 || math.Abs(clip.Seconds-.1) > 1e-6 {
		t.Fatal("Video sample metadata lost", clip)
	}
	if samples, err := s.Sound(clip.ID); err != nil || len(samples) != 4800 {
		t.Fatal("Sample cannot play through native engine", err)
	}
	if _, err = s.RemoveSound(clip.ID); err != nil {
		t.Fatal(err)
	}
	path, err := s.SourcePath(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("Removing pad deleted source video")
	}
	meta.Video.Crop.Width = 2
	if _, err = s.CreateVideoSample(meta, samplerWavFixture(4800)); err == nil {
		t.Fatal("Invalid crop accepted")
	}
	meta.Video.Crop.Width = .5
	meta.Video.Out = 3
	if _, err = s.CreateVideoSample(meta, samplerWavFixture(4800)); err == nil {
		t.Fatal("Audio/video duration mismatch accepted")
	}
}
func TestSamplerHTTPRangesUploadsAndOriginChecks(t *testing.T) {
	s, state, source := samplerFixture(t)
	handler := (&samplerHTTP{prefix: "/token/", store: s, recorder: &samplerRecorder{store: s}, engine: &nativeEngine{}}).handler(http.NotFoundHandler())
	r := httptest.NewRequest("GET", "http://local/token/sampler/source/"+source.ID, nil)
	r.Header.Set("Range", "bytes=0-4")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "video" {
		t.Fatal("Video range requests failed", w.Code, w.Body.String())
	}
	meta := CreateVideoSample{BoardID: state.Library.Boards[1].ID, Name: "Test", Video: VideoSample{SourceID: source.ID, In: 0, Out: .1, Crop: VideoCrop{Width: 1, Height: 1}}}
	encoded, _ := json.Marshal(meta)
	r = httptest.NewRequest("POST", "http://local/token/sampler/sample", bytes.NewReader(samplerWavFixture(4800)))
	r.Header.Set("Origin", "http://local")
	r.Header.Set("X-QuietLab-Sample", base64.StdEncoding.EncodeToString(encoded))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("Sample upload failed", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("POST", "http://local/token/sampler/sample", strings.NewReader("bad"))
	r.Header.Set("Origin", "https://outside.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Cross-origin upload allowed")
	}
	r = httptest.NewRequest("GET", "http://local/token/sampler/source/../../preferences.json", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("Media path escaped library", w.Code)
	}
}
func TestSamplerRecordingSavesExactChunksExportsAndRecovers(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	r := &samplerRecorder{store: s}
	id, err := r.begin("Performance")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte{0x1a, 0x45, 0xdf, 0xa3, 1, 2, 3, 4}
	if err = r.append(id, bytes.NewReader(data[:4])); err != nil {
		t.Fatal(err)
	}
	r.append(id, bytes.NewReader(data[4:]))
	recording, err := r.finish(id, 1.2)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.RecordingPath(recording.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(actual, data) {
		t.Fatal("Recording chunks changed")
	}
	exported := filepath.Join(t.TempDir(), "performance.webm")
	if err = s.ExportRecording(id, exported); err != nil {
		t.Fatal(err)
	}
	actual, _ = os.ReadFile(exported)
	if !bytes.Equal(actual, data) {
		t.Fatal("Export changed video")
	}
	id, err = r.begin("Interrupted")
	if err != nil {
		t.Fatal(err)
	}
	r.append(id, bytes.NewReader(data))
	r.jobs[id].file.Close()
	recovered := &samplerRecorder{store: s}
	if err = recovered.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordingPath(id); err != nil {
		t.Fatal("Interrupted recording not recovered", err)
	}
}
func TestRemoveSamplerRecordingKeepsSourcesAndHandlesMissingFiles(t *testing.T) {
	for _, mode := range []string{"unused", "source", "missing"} {
		t.Run(mode, func(t *testing.T) {
			store := &appStore{root: t.TempDir()}
			recorder := &samplerRecorder{store: store}
			id, err := recorder.begin("Performance")
			if err != nil {
				t.Fatal(err)
			}
			if err = recorder.append(id, bytes.NewReader([]byte{0x1a, 0x45, 0xdf, 0xa3})); err != nil {
				t.Fatal(err)
			}
			if _, err = recorder.finish(id, 1); err != nil {
				t.Fatal(err)
			}
			path, err := store.RecordingPath(id)
			if err != nil {
				t.Fatal(err)
			}
			var source VideoSource
			if mode == "source" {
				source, err = store.RecordingAsSource(id)
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "missing" {
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err = store.RemoveRecording(id); err != nil {
				t.Fatal(err)
			}
			if _, err = store.RecordingPath(id); err == nil {
				t.Fatal("Deleted recording still listed")
			}
			if mode == "source" {
				retained, err := store.rawSourcePath(source.ID)
				if err != nil || retained != path {
					t.Fatalf("Source no longer resolves: %q %v", retained, err)
				}
				if _, err = os.Stat(retained); err != nil {
					t.Fatal(err)
				}
			} else if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("Recording file still present: %v", err)
			}
		})
	}
}
func TestSamplerTapRecordsSelectedPostEffectMix(t *testing.T) {
	e := &nativeEngine{captures: []*captureEndpoint{{gain: 1}}}
	defer e.vst.close()
	e.running.Store(true)
	r := &renderEndpoint{ID: "target", readers: make([]ringReader, 1)}
	e.renders = []*renderEndpoint{r}
	options := AudioOptions{Volume: .5, BoardVolume: 1, Filters: []MicFilter{{ID: "gain", Kind: "gain", Enabled: true, GainDB: 20 * math.Log10(2)}}, Patch: boardEffectPatch()}
	if err := e.Configure(options); err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 1000)
	for i := range samples {
		samples[i] = .2
	}
	e.voices.Store(&boardVoices{Slots: [8]boardVoice{{ID: "clip", BoardID: "music", Sequence: 1, Samples: samples}}})
	if err := e.beginSamplerTap("session", "target"); err != nil {
		t.Fatal(err)
	}
	output := make([]byte, 480*8)
	e.render(r, output, 480, true)
	data, cursor, dropped, err := e.samplerAudio("session", 0)
	if err != nil || cursor != 480 || dropped != 0 || len(data) != 480*4 {
		t.Fatal("Recording tap missing frames", cursor, err)
	}
	for i := 0; i < 480; i++ {
		if binary.LittleEndian.Uint32(data[i*4:]) != binary.LittleEndian.Uint32(output[i*8:]) {
			t.Fatal("Recorded mix differs from processed output")
		}
	}
	if math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(data[300*4:])))-.2) > .0001 {
		t.Fatal("Effects/master volume absent in recorded audio")
	}
	other := &renderEndpoint{ID: "other", readers: make([]ringReader, 1)}
	e.render(other, make([]byte, 128*8), 128, false)
	if e.samplerTap.Load().write.Load() != 480 {
		t.Fatal("Wrong output entered recording")
	}
	e.endSamplerTap("session")
	if _, _, _, err = e.samplerAudio("session", 0); err == nil {
		t.Fatal("Ended tap remained active")
	}
}
func TestVideoSamplerShortcutScopeAndBoardDeletion(t *testing.T) {
	l := libraryState{Boards: []SoundBoard{{ID: "a", Sounds: []SoundEntry{{ID: "a1"}}}, {ID: "v1", Kind: "video", Sounds: []SoundEntry{{ID: "video1"}}}, {ID: "v2", Kind: "video", Sounds: []SoundEntry{{ID: "video2"}}}}}
	active := activeTriggerLibrary(l, Preferences{BoardID: "a", SamplerBoardID: "v2"})
	if len(active.Boards) != 3 || active.Boards[1].ID != "v2" {
		t.Fatal("Sampler shortcut scope incorrect", active)
	}
	s := &appStore{root: t.TempDir()}
	s.write("soundboards.json", libraryState{Boards: l.Boards[:2]})
	result, err := s.RemoveBoard("v1")
	if err != nil {
		t.Fatal(err)
	}
	video := 0
	for _, b := range result.Library.Boards {
		if b.Kind == "video" {
			video++
		}
	}
	if video != 1 {
		t.Fatal("Deleting last video board did not leave a usable empty sampler")
	}
}
