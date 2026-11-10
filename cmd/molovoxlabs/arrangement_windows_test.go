//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArrangementValidationAndPersistence(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	asset := ArrangementAsset{ID: "audio", Kind: "audio", Seconds: 10, File: "audio.wav"}
	if err := s.appendArrangementAsset(asset); err != nil {
		t.Fatal(err)
	}
	p := defaultArrangement()
	p.Clips = []ArrangementClip{{ID: "clip", AssetID: "audio", TrackID: p.Tracks[0].ID, At: 2, In: 1, Duration: 4, Volume: 1}}
	if err := s.SaveArrangement(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadArrangement()
	if err != nil || len(got.Project.Clips) != 1 || got.Project.Clips[0].At != 2 {
		t.Fatal(got, err)
	}
	cases := []ArrangementClip{p.Clips[0], p.Clips[0], p.Clips[0], p.Clips[0]}
	cases[0].In = 9
	cases[1].At = math.NaN()
	cases[2].TrackID = "missing"
	cases[3].Volume = 3
	for _, c := range cases {
		p.Clips = []ArrangementClip{c}
		if _, err := validateArrangement(p, []ArrangementAsset{asset}); err == nil {
			t.Fatal("Accepted invalid clip", c)
		}
	}
}
func TestArrangementOwnsPadMediaAfterPadDeletion(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	path := filepath.Join(t.TempDir(), "original.wav")
	os.WriteFile(path, samplerWavFixture(4800), 0600)
	lib, err := s.ImportFile("default", path)
	if err != nil {
		t.Fatal(err)
	}
	pad := lib.Boards[0].Sounds[0]
	asset, err := s.ArrangementFromPad(pad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RemoveSound(pad.ID); err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	copied, err := os.ReadFile(s.arrangementPath(asset.File))
	if err != nil || len(copied) != 44+9600 {
		t.Fatal("Arrangement lost its owned audio", err)
	}
}
func requireArrangementFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := mediaTool("ffmpeg"); err != nil {
		t.Skip(err)
	}
	if _, err := mediaTool("ffprobe"); err != nil {
		t.Skip(err)
	}
}
func TestArrangementRenderMixDelayTrimAndMute(t *testing.T) {
	requireArrangementFFmpeg(t)
	s := &appStore{root: t.TempDir()}
	path := filepath.Join(t.TempDir(), "tone.wav")
	os.WriteFile(path, samplerWavFixture(48000), 0600)
	a, err := s.ImportArrangementAsset(path, "")
	if err != nil {
		t.Fatal(err)
	}
	p := defaultArrangement()
	p.Tracks[1].Muted = true
	p.Clips = []ArrangementClip{{ID: "one", AssetID: a.ID, TrackID: p.Tracks[0].ID, At: .2, In: .1, Duration: .3, Volume: 1}, {ID: "two", AssetID: a.ID, TrackID: p.Tracks[0].ID, At: .3, In: .2, Duration: .2, Volume: 1}, {ID: "muted", AssetID: a.ID, TrackID: p.Tracks[1].ID, At: 0, Duration: .5, Volume: 2}}
	r := &arrangementRenderer{store: s}
	target := filepath.Join(t.TempDir(), "mix.wav")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = r.render(ctx, p, []ArrangementAsset{a}, ArrangementRenderStatus{Kind: "audio"}, target, .5); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	pcm, err := decodeSound(data, ".wav")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(len(pcm))/48000-.5) > .01 {
		t.Fatal("Wrong duration", len(pcm))
	}
	near := func(at float64) float32 { return pcm[int(at*48000)] }
	if math.Abs(float64(near(.1))) > .001 {
		t.Fatal("Muted track or clip delay leaked", near(.1))
	}
	if math.Abs(float64(near(.25)-1000.0/32768)) > .002 {
		t.Fatal("Wrong single clip", near(.25))
	}
	if math.Abs(float64(near(.4)-2000.0/32768)) > .002 {
		t.Fatal("Overlapping clips did not sum", near(.4))
	}
}
func TestArrangementImageVideoPreviewAndNativeSeek(t *testing.T) {
	requireArrangementFFmpeg(t)
	s := &appStore{root: t.TempDir()}
	path := filepath.Join(t.TempDir(), "picture.png")
	f, _ := os.Create(path)
	pic := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			pic.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	png.Encode(f, pic)
	f.Close()
	a, err := s.ImportArrangementAsset(path, "")
	if err != nil {
		t.Fatal(err)
	}
	p := defaultArrangement()
	p.Clips = []ArrangementClip{{ID: "image", AssetID: a.ID, TrackID: p.Tracks[0].ID, At: 0, Duration: .3, Volume: 1}}
	r := &arrangementRenderer{store: s}
	job, err := r.begin(p, "preview", "")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		job = r.snapshot()
		if job.State != "rendering" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.State != "ready" {
		r.stop()
		t.Fatal(job)
	}
	audio, err := r.previewPath(job.ID, ".f32")
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(audio)
	if err != nil || stat.Size() < 48000*.25*4 {
		t.Fatal("Missing native preview audio", err)
	}
	stream, err := newArrangementPCMStream(audio, 4800, stat.Size()/4)
	if err != nil {
		t.Fatal(err)
	}
	defer close(stream.done)
	for i := 0; i < 200; i++ {
		if stream.sample() != 0 {
			t.Fatal("Silent image generated audio")
		}
	}
	if stream.consumed.Load() != 200 {
		t.Fatal("Native preview failed to advance")
	}
	// Import rendered MP4 and render its video and audio tracks at a trimmed offset.
	video, err := s.ImportArrangementAsset(job.Path, "")
	if err != nil || video.Kind != "video" {
		t.Fatal(video, err)
	}
	p.Clips[0].AssetID = video.ID
	p.Clips[0].In = .05
	p.Clips[0].Duration = .2
	target := filepath.Join(t.TempDir(), "export.mp4")
	if err = r.render(context.Background(), p, []ArrangementAsset{video}, ArrangementRenderStatus{Kind: "video"}, target, .2); err != nil {
		t.Fatal(err)
	}
}
func TestArrangementPCMSeekSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.f32")
	raw := make([]byte, 400)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(float32(i)/100))
	}
	os.WriteFile(path, raw, 0600)
	s, err := newArrangementPCMStream(path, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer close(s.done)
	for i := 40; i < 100; i++ {
		if got := s.sample(); got != float32(i)/100 {
			t.Fatalf("sample %d got %f", i, got)
		}
	}
	if s.sample() != 0 || s.consumed.Load() != 60 {
		t.Fatal("Preview did not stop at EOF")
	}
}

