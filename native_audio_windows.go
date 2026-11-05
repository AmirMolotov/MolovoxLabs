//go:build windows

package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	audio "github.com/intermernet/gominiaudio"
)

const sampleRate = 48000
const ringSize = 8192
const scopeSize = 256

type AudioOptions struct {
	Patch         PatchBay
	patchProgram  *patchProgram
	BypassFilters bool
	InputGains    map[string]float32
	Filters       []MicFilter
	BoardVolume   float32
	MonitorMic    bool
	Volume        float32
	Enabled       bool
	Inverted      bool
	DelayMS       float32
	Filter        bool
}
type InputSelection struct {
	ID   string
	Gain float32
}
type StartRequest struct {
	Inputs  []InputSelection
	Outputs []string
	Options AudioOptions
}
type Endpoint struct {
	ID      string
	Name    string
	Default bool
}
type DeviceList struct {
	Inputs  []Endpoint
	Outputs []Endpoint
	Backend string
}
type NativeSnapshot struct {
	Playback  []PlaybackInfo
	Running   bool
	Mic       []float32
	Output    []float32
	InputDB   float64
	OutputDB  float64
	Underruns uint64
	Clipped   uint64
	Inputs    int
	Outputs   int
	Fault     bool
	Backend   string
}
type sampleRing struct {
	data  [ringSize]atomic.Uint32
	write atomic.Uint64
}

func (r *sampleRing) push(data []byte) {
	at := r.write.Load()
	for i := 0; i+4 <= len(data); i += 4 {
		r.data[at%ringSize].Store(binary.LittleEndian.Uint32(data[i:]))
		at++
	}
	r.write.Store(at)
}

const bridgeFrames = 960 // 20 ms; independent-device scheduling margin.
type ringReader struct {
	pos         uint64
	ready       bool
	initialized bool
}

func (r *ringReader) prepare(ring *sampleRing) {
	end := ring.write.Load()
	if !r.initialized {
		if end < bridgeFrames {
			return
		}
		r.pos = end - bridgeFrames
		r.initialized = true
	}
	if end-r.pos > bridgeFrames*2 {
		r.pos = end - bridgeFrames
	}
	if !r.ready && end-r.pos >= bridgeFrames {
		r.ready = true
	}
}
func (r *ringReader) next(ring *sampleRing) (float32, bool) {
	if !r.ready || r.pos >= ring.write.Load() {
		r.ready = false
		return 0, false
	}
	v := math.Float32frombits(ring.data[r.pos%ringSize].Load())
	r.pos++
	return v, true
}

type processor struct {
	gain                 float32
	delay                [2401]float32
	at                   int
	lastInput, high, low float32
}

func (p *processor) sample(x float32, o *AudioOptions) (float32, bool) {
	if o.Filter {
		p.high = .994777 * (p.high + x - p.lastInput)
		p.lastInput = x
		p.low += .099423 * (p.high - p.low)
		x = p.low
	}
	p.delay[p.at] = x
	n := int(o.DelayMS * 48)
	if n < 0 {
		n = 0
	}
	if n > 2400 {
		n = 2400
	}
	x = p.delay[(p.at-n+len(p.delay))%len(p.delay)]
	p.at = (p.at + 1) % len(p.delay)
	target := float32(0)
	if o.Enabled {
		target = o.Volume
	}
	if o.Inverted {
		target = -target
	}
	p.gain += (target - p.gain) * .004
	y := x * p.gain
	if math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
		return 0, true
	}
	if y > .999 {
		return .999, true
	}
	if y < -.999 {
		return -.999, true
	}
	return y, false
}

