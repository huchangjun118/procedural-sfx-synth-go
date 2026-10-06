package sfx

import (
	"errors"
	"math"
	"testing"
)

// validParams returns a rich parameter set exercising every stage.
func validParams() *Params {
	return &Params{
		Seed:        42,
		SampleRate:  44100,
		DurationSec: 0.5,
		Oscillator:  Oscillator{Waveform: WaveSaw, FreqHz: 220, Gain: 0.8},
		Noise:       Noise{Enabled: true, Gain: 0.2},
		Envelope:    Envelope{Attack: 0.01, Decay: 0.1, Sustain: 0.7, Release: 0.1},
		Filter:      Filter{Type: FilterLowPass, CutoffHz: 3000, Resonance: 2},
		LFO:         LFO{Target: ModCutoff, Waveform: WaveSine, FreqHz: 5, Depth: 0.5},
		Drive:       Drive{Amount: 3, Mix: 0.5},
		GainAutomation: []Breakpoint{
			{TimeSec: 0, Value: 1},
			{TimeSec: 0.25, Value: 0.5},
			{TimeSec: 0.5, Value: 1},
		},
	}
}

func identical(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func TestDeterministic(t *testing.T) {
	a, err := Render(validParams())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(validParams())
	if err != nil {
		t.Fatal(err)
	}
	if !identical(a, b) {
		t.Fatal("same params and seed produced different output")
	}
	p := validParams()
	p.Seed = 43
	c, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	if identical(a, c) {
		t.Fatal("different seeds produced identical output")
	}
}

func TestBlockContinuity(t *testing.T) {
	p := validParams()
	full, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range []int{1, 7, 1000, SegmentSamples - 1, SegmentSamples, SegmentSamples + 1, 8192} {
		r, err := NewRenderer(p)
		if err != nil {
			t.Fatal(err)
		}
		var got []float64
		buf := make([]float64, block)
		for len(got) < len(full) {
			n := block
			if len(full)-len(got) < n {
				n = len(full) - len(got)
			}
			r.RenderBlock(buf[:n])
			got = append(got, buf[:n]...)
		}
		if !identical(full, got) {
			t.Fatalf("block size %d: output differs from one-shot render", block)
		}
	}
}

func TestValidation(t *testing.T) {
	cases := []struct {
		field  string
		mutate func(*Params)
	}{
		{"SampleRate", func(p *Params) { p.SampleRate = 100 }},
		{"DurationSec", func(p *Params) { p.DurationSec = 0 }},
		{"Oscillator.FreqHz", func(p *Params) { p.Oscillator.FreqHz = 99999 }},
		{"Oscillator.Gain", func(p *Params) { p.Oscillator.Gain = -1 }},
		{"Noise.Gain", func(p *Params) { p.Noise.Enabled = false }},
		{"Noise.Gain", func(p *Params) { p.Noise.Gain = 0 }},
		{"Envelope.Sustain", func(p *Params) { p.Envelope.Sustain = 1.5 }},
		{"Filter.CutoffHz", func(p *Params) { p.Filter.Type = FilterNone }},
		{"Filter.CutoffHz", func(p *Params) { p.Filter.CutoffHz = 22050 }},
		{"Filter.Resonance", func(p *Params) { p.Filter.Resonance = 0.1 }},
		{"LFO.Target", func(p *Params) { p.Filter.Type = FilterNone; p.Filter.CutoffHz = 0; p.Filter.Resonance = 0 }},
		{"LFO.FreqHz", func(p *Params) { p.LFO.Target = ModNone }},
		{"Drive.Mix", func(p *Params) { p.Drive.Amount = 0 }},
		{"GainAutomation", func(p *Params) { p.GainAutomation[1].Value = 99 }},
		{"GainAutomation", func(p *Params) { p.GainAutomation[0].TimeSec = 0.4 }},
		{"Oscillator.Gain", func(p *Params) { p.Oscillator.Gain = 0; p.Noise.Enabled = false; p.Noise.Gain = 0 }},
	}
	for _, tc := range cases {
		p := validParams()
		tc.mutate(p)
		err := p.Validate()
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected ValidationError for %s, got %v", tc.field, err)
		}
		if ve.Field != tc.field {
			t.Fatalf("expected field %s, got %s", tc.field, ve.Field)
		}
	}
	if err := validParams().Validate(); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
}