func TestArrangementUploadsAndRangeRequests(t *testing.T) {
	requireArrangementFFmpeg(t)
	s := &appStore{root: t.TempDir()}
	h := (&samplerHTTP{prefix: "/token/", store: s}).handler(http.NotFoundHandler())
	request := httptest.NewRequest("POST", "http://local/token/sampler/arrangement/upload?name=clip.wav", bytes.NewReader(samplerWavFixture(4800)))
	request.Header.Set("Origin", "http://local")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var asset ArrangementAsset
	if err := json.Unmarshal(response.Body.Bytes(), &asset); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("GET", "http://local/token/sampler/arrangement/asset/"+asset.ID, nil)
	request.Header.Set("Range", "bytes=0-3")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 206 || response.Body.String() != "RIFF" {
		t.Fatal(response.Code, response.Body.String())
	}
	request = httptest.NewRequest("POST", "http://local/token/sampler/arrangement/upload?name=clip.wav", bytes.NewReader(samplerWavFixture(4800)))
	request.Header.Set("Origin", "http://foreign")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("Cross-origin upload accepted")
	}
	request = httptest.NewRequest("POST", "http://local/token/sampler/arrangement/upload?name=clip.av", bytes.NewReader([]byte("bad")))
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatal("Partial extension accepted")
	}
}
func TestArrangementCancelledExportPreservesDestination(t *testing.T) {
	requireArrangementFFmpeg(t)
	s := &appStore{root: t.TempDir()}
	target := filepath.Join(t.TempDir(), "existing.wav")
	os.WriteFile(target, []byte("original"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &arrangementRenderer{store: s}
	if err := r.render(ctx, defaultArrangement(), nil, ArrangementRenderStatus{Kind: "audio"}, target, 1); err == nil {
		t.Fatal("Cancelled export succeeded")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Fatal("Cancelled export replaced previous file")
	}
}

func TestRemoveArrangementMediaDeletesInstancesButPreservesOriginalAndOtherAssets(t *testing.T) {
	s := &appStore{root: t.TempDir()}
	original := filepath.Join(t.TempDir(), "original.wav")
	os.WriteFile(original, samplerWavFixture(4800), 0600)
	a := ArrangementAsset{ID: "delete", Name: "Delete", Kind: "audio", File: "delete.wav", Seconds: 1, HasAudio: true}
	b := ArrangementAsset{ID: "keep", Name: "Keep", Kind: "audio", File: "keep.wav", Seconds: 1}
	os.MkdirAll(filepath.Dir(s.arrangementPath(a.File)), 0700)
	os.WriteFile(s.arrangementPath(a.File), []byte("owned"), 0600)
	os.WriteFile(s.arrangementPath(b.File), []byte("keep"), 0600)
	s.appendArrangementAsset(a)
	s.appendArrangementAsset(b)
	p := defaultArrangement()
	p.Clips = []ArrangementClip{{ID: "one", AssetID: a.ID, TrackID: p.Tracks[0].ID, Duration: 1, Volume: 1}, {ID: "two", AssetID: b.ID, TrackID: p.Tracks[1].ID, Duration: 1, Volume: 1}}
	if err := s.SaveArrangement(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.RemoveArrangementAsset(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 1 || len(got.Project.Clips) != 1 || got.Project.Clips[0].AssetID != b.ID {
		t.Fatal("Deletion removed other media or left dangling instances", got)
	}
	if _, err = os.Stat(s.arrangementPath(a.File)); !os.IsNotExist(err) {
		t.Fatal("Owned media remains", err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatal("Original file changed", err)
	}
	// Stale/missing files must not prevent removing an import from the library.
	os.Remove(s.arrangementPath(b.File))
	if _, err = s.RemoveArrangementAsset(b.ID); err != nil {
		t.Fatal(err)
	}
	saved, err := s.LoadArrangement()
	if err != nil || len(saved.Project.Clips) != 0 || len(saved.Assets) != 0 {
		t.Fatal(saved, err)
	}
}

func TestArrangementNativeLoopRepeatsFromSourceStartAfterSeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loop.f32")
	raw := make([]byte, 400)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(float32(i+1)/100))
	}
	os.WriteFile(path, raw, 0600)
	stream, err := newArrangementPCMStream(path, 40, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	defer close(stream.done)
	deadline := time.Now().Add(time.Second)
	for len(stream.ready) < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 240; i++ {
		want := float32((40+i)%100+1) / 100
		if got := stream.sample(); got != want {
			t.Fatalf("Loop sample %d got %f want %f", i, got, want)
		}
	}
	e := &nativeEngine{}
	e.arrangement.Store(&arrangementTransport{streams: []*arrangementPCMStream{stream}, start: 40.0 / 48000, total: 100.0 / 48000, loop: true})
	status := e.ArrangementPreviewStatus()
	if !status.Playing || math.Abs(status.Position-80.0/48000) > 1e-7 {
		t.Fatal("Loop position did not wrap", status)
	}
}