type captureEndpoint struct {
	ID     string
	ring   sampleRing
	gain   float32
	device *audio.Device
}
type renderEndpoint struct {
	ID        string
	index     int
	liveGains [8]float32
	published [8]atomic.Uint64
	filters   micFilterChain
	soundIDs  [8]uint64
	soundAt   [8]int
	device    *audio.Device
	readers   []ringReader
	dsp       processor
	toneSeen  uint64
	toneLeft  int
}
type nativeEngine struct {
	micRecordTap   atomic.Pointer[samplerAudioTap]
	soloClips      bool
	samplerTap     atomic.Pointer[samplerAudioTap]
	configMu       sync.Mutex
	vst            vstRack
	outputCount    atomic.Int32
	voices         atomic.Pointer[boardVoices]
	soundSequence  atomic.Uint64
	renderedFrames atomic.Uint64
	mu             sync.Mutex
	context        *audio.Context
	captures       []*captureEndpoint
	renders        []*renderEndpoint
	options        atomic.Pointer[AudioOptions]
	running        atomic.Bool
	fault          atomic.Bool
	closing        atomic.Bool
	micScope       [scopeSize]atomic.Uint32
	outScope       [scopeSize]atomic.Uint32
	inputDB        atomic.Uint64
	outputDB       atomic.Uint64
	underruns      atomic.Uint64
	clipped        atomic.Uint64
	tone           atomic.Uint64
}

