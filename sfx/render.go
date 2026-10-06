package sfx

import "math"

// Renderer is a stateful, block-streaming synthesizer. All state is
// carried between RenderBlock calls, so any partitioning of the
// total length into blocks yields exactly the same samples as one
// full Render. All arithmetic is deterministic float64 with no
// process-global state.
type Renderer struct {
	p        Params
	total    int
	pos      int
	oscPhase float64 // cycles, kept in [0,1) to avoid drift
	lfoPhase float64 // cycles, kept in [0,1)
	noise    *rng
	bx1, bx2 float64 // biquad input history
	by1, by2 float64 // biquad output history
}

// NewRenderer validates params and creates a streaming renderer.
func NewRenderer(p *Params) (*Renderer, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	cp := *p
	// Copy automation slices so later caller mutation cannot affect us.
	cp.GainAutomation = append([]Breakpoint(nil), p.GainAutomation...)
	cp.CutoffAutomation = append([]Breakpoint(nil), p.CutoffAutomation...)
	return &Renderer{
		p:     cp,
		total: int(math.Round(cp.DurationSec * float64(cp.SampleRate))),
		noise: newRNG(cp.Seed),
	}, nil
}

// TotalSamples returns the exact number of samples Render produces.
func (r *Renderer) TotalSamples() int { return r.total }

// RenderBlock fills buf with the next len(buf) samples. Rendering
// past the total length yields silence.
func (r *Renderer) RenderBlock(buf []float64) {
	sr := float64(r.p.SampleRate)
	for i := range buf {
		if r.pos >= r.total {
			buf[i] = 0
			continue
		}
		buf[i] = r.sample(sr)
		r.pos++
	}
}

// sample renders one sample at the current position and advances
// all per-sample state.
func (r *Renderer) sample(sr float64) float64 {
	p := &r.p
	t := float64(r.pos) / sr
	dur := p.DurationSec

	// 1. LFO (state advanced exactly once per sample).
	lfo := 0.0
	if p.LFO.Target != ModNone {
		lfo = waveform(p.LFO.Waveform, r.lfoPhase) * p.LFO.Depth
		r.lfoPhase = math.Mod(r.lfoPhase+p.LFO.FreqHz/sr, 1)
	}

	// 2. Effective parameters for this sample.
	freq := p.Oscillator.FreqHz
	gain := 1.0
	cutoff := p.Filter.CutoffHz
	if len(p.GainAutomation) > 0 {
		gain *= evalAutomation(p.GainAutomation, t)
	}
	if len(p.CutoffAutomation) > 0 {
		cutoff = evalAutomation(p.CutoffAutomation, t)
	}
	switch p.LFO.Target {
	case ModFreq:
		freq *= 1 + lfo
	case ModGain:
		gain *= 1 + lfo
	case ModCutoff:
		cutoff *= 1 + lfo
	}
	// Modulated values are clamped to the stable domain (documented
	// policy; static parameters are instead rejected by Validate).
	freq = clamp(freq, 0.01, sr/2)
	cutoff = clamp(cutoff, 1, sr/2*0.999)

	// 3. Sources.
	x := 0.0
	if p.Oscillator.Gain > 0 {
		x += p.Oscillator.Gain * waveform(p.Oscillator.Waveform, r.oscPhase)
		r.oscPhase = math.Mod(r.oscPhase+freq/sr, 1)
	}
	if p.Noise.Enabled {
		x += p.Noise.Gain * r.noise.bipolar()
	}

	// 4. Envelope and gain.
	x *= envelope(&p.Envelope, t, dur) * gain

	// 5. Filter (coefficients follow the modulated cutoff).
	if p.Filter.Type != FilterNone {
		x = r.filter(x, cutoff, sr)
	}

	// 6. Nonlinear drive: tanh is bounded, so the stage cannot
	// diverge; the sanitize guard removes any residual NaN/Inf.
	if p.Drive.Amount > 0 {
		d := p.Drive
		shaped := math.Tanh(d.Amount*x) / math.Tanh(d.Amount)
		x = (1-d.Mix)*x + d.Mix*shaped
	}
	return sanitize(x)
}

// waveform evaluates a periodic shape at phase (cycles, [0,1)).
func waveform(w Waveform, phase float64) float64 {
	switch w {
	case WaveSine:
		return math.Sin(2 * math.Pi * phase)
	case WaveSquare:
		if phase < 0.5 {
			return 1
		}
		return -1
	case WaveSaw:
		return 2*phase - 1
	case WaveTriangle:
		return 4*math.Abs(phase-0.5) - 1
	}
	return 0
}

// envelope evaluates the ADSR at time t for a sound of duration dur.
// It is a pure function of time, so it is continuous across blocks
// by construction.
func envelope(e *Envelope, t, dur float64) float64 {
	relStart := dur - e.Release
	if e.Release > 0 && t >= relStart {
		return e.Sustain * math.Max(0, (dur-t)/e.Release)
	}
	if e.Attack > 0 && t < e.Attack {
		return t / e.Attack
	}
	t -= e.Attack
	if e.Decay > 0 && t < e.Decay {
		return 1 - (1-e.Sustain)*(t/e.Decay)
	}
	return e.Sustain
}

// filter applies one biquad section (RBJ cookbook) with coefficients
// derived from the current cutoff and the configured resonance.
func (r *Renderer) filter(x, cutoff, sr float64) float64 {
	w0 := 2 * math.Pi * cutoff / sr
	sinw, cosw := math.Sin(w0), math.Cos(w0)
	alpha := sinw / (2 * r.p.Filter.Resonance)
	var b0, b1, b2 float64
	if r.p.Filter.Type == FilterLowPass {
		b0 = (1 - cosw) / 2
		b1 = 1 - cosw
		b2 = b0
	} else {
		b0 = (1 + cosw) / 2
		b1 = -(1 + cosw)
		b2 = b0
	}
	a0 := 1 + alpha
	a1 := -2 * cosw
	a2 := 1 - alpha
	inv := 1 / a0
	b0, b1, b2, a1, a2 = b0*inv, b1*inv, b2*inv, a1*inv, a2*inv

	y := b0*x + b1*r.bx1 + b2*r.bx2 - a1*r.by1 - a2*r.by2
	r.bx2, r.bx1 = r.bx1, x
	r.by2, r.by1 = r.by1, y
	return y
}

// evalAutomation linearly interpolates sorted breakpoints, holding
// the edge values outside the covered range.
func evalAutomation(bps []Breakpoint, t float64) float64 {
	if t <= bps[0].TimeSec {
		return bps[0].Value
	}
	last := bps[len(bps)-1]
	if t >= last.TimeSec {
		return last.Value
	}
	for i := 1; i < len(bps); i++ {
		if t < bps[i].TimeSec {
			dt := bps[i].TimeSec - bps[i-1].TimeSec
			if dt <= 0 {
				return bps[i].Value
			}
			f := (t - bps[i-1].TimeSec) / dt
			return bps[i-1].Value + f*(bps[i].Value-bps[i-1].Value)
		}
	}
	return last.Value
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// sanitize is the final safety net: non-finite samples are replaced
// by 0 and finite samples are clamped to [-4, 4] (the maximum gain a
// valid parameter set can produce before the bounded drive stage).
func sanitize(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return clamp(x, -maxGain, maxGain)
}

// Render renders the whole sound in one call.
func Render(p *Params) ([]float64, error) {
	r, err := NewRenderer(p)
	if err != nil {
		return nil, err
	}
	out := make([]float64, r.TotalSamples())
	r.RenderBlock(out)
	return out, nil
}
