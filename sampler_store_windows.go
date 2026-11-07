//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxVideoBytes = 8 * 1024 * 1024 * 1024

type VideoCrop struct{ X, Y, Width, Height float64 }
type VideoSample struct {
	SourceID  string
	In, Out   float64
	Crop      VideoCrop
	Thumbnail string
}
type VideoSource struct {
	Kind           string `json:",omitempty"`
	ID, Name, File string
	Bytes          int64
	RecordingID    string  `json:",omitempty"`
	Seconds        float64 `json:",omitempty"`
}
type VideoRecording struct {
	ID, Name, File string
	Seconds        float64
	Bytes          int64
	Created        string
}
type SamplerLibrary struct {
	Sources    []VideoSource
	Recordings []VideoRecording
	Library    libraryState
}
type CreateVideoSample struct {
	BoardID, Name string
	Video         VideoSample
}

func chooseVideoFile(owner uintptr) (string, error) {
	return chooseMediaFile(owner, "Import video into Molovox Labs", "*.mp4;*.webm;*.mov;*.m4v;*.mkv;*.avi;*.wmv;*.flv;*.mpeg;*.mpg;*.ts;*.mts;*.m2ts;*.mxf;*.ogv;*.3gp")
}
func chooseMediaFile(owner uintptr, titleText, extensions string) (string, error) {
	description, _ := windows.UTF16FromString("Media files")
	pattern, _ := windows.UTF16FromString(extensions)
	filter := append(description, pattern...)
	allDescription, _ := windows.UTF16FromString("All files")
	allPattern, _ := windows.UTF16FromString("*.*")
	filter = append(filter, allDescription...)
	filter = append(filter, allPattern...)
	filter = append(filter, 0)
	file := make([]uint16, 32768)
	title, _ := windows.UTF16PtrFromString(titleText)
	args := openFileName{Owner: owner, Filter: &filter[0], File: &file[0], MaxFile: uint32(len(file)), Title: title, Flags: 0x1000 | 0x800 | 0x80000 | 8}
	args.Size = uint32(unsafe.Sizeof(args))
	ok, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&args)))
	if ok == 0 {
		code, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return "", fmt.Errorf("Video picker failed: %x", code)
		}
		return "", nil
	}
	return windows.UTF16ToString(file), nil
}
func (s *appStore) videoSourcesLocked() ([]VideoSource, error) {
	var list []VideoSource
	err := s.read("sampler-sources.json", &list)
	if os.IsNotExist(err) {
		return []VideoSource{}, nil
	}
	return list, err
}
func (s *appStore) videoRecordingsLocked() ([]VideoRecording, error) {
	var list []VideoRecording
	err := s.read("sampler-recordings.json", &list)
	if os.IsNotExist(err) {
		return []VideoRecording{}, nil
	}
	return list, err
}
func (s *appStore) SamplerLibrary() (SamplerLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sources, err := s.videoSourcesLocked()
	if err != nil {
		return SamplerLibrary{}, err
	}
	recordings, err := s.videoRecordingsLocked()
	if err != nil {
		return SamplerLibrary{}, err
	}
	l, err := s.loadLibrary()
	if err != nil {
		return SamplerLibrary{}, err
	}
	found := false
	for _, b := range l.Boards {
		if b.Kind == "video" {
			found = true
		}
	}
	if !found {
		l.Boards = append(l.Boards, SoundBoard{ID: newID(), Kind: "video", Name: "Video sampler", Sounds: []SoundEntry{}})
		if err = s.write("soundboards.json", l); err != nil {
			return SamplerLibrary{}, err
		}
	}
	return SamplerLibrary{Sources: sources, Recordings: recordings, Library: l}, nil
}
func (s *appStore) CreateSamplerBoard(name string) (libraryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return l, errors.New("Enter a sampler board name (up to 80 characters)")
	}
	count := 0
	for _, b := range l.Boards {
		if b.Kind == "video" {
			count++
		}
	}
	if count >= 20 {
		return l, errors.New("Maximum 20 video sampler boards")
	}
	l.Boards = append(l.Boards, SoundBoard{ID: newID(), Kind: "video", Name: name, Sounds: []SoundEntry{}})
	return l, s.write("soundboards.json", l)
}
func (s *appStore) ImportVideo(path string) (VideoSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return VideoSource{}, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if info.IsDir() || info.Size() == 0 || info.Size() > maxVideoBytes {
		return VideoSource{}, errors.New("Choose a media file up to 8 GB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.videoSourcesLocked()
	if err != nil {
		return VideoSource{}, err
	}
	if len(list) >= 100 {
		return VideoSource{}, errors.New("Maximum 100 source videos")
	}
	source := VideoSource{ID: newID(), Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Bytes: info.Size()}
	source.File = source.ID + ext
	dir := filepath.Join(s.root, "sampler", "videos")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return VideoSource{}, err
	}
	target := filepath.Join(dir, source.File)
	in, err := os.Open(path)
	if err != nil {
		return VideoSource{}, err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return VideoSource{}, err
	}
	n, err := io.Copy(out, io.LimitReader(in, maxVideoBytes+1))
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || n > maxVideoBytes {
		os.Remove(target)
		return VideoSource{}, errors.New("Could not copy video")
	}
	source.Bytes = n
	list = append(list, source)
	if err = s.write("sampler-sources.json", list); err != nil {
		os.Remove(target)
		return VideoSource{}, err
	}
	return source, nil
}
func (s *appStore) SourcePath(id string) (string, error) {
	raw, err := s.rawSourcePath(id)
	if err != nil {
		return "", err
	}
	if stat, e := os.Stat(s.mediaPath(id, ".mp4")); e == nil && stat.Size() > 0 {
		return s.mediaPath(id, ".mp4"), nil
	}
	return raw, nil
}
func (s *appStore) rawSourcePath(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sources, err := s.videoSourcesLocked()
	if err != nil {
		return "", err
	}
	for _, source := range sources {
		if source.ID == id && filepath.Base(source.File) == source.File {
			if source.RecordingID != "" {
				return filepath.Join(s.root, "sampler", "recordings", source.File), nil
			}
			return filepath.Join(s.root, "sampler", "videos", source.File), nil
		}
	}
	return "", errors.New("Source video not found")
}
func validateVideoSample(v VideoSample) error {
	for _, x := range []float64{v.In, v.Out, v.Crop.X, v.Crop.Y, v.Crop.Width, v.Crop.Height} {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return errors.New("Invalid video trim or crop")
		}
	}
	if v.SourceID == "" || v.In < 0 || v.Out-v.In < .05-1e-7 || v.Out-v.In > 120+1e-7 {
		return errors.New("Choose a sample between 0 and 120 seconds")
	}
	c := v.Crop
	if c.X < 0 || c.Y < 0 || c.Width < .02-1e-7 || c.Height < .02-1e-7 || c.X+c.Width > 1.00001 || c.Y+c.Height > 1.00001 {
		return errors.New("Crop must stay inside the video")
	}
	if len(v.Thumbnail) > 100000 || (v.Thumbnail != "" && !strings.HasPrefix(v.Thumbnail, "data:image/jpeg;base64,")) {
		return errors.New("Invalid sample thumbnail")
	}
	return nil
}
func (s *appStore) CreateVideoSample(meta CreateVideoSample, audio []byte) (libraryState, error) {
	if err := validateVideoSample(meta.Video); err != nil {
		return libraryState{}, err
	}
	name := strings.TrimSpace(meta.Name)
	if name == "" || len(name) > 80 {
		return libraryState{}, errors.New("Enter a sample name (up to 80 characters)")
	}
	if len(audio) > 12*1024*1024 {
		return libraryState{}, errors.New("Audio sample is too large")
	}
	samples, err := decodeSound(audio, ".wav")
	if err != nil {
		return libraryState{}, err
	}
	if math.Abs(float64(len(samples))/sampleRate-(meta.Video.Out-meta.Video.In)) > .05 {
		return libraryState{}, errors.New("Trimmed audio does not match the selected video range")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sources, err := s.videoSourcesLocked()
	if err != nil {
		return libraryState{}, err
	}
	found := false
	for _, source := range sources {
		if source.ID == meta.Video.SourceID {
			found = true
		}
	}
	if !found {
		return libraryState{}, errors.New("Source video not found")
	}
	l, err := s.loadLibrary()
	if err != nil {
		return l, err
	}
	for i, b := range l.Boards {
		if b.ID != meta.BoardID || b.Kind != "video" {
			continue
		}
		if len(b.Sounds) >= 64 {
			return l, errors.New("Maximum 64 pads per sampler board")
		}
		id := newID()
		file := id + ".wav"
		if err = os.MkdirAll(filepath.Join(s.root, "sounds"), 0700); err != nil {
			return l, err
		}
		target := filepath.Join(s.root, "sounds", file)
		if err = os.WriteFile(target, audio, 0600); err != nil {
			return l, err
		}
		video := meta.Video
		l.Boards[i].Sounds = append(l.Boards[i].Sounds, SoundEntry{ID: id, Name: name, File: file, Seconds: float64(len(samples)) / sampleRate, Waveform: soundWaveform(samples), Video: &video})
		if err = s.write("soundboards.json", l); err != nil {
			os.Remove(target)
			return l, err
		}
		return l, nil
	}
	return l, errors.New("Choose a video sampler board")
}
func (s *appStore) RecordingPath(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.videoRecordingsLocked()
	if err != nil {
		return "", err
	}
	for _, r := range list {
		if r.ID == id && filepath.Base(r.File) == r.File {
			return filepath.Join(s.root, "sampler", "recordings", r.File), nil
		}
	}
	return "", errors.New("Recording not found")
}
func (s *appStore) RemoveRecording(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.videoRecordingsLocked()
	if err != nil {
		return err
	}
	for i, recording := range list {
		if recording.ID != id {
			continue
		}
		if filepath.Ext(recording.File) != ".webm" || filepath.Base(recording.File) != recording.File {
			return errors.New("Invalid recording filename")
		}
		sources, err := s.videoSourcesLocked()
		if err != nil {
			return err
		}
		remaining := make([]VideoRecording, 0, len(list)-1)
		remaining = append(remaining, list[:i]...)
		remaining = append(remaining, list[i+1:]...)
		if err = s.write("sampler-recordings.json", remaining); err != nil {
			return err
		}
		for _, source := range sources {
			if source.RecordingID != "" && source.File == recording.File {
				return nil // A source and its samples still use the backing file.
			}
		}
		path := filepath.Join(s.root, "sampler", "recordings", recording.File)
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.Join(err, s.write("sampler-recordings.json", list))
		}
		return nil
	}
	return nil
}

func recordingName() string { return "Performance " + time.Now().Format("2006-01-02 15-04-05") }

func (s *appStore) RecordingAsSource(id string) (VideoSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordings, err := s.videoRecordingsLocked()
	if err != nil {
		return VideoSource{}, err
	}
	var source VideoSource
	for _, recording := range recordings {
		if recording.ID == id {
			source = VideoSource{ID: newID(), Name: recording.Name, File: recording.File, Bytes: recording.Bytes, Seconds: recording.Seconds, RecordingID: id}
			break
		}
	}
	if source.ID == "" {
		return source, errors.New("Recording not found")
	}
	list, err := s.videoSourcesLocked()
	if err != nil {
		return source, err
	}
	if len(list) >= 100 {
		return source, errors.New("Maximum 100 source videos")
	}
	list = append(list, source)
	return source, s.write("sampler-sources.json", list)
}