func (e *nativeEngine) initContext() error {
	if e.context != nil {
		return nil
	}
	var err error
	e.context, err = audio.InitContext([]audio.Backend{audio.BackendWASAPI}, nil)
	return err
}
func endpointID(id audio.DeviceID) string { return hex.EncodeToString(id[:]) }
func (e *nativeEngine) Devices() (DeviceList, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := DeviceList{Backend: "Go / WASAPI", Inputs: []Endpoint{}, Outputs: []Endpoint{}}
	if err := e.initContext(); err != nil {
		return result, err
	}
	ins, err := e.context.Devices(audio.DeviceTypeCapture)
	if err != nil {
		return result, err
	}
	outs, err := e.context.Devices(audio.DeviceTypePlayback)
	if err != nil {
		return result, err
	}
	for _, d := range ins {
		result.Inputs = append(result.Inputs, Endpoint{endpointID(d.ID), d.Name, d.IsDefault})
	}
	for _, d := range outs {
		result.Outputs = append(result.Outputs, Endpoint{endpointID(d.ID), d.Name, d.IsDefault})
	}
	return result, nil
}
func parseDeviceID(s string) (*audio.DeviceID, error) {
	b, err := hex.DecodeString(s)
	var id audio.DeviceID
	if err != nil || len(b) != len(id) {
		return nil, errors.New("Invalid device selection; reconnect devices")
	}
	copy(id[:], b)
	return &id, nil
}
func (e *nativeEngine) Configure(o AudioOptions) error {
	e.configMu.Lock()
	defer e.configMu.Unlock()
	for _, g := range o.InputGains {
		if g < 0 || g > 1 || math.IsNaN(float64(g)) {
			return errors.New("Invalid microphone gain")
		}
	}
	if err := validateFilters(o.Filters); err != nil {
		return err
	}
	compiled, err := compilePatch(o.Patch, o.Filters)
	if err != nil {
		return err
	}
	o.patchProgram = compiled
	if o.BoardVolume < 0 || o.BoardVolume > 1 || math.IsNaN(float64(o.BoardVolume)) {
		return errors.New("Invalid soundboard volume")
	}
	if o.Volume < 0 || o.Volume > 1 || o.DelayMS < 0 || o.DelayMS > 50 || math.IsNaN(float64(o.Volume)) || math.IsNaN(float64(o.DelayMS)) {
		return errors.New("Invalid audio controls")
	}
	e.vst.configure(o.Filters, int(e.outputCount.Load()))
	e.options.Store(&o)
	return nil
}
func config(kind audio.DeviceType, id *audio.DeviceID) audio.DeviceConfig {
	c := audio.DeviceConfigInit(kind)
	c.SampleRate = sampleRate
	c.PeriodSizeInMilliseconds = 5
	c.Periods = 2
	c.Capture.DeviceID = id
	c.Capture.Format = audio.FormatF32
	c.Capture.Channels = 1
	c.Playback.DeviceID = id
	c.Playback.Format = audio.FormatF32
	c.Playback.Channels = 2
	return c
}
func (e *nativeEngine) Start(req StartRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running.Load() {
		return errors.New("Stop the current session first")
	}
	if len(req.Inputs) < 1 || len(req.Inputs) > 8 || len(req.Outputs) < 1 || len(req.Outputs) > 8 {
		return errors.New("Select 1–8 microphones and 1–8 outputs")
	}
	if err := e.initContext(); err != nil {
		return err
	}
	e.outputCount.Store(int32(len(req.Outputs)))
	if err := e.Configure(req.Options); err != nil {
		return err
	}
	e.closing.Store(false)
	e.fault.Store(false)
	e.underruns.Store(0)
	e.clipped.Store(0)
	seen := map[string]bool{}
	for _, selection := range req.Inputs {
		if seen[selection.ID] || selection.Gain < 0 || selection.Gain > 1 {
			e.stopLocked()
			return errors.New("Invalid or duplicate microphone")
		}
		seen[selection.ID] = true
		id, err := parseDeviceID(selection.ID)
		if err != nil {
			e.stopLocked()
			return err
		}
		capture := &captureEndpoint{gain: selection.Gain, ID: selection.ID}
		dev, err := audio.InitDevice(e.context, config(audio.DeviceTypeCapture, id), audio.DeviceCallbacks{
			Data: func(_ *audio.Device, _, input []byte, _ uint32) { capture.ring.push(input) },
			Stop: func(_ *audio.Device) {
				if !e.closing.Load() {
					e.fault.Store(true)
				}
			},
		})
		if err != nil {
			e.stopLocked()
			return fmt.Errorf("Open microphone: %w", err)
		}
		capture.device = dev
		e.captures = append(e.captures, capture)
	}
	seen = map[string]bool{}
	for index, selection := range req.Outputs {
		if seen[selection] {
			e.stopLocked()
			return errors.New("Duplicate output device")
		}
		seen[selection] = true
		id, err := parseDeviceID(selection)
		if err != nil {
			e.stopLocked()
			return err
		}
		render := &renderEndpoint{ID: selection, index: index, readers: make([]ringReader, len(e.captures)), toneSeen: e.tone.Load()}
		primary := index == 0
		dev, err := audio.InitDevice(e.context, config(audio.DeviceTypePlayback, id), audio.DeviceCallbacks{
			Data: func(_ *audio.Device, output, _ []byte, frames uint32) { e.render(render, output, int(frames), primary) },
			Stop: func(_ *audio.Device) {
				if !e.closing.Load() {
					e.fault.Store(true)
				}
			},
		})
		if err != nil {
			e.stopLocked()
			return fmt.Errorf("Open output: %w", err)
		}
		render.device = dev
		e.renders = append(e.renders, render)
	}
	for _, c := range e.captures {
		if err := c.device.Start(); err != nil {
			e.stopLocked()
			return fmt.Errorf("Start microphone: %w", err)
		}
	}
	for _, r := range e.renders {
		if err := r.device.Start(); err != nil {
			e.stopLocked()
			return fmt.Errorf("Start output: %w", err)
		}
	}
	e.running.Store(true)
	return nil
}
func (e *nativeEngine) render(r *renderEndpoint, output []byte, frames int, primary bool) {
	o := e.options.Load()
	if o == nil {
		clear(output)
		return
	}
	for i, c := range e.captures {
		r.readers[i].prepare(&c.ring)
		r.liveGains[i] = c.gain
		if live, ok := o.InputGains[c.ID]; ok {
			r.liveGains[i] = live
		}
	}
	rack := e.vst.current.Load()
	r.filters.vstVoice = r.index
	for i := range r.filters.vstSlots {
		r.filters.vstSlots[i] = nil
	}
	if rack != nil {
		for i, f := range o.Filters {
			if f.Kind == "vst3" {
				r.filters.vstSlots[i] = rack.units[f.ID]
			}
		}
	}
	sequence := e.tone.Load()
	if r.toneSeen != sequence {
		r.toneSeen = sequence
		r.toneLeft = 24000
	}
	tap := e.samplerTap.Load()
	var micTap *samplerAudioTap
	if primary {
		micTap = e.micRecordTap.Load()
	}
	var micAt uint64
	if micTap != nil {
		micAt = micTap.write.Load()
	}
	sounds := e.voices.Load()
	if tap != nil && tap.OutputID != r.ID {
		tap = nil
	}
	tapAt := uint64(0)
	if tap != nil {
		tapAt = tap.write.Load()
	}
	var inputSum, outputSum float64
	missing := false
	for frame := 0; frame < frames; frame++ {
		x := float32(0)
		for i, c := range e.captures {
			v, ok := r.readers[i].next(&c.ring)
			if !ok {
				missing = true
			}
			x += v * r.liveGains[i]
		}
		x /= float32(len(e.captures))
		if micTap != nil {
			micTap.data[(micAt+uint64(frame))%captureRingSize].Store(math.Float32bits(x))
		}
		var clips [8]float32
		boardMix := r.boardSamples(sounds, &clips)
		var y float32
		clipped := false
		if o.patchProgram != nil {
			y = o.patchProgram.sample(r, x, sounds, &clips, o)
		} else {
			processed := x
			if !o.BypassFilters {
				processed = r.filters.sample(x, o.Filters)
			}
			y, clipped = r.dsp.sample(processed, o)
			if o.MonitorMic && !o.Enabled {
				y += processed * o.Volume
			}
			y += boardMix * o.BoardVolume * o.Volume
		}
		if y > .999 {
			y = .999
			clipped = true
		}
		if y < -.999 {
			y = -.999
			clipped = true
		}
		if clipped {
			e.clipped.Add(1)
		}
		if r.toneLeft > 0 {
			t := 24000 - r.toneLeft
			fade := math.Min(1, math.Min(float64(t)/480, float64(r.toneLeft)/480))
			y += float32(.02 * fade * math.Sin(2*math.Pi*440*float64(t)/sampleRate))
			r.toneLeft--
			y = float32(math.Max(-.999, math.Min(.999, float64(y))))
		}
		inputSum += float64(x * x)
		if tap != nil {
			tap.data[(tapAt+uint64(frame))%captureRingSize].Store(math.Float32bits(y))
		}
		outputSum += float64(y * y)
		at := frame * 8
		if at+8 <= len(output) {
			bits := math.Float32bits(y)
			binary.LittleEndian.PutUint32(output[at:], bits)
			binary.LittleEndian.PutUint32(output[at+4:], bits)
		}
		if primary && frame < scopeSize {
			e.micScope[frame].Store(math.Float32bits(x))
			e.outScope[frame].Store(math.Float32bits(y))
		}
	}
	if tap != nil {
		tap.write.Store(tapAt + uint64(frames))
	}
	if micTap != nil {
		micTap.write.Store(micAt + uint64(frames))
	}
	if primary {
		e.renderedFrames.Add(uint64(frames))
		if missing {
			e.underruns.Add(1)
		}
		if frames > 0 {
			e.inputDB.Store(math.Float64bits(10 * math.Log10(math.Max(inputSum/float64(frames), 1e-12))))
			e.outputDB.Store(math.Float64bits(10 * math.Log10(math.Max(outputSum/float64(frames), 1e-12))))
		}
	}
}
func (e *nativeEngine) stopLocked() {
	e.micRecordTap.Store(nil)
	e.samplerTap.Store(nil)
	e.closing.Store(true)
	e.voices.Store(nil)
	e.running.Store(false)
	// Stop playback before releasing any capture rings.
	for _, r := range e.renders {
		r.device.Uninit()
	}
	for _, c := range e.captures {
		c.device.Uninit()
	}
	e.renders = nil
	e.vst.collectRetired()
	e.captures = nil
	for i := 0; i < scopeSize; i++ {
		e.micScope[i].Store(0)
		e.outScope[i].Store(0)
	}
}
func (e *nativeEngine) Stop() error { e.mu.Lock(); defer e.mu.Unlock(); e.stopLocked(); return nil }
func (e *nativeEngine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
	e.vst.close()
	if e.context != nil {
		e.context.Uninit()
		e.context = nil
	}
}
func (e *nativeEngine) Snapshot() NativeSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := NativeSnapshot{Running: e.running.Load(), Fault: e.fault.Load(), Inputs: len(e.captures), Outputs: len(e.renders), Backend: "Native Go · WASAPI · 48 kHz", Mic: make([]float32, scopeSize), Output: make([]float32, scopeSize), InputDB: -120, OutputDB: -120, Underruns: e.underruns.Load(), Clipped: e.clipped.Load()}
	if s.Running {
		s.InputDB = math.Float64frombits(e.inputDB.Load())
		s.OutputDB = math.Float64frombits(e.outputDB.Load())
	}
	if voices := e.voices.Load(); voices != nil && len(e.renders) > 0 {
		for i, v := range voices.Slots {
			if v.Sequence == 0 {
				continue
			}
			published := e.renders[0].published[i].Load()
			position := uint64(0)
			if published>>32 == v.Sequence {
				position = published & 0xffffffff
			}
			if v.Loop && len(v.Samples) > 0 {
				position %= uint64(len(v.Samples))
			}
			if position < uint64(len(v.Samples)) {
				duration := float64(len(v.Samples)) / sampleRate
				seconds := float64(position) / sampleRate
				s.Playback = append(s.Playback, PlaybackInfo{ID: v.ID, Sequence: v.Sequence, Position: seconds, Duration: duration, Remaining: duration - seconds, Progress: seconds / duration})
			}
		}
	}
	for i := 0; i < scopeSize; i++ {
		s.Mic[i] = math.Float32frombits(e.micScope[i].Load())
		s.Output[i] = math.Float32frombits(e.outScope[i].Load())
	}
	return s
}
func (e *nativeEngine) TestTone() error {
	if !e.running.Load() {
		return errors.New("Start listening first")
	}
	e.tone.Add(1)
	return nil
}
func nativeSelfTest() map[string]interface{} {
	p := processor{gain: -1}
	o := AudioOptions{Volume: 1, Enabled: true, Inverted: true}
	maxError := float64(0)
	for i := 0; i < 4096; i++ {
		x := float32(.3 * math.Sin(float64(i)*.13))
		y, _ := p.sample(x, &o)
		maxError = math.Max(maxError, math.Abs(float64(y+x)))
	}
	return map[string]interface{}{"passed": maxError < 1e-6, "maxError": maxError, "samples": 4096, "engine": "native Go DSP"}
}
func nativeSilentSmoke() map[string]interface{} {
	ctx, err := audio.InitContext([]audio.Backend{audio.BackendWASAPI}, nil)
	if err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	defer ctx.Uninit()
	inputs, _ := ctx.Devices(audio.DeviceTypeCapture)
	outputs, err := ctx.Devices(audio.DeviceTypePlayback)
	if err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	var callbacks atomic.Uint64
	d, err := audio.InitDevice(ctx, config(audio.DeviceTypePlayback, nil), audio.DeviceCallbacks{Data: func(_ *audio.Device, out, _ []byte, _ uint32) { clear(out); callbacks.Add(1) }})
	if err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	defer d.Uninit()
	if err = d.Start(); err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	time.Sleep(time.Second)
	return map[string]interface{}{"passed": callbacks.Load() > 0, "callbacks": callbacks.Load(), "inputs": len(inputs), "outputs": len(outputs), "backend": "WASAPI", "test": "silent playback only; no microphone opened"}
}

