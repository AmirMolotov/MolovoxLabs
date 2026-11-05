//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const captureRingSize = 1 << 18

type samplerAudioTap struct {
	ID, OutputID string
	Mode         string
	data         [captureRingSize]atomic.Uint32
	write        atomic.Uint64
}

func (e *nativeEngine) beginSamplerTap(id, output string, modes ...string) error {
	mode := "layered"
	if len(modes) > 0 && modes[0] != "" {
		mode = modes[0]
	}
	if mode != "single" && mode != "layered" {
		return errors.New("Choose Single clip or Layered recording mode")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running.Load() {
		return errors.New("Resume listening before recording")
	}
	found := false
	for _, r := range e.renders {
		if r.ID == output {
			found = true
		}
	}
	if !found {
		return errors.New("Choose an active audio output to record")
	}
	if mode == "single" {
		selected := &boardVoices{}
		chosen := -1
		var newest uint64
		if voices := e.voices.Load(); voices != nil {
			for slot, voice := range voices.Slots {
				if voice.Sequence == 0 || len(voice.Samples) == 0 {
					continue
				}
				alive := voice.Loop || time.Since(voice.Started).Seconds() <= float64(len(voice.Samples))/sampleRate+.1
				for _, endpoint := range e.renders {
					if endpoint.ID == output {
						published := endpoint.published[slot].Load()
						if !voice.Loop && published>>32 == voice.Sequence && published&0xffffffff >= uint64(len(voice.Samples)) {
							alive = false
						}
					}
				}
				if alive && voice.Sequence > newest {
					chosen = slot
					newest = voice.Sequence
				}
			}
			if chosen >= 0 {
				selected.Slots[chosen] = voices.Slots[chosen]
			}
		}
		e.voices.Store(selected)
	}
	e.samplerTap.Store(&samplerAudioTap{ID: id, OutputID: output, Mode: mode})
	return nil
}
func (e *nativeEngine) endSamplerTap(id string) {
	if tap := e.samplerTap.Load(); tap != nil && tap.ID == id {
		e.samplerTap.CompareAndSwap(tap, nil)
	}
}
func (e *nativeEngine) samplerAudio(id string, cursor uint64) ([]byte, uint64, uint64, error) {
	tap := e.samplerTap.Load()
	if tap == nil || tap.ID != id {
		return nil, 0, 0, errors.New("Recording audio stream has stopped")
	}
	end := tap.write.Load()
	dropped := uint64(0)
	if cursor > end {
		cursor = end
	}
	if end-cursor > captureRingSize {
		dropped = end - cursor - captureRingSize
		cursor = end - captureRingSize
	}
	if end-cursor > 96000 {
		end = cursor + 96000
	}
	data := make([]byte, int(end-cursor)*4)
	for i := cursor; i < end; i++ {
		binary.LittleEndian.PutUint32(data[int(i-cursor)*4:], tap.data[i%captureRingSize].Load())
	}
	return data, end, dropped, nil
}

type samplerRecordingJob struct {
	ID, Name, File string
	Size           int64
	Started        time.Time
	file           *os.File
}
type samplerRecorder struct {
	mu    sync.Mutex
	store *appStore
	jobs  map[string]*samplerRecordingJob
}

func (r *samplerRecorder) begin(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.jobs) > 0 {
		return "", errors.New("A sampler recording is already running")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = recordingName()
	}
	if len(name) > 80 {
		return "", errors.New("Recording name must be up to 80 characters")
	}
	id := newID()
	dir := filepath.Join(r.store.root, "sampler", "recordings")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(filepath.Join(dir, id+".part"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	if r.jobs == nil {
		r.jobs = map[string]*samplerRecordingJob{}
	}
	r.jobs[id] = &samplerRecordingJob{ID: id, Name: name, File: id + ".part", Started: time.Now(), file: file}
	r.store.mu.Lock()
	err = r.store.write("sampler-recording-pending.json", VideoRecording{ID: id, Name: name, File: id + ".part", Created: time.Now().Format(time.RFC3339)})
	r.store.mu.Unlock()
	if err != nil {
		file.Close()
		os.Remove(filepath.Join(dir, id+".part"))
		delete(r.jobs, id)
		return "", err
	}
	return id, nil
}
func (r *samplerRecorder) append(id string, reader io.Reader) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[id]
	if job == nil {
		return errors.New("Recording session not found")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 16*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 16*1024*1024 || job.Size+int64(len(data)) > 500*1024*1024 {
		return errors.New("Recording reached its 500 MB limit")
	}
	n, err := job.file.Write(data)
	job.Size += int64(n)
	return err
}
func (r *samplerRecorder) finish(id string, seconds float64) (VideoRecording, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[id]
	if job == nil {
		return VideoRecording{}, errors.New("Recording session not found")
	}
	if seconds <= 0 || seconds > 901 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return VideoRecording{}, errors.New("Invalid recording duration (maximum 15 minutes)")
	}
	if job.Size == 0 {
		return VideoRecording{}, errors.New("No video was recorded")
	}
	if err := job.file.Sync(); err != nil {
		return VideoRecording{}, err
	}
	if err := job.file.Close(); err != nil {
		return VideoRecording{}, err
	}
	file := id + ".webm"
	dir := filepath.Join(r.store.root, "sampler", "recordings")
	if err := os.Rename(filepath.Join(dir, job.File), filepath.Join(dir, file)); err != nil {
		return VideoRecording{}, err
	}
	recording := VideoRecording{ID: id, Name: job.Name, File: file, Seconds: seconds, Bytes: job.Size, Created: job.Started.Format(time.RFC3339)}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	list, err := r.store.videoRecordingsLocked()
	if err != nil {
		return VideoRecording{}, err
	}
	list = append(list, recording)
	if err = r.store.write("sampler-recordings.json", list); err != nil {
		return VideoRecording{}, err
	}
	delete(r.jobs, id)
	r.store.write("sampler-recording-pending.json", nil)
	return recording, nil
}
func (r *samplerRecorder) abort(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if job := r.jobs[id]; job != nil {
		job.file.Close()
		os.Remove(filepath.Join(r.store.root, "sampler", "recordings", job.File))
		delete(r.jobs, id)
		r.store.mu.Lock()
		r.store.write("sampler-recording-pending.json", nil)
		r.store.mu.Unlock()
	}
}
func (r *samplerRecorder) close() {
	r.mu.Lock()
	var ids []string
	for id := range r.jobs {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.mu.Lock()
		job := r.jobs[id]
		r.mu.Unlock()
		if job != nil && job.Size > 0 {
			r.finish(id, math.Min(900, math.Max(.1, time.Since(job.Started).Seconds())))
		} else {
			r.abort(id)
		}
	}
}

func (r *samplerRecorder) recoverPending() error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	var pending *VideoRecording
	if err := r.store.read("sampler-recording-pending.json", &pending); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if pending == nil {
		return nil
	}
	if pending.ID == "" || pending.File != pending.ID+".part" || filepath.Base(pending.File) != pending.File {
		return errors.New("Invalid pending recording")
	}
	dir := filepath.Join(r.store.root, "sampler", "recordings")
	info, err := os.Stat(filepath.Join(dir, pending.File))
	if os.IsNotExist(err) {
		return r.store.write("sampler-recording-pending.json", nil)
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		os.Remove(filepath.Join(dir, pending.File))
		return r.store.write("sampler-recording-pending.json", nil)
	}
	file := pending.ID + ".webm"
	if err = os.Rename(filepath.Join(dir, pending.File), filepath.Join(dir, file)); err != nil {
		return err
	}
	pending.File = file
	pending.Bytes = info.Size()
	pending.Name += " (interrupted)"
	list, err := r.store.videoRecordingsLocked()
	if err != nil {
		return err
	}
	list = append(list, *pending)
	if err = r.store.write("sampler-recordings.json", list); err != nil {
		return err
	}
	return r.store.write("sampler-recording-pending.json", nil)
}
func (s *appStore) ExportRecording(id, path string) error {
	source, err := s.RecordingPath(id)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(path), ".quiet-recording-")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
