package sfx

import "fmt"

// Waveform identifies a periodic oscillator shape.
type Waveform int

const (
	WaveSine Waveform = iota
	WaveSquare
	WaveSaw
	WaveTriangle
)

// FilterType identifies the biquad filter mode.
type FilterType int

const (
	FilterNone FilterType = iota
	FilterLowPass
	FilterHighPass
)

// ModTarget identifies which parameter an LFO drives.
type ModTarget int

const (
	ModNone ModTarget = iota
	ModFreq
	ModGain
	ModCutoff
)

// Oscillator describes the periodic source.
type Oscillator struct {
	Waveform Waveform
	FreqHz   float64
	Gain     float64
}

// Noise describes the noise source.
type Noise struct {
	Enabled bool
	Gain    float64
}

// Envelope is a classic ADSR in seconds; Sustain is a level in [0,1].
type Envelope struct {
	Attack  float64
	Decay   float64
	Sustain float64
	Release float64
}

// Filter describes a biquad filter stage.
type Filter struct {
	Type      FilterType
	CutoffHz  float64
	Resonance float64 // Q
}

// LFO describes a low-frequency modulator.
type LFO struct {
	Target   ModTarget
	Waveform Waveform
	FreqHz   float64
	Depth    float64
}

// Drive describes a nonlinear waveshaping stage.
type Drive struct {
	Amount float64 // >= 0; 0 disables the stage
	Mix    float64 // [0,1]
}

// Breakpoint is a single automation point; values are linearly
// interpolated between points and held constant outside the range.
type Breakpoint struct {
	TimeSec float64
	Value   float64
}

// Params is the full parameter description of one sound effect.
type Params struct {
	Seed        int64
	SampleRate  int
	DurationSec float64

	Oscillator Oscillator
	Noise      Noise
	Envelope   Envelope
	Filter     Filter
	LFO        LFO
	Drive      Drive

	// Optional parameter automation over time.
	GainAutomation   []Breakpoint
	CutoffAutomation []Breakpoint
}

const (
	minSampleRate = 8000
	maxSampleRate = 192000
	maxDuration   = 3600.0
	maxGain       = 4.0
	maxLFOFreq    = 100.0
	minQ          = 0.5
	maxQ          = 100.0
	maxDrive      = 100.0
)

func validWaveform(w Waveform) bool { return w >= WaveSine && w <= WaveTriangle }

