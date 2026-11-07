//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type MediaInfo struct {
	ID, Name           string
	Seconds            float64
	HasAudio, HasVideo bool
	Width, Height      int
	Frames             int64
}

// Use the installed codec tools without shipping another application executable.
func mediaTool(name string) (string, error) {
	if path, err := exec.LookPath(name + ".exe"); err == nil {
		return path, nil
	}
	matches, _ := filepath.Glob(filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Packages", "Gyan.FFmpeg_*", "*", "bin", name+".exe"))
	if len(matches) > 0 {
		return matches[len(matches)-1], nil
	}
	return "", errors.New("FFmpeg is needed to open this media. Install FFmpeg and restart Molovox Labs")
}
func mediaCommand(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	tool, err := mediaTool(name)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd, nil
}
func (s *appStore) mediaPath(id, ext string) string {
	return filepath.Join(s.root, "sampler", "media", id+ext)
}

func (s *appStore) PrepareMedia(id string) (MediaInfo, error) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	raw, err := s.rawSourcePath(id)
	if err != nil {
		return MediaInfo{}, err
	}
	var info MediaInfo
	cached := s.mediaPath(id, ".json")
	if data, e := os.ReadFile(cached); e == nil && json.Unmarshal(data, &info) == nil && info.ID == id && info.Seconds > 0 {
		if !info.HasAudio {
			return info, nil
		}
		pcm, e1 := os.Stat(s.mediaPath(id, ".f32"))
		peaks, e2 := os.Stat(s.mediaPath(id, ".peaks"))
		if e1 == nil && e2 == nil && pcm.Size() == info.Frames*4 && peaks.Size() > 0 {
			return info, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	probe, err := mediaCommand(ctx, "ffprobe", "-v", "error", "-show_format", "-show_streams", "-of", "json", raw)
	if err != nil {
		return info, err
	}
	data, err := probe.Output()
	if err != nil {
		return info, errors.New("This media could not be read. It may be incomplete, encrypted or use an unsupported codec")
	}
	var description struct {
		Format struct {
			Duration string `json:"duration"`
		}
		Streams []struct {
			Type          string `json:"codec_type"`
			Codec         string `json:"codec_name"`
			PixelFormat   string `json:"pix_fmt"`
			Width, Height int
			Duration      string `json:"duration"`
		}
	}
	if err = json.Unmarshal(data, &description); err != nil {
		return info, err
	}
	info = MediaInfo{ID: id, Name: strings.TrimSuffix(filepath.Base(raw), filepath.Ext(raw))}
	info.Seconds, _ = strconv.ParseFloat(description.Format.Duration, 64)
	videoCodec := ""
	pixelFormat := ""
	for _, stream := range description.Streams {
		if stream.Type == "audio" {
			info.HasAudio = true
		}
		if stream.Type == "video" && stream.Width > 0 {
			info.HasVideo = true
			info.Width = stream.Width
			info.Height = stream.Height
			videoCodec = stream.Codec
			pixelFormat = stream.PixelFormat
		}
		duration, _ := strconv.ParseFloat(stream.Duration, 64)
		if duration > info.Seconds {
			info.Seconds = duration
		}
	}
	if !info.HasAudio && !info.HasVideo {
		return info, errors.New("No audio or video track found")
	}
	if info.Seconds <= 0 || math.IsInf(info.Seconds, 0) || math.IsNaN(info.Seconds) {
		return info, errors.New("Could not determine the media duration")
	}
	if err = os.MkdirAll(filepath.Dir(cached), 0700); err != nil {
		return info, err
	}
	if info.HasAudio {
		if err = s.decodeMediaPCM(ctx, id, raw); err != nil {
			return info, err
		}
		stat, e := os.Stat(s.mediaPath(id, ".f32"))
		if e != nil {
			return info, e
		}
		info.Frames = stat.Size() / 4
	} else {
		info.Frames = int64(math.Round(info.Seconds * sampleRate))
	}
	ext := strings.ToLower(filepath.Ext(raw))
	copyVideo := videoCodec == "h264" && (pixelFormat == "yuv420p" || pixelFormat == "yuvj420p")
	compatible := (ext == ".mp4" || ext == ".m4v") && copyVideo || ext == ".webm" && (videoCodec == "vp8" || videoCodec == "vp9") && pixelFormat == "yuv420p"
	if info.HasVideo && !compatible {
		temp := s.mediaPath(id, ".part.mp4")
		defer os.Remove(temp)
		args := []string{"-v", "error", "-nostdin", "-y", "-i", raw, "-map", "0:v:0", "-an"}
		if copyVideo {
			args = append(args, "-c:v", "copy")
		} else {
			args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-pix_fmt", "yuv420p")
		}
		args = append(args, "-movflags", "+faststart", temp)
		cmd, e := mediaCommand(ctx, "ffmpeg", args...)
		if e != nil {
			return info, e
		}
		if e = cmd.Run(); e != nil {
			return info, errors.New("Could not prepare this video's playback track")
		}
		if e = os.Rename(temp, s.mediaPath(id, ".mp4")); e != nil {
			return info, e
		}
	}
	data, _ = json.Marshal(info)
	temp := cached + ".tmp"
	if err = os.WriteFile(temp, data, 0600); err != nil {
		return info, err
	}
	if err = os.Rename(temp, cached); err != nil {
		return info, err
	}
	return info, nil
}
func (s *appStore) decodeMediaPCM(ctx context.Context, id, raw string) error {
	cmd, err := mediaCommand(ctx, "ffmpeg", "-v", "error", "-nostdin", "-i", raw, "-map", "0:a:0", "-vn", "-ar", "48000", "-ac", "1", "-f", "f32le", "-c:a", "pcm_f32le", "pipe:1")
	if err != nil {
		return err
	}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	pcmTemp, peaksTemp := s.mediaPath(id, ".part.f32"), s.mediaPath(id, ".part.peaks")
	defer os.Remove(pcmTemp)
	defer os.Remove(peaksTemp)
	pcm, err := os.Create(pcmTemp)
	if err != nil {
		return err
	}
	defer pcm.Close()
	peaks, err := os.Create(peaksTemp)
	if err != nil {
		return err
	}
	defer peaks.Close()
	peakWriter := bufio.NewWriterSize(peaks, 64*1024)
	if err = cmd.Start(); err != nil {
		return err
	}
	buffer := make([]byte, 64*1024)
	var peak float32
	count := 0
	var peakBytes [4]byte
	writePeak := func() error {
		binary.LittleEndian.PutUint32(peakBytes[:], math.Float32bits(peak))
		_, e := peakWriter.Write(peakBytes[:])
		peak = 0
		count = 0
		return e
	}
	for {
		n, e := io.ReadFull(pipe, buffer)
		if n > 0 {
			if n%4 != 0 {
				err = errors.New("Invalid decoded audio")
				break
			}
			if _, err = pcm.Write(buffer[:n]); err != nil {
				break
			}
			for at := 0; at < n; at += 4 {
				v := float32(math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buffer[at:])))))
				if v > peak && !math.IsInf(float64(v), 0) {
					peak = v
				}
				count++
				if count == 256 {
					if err = writePeak(); err != nil {
						break
					}
				}
			}
			if err != nil {
				break
			}
		}
		if e != nil {
			if e != io.EOF && e != io.ErrUnexpectedEOF {
				err = e
			}
			break
		}
	}
	if err != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if err != nil {
		return err
	}
	if waitErr != nil {
		return errors.New("Could not decode the audio track")
	}
	if count > 0 {
		if err = writePeak(); err != nil {
			return err
		}
	}
	if err = peakWriter.Flush(); err != nil {
		return err
	}
	if err = pcm.Close(); err != nil {
		return err
	}
	if err = peaks.Close(); err != nil {
		return err
	}
	if err = os.Rename(pcmTemp, s.mediaPath(id, ".f32")); err != nil {
		return err
	}
	return os.Rename(peaksTemp, s.mediaPath(id, ".peaks"))
}
func (s *appStore) mediaInfo(id string) (MediaInfo, error) {
	if _, err := s.rawSourcePath(id); err != nil {
		return MediaInfo{}, err
	}
	var info MediaInfo
	data, err := os.ReadFile(s.mediaPath(id, ".json"))
	if err != nil {
		return info, errors.New("Prepare this source before sampling")
	}
	err = json.Unmarshal(data, &info)
	return info, err
}
func (s *appStore) MediaWave(id string, start, end float64, width int) ([]float32, error) {
	info, err := s.mediaInfo(id)
	if err != nil {
		return nil, err
	}
	if width < 1 || width > 8192 || !validMediaRange(info, start, end, false) {
		return nil, errors.New("Invalid waveform range")
	}
	result := make([]float32, width)
	if !info.HasAudio {
		return result, nil
	}
	step := int64(1)
	ext := ".f32"
	if (end-start)*sampleRate/float64(width) >= 256 {
		step = 256
		ext = ".peaks"
	}
	f, err := os.Open(s.mediaPath(id, ext))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	first := int64(start*sampleRate) / step
	last := int64(math.Ceil(end * sampleRate / float64(step)))
	if last <= first {
		return result, nil
	}
	if _, err = f.Seek(first*4, io.SeekStart); err != nil {
		return nil, err
	}
	buffer := make([]byte, 64*1024)
	total := last - first
	var index int64
	for index < total {
		want := int64(len(buffer) / 4)
		if want > total-index {
			want = total - index
		}
		n, e := io.ReadFull(f, buffer[:want*4])
		for at := 0; at+4 <= n; at += 4 {
			v := float32(math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buffer[at:])))))
			column := int(index * int64(width) / total)
			if v > result[column] && !math.IsInf(float64(v), 0) {
				result[column] = v
			}
			index++
		}
		if e != nil {
			if e == io.EOF || e == io.ErrUnexpectedEOF {
				break
			}
			return nil, e
		}
	}
	return result, nil
}
func validMediaRange(info MediaInfo, start, end float64, clip bool) bool {
	return !math.IsNaN(start) && !math.IsNaN(end) && !math.IsInf(start, 0) && !math.IsInf(end, 0) && start >= 0 && end > start && end <= info.Seconds+.05 && (!clip || end-start >= .05-1e-7 && end-start <= 120+1e-7)
}
func (s *appStore) MediaWAV(id string, start, end float64, silent bool) ([]byte, error) {
	info, err := s.mediaInfo(id)
	if err != nil {
		return nil, err
	}
	if !validMediaRange(info, start, end, true) {
		return nil, errors.New("Select between 0.05 and 120 seconds within the source")
	}
	frames := int(math.Round((end - start) * sampleRate))
	data := make([]byte, 44+frames*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], sampleRate)
	binary.LittleEndian.PutUint32(data[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*2))
	if silent || !info.HasAudio {
		return data, nil
	}
	f, err := os.Open(s.mediaPath(id, ".f32"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw := make([]byte, frames*4)
	n, err := f.ReadAt(raw, int64(math.Round(start*sampleRate))*4)
	if err != nil && err != io.EOF {
		return nil, err
	}
	for i := 0; i < n/4; i++ {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:])))
		if math.IsNaN(v) {
			v = 0
		}
		v = math.Max(-1, math.Min(1, v))
		scale := 32767.
		if v < 0 {
			scale = 32768
		}
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(int16(math.Round(v*scale))))
	}
	return data, nil
}
func (s *appStore) MediaFrame(id string, at float64, width int) ([]byte, error) {
	info, err := s.mediaInfo(id)
	if err != nil {
		return nil, err
	}
	if !info.HasVideo || math.IsNaN(at) || math.IsInf(at, 0) || at < 0 || at > info.Seconds || width < 32 || width > 1920 {
		return nil, errors.New("Invalid frame")
	}
	raw, err := s.rawSourcePath(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd, err := mediaCommand(ctx, "ffmpeg", "-v", "error", "-nostdin", "-ss", fmt.Sprintf("%.6f", math.Min(at, math.Max(0, info.Seconds-.04))), "-i", raw, "-map", "0:v:0", "-frames:v", "1", "-vf", fmt.Sprintf("scale=%d:-2", width), "-q:v", "2", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
	if err != nil {
		return nil, err
	}
	data, err := cmd.Output()
	if err != nil || len(data) == 0 {
		return nil, errors.New("Could not decode this frame")
	}
	return data, nil
}

func (s *appStore) MediaThumbnail(v VideoSample) (string, error) {
	data, err := s.MediaFrame(v.SourceID, v.In, 640)
	if err != nil {
		return "", err
	}
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	b := src.Bounds()
	crop := v.Crop
	sw, sh := float64(b.Dx())*crop.Width, float64(b.Dy())*crop.Height
	scale := math.Min(240/sw, 135/sh)
	dw, dh := int(sw*scale), int(sh*scale)
	out := image.NewRGBA(image.Rect(0, 0, 240, 135))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx := int(float64(b.Dx())*crop.X + float64(x)/scale)
			sy := int(float64(b.Dy())*crop.Y + float64(y)/scale)
			out.Set((240-dw)/2+x, (135-dh)/2+y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, out, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}

func (s *appStore) ImportAudioMedia(path string) (MediaInfo, error) {
	source, err := s.ImportVideo(path)
	if err != nil {
		return MediaInfo{}, err
	}
	s.mu.Lock()
	list, err := s.videoSourcesLocked()
	if err == nil {
		for i := range list {
			if list[i].ID == source.ID {
				list[i].Kind = "audio"
			}
		}
		err = s.write("sampler-sources.json", list)
	}
	s.mu.Unlock()
	if err != nil {
		return MediaInfo{}, err
	}
	info, err := s.PrepareMedia(source.ID)
	info.Name = source.Name
	if err == nil && !info.HasAudio {
		err = errors.New("This file has no audio track")
	}
	return info, err
}
