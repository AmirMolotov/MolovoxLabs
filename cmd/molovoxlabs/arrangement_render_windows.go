//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ArrangementRenderStatus struct {
	ID, State, Kind, Error, Path string
	Progress, Seconds            float64
}
type arrangementRenderer struct {
	mu        sync.Mutex
	store     *appStore
	status    ArrangementRenderStatus
	cancel    context.CancelFunc
	previewID string
}

func (r *arrangementRenderer) snapshot() ArrangementRenderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}
func (r *arrangementRenderer) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}
func (r *arrangementRenderer) begin(project ArrangementProject, kind, target string) (ArrangementRenderStatus, error) {
	if kind != "preview" && kind != "video" && kind != "audio" {
		return ArrangementRenderStatus{}, errors.New("Choose preview, video or audio export")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status.State == "rendering" {
		return r.status, errors.New("Wait for the current render or cancel it first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	r.cancel = cancel
	r.status = ArrangementRenderStatus{ID: newID(), State: "rendering", Kind: kind}
	status := r.status
	go r.run(ctx, cancel, project, status, target)
	return status, nil
}
func (r *arrangementRenderer) previewPath(id, extension string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == "" || id != r.previewID {
		return "", errors.New("Render the arrangement preview first")
	}
	return filepath.Join(r.store.root, "arrangement", "renders", id+extension), nil
}
func (r *arrangementRenderer) run(ctx context.Context, cancel context.CancelFunc, project ArrangementProject, status ArrangementRenderStatus, target string) {
	defer cancel()
	r.store.mu.Lock()
	assets, err := r.store.arrangementAssetsLocked()
	r.store.mu.Unlock()
	duration := 0.0
	if err == nil {
		duration, err = validateArrangement(project, assets)
	}
	if err == nil && duration == 0 {
		err = errors.New("Add clips before rendering the arrangement")
	}
	if err == nil {
		if status.Kind == "preview" {
			dir := filepath.Join(r.store.root, "arrangement", "renders")
			err = os.MkdirAll(dir, 0700)
			target = filepath.Join(dir, status.ID+".mp4")
		}
	}
	if err == nil {
		err = r.render(ctx, project, assets, status, target, duration)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Seconds = duration
	if err != nil {
		r.status.State = "error"
		r.status.Error = err.Error()
		if ctx.Err() != nil {
			r.status.State = "cancelled"
			r.status.Error = "Render cancelled."
		}
		return
	}
	r.status.State = "ready"
	r.status.Progress = 1
	r.status.Path = target
	if status.Kind == "preview" {
		old := r.previewID
		r.previewID = status.ID
		if old != "" {
			os.Remove(filepath.Join(r.store.root, "arrangement", "renders", old+".mp4"))
			os.Remove(filepath.Join(r.store.root, "arrangement", "renders", old+".f32"))
		}
	}
}
func (r *arrangementRenderer) render(ctx context.Context, project ArrangementProject, assets []ArrangementAsset, status ArrangementRenderStatus, target string, duration float64) error {
	known := map[string]ArrangementAsset{}
	for _, asset := range assets {
		known[asset.ID] = asset
	}
	order := map[string]int{}
	muted := map[string]bool{}
	for i, track := range project.Tracks {
		order[track.ID] = i
		muted[track.ID] = track.Muted
	}
	clips := append([]ArrangementClip(nil), project.Clips...)
	// Lower playlist rows form the background; upper rows are foreground layers.
	sort.SliceStable(clips, func(i, j int) bool { return order[clips[i].TrackID] > order[clips[j].TrackID] })
	width, height := "1280", "720"
	if status.Kind == "preview" {
		width, height = "640", "360"
	}
	filters := []string{"anullsrc=r=48000:cl=mono,atrim=duration=" + arrangementNumber(duration) + "[silence]"}
	if status.Kind != "audio" {
		filters = append(filters, "color=c=0x101916:s="+width+"x"+height+":r=30:d="+arrangementNumber(duration)+"[canvas0]")
	}
	args := []string{"-v", "error", "-nostdin", "-y", "-filter_complex_threads", "1"}
	inputs, visuals := 0, 0
	audioLabels := []string{"[silence]"}
	for _, clip := range clips {
		if muted[clip.TrackID] {
			continue
		}
		asset := known[clip.AssetID]
		input := inputs
		inputIn := clip.In
		if asset.Kind == "video" {
			inputIn += asset.VideoIn
		}
		if asset.Kind == "image" {
			args = append(args, "-loop", "1", "-framerate", "30")
		} else {
			args = append(args, "-ss", arrangementNumber(inputIn))
		}
		args = append(args, "-t", arrangementNumber(clip.Duration), "-i", r.store.arrangementPath(asset.File))
		inputs++
		if asset.Kind != "audio" && status.Kind != "audio" {
			visuals++
			source := fmt.Sprintf("[%d:v:0]", input)
			if asset.Crop != nil {
				c := asset.Crop
				source += fmt.Sprintf("crop=iw*%s:ih*%s:iw*%s:ih*%s,", arrangementNumber(c.Width), arrangementNumber(c.Height), arrangementNumber(c.X), arrangementNumber(c.Y))
			}
			source += "trim=duration=" + arrangementNumber(clip.Duration) + ",setpts=PTS-STARTPTS+" + arrangementNumber(clip.At) + "/TB,scale=" + width + ":" + height + ":force_original_aspect_ratio=decrease,pad=" + width + ":" + height + ":(ow-iw)/2:(oh-ih)/2,setsar=1"
			filters = append(filters, fmt.Sprintf("%s[visual%d]", source, visuals))
			filters = append(filters, fmt.Sprintf("[canvas%d][visual%d]overlay=eof_action=pass:repeatlast=0:enable='gte(t,%s)*lt(t,%s)'[canvas%d]", visuals-1, visuals, arrangementNumber(clip.At), arrangementNumber(clip.At+clip.Duration), visuals))
		}
		if asset.HasAudio {
			audioInput := input
			if asset.AudioFile != "" {
				audioInput = inputs
				args = append(args, "-ss", arrangementNumber(clip.In), "-t", arrangementNumber(clip.Duration), "-i", r.store.arrangementPath(asset.AudioFile))
				inputs++
			}
			label := fmt.Sprintf("sound%d", len(audioLabels))
			filters = append(filters, fmt.Sprintf("[%d:a:0]atrim=duration=%s,asetpts=PTS-STARTPTS,aresample=48000,aformat=sample_fmts=fltp:channel_layouts=mono,volume=%s,adelay=%d:all=1[%s]", audioInput, arrangementNumber(clip.Duration), arrangementNumber(clip.Volume), int64(clip.At*1000+.5), label))
			audioLabels = append(audioLabels, "["+label+"]")
		}
	}
	filters = append(filters, strings.Join(audioLabels, "")+fmt.Sprintf("amix=inputs=%d:normalize=0:duration=longest,alimiter=limit=0.98:level=0:latency=1,atrim=duration=%s[mix]", len(audioLabels), arrangementNumber(duration)))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	graph, err := os.CreateTemp(filepath.Dir(target), ".arrangement-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(graph.Name())
	_, err = graph.WriteString(strings.Join(filters, ";"))
	closeErr := graph.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".arrangement-*"+filepath.Ext(target))
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	temp.Close()
	defer os.Remove(tempPath)
	args = append(args, "-/filter_complex", graph.Name())
	if status.Kind == "audio" {
		// Audio-only exports use the same timing and mix as the video render.
		args = append(args, "-map", "[mix]", "-vn", "-c:a", "pcm_s16le")
	} else {
		args = append(args, "-map", fmt.Sprintf("[canvas%d]", visuals), "-map", "[mix]", "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart")
	}
	args = append(args, "-t", arrangementNumber(duration), "-progress", "pipe:1", "-nostats", tempPath)
	cmd, err := mediaCommand(ctx, "ffmpeg", args...)
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && key == "out_time_us" {
			microseconds, _ := strconv.ParseFloat(value, 64)
			r.mu.Lock()
			r.status.Progress = min(.98, max(0, microseconds/1e6/duration))
			r.mu.Unlock()
		}
	}
	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("Could not render the arrangement: %s", strings.TrimSpace(stderr.String()))
	}
	if status.Kind == "preview" {
		pcm := filepath.Join(filepath.Dir(target), status.ID+".f32")
		command, e := mediaCommand(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", tempPath, "-t", arrangementNumber(duration), "-vn", "-ac", "1", "-ar", "48000", "-f", "f32le", pcm)
		if e != nil {
			return e
		}
		if e = command.Run(); e != nil {
			os.Remove(pcm)
			return errors.New("Could not prepare native arrangement audio")
		}
	}
	if err = os.Rename(tempPath, target); err != nil {
		if status.Kind == "preview" {
			os.Remove(filepath.Join(filepath.Dir(target), status.ID+".f32"))
		}
	}
	return err
}
