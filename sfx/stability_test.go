package sfx

import (
	"math"
	"testing"
)

func assertFiniteAndBounded(t *testing.T, out []float64, bound float64) {
	t.Helper()
	for i, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite sample at %d", i)
		}
		if math.Abs(v) > bound {
			t.Fatalf("sample %d exceeds bound: %v", i, v)
		}
	}
}

// 极值非线性参数下输出不得出现 NaN/Inf，也不得发散。
func TestExtremeDriveStability(t *testing.T) {
	p := baseParams()
	p.Drive = DriveParams{Drive: 100, Mix: 1} // 最大合法驱动
	p.Gain = 1
	out := RenderAll(p)
	assertFiniteAndBounded(t, out, 1.0)
}

// 高谐振 + 调制滤波扫频下输出稳定。
func TestExtremeFilterStability(t *testing.T) {
	p := baseParams()
	p.Filter = FilterParams{Type: FilterLowPass, Cutoff: 200, CutoffEnd: 20000, Resonance: 30}
	p.LFO.FilterDepth = 20000
	p.LFO.Rate = 40
	out := RenderAll(p)
	// 高谐振瞬时峰值有界即可，关键是不出现非数值且不发散
	assertFiniteAndBounded(t, out, 1.0)
	half := len(out) / 2
	var e1, e2 float64
	for i, v := range out {
		if i < half {
			e1 += v * v
		} else {
			e2 += v * v
		}
	}
	if e2 > e1*5 {
		t.Fatalf("resonant filter output diverging: %v -> %v", e1, e2)
	}
}

// 满幅输入 + 最大增益 + 全部调制同时作用。
func TestCombinedExtremesStability(t *testing.T) {
	p := baseParams()
	p.Osc.Level = 1
	p.Noise.Level = 1
	p.Gain = 1
	p.LFO.AmpDepth = 1
	p.LFO.PitchDepth = 24
	p.Drive = DriveParams{Drive: 100, Mix: 1}
	out := RenderAll(p)
	assertFiniteAndBounded(t, out, 1.0)
}

// 长时间高驱动渲染不发散：后段能量不得超过前段的合理倍数。
func TestNoDivergenceOverTime(t *testing.T) {
	p := baseParams()
	p.Duration = 20
	p.Drive = DriveParams{Drive: 50, Mix: 1}
	p.Envelope = EnvParams{Attack: 0.01, Sustain: 1, Release: 0.5}
	out := RenderAll(p)
	assertFiniteAndBounded(t, out, 1.0)
	half := len(out) / 2
	var e1, e2 float64
	for i, v := range out {
		if i < half {
			e1 += v * v
		} else {
			e2 += v * v
		}
	}
	if e2 > e1*3 {
		t.Fatalf("energy diverging: first %v second %v", e1, e2)
	}
}