// nativeMutedCaptureSmoke opens the default input and output for one second.
// Samples stay in bounded RAM buffers and output remains muted.
func nativeMutedCaptureSmoke() map[string]interface{} {
	e := &nativeEngine{}
	defer e.Close()
	devices, err := e.Devices()
	if err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	if len(devices.Inputs) == 0 || len(devices.Outputs) == 0 {
		return map[string]interface{}{"passed": false, "error": "No capture or playback endpoint"}
	}
	input, output := devices.Inputs[0], devices.Outputs[0]
	for _, d := range devices.Inputs {
		if d.Default {
			input = d
			break
		}
	}
	for _, d := range devices.Outputs {
		if d.Default {
			output = d
			break
		}
	}
	err = e.Start(StartRequest{Inputs: []InputSelection{{ID: input.ID, Gain: 1}}, Outputs: []string{output.ID}, Options: AudioOptions{Volume: 1, Inverted: true, Enabled: false}})
	if err != nil {
		return map[string]interface{}{"passed": false, "error": err.Error()}
	}
	time.Sleep(time.Second)
	snapshot := e.Snapshot()
	captured := e.captures[0].ring.write.Load()
	rendered := e.renderedFrames.Load()
	return map[string]interface{}{"passed": captured > 0 && rendered > 0 && !snapshot.Fault, "captureFrames": captured, "renderFrames": rendered, "outputDBFS": snapshot.OutputDB, "underruns": snapshot.Underruns, "test": "native mic capture and playback, output muted; no recording saved"}
}

