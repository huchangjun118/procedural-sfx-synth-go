package sfx

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// baseParams 返回一份覆盖全部处理阶段的合法参数。
func baseParams() *Params {
	return &Params{
		Name:       "laser",
		Duration:   0.7,
		SampleRate: 44100,
		Seed:       42,
		Osc: OscParams{
			Waveform:   WaveSquare,
			Freq:       880,
			FreqEnd:    110,
			Level:      0.6,
			PulseWidth: 0.4,
		},
		Noise: NoiseParams{Type: NoisePink, Level: 0.2},
		Envelope: EnvParams{
			Attack:  0.01,
			Decay:   0.1,
			Sustain: 0.6,
			Release: 0.2,
		},
		Filter: FilterParams{
			Type:      FilterLowPass,
			Cutoff:    4000,
			CutoffEnd: 800,
			Resonance: 2.5,
		},
		LFO: LFOParams{
			Rate:        6,
			Depth:       0.8,
			Waveform:    WaveSine,
			PitchDepth:  2,
			FilterDepth: 500,
			AmpDepth:    0.3,
		},
		Drive: DriveParams{Drive: 4, Mix: 0.5},
		Gain:  0.8,
	}
}

func floatsEqual(a, b []float64) bool {
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

// 确定性：相同参数与种子多次渲染逐样本一致。
func TestDeterministicRendering(t *testing.T) {
	p := baseParams()
	a := RenderAll(p)
	b := RenderAll(p)
	if !floatsEqual(a, b) {
		t.Fatal("same params rendered twice differ")
	}
	// 不同种子必须产生不同输出（噪声源生效）
	p2 := baseParams()
	p2.Seed = 43
	c := RenderAll(p2)
	if floatsEqual(a, c) {
		t.Fatal("different seeds produced identical output")
	}
}

// 分块连续性：任意块长分块渲染与一次性渲染逐样本一致。
func TestBlockRenderingContinuity(t *testing.T) {
	p := baseParams()
	want := RenderAll(p)
	for _, block := range []int{1, 3, 7, 64, 1000, 4096, len(want)} {
		r := NewRenderer(p)
		var got []float64
		buf := make([]float64, block)
		for {
			n := r.Render(buf)
			if n == 0 {
				break
			}
			got = append(got, buf[:n]...)
		}
		if !floatsEqual(want, got) {
			t.Fatalf("block size %d: chunked render differs from one-shot", block)
		}
	}
}

// 长时间渲染：无相位/增益漂移迹象，输出始终有界且无 NaN/Inf。
func TestLongRenderStability(t *testing.T) {
	p := baseParams()
	p.Duration = 30
	p.Osc.FreqEnd = 0
	p.Filter.CutoffEnd = 0
	out := RenderAll(p)
	if len(out) != 30*44100 {
		t.Fatalf("unexpected length %d", len(out))
	}
	third := len(out) / 3
	var maxFirst, maxLast float64
	for i, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite sample at %d", i)
		}
		a := math.Abs(v)
		switch {
		case i < third:
			if a > maxFirst {
				maxFirst = a
			}
		case i >= 2*third:
			if a > maxLast {
				maxLast = a
			}
		}
	}
	// 持续段增益不得漂移：后段峰值与前段峰值应处于同一量级
	if maxLast > maxFirst*1.5+1e-9 {
		t.Fatalf("gain drift detected: first %v last %v", maxFirst, maxLast)
	}
}

// 渲染输出可作为确定性哈希输入（跨批次一致性的字节级验证）。
func TestRenderByteStability(t *testing.T) {
	p := baseParams()
	h1 := hashParts(floatsToBytes(RenderAll(p)))
	h2 := hashParts(floatsToBytes(RenderAll(p)))
	if h1 != h2 {
		t.Fatal("render hash unstable across runs")
	}
}

func TestWAVHeaderRoundTrip(t *testing.T) {
	f := Format{SampleRate: 22050, BitDepth: 16, Channels: 2}
	samples := []int32{0, 1 << 20, -(1 << 20), 100}
	wav := EncodeWAV(samples, f)
	if !bytes.Equal(wav[:4], []byte("RIFF")) || !bytes.Equal(wav[8:12], []byte("WAVE")) {
		t.Fatal("bad RIFF header")
	}
	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	if int(dataSize) != len(samples)*2 {
		t.Fatalf("data size %d, want %d", dataSize, len(samples)*2)
	}
	if len(wav) != 44+int(dataSize) {
		t.Fatalf("wav length %d", len(wav))
	}
}
