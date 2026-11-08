//go:build windows

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Stream dropped files to disk; large tracks use the same native decoder as the picker.
func (h *samplerHTTP) importTrimUpload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Query().Get("name"))
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" || !strings.Contains(" .wav .mp3 .flac .ogg .opus .aac .m4a .aiff .aif .wma .mp4 .mkv .webm .mov .avi .m4v .mpeg .mpg .ts .m2ts .wmv .flv ", " "+ext+" ") {
		http.Error(w, "Choose an audio or video file", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(h.store.root, 0700); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	dir, err := os.MkdirTemp(h.store.root, ".trim-upload-")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, err = io.Copy(file, http.MaxBytesReader(w, r.Body, maxVideoBytes))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		http.Error(w, "The media upload could not be saved (maximum 8 GB)", http.StatusBadRequest)
		return
	}
	source, err := h.store.ImportAudioMedia(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(source)
}