// Validate checks the parameters and returns a *ValidationError
// describing the first problem found, or nil if valid. Invalid
// parameters are never silently clamped.
func (p *Params) Validate() error {
	fail := func(field, format string, args ...interface{}) error {
		return &ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)}
	}

	if p.SampleRate < minSampleRate || p.SampleRate > maxSampleRate {
		return fail("SampleRate", "must be in [%d, %d], got %d", minSampleRate, maxSampleRate, p.SampleRate)
	}
	if !(p.DurationSec > 0) || p.DurationSec > maxDuration {
		return fail("DurationSec", "must be in (0, %v], got %v", maxDuration, p.DurationSec)
	}
	nyquist := float64(p.SampleRate) / 2

	// Sources: at least one audible source is required.
	if p.Oscillator.Gain == 0 && !p.Noise.Enabled {
		return fail("Oscillator.Gain", "at least one source is required: set Oscillator.Gain > 0 or enable Noise")
	}
	if !validWaveform(p.Oscillator.Waveform) {
		return fail("Oscillator.Waveform", "unknown waveform %d", p.Oscillator.Waveform)
	}
	if p.Oscillator.Gain < 0 || p.Oscillator.Gain > maxGain {
		return fail("Oscillator.Gain", "must be in [0, %v], got %v", maxGain, p.Oscillator.Gain)
	}
	if p.Oscillator.Gain > 0 && (p.Oscillator.FreqHz <= 0 || p.Oscillator.FreqHz > nyquist) {
		return fail("Oscillator.FreqHz", "must be in (0, %v], got %v", nyquist, p.Oscillator.FreqHz)
	}

	// Noise: Enabled and Gain are mutually dependent.
	if !p.Noise.Enabled && p.Noise.Gain != 0 {
		return fail("Noise.Gain", "must be 0 when Noise.Enabled is false, got %v", p.Noise.Gain)
	}
	if p.Noise.Enabled && (p.Noise.Gain <= 0 || p.Noise.Gain > maxGain) {
		return fail("Noise.Gain", "must be in (0, %v] when Noise.Enabled is true, got %v", maxGain, p.Noise.Gain)
	}

	// Envelope.
	if p.Envelope.Attack < 0 {
		return fail("Envelope.Attack", "must be >= 0, got %v", p.Envelope.Attack)
	}
	if p.Envelope.Decay < 0 {
		return fail("Envelope.Decay", "must be >= 0, got %v", p.Envelope.Decay)
	}
	if p.Envelope.Release < 0 {
		return fail("Envelope.Release", "must be >= 0, got %v", p.Envelope.Release)
	}
	if p.Envelope.Sustain < 0 || p.Envelope.Sustain > 1 {
		return fail("Envelope.Sustain", "must be in [0, 1], got %v", p.Envelope.Sustain)
	}

	// Filter: Type None requires the other fields to be unset.
	if p.Filter.Type == FilterNone {
		if p.Filter.CutoffHz != 0 || p.Filter.Resonance != 0 {
			return fail("Filter.CutoffHz", "must be 0 when Filter.Type is FilterNone")
		}
	} else if p.Filter.Type == FilterLowPass || p.Filter.Type == FilterHighPass {
		if p.Filter.CutoffHz <= 0 || p.Filter.CutoffHz >= nyquist {
			return fail("Filter.CutoffHz", "must be in (0, %v), got %v", nyquist, p.Filter.CutoffHz)
		}
		if p.Filter.Resonance < minQ || p.Filter.Resonance > maxQ {
			return fail("Filter.Resonance", "must be in [%v, %v], got %v", minQ, maxQ, p.Filter.Resonance)
		}
	} else {
		return fail("Filter.Type", "unknown filter type %d", p.Filter.Type)
	}

	// LFO: ModNone requires the rest to be unset.
	if p.LFO.Target == ModNone {
		if p.LFO.FreqHz != 0 || p.LFO.Depth != 0 {
			return fail("LFO.FreqHz", "must be 0 when LFO.Target is ModNone")
		}
	} else if p.LFO.Target >= ModFreq && p.LFO.Target <= ModCutoff {
		if !validWaveform(p.LFO.Waveform) {
			return fail("LFO.Waveform", "unknown waveform %d", p.LFO.Waveform)
		}
		if p.LFO.FreqHz <= 0 || p.LFO.FreqHz > maxLFOFreq {
			return fail("LFO.FreqHz", "must be in (0, %v], got %v", maxLFOFreq, p.LFO.FreqHz)
		}
		if p.LFO.Depth <= 0 || p.LFO.Depth > 1 {
			return fail("LFO.Depth", "must be in (0, 1], got %v", p.LFO.Depth)
		}
		if p.LFO.Target == ModCutoff && p.Filter.Type == FilterNone {
			return fail("LFO.Target", "ModCutoff requires an active Filter")
		}
	} else {
		return fail("LFO.Target", "unknown modulation target %d", p.LFO.Target)
	}

	// Drive.
	if p.Drive.Amount < 0 || p.Drive.Amount > maxDrive {
		return fail("Drive.Amount", "must be in [0, %v], got %v", maxDrive, p.Drive.Amount)
	}
	if p.Drive.Amount == 0 && p.Drive.Mix != 0 {
		return fail("Drive.Mix", "must be 0 when Drive.Amount is 0, got %v", p.Drive.Mix)
	}
	if p.Drive.Amount > 0 && (p.Drive.Mix <= 0 || p.Drive.Mix > 1) {
		return fail("Drive.Mix", "must be in (0, 1] when Drive.Amount > 0, got %v", p.Drive.Mix)
	}

	// Automation breakpoints must be sorted, in range, and in-domain.
	if err := validateAutomation("GainAutomation", p.GainAutomation, p.DurationSec, 0, maxGain); err != nil {
		return err
	}
	if err := validateAutomation("CutoffAutomation", p.CutoffAutomation, p.DurationSec, 1, nyquist); err != nil {
		return err
	}
	if len(p.CutoffAutomation) > 0 && p.Filter.Type == FilterNone {
		return fail("CutoffAutomation", "requires an active Filter")
	}
	return nil
}

func validateAutomation(field string, bps []Breakpoint, dur, minV, maxV float64) error {
	prev := -1.0
	for i, bp := range bps {
		if bp.TimeSec < 0 || bp.TimeSec > dur {
			return &ValidationError{Field: field, Reason: fmt.Sprintf("point %d time %v out of [0, %v]", i, bp.TimeSec, dur)}
		}
		if bp.TimeSec < prev {
			return &ValidationError{Field: field, Reason: fmt.Sprintf("point %d time %v is before previous point %v", i, bp.TimeSec, prev)}
		}
		if bp.Value < minV || bp.Value > maxV {
			return &ValidationError{Field: field, Reason: fmt.Sprintf("point %d value %v out of [%v, %v]", i, bp.Value, minV, maxV)}
		}
		prev = bp.TimeSec
	}
	return nil
}
