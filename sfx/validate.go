package sfx

import "fmt"

// ValidationError 描述一个可定位的参数错误。
type ValidationError struct {
	Field  string // 参数路径，如 "osc.freq"
	Reason string // 失败原因
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid parameter %s: %s", e.Field, e.Reason)
}

func verr(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

// Validate 校验参数，返回所有可定位错误；全部合法时返回 nil。
// 校验只拒绝非法输入，绝不静默裁剪参数。
func Validate(p *Params) []error {
	var errs []error
	add := func(e *ValidationError) {
		if e != nil {
			errs = append(errs, e)
		}
	}

	if p.Duration <= 0 {
		add(verr("duration", "must be > 0, got %v", p.Duration))
	}
	if p.Duration > 600 {
		add(verr("duration", "must be <= 600s, got %v", p.Duration))
	}
	if p.SampleRate < 8000 || p.SampleRate > 192000 {
		add(verr("sampleRate", "must be in [8000, 192000], got %d", p.SampleRate))
	}
	if p.Gain < 0 || p.Gain > 1 {
		add(verr("gain", "must be in [0, 1], got %v", p.Gain))
	}

	// 振荡器
	switch p.Osc.Waveform {
	case WaveSine, WaveSquare, WaveSaw, WaveTriangle, WaveNone:
	default:
		add(verr("osc.waveform", "unknown waveform %q", p.Osc.Waveform))
	}
	oscActive := p.Osc.Waveform != WaveNone && p.Osc.Level > 0
	if p.Osc.Waveform != WaveNone {
		if p.Osc.Freq <= 0 || p.Osc.Freq > float64(p.SampleRate)/2 {
			add(verr("osc.freq", "must be in (0, %v], got %v", float64(p.SampleRate)/2, p.Osc.Freq))
		}
		if p.Osc.FreqEnd < 0 || p.Osc.FreqEnd > float64(p.SampleRate)/2 {
			add(verr("osc.freqEnd", "must be in [0, %v], got %v", float64(p.SampleRate)/2, p.Osc.FreqEnd))
		}
		if p.Osc.Level < 0 || p.Osc.Level > 1 {
			add(verr("osc.level", "must be in [0, 1], got %v", p.Osc.Level))
		}
		if p.Osc.Waveform == WaveSquare && (p.Osc.PulseWidth <= 0 || p.Osc.PulseWidth >= 1) {
			add(verr("osc.pulseWidth", "square wave requires 0 < pulseWidth < 1, got %v", p.Osc.PulseWidth))
		}
	} else if p.Osc.Level > 0 {
		add(verr("osc.level", "waveform is none but level %v > 0 (mutually exclusive)", p.Osc.Level))
	}

	// 噪声源
	switch p.Noise.Type {
	case NoiseNone, NoiseWhite, NoisePink:
	default:
		add(verr("noise.type", "unknown noise type %q", p.Noise.Type))
	}
	noiseActive := p.Noise.Type != NoiseNone && p.Noise.Level > 0
	if p.Noise.Type != NoiseNone {
		if p.Noise.Level <= 0 || p.Noise.Level > 1 {
			add(verr("noise.level", "must be in (0, 1], got %v", p.Noise.Level))
		}
	} else if p.Noise.Level > 0 {
		add(verr("noise.level", "noise type is none but level %v > 0 (mutually exclusive)", p.Noise.Level))
	}

	// 必需项：至少一个声源
	if !oscActive && !noiseActive {
		add(verr("osc/noise", "at least one active source is required (oscillator or noise)"))
	}

	// 包络
	if p.Envelope.Attack < 0 {
		add(verr("envelope.attack", "must be >= 0, got %v", p.Envelope.Attack))
	}
	if p.Envelope.Decay < 0 {
		add(verr("envelope.decay", "must be >= 0, got %v", p.Envelope.Decay))
	}
	if p.Envelope.Sustain < 0 || p.Envelope.Sustain > 1 {
		add(verr("envelope.sustain", "must be in [0, 1], got %v", p.Envelope.Sustain))
	}
	if p.Envelope.Release < 0 {
		add(verr("envelope.release", "must be >= 0, got %v", p.Envelope.Release))
	}
	if p.Envelope.Attack+p.Envelope.Decay+p.Envelope.Release <= 0 && p.Envelope.Sustain <= 0 {
		add(verr("envelope", "envelope is entirely zero; sound would be silent"))
	}

	// 滤波器
	switch p.Filter.Type {
	case FilterNone, FilterLowPass, FilterHighPass:
	default:
		add(verr("filter.type", "unknown filter type %q", p.Filter.Type))
	}
	if p.Filter.Type != FilterNone {
		nyq := float64(p.SampleRate) / 2
		if p.Filter.Cutoff <= 0 || p.Filter.Cutoff >= nyq {
			add(verr("filter.cutoff", "must be in (0, %v), got %v", nyq, p.Filter.Cutoff))
		}
		if p.Filter.CutoffEnd < 0 || p.Filter.CutoffEnd >= nyq {
			add(verr("filter.cutoffEnd", "must be in [0, %v), got %v", nyq, p.Filter.CutoffEnd))
		}
		if p.Filter.Resonance < 0.5 || p.Filter.Resonance > 30 {
			add(verr("filter.resonance", "must be in [0.5, 30], got %v", p.Filter.Resonance))
		}
	}

	// LFO
	if p.LFO.Rate < 0 || p.LFO.Rate > 40 {
		add(verr("lfo.rate", "must be in [0, 40] Hz, got %v", p.LFO.Rate))
	}
	if p.LFO.Rate > 0 {
		switch p.LFO.Waveform {
		case WaveSine, WaveTriangle, WaveSquare:
		default:
			add(verr("lfo.waveform", "lfo waveform must be sine, triangle or square, got %q", p.LFO.Waveform))
		}
		if p.LFO.Depth <= 0 || p.LFO.Depth > 1 {
			add(verr("lfo.depth", "must be in (0, 1], got %v", p.LFO.Depth))
		}
		if p.LFO.PitchDepth < 0 || p.LFO.PitchDepth > 24 {
			add(verr("lfo.pitchDepth", "must be in [0, 24] semitones, got %v", p.LFO.PitchDepth))
		}
		if p.LFO.FilterDepth < 0 || p.LFO.FilterDepth > float64(p.SampleRate)/2 {
			add(verr("lfo.filterDepth", "must be in [0, %v], got %v", float64(p.SampleRate)/2, p.LFO.FilterDepth))
		}
		if p.LFO.AmpDepth < 0 || p.LFO.AmpDepth > 1 {
			add(verr("lfo.ampDepth", "must be in [0, 1], got %v", p.LFO.AmpDepth))
		}
		if p.LFO.PitchDepth > 0 && !oscActive {
			add(verr("lfo.pitchDepth", "pitch modulation requires an active oscillator"))
		}
		if p.LFO.FilterDepth > 0 && p.Filter.Type == FilterNone {
			add(verr("lfo.filterDepth", "filter modulation requires an active filter"))
		}
	}

	// 非线性失真：极端参数在此识别并拒绝，保证渲染稳定
	if p.Drive.Drive != 0 {
		if p.Drive.Drive < 1 || p.Drive.Drive > 100 {
			add(verr("drive.drive", "must be in [1, 100], got %v", p.Drive.Drive))
		}
		if p.Drive.Mix < 0 || p.Drive.Mix > 1 {
			add(verr("drive.mix", "must be in [0, 1], got %v", p.Drive.Mix))
		}
	} else if p.Drive.Mix != 0 {
		add(verr("drive.mix", "drive is bypassed (0) but mix %v != 0 (mutually exclusive)", p.Drive.Mix))
	}

	return errs
}

// ValidateFormat 校验目标格式。
func ValidateFormat(f *Format) []error {
	var errs []error
	add := func(e *ValidationError) {
		if e != nil {
			errs = append(errs, e)
		}
	}
	if f.SampleRate < 8000 || f.SampleRate > 192000 {
		add(verr("format.sampleRate", "must be in [8000, 192000], got %d", f.SampleRate))
	}
	switch f.BitDepth {
	case 8, 16, 24, 32:
	default:
		add(verr("format.bitDepth", "must be 8, 16, 24 or 32, got %d", f.BitDepth))
	}
	if f.Channels != 1 && f.Channels != 2 {
		add(verr("format.channels", "must be 1 or 2, got %d", f.Channels))
	}
	if f.ClipMode != "" && f.ClipMode != "clip" {
		add(verr("format.clipMode", "unsupported clip mode %q (only \"clip\")", f.ClipMode))
	}
	return errs
}
