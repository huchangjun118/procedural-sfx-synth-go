package sfx

import (
	"strings"
	"testing"
)

func hasFieldErr(errs []error, field string) bool {
	for _, e := range errs {
		if ve, ok := e.(*ValidationError); ok && ve.Field == field {
			return true
		}
	}
	return false
}

func TestValidateAcceptsBaseParams(t *testing.T) {
	if errs := Validate(baseParams()); len(errs) > 0 {
		t.Fatalf("base params rejected: %v", errs)
	}
}

// 取值越界必须被拒绝并给出可定位字段。
func TestValidateOutOfRange(t *testing.T) {
	cases := []struct {
		mutate func(*Params)
		field  string
	}{
		{func(p *Params) { p.Duration = -1 }, "duration"},
		{func(p *Params) { p.SampleRate = 100 }, "sampleRate"},
		{func(p *Params) { p.Gain = 1.5 }, "gain"},
		{func(p *Params) { p.Osc.Freq = 0 }, "osc.freq"},
		{func(p *Params) { p.Osc.Freq = 30000 }, "osc.freq"},
		{func(p *Params) { p.Osc.Level = 2 }, "osc.level"},
		{func(p *Params) { p.Osc.PulseWidth = 1.5 }, "osc.pulseWidth"},
		{func(p *Params) { p.Noise.Level = 1.2 }, "noise.level"},
		{func(p *Params) { p.Envelope.Attack = -0.1 }, "envelope.attack"},
		{func(p *Params) { p.Envelope.Sustain = 1.1 }, "envelope.sustain"},
		{func(p *Params) { p.Filter.Cutoff = 22050 }, "filter.cutoff"},
		{func(p *Params) { p.Filter.Resonance = 100 }, "filter.resonance"},
		{func(p *Params) { p.LFO.Rate = 100 }, "lfo.rate"},
		{func(p *Params) { p.Drive.Drive = 0.5 }, "drive.drive"},
		{func(p *Params) { p.Drive.Mix = 2 }, "drive.mix"},
	}
	for _, c := range cases {
		p := baseParams()
		c.mutate(p)
		errs := Validate(p)
		if !hasFieldErr(errs, c.field) {
			t.Errorf("expected error on field %s, got %v", c.field, errs)
		}
	}
}

// 互斥设置必须被拒绝。
func TestValidateMutualExclusion(t *testing.T) {
	p := baseParams()
	p.Osc.Waveform = WaveNone // 振荡器关闭但 level 仍 > 0
	if !hasFieldErr(Validate(p), "osc.level") {
		t.Error("expected osc.level mutual-exclusion error")
	}

	p = baseParams()
	p.Noise.Type = NoiseNone
	if !hasFieldErr(Validate(p), "noise.level") {
		t.Error("expected noise.level mutual-exclusion error")
	}

	p = baseParams()
	p.Drive.Drive = 0 // 失真旁路但 mix 非零
	if !hasFieldErr(Validate(p), "drive.mix") {
		t.Error("expected drive.mix mutual-exclusion error")
	}

	p = baseParams()
	p.Filter.Type = FilterNone // 滤波关闭但 LFO 仍调制滤波
	if !hasFieldErr(Validate(p), "lfo.filterDepth") {
		t.Error("expected lfo.filterDepth dependency error")
	}
}

// 缺少必需项（无任何声源）必须被拒绝。
func TestValidateMissingRequired(t *testing.T) {
	p := baseParams()
	p.Osc.Waveform = WaveNone
	p.Osc.Level = 0
	p.Noise.Type = NoiseNone
	p.Noise.Level = 0
	errs := Validate(p)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "at least one active source") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing-source error, got %v", errs)
	}
}

// 错误信息必须可定位（包含字段路径）。
func TestValidationErrorLocatable(t *testing.T) {
	p := baseParams()
	p.Osc.Freq = -5
	errs := Validate(p)
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %v", errs)
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "osc.freq") {
		t.Fatalf("error not locatable: %q", msg)
	}
}

func TestValidateFormat(t *testing.T) {
	ok := Format{SampleRate: 48000, BitDepth: 16, Channels: 2}
	if errs := ValidateFormat(&ok); len(errs) > 0 {
		t.Fatalf("valid format rejected: %v", errs)
	}
	bad := Format{SampleRate: 1, BitDepth: 12, Channels: 3, ClipMode: "wrap"}
	errs := ValidateFormat(&bad)
	for _, f := range []string{"format.sampleRate", "format.bitDepth", "format.channels", "format.clipMode"} {
		if !hasFieldErr(errs, f) {
			t.Errorf("expected format error on %s, got %v", f, errs)
		}
	}
}
