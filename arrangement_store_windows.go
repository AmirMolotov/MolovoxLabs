//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ArrangementAsset struct {
	ID, Name, Kind, File, AudioFile, Poster string
	Seconds, VideoIn                        float64
	HasAudio                                bool
	Crop                                    *VideoCrop
	Waveform                                []float32
}
type ArrangementTrack struct {
	ID, Name string
	Muted    bool
}
type ArrangementClip struct {
	ID, AssetID, TrackID     string
	At, In, Duration, Volume float64
}
type ArrangementProject struct {
	Version int
	Name    string
	BPM     float64
	Snap    bool
	Tracks  []ArrangementTrack
	Clips   []ArrangementClip
}
type ArrangementLibrary struct {
	Project ArrangementProject
	Assets  []ArrangementAsset
}

func defaultArrangement() ArrangementProject {
	return ArrangementProject{Version: 1, Name: "My arrangement", BPM: 120, Snap: true, Tracks: []ArrangementTrack{{ID: "track-1", Name: "Track 1"}, {ID: "track-2", Name: "Track 2"}, {ID: "track-3", Name: "Track 3"}, {ID: "track-4", Name: "Track 4"}}, Clips: []ArrangementClip{}}
}
func (s *appStore) arrangementAssetsLocked() ([]ArrangementAsset, error) {
	list := []ArrangementAsset{}
	err := s.read("arrangement-assets.json", &list)
	if os.IsNotExist(err) {
		err = nil
	}
	return list, err
}
func (s *appStore) LoadArrangement() (ArrangementLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := ArrangementLibrary{Project: defaultArrangement()}
	var err error
	result.Assets, err = s.arrangementAssetsLocked()
	if err != nil {
		return result, err
	}
	err = s.read("arrangement.json", &result.Project)
	if os.IsNotExist(err) {
		err = nil
	}
	return result, err
}
func validateArrangement(project ArrangementProject, assets []ArrangementAsset) (float64, error) {
	if project.Version != 1 || strings.TrimSpace(project.Name) == "" || len(project.Name) > 120 || !(project.BPM >= 20 && project.BPM <= 300) {
		return 0, errors.New("Use a project name and a tempo from 20 to 300 BPM")
	}
	tracks := map[string]bool{}
	for _, track := range project.Tracks {
		if track.ID == "" || tracks[track.ID] || len(track.Name) > 120 {
			return 0, errors.New("Invalid arrangement track")
		}
		tracks[track.ID] = true
	}
	known := map[string]ArrangementAsset{}
	for _, asset := range assets {
		known[asset.ID] = asset
	}
	ids := map[string]bool{}
	duration := 0.0
	for _, clip := range project.Clips {
		asset, ok := known[clip.AssetID]
		if !ok || !tracks[clip.TrackID] || clip.ID == "" || ids[clip.ID] {
			return 0, errors.New("An arrangement clip is missing its media or track")
		}
		ids[clip.ID] = true
		for _, v := range []float64{clip.At, clip.In, clip.Duration, clip.Volume} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, errors.New("Invalid clip timing or level")
			}
		}
		if clip.At < 0 || clip.In < 0 || clip.Duration < .05 || clip.Volume < 0 || clip.Volume > 2 || clip.At+clip.Duration > 24*60*60 {
			return 0, errors.New("Use positive clip times within 24 hours and a volume from 0 to 200%")
		}
		if asset.Kind != "image" && clip.In+clip.Duration > asset.Seconds+.001 {
			return 0, errors.New("A trim extends past the end of its media")
		}
		duration = math.Max(duration, clip.At+clip.Duration)
	}
	return duration, nil
}
func (s *appStore) SaveArrangement(project ArrangementProject) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	assets, err := s.arrangementAssetsLocked()
	if err != nil {
		return err
	}
	if _, err = validateArrangement(project, assets); err != nil {
		return err
	}
	return s.write("arrangement.json", project)
}
func (s *appStore) arrangementPath(name string) string {
	return filepath.Join(s.root, "arrangement", "assets", name)
}
func (s *appStore) ArrangementAssetPath(id string, poster bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.arrangementAssetsLocked()
	if err != nil {
		return "", err
	}
	for _, asset := range list {
		if asset.ID != id {
			continue
		}
		name := asset.File
		if poster {
			name = asset.Poster
		}
		if name != "" && filepath.Base(name) == name {
			return s.arrangementPath(name), nil
		}
	}
	return "", errors.New("Arrangement media not found")
}
func copyArrangementFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(target), ".import-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(out.Name(), target)
}
func (s *appStore) appendArrangementAsset(asset ArrangementAsset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.arrangementAssetsLocked()
	if err != nil {
		return err
	}
	return s.write("arrangement-assets.json", append(list, asset))
}
func (s *appStore) arrangementPoster(asset *ArrangementAsset) {
	if asset.Kind == "audio" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	poster := asset.ID + ".jpg"
	args := []string{"-v", "error", "-nostdin", "-y"}
	if asset.Kind == "video" {
		args = append(args, "-ss", strconv.FormatFloat(asset.VideoIn, 'f', 6, 64))
	}
	args = append(args, "-i", s.arrangementPath(asset.File), "-frames:v", "1", "-vf", "scale=320:180:force_original_aspect_ratio=decrease", "-update", "1", s.arrangementPath(poster))
	cmd, err := mediaCommand(ctx, "ffmpeg", args...)
	if err == nil && cmd.Run() == nil {
		asset.Poster = poster
	}
}
func (s *appStore) ImportArrangementAsset(path, name string) (asset ArrangementAsset, err error) {
	stat, err := os.Stat(path)
	if err != nil {
		return asset, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxVideoBytes || stat.Size() == 0 {
		return asset, errors.New("Choose a media file up to 8 GB")
	}
	ext := strings.ToLower(filepath.Ext(path))
	asset = ArrangementAsset{ID: newID(), Name: name}
	if asset.Name == "" {
		asset.Name = filepath.Base(path)
	}
	asset.File = asset.ID + ext
	target := s.arrangementPath(asset.File)
	if err = copyArrangementFile(path, target); err != nil {
		return asset, err
	}
	defer func() {
		if err != nil {
			os.Remove(target)
			os.Remove(s.arrangementPath(asset.ID + ".jpg"))
		}
	}()
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".tif", ".tiff":
		asset.Kind = "image"
		asset.Seconds = 5
		// Normalize still images so all supported formats use a loopable input.
		normalized := s.arrangementPath(asset.ID + "-still.png")
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd, e := mediaCommand(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", target, "-frames:v", "1", "-update", "1", normalized)
		if e != nil {
			return asset, e
		}
		if e = cmd.Run(); e != nil {
			os.Remove(normalized)
			return asset, errors.New("This image could not be read")
		}
		os.Remove(target)
		asset.File = filepath.Base(normalized)
		target = normalized
	default:
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd, e := mediaCommand(ctx, "ffprobe", "-v", "error", "-show_format", "-show_streams", "-of", "json", target)
		if e != nil {
			return asset, e
		}
		data, e := cmd.Output()
		if e != nil {
			return asset, errors.New("This audio or video file could not be read")
		}
		var info struct {
			Format  struct{ Duration string }
			Streams []struct {
				Type     string `json:"codec_type"`
				Duration string
			}
		}
		if err = json.Unmarshal(data, &info); err != nil {
			return asset, err
		}
		asset.Seconds, _ = strconv.ParseFloat(info.Format.Duration, 64)
		asset.Kind = "audio"
		video := false
		for _, stream := range info.Streams {
			if stream.Type == "audio" {
				asset.HasAudio = true
			}
			if stream.Type == "video" {
				video = true
			}
			duration, _ := strconv.ParseFloat(stream.Duration, 64)
			asset.Seconds = math.Max(asset.Seconds, duration)
		}
		if video {
			asset.Kind = "video"
		}
		if !(asset.Seconds > 0 && asset.Seconds <= 24*60*60) || (!video && !asset.HasAudio) {
			return asset, errors.New("The media has no supported track or duration")
		}
	}
	s.arrangementPoster(&asset)
	err = s.appendArrangementAsset(asset)
	return asset, err
}
func (s *appStore) ArrangementFromPad(id string) (asset ArrangementAsset, err error) {
	s.mu.Lock()
	library, e := s.loadLibrary()
	s.mu.Unlock()
	if e != nil {
		return asset, e
	}
	var sound *SoundEntry
	for _, board := range library.Boards {
		for _, entry := range board.Sounds {
			if entry.ID == id {
				copy := entry
				sound = &copy
			}
		}
	}
	if sound == nil {
		return asset, errors.New("Sound or video pad not found")
	}
	audio, err := s.AudioClipPath(id)
	if err != nil {
		return asset, err
	}
	asset = ArrangementAsset{ID: newID(), Name: sound.Name, Kind: "audio", Seconds: sound.Seconds, HasAudio: true, Waveform: sound.Waveform}
	asset.File = asset.ID + filepath.Ext(audio)
	defer func() {
		if err != nil {
			os.Remove(s.arrangementPath(asset.File))
			os.Remove(s.arrangementPath(asset.ID + ".jpg"))
			if asset.AudioFile != "" {
				os.Remove(s.arrangementPath(asset.AudioFile))
			}
		}
	}()
	if sound.Video != nil {
		visual, e := s.SourcePath(sound.Video.SourceID)
		if e != nil {
			return asset, e
		}
		asset.Kind = "video"
		asset.VideoIn = sound.Video.In
		crop := sound.Video.Crop
		asset.Crop = &crop
		asset.AudioFile = asset.File
		asset.File = asset.ID + filepath.Ext(visual)
		if asset.File == asset.AudioFile {
			asset.File = asset.ID + "-video" + filepath.Ext(visual)
		}
		if err = copyArrangementFile(visual, s.arrangementPath(asset.File)); err != nil {
			return asset, err
		}
	}
	target := asset.File
	if asset.AudioFile != "" {
		target = asset.AudioFile
	}
	if err = copyArrangementFile(audio, s.arrangementPath(target)); err != nil {
		return asset, err
	}
	s.arrangementPoster(&asset)
	err = s.appendArrangementAsset(asset)
	return asset, err
}
func arrangementNumber(value float64) string { return fmt.Sprintf("%.6f", value) }
