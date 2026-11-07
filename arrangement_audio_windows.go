//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"sync/atomic"
)

type arrangementPCMStream struct {
	done     chan struct{}
	free     chan []float32
	ready    chan []float32
	current  []float32
	cursor   int
	consumed atomic.Int64
	total    int64
}

func newArrangementPCMStream(path string, at, frames int64) (*arrangementPCMStream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err = file.Seek(at*4, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	stream := &arrangementPCMStream{done: make(chan struct{}), free: make(chan []float32, 8), ready: make(chan []float32, 8), total: frames - at}
	for i := 0; i < 8; i++ {
		stream.free <- make([]float32, 4096)
	}
	// Prepare the first block before starting the audio clock.
	buffer := <-stream.free
	raw := make([]byte, len(buffer)*4)
	n, readErr := io.ReadFull(file, raw)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		file.Close()
		return nil, readErr
	}
	for i := 0; i < n/4; i++ {
		buffer[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	if n > 0 {
		stream.ready <- buffer[:n/4]
	} else {
		stream.free <- buffer
	}
	go func() {
		defer file.Close()
		remaining := frames - at - int64(n/4)
		for remaining > 0 {
			var block []float32
			select {
			case <-stream.done:
				return
			case block = <-stream.free:
			}
			count := min(int64(len(block)), remaining)
			size, e := io.ReadFull(file, raw[:count*4])
			if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
				return
			}
			if size == 0 {
				return
			}
			for i := 0; i < size/4; i++ {
				block[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
			}
			select {
			case <-stream.done:
				return
			case stream.ready <- block[:size/4]:
			}
			remaining -= int64(size / 4)
			if e != nil {
				return
			}
		}
	}()
	return stream, nil
}
func (s *arrangementPCMStream) sample() float32 {
	if s.consumed.Load() >= s.total {
		return 0
	}
	if s.cursor >= len(s.current) {
		if s.current != nil {
			select {
			case s.free <- s.current[:cap(s.current)]:
			default:
			}
			s.current = nil
		}
		select {
		case s.current = <-s.ready:
			s.cursor = 0
		default:
			return 0
		}
	}
	value := s.current[s.cursor]
	s.cursor++
	s.consumed.Add(1)
	return value
}

type arrangementTransport struct {
	streams      []*arrangementPCMStream
	start, total float64
	once         sync.Once
}

func (p *arrangementTransport) close() {
	p.once.Do(func() {
		for _, stream := range p.streams {
			close(stream.done)
		}
	})
}

type ArrangementPreviewStatus struct {
	Playing            bool
	Position, Duration float64
}

func (e *nativeEngine) StopArrangementPreview() error {
	if old := e.arrangement.Swap(nil); old != nil {
		old.close()
	}
	return nil
}
func (e *nativeEngine) ArrangementPreviewStatus() ArrangementPreviewStatus {
	transport := e.arrangement.Load()
	if transport == nil || len(transport.streams) == 0 {
		return ArrangementPreviewStatus{}
	}
	stream := transport.streams[0]
	position := transport.start + float64(stream.consumed.Load())/sampleRate
	return ArrangementPreviewStatus{Playing: position < transport.total-.00001, Position: min(position, transport.total), Duration: transport.total}
}
func (e *nativeEngine) playArrangementPreview(path string, start float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running.Load() || len(e.renders) == 0 {
		return errors.New("Resume listening and select an output before previewing")
	}
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	frames := stat.Size() / 4
	if math.IsNaN(start) || math.IsInf(start, 0) || start < 0 || start >= float64(frames)/sampleRate {
		return errors.New("Choose a position inside the arrangement")
	}
	transport := &arrangementTransport{start: start, total: float64(frames) / sampleRate}
	for range e.renders {
		stream, e := newArrangementPCMStream(path, int64(start*sampleRate), frames)
		if e != nil {
			transport.close()
			return e
		}
		transport.streams = append(transport.streams, stream)
	}
	if old := e.arrangement.Swap(transport); old != nil {
		old.close()
	}
	return nil
}