func TestResampleBoundaries(t *testing.T) {
	src := make([]float64, 1000)
	for i := range src {
		src[i] = math.Sin(2 * math.Pi * float64(i) / 50)
	}
	// Non-integer ratio: length must preserve duration exactly.
	up, err := Resample(src, 44100, 48000)
	if err != nil {
		t.Fatal(err)
	}
	want := int(math.Round(1000.0 * 48000 / 44100))
	if len(up) != want {
		t.Fatalf("upsample length = %d, want %d", len(up), want)
	}
	down, err := Resample(src, 44100, 22049)
	if err != nil {
		t.Fatal(err)
	}
	want = int(math.Round(1000.0 * 22049 / 44100))
	if len(down) != want {
		t.Fatalf("downsample length = %d, want %d", len(down), want)
	}
	// Edge samples: first equals source first, last equals source last.
	if up[0] != src[0] {
		t.Fatal("first sample not preserved")
	}
	if up[len(up)-1] != src[len(src)-1] {
		t.Fatal("last sample not preserved")
	}
	// Identity and errors.
	id, err := Resample(src, 44100, 44100)
	if err != nil || !identical(id, src) {
		t.Fatal("same-rate resample not identical")
	}
	if _, err := Resample(src, 0, 48000); err == nil {
		t.Fatal("expected error for zero rate")
	}
	if _, err := Resample(nil, 44100, 48000); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestQuantizeAndChannels(t *testing.T) {
	src := []float64{-2, -1, -0.5, 0, 0.5, 1, 2, math.NaN(), math.Inf(1)}
	q, err := Quantize(src, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range q {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("sample %d not finite", i)
		}
		if v < -1 || v > 1 {
			t.Fatalf("sample %d = %v out of [-1,1]", i, v)
		}
	}
	if q[0] != -1 || q[5] != 1 {
		t.Fatalf("clipping policy wrong: %v, %v", q[0], q[5])
	}
	if q[3] != 0 {
		t.Fatal("zero not preserved")
	}
	if _, err := Quantize(src, 1); err == nil {
		t.Fatal("expected error for bits=1")
	}

	mono := []float64{0.1, -0.2, 0.3}
	st, err := ConvertChannels(mono, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 6 || st[0] != 0.1 || st[1] != 0.1 {
		t.Fatal("mono->stereo wrong")
	}
	back, err := ConvertChannels(st, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !identical(back, mono) {
		t.Fatal("stereo->mono roundtrip wrong")
	}
	if _, err := ConvertChannels(mono, 1, 3); err == nil {
		t.Fatal("expected error for 3 channels")
	}
	if _, err := ConvertChannels(mono, 2, 1); err == nil {
		t.Fatal("expected error for odd stereo length")
	}
}

func TestNonlinearStability(t *testing.T) {
	p := validParams()
	p.DurationSec = 5
	p.Oscillator.Gain = 4
	p.Noise.Gain = 4
	p.Drive = Drive{Amount: 100, Mix: 1}
	p.Filter.Resonance = 100
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	tail := 0.0
	for i, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("sample %d not finite", i)
		}
		if i >= len(out)-44100 {
			tail = math.Max(tail, math.Abs(v))
		}
	}
	if tail > maxGain {
		t.Fatalf("output diverged: tail max %v", tail)
	}
}

func TestConcurrentBatchConsistency(t *testing.T) {
	tasks := []Task{
		{ID: "a", Params: *validParams()},
		{ID: "b", Params: *validParams()},
		{ID: "bad", Params: Params{SampleRate: 1}},
		{ID: "c", Params: *validParams()},
	}
	p2 := validParams()
	p2.Seed = 7
	p2.LFO.Target = ModFreq
	tasks[1].Params = *p2
	p3 := validParams()
	p3.Oscillator.Waveform = WaveSquare
	tasks[3].Params = *p3

	base := BakeBatch(tasks, 1)
	for _, workers := range []int{2, 3, 8} {
		got := BakeBatch(tasks, workers)
		if len(got) != len(base) {
			t.Fatal("result count mismatch")
		}
		for i := range base {
			if base[i].ID != got[i].ID {
				t.Fatal("result order changed")
			}
			if (base[i].Err == nil) != (got[i].Err == nil) {
				t.Fatalf("task %s error presence changed with workers=%d", base[i].ID, workers)
			}
			if !identical(base[i].Samples, got[i].Samples) {
				t.Fatalf("task %s samples differ with workers=%d", base[i].ID, workers)
			}
		}
	}
	// Failure isolation and traceability.
	if base[2].Err == nil {
		t.Fatal("invalid task should have failed")
	}
	var be *BakeError
	if !errors.As(base[2].Err, &be) || be.TaskID != "bad" {
		t.Fatalf("error not traceable to task: %v", base[2].Err)
	}
	for _, i := range []int{0, 1, 3} {
		if base[i].Err != nil || len(base[i].Samples) == 0 {
			t.Fatalf("task %s wrongly affected by sibling failure", base[i].ID)
		}
	}
}

func TestCacheReuse(t *testing.T) {
	c := NewCache()
	p := validParams()
	full, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Bake(p)
	if err != nil {
		t.Fatal(err)
	}
	if !identical(first, full) {
		t.Fatal("cached bake differs from full render")
	}
	h, m := c.Stats()
	if h != 0 || m == 0 {
		t.Fatalf("first bake: hits=%d misses=%d", h, m)
	}

	// Unchanged params: everything reused, byte-identical.
	second, err := c.Bake(p)
	if err != nil {
		t.Fatal(err)
	}
	if !identical(second, full) {
		t.Fatal("cache reuse not byte-identical")
	}
	h2, m2 := c.Stats()
	if m2 != m || h2-h != m {
		t.Fatalf("second bake should reuse all: hits=%d->%d misses=%d->%d", h, h2, m, m2)
	}

	// Local change: move the last automation point. Only segments at
	// or after the affected region may be rebuilt.
	p2 := validParams()
	p2.GainAutomation[2].Value = 0.8
	want, err := Render(p2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Bake(p2)
	if err != nil {
		t.Fatal(err)
	}
	if !identical(got, want) {
		t.Fatal("incremental rebake differs from full render")
	}
	_, m3 := c.Stats()
	totalSegs := (len(full) + SegmentSamples - 1) / SegmentSamples
	if m3-m2 == 0 || m3-m2 >= totalSegs {
		t.Fatalf("expected partial rebuild, got %d new misses of %d segments", m3-m2, totalSegs)
	}
}
