//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type samplerHTTP struct {
	prefix           string
	store            *appStore
	recorder         *samplerRecorder
	engine           *nativeEngine
	onLibraryChanged func() error
}

func (h *samplerHTTP) handler(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, h.prefix) {
			fallback.ServeHTTP(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, h.prefix)
		if !strings.HasPrefix(path, "sampler/") {
			fallback.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Different-origin requests are not allowed", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodGet && (strings.HasPrefix(path, "sampler/source/") || strings.HasPrefix(path, "sampler/recording/")) {
			id := filepath.Base(path)
			var file string
			var err error
			if strings.HasPrefix(path, "sampler/source/") {
				file, err = h.store.SourcePath(id)
			} else {
				file, err = h.store.RecordingPath(id)
			}
			if err != nil {
				http.NotFound(w, r)
				return
			}
			f, err := os.Open(file)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			kind := mime.TypeByExtension(filepath.Ext(file))
			if kind == "" {
				kind = "video/mp4"
			}
			w.Header().Set("Content-Type", kind)
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "sampler/clip/") {
			file, err := h.store.AudioClipPath(strings.TrimPrefix(path, "sampler/clip/"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, file)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "sampler/media-wave/") {
			start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
			end, _ := strconv.ParseFloat(r.URL.Query().Get("end"), 64)
			width, _ := strconv.Atoi(r.URL.Query().Get("width"))
			peaks, err := h.store.MediaWave(strings.TrimPrefix(path, "sampler/media-wave/"), start, end, width)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(peaks)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "sampler/media-wav/") {
			start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
			end, _ := strconv.ParseFloat(r.URL.Query().Get("end"), 64)
			data, err := h.store.MediaWAV(strings.TrimPrefix(path, "sampler/media-wav/"), start, end, r.URL.Query().Get("silent") == "true")
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			w.Write(data)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "sampler/audio/") {
			cursor, _ := strconv.ParseUint(r.URL.Query().Get("cursor"), 10, 64)
			data, end, dropped, err := h.engine.samplerAudio(strings.TrimPrefix(path, "sampler/audio/"), cursor)
			if err != nil {
				http.Error(w, err.Error(), http.StatusGone)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Audio-Cursor", strconv.FormatUint(end, 10))
			w.Header().Set("X-Audio-Dropped", strconv.FormatUint(dropped, 10))
			w.Write(data)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "sampler/mic-audio/") {
			h.writeMicAudio(w, r, strings.TrimPrefix(path, "sampler/mic-audio/"), false)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "Same-origin request required", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(path, "sampler/mic-finish/") {
			h.writeMicAudio(w, r, strings.TrimPrefix(path, "sampler/mic-finish/"), true)
			return
		}
		if path == "sampler/bite" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 12*1024*1024))
			if err != nil {
				http.Error(w, "Sound bite is too large", 413)
				return
			}
			name, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Bite-Name"))
			if err != nil {
				http.Error(w, "Invalid name", 400)
				return
			}
			library, err := h.store.CreateAudioBite(r.Header.Get("X-Bite-Board"), string(name), body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if h.onLibraryChanged != nil {
				h.onLibraryChanged()
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(library)
			return
		}
		if path == "sampler/create" {
			var request struct {
				CreateVideoSample
				Silent bool
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 160*1024)).Decode(&request); err != nil {
				http.Error(w, "Invalid video sample", 400)
				return
			}
			if err := validateVideoSample(request.Video); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			wav, err := h.store.MediaWAV(request.Video.SourceID, request.Video.In, request.Video.Out, request.Silent)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if thumbnail, e := h.store.MediaThumbnail(request.Video); e == nil {
				request.Video.Thumbnail = thumbnail
			}
			library, err := h.store.CreateVideoSample(request.CreateVideoSample, wav)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if h.onLibraryChanged != nil {
				h.onLibraryChanged()
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(library)
			return
		}
		if path == "sampler/sample" {
			var metaBytes, body []byte
			var err error
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				r.Body = http.MaxBytesReader(w, r.Body, 13*1024*1024)
				if err = r.ParseMultipartForm(1024 * 1024); err != nil {
					http.Error(w, "Could not read video sample", 400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				metaBytes = []byte(r.FormValue("metadata"))
				file, _, openErr := r.FormFile("audio")
				if openErr != nil {
					http.Error(w, "Missing sample audio", 400)
					return
				}
				defer file.Close()
				body, err = io.ReadAll(io.LimitReader(file, 12*1024*1024+1))
			} else {
				metaBytes, err = base64.StdEncoding.DecodeString(r.Header.Get("X-QuietLab-Sample"))
				if err == nil {
					body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 12*1024*1024))
				}
			}
			if err != nil || len(metaBytes) > 128*1024 {
				http.Error(w, "Invalid sample metadata", 400)
				return
			}
			var meta CreateVideoSample
			if json.Unmarshal(metaBytes, &meta) != nil {
				http.Error(w, "Invalid sample metadata", 400)
				return
			}
			if len(body) > 12*1024*1024 {
				http.Error(w, "Audio sample is too large", 413)
				return
			}
			library, err := h.store.CreateVideoSample(meta, body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if h.onLibraryChanged != nil {
				h.onLibraryChanged()
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(library)
			return
		}
		if path == "sampler/preview" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 12*1024*1024))
			if err != nil {
				http.Error(w, "Preview is too large", 413)
				return
			}
			samples, err := decodeSound(body, ".wav")
			if err == nil {
				id := "@video-preview"
				if r.Header.Get("X-Preview-ID") == "@audio-preview" {
					id = "@audio-preview"
				}
				err = h.engine.PlayPreview(id, samples, r.Header.Get("X-Sampler-Board"), r.Header.Get("X-Preview-Loop") == "true")
			}
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(path, "sampler/chunks/") {
			id := strings.TrimPrefix(path, "sampler/chunks/")
			if err := h.recorder.append(id, http.MaxBytesReader(w, r.Body, 16*1024*1024)); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, fmt.Sprintf("Unknown sampler operation"), 404)
	})
}

func (h *samplerHTTP) writeMicAudio(w http.ResponseWriter, r *http.Request, id string, finish bool) {
	cursor, _ := strconv.ParseUint(r.URL.Query().Get("cursor"), 10, 64)
	data, end, dropped, err := h.engine.micRecordingAudio(id, cursor, finish)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Audio-Cursor", strconv.FormatUint(end, 10))
	w.Header().Set("X-Audio-Dropped", strconv.FormatUint(dropped, 10))
	w.Write(data)
}