type boardVoice struct {
	Loop     bool
	BoardID  string
	Sequence uint64
	ID       string
	Samples  []float32
	Started  time.Time
}
type boardVoices struct{ Slots [8]boardVoice }

func (r *renderEndpoint) boardSample(sounds *boardVoices) float32 {
	return r.boardSamples(sounds, nil)
}
func (r *renderEndpoint) boardSamples(sounds *boardVoices, clips *[8]float32) float32 {
	if sounds == nil {
		return 0
	}
	var mixed float32
	for i, v := range sounds.Slots {
		if v.Sequence == 0 {
			continue
		}
		if r.soundIDs[i] != v.Sequence {
			r.soundIDs[i] = v.Sequence
			r.soundAt[i] = 0
		}
		at := r.soundAt[i]
		if v.Loop && len(v.Samples) > 0 && at >= len(v.Samples) {
			at = 0
			r.soundAt[i] = 0
		}
		if at >= len(v.Samples) {
			continue
		}
		fade := float32(math.Min(1, math.Min(float64(at)/240, float64(len(v.Samples)-at)/240)))
		value := v.Samples[at] * fade
		mixed += value
		if clips != nil {
			clips[i] = value
		}
		r.soundAt[i]++
		r.published[i].Store((v.Sequence << 32) | uint64(r.soundAt[i]))
	}
	return mixed
}
func (e *nativeEngine) PlaySound(id string, samples []float32, boardIDs ...string) error {
	return e.playSound(id, samples, false, boardIDs...)
}
func (e *nativeEngine) PlayPreview(id string, samples []float32, board string, loop bool) error {
	return e.playSound(id, samples, loop, board)
}
func (e *nativeEngine) playSound(id string, samples []float32, loop bool, boardIDs ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running.Load() {
		return errors.New("Resume listening before playing sounds")
	}
	next := &boardVoices{}
	if old := e.voices.Load(); old != nil {
		solo := e.soloClips
		if tap := e.samplerTap.Load(); tap != nil {
			solo = tap.Mode == "single"
		}
		if !solo {
			*next = *old
		}
	}
	for i, v := range next.Slots {
		if v.Sequence == 0 || !v.Loop && time.Since(v.Started).Seconds() > float64(len(v.Samples))/sampleRate+.1 {
			boardID := ""
			if len(boardIDs) > 0 {
				boardID = boardIDs[0]
			}
			next.Slots[i] = boardVoice{Loop: loop, BoardID: boardID, Sequence: e.soundSequence.Add(1), ID: id, Samples: samples, Started: time.Now()}
			e.voices.Store(next)
			return nil
		}
	}
	return errors.New("Eight sounds are already playing; stop sounds before adding another")
}
func (e *nativeEngine) StopSounds() error { e.voices.Store(nil); return nil }

type PlaybackInfo struct {
	ID                                      string
	Sequence                                uint64
	Position, Duration, Remaining, Progress float64
}

func (e *nativeEngine) StopSound(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	next := &boardVoices{}
	if old := e.voices.Load(); old != nil {
		*next = *old
	}
	for i, v := range next.Slots {
		if v.ID == id {
			next.Slots[i] = boardVoice{}
		}
	}
	e.voices.Store(next)
	return nil
}
func soundWaveform(samples []float32) []float32 {
	const bins = 40
	result := make([]float32, bins)
	peak := float32(.001)
	for i, v := range samples {
		v = float32(math.Abs(float64(v)))
		bin := i * bins / len(samples)
		if v > result[bin] {
			result[bin] = v
		}
		if v > peak {
			peak = v
		}
	}
	for i := range result {
		result[i] /= peak
	}
	return result
}
