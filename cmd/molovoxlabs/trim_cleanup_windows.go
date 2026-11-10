//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

// Release only app-owned audio-trimmer sources. Saved pads have independent WAVs.
func (s *appStore) ReleaseAudioTrimSource(id string) error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	sources, err := s.videoSourcesLocked()
	if err != nil {
		return err
	}
	at := -1
	for i, source := range sources {
		if source.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	source := sources[at]
	if source.Kind != "audio" {
		return errors.New("Only a temporary audio-trimmer source can be released")
	}
	if id == "" || filepath.Base(id) != id || filepath.Base(source.File) != source.File {
		return errors.New("Invalid temporary source path")
	}
	library, err := s.loadLibrary()
	if err != nil {
		return err
	}
	for _, b := range library.Boards {
		for _, sound := range b.Sounds {
			if sound.Video != nil && sound.Video.SourceID == id {
				return errors.New("This source is still used by a video pad")
			}
		}
	}
	// Remove derived caches first; the copied source remains recoverable if cleanup fails.
	for _, ext := range []string{".json", ".f32", ".peaks", ".mp4", ".part.mp4", ".part.f32", ".part.peaks"} {
		if err = os.Remove(s.mediaPath(id, ext)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err = os.Remove(filepath.Join(s.root, "sampler", "videos", source.File)); err != nil && !os.IsNotExist(err) {
		return err
	}
	sources = append(sources[:at], sources[at+1:]...)
	return s.write("sampler-sources.json", sources)
}
