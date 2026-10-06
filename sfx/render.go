package sfx

import "math"

// splitmix64 确定性伪随机源：相同种子跨进程、跨批次逐样本一致。
type prng struct{ s uint64 }

func newPRNG(seed int64) *prng { return &prng{s: uint64(seed)} }

func (g *prng) next() uint64 {
	g.s += 0x9E3779B97F4A7C15
	z := g.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// uniform 返回 [-1, 1) 的均匀分布样本。
func (g *prng) uniform() float64 {
	return float64(g.next()>>11)*(1.0/(1<<52))*2 - 1
}

// pinkState Paul Kellet 三极点粉红噪声滤波器状态。
type pinkState struct{ b0, b1, b2 float64 }

func (p *pinkState) next(white float64) float64 {
	p.b0 = 0.99765*p.b0 + white*0.0990460
	p.b1 = 0.96300*p.b1 + white*0.2965164
	p.b2 = 0.57000*p.b2 + white*1.0526913
	return (p.b0 + p.b1 + p.b2 + white*0.1848) * 0.25
}

// biquad RBJ 双二阶滤波器，系数随调制逐样本重算，状态跨块连续。
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (b *biquad) setLowPass(cutoff, q, sr float64) {
	w0 := 2 * math.Pi * cutoff / sr
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	a0 := 1 + alpha
	b.b0 = (1 - cw) / 2 / a0
	b.b1 = (1 - cw) / a0
	b.b2 = b.b0
	b.a1 = -2 * cw / a0
	b.a2 = (1 - alpha) / a0
}

func (b *biquad) setHighPass(cutoff, q, sr float64) {
	w0 := 2 * math.Pi * cutoff / sr
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	a0 := 1 + alpha
	b.b0 = (1 + cw) / 2 / a0
	b.b1 = -(1 + cw) / a0
	b.b2 = b.b0
	b.a1 = -2 * cw / a0
	b.a2 = (1 - alpha) / a0
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

func oscWave(w Waveform, phase, pw float64) float64 {
	switch w {
	case WaveSine:
		return math.Sin(2 * math.Pi * phase)
	case WaveSquare:
		if phase < pw {
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

// lfoValue 由绝对采样序号推导 LFO 输出 [-1, 1]，纯函数无累积漂移。
func lfoValue(p *Params, idx int) float64 {
	if p.LFO.Rate <= 0 {
		return 0
	}
	phase := math.Mod(float64(idx)*p.LFO.Rate/float64(p.SampleRate), 1.0)
	return p.LFO.Depth * oscWave(p.LFO.Waveform, phase, 0.5)
}

// envelope 由绝对采样序号计算 ADSR 包络增益，纯函数无累积漂移。
func envelope(p *Params, idx, total int) float64 {
	sr := float64(p.SampleRate)
	a := p.Envelope.Attack * sr
	d := p.Envelope.Decay * sr
	rel := p.Envelope.Release * sr
	s := p.Envelope.Sustain
	i := float64(idx)

	if i < a && a > 0 {
		return i / a
	}
	i -= a
	if i < d && d > 0 {
		return 1 - (1-s)*(i/d)
	}
	remain := float64(total) - float64(idx)
	if remain < rel && rel > 0 {
		return s * (remain / rel)
	}
	return s
}

// sanitize 输出安全策略：非有限样本替换为 0，并硬截断到 [-1, 1]。
// 保证任意（合法）参数下输出绝无 NaN/Inf 且不发散、不爆音。
func sanitize(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return math.Max(-1, math.Min(1, x))
}

// sourceGen 阶段一：振荡器 + 噪声 + 音高滑动/LFO，输出干声。
type sourceGen struct {
	p     *Params
	rng   *prng
	pink  pinkState
	phase float64 // 振荡器相位（周）
}

func newSourceGen(p *Params) *sourceGen {
	return &sourceGen{p: p, rng: newPRNG(p.Seed)}
}

func (s *sourceGen) next(idx, total int) float64 {
	p := s.p
	sr := float64(p.SampleRate)
	frac := float64(idx) / float64(total)
	var x float64
	if p.Osc.Waveform != WaveNone && p.Osc.Level > 0 {
		freq := p.Osc.Freq
		if p.Osc.FreqEnd > 0 {
			freq += (p.Osc.FreqEnd - p.Osc.Freq) * frac
		}
		if p.LFO.PitchDepth > 0 {
			freq *= math.Pow(2, lfoValue(p, idx)*p.LFO.PitchDepth/12)
		}
		s.phase += freq / sr
		s.phase -= math.Floor(s.phase)
		x += p.Osc.Level * oscWave(p.Osc.Waveform, s.phase, p.Osc.PulseWidth)
	}
	if p.Noise.Type != NoiseNone && p.Noise.Level > 0 {
		w := s.rng.uniform()
		if p.Noise.Type == NoisePink {
			w = s.pink.next(w)
		}
		x += p.Noise.Level * w
	}
	return x
}

// shaper 阶段二：滤波 + 包络 + 幅度 LFO + 非线性失真 + 总增益。
type shaper struct {
	p  *Params
	bq biquad
}

func newShaper(p *Params) *shaper { return &shaper{p: p} }

func (s *shaper) next(idx, total int, x float64) float64 {
	p := s.p
	sr := float64(p.SampleRate)
	frac := float64(idx) / float64(total)
	lfo := lfoValue(p, idx)

	// 滤波（截止频率可滑动并被 LFO 调制，调制结果按既定策略夹取）
	if p.Filter.Type != FilterNone {
		cutoff := p.Filter.Cutoff
		if p.Filter.CutoffEnd > 0 {
			cutoff += (p.Filter.CutoffEnd - p.Filter.Cutoff) * frac
		}
		cutoff += lfo * p.LFO.FilterDepth
		cutoff = math.Max(20, math.Min(cutoff, 0.45*sr))
		if p.Filter.Type == FilterLowPass {
			s.bq.setLowPass(cutoff, p.Filter.Resonance, sr)
		} else {
			s.bq.setHighPass(cutoff, p.Filter.Resonance, sr)
		}
		x = s.bq.process(x)
	}

	// 包络与幅度调制
	x *= envelope(p, idx, total)
	if p.LFO.AmpDepth > 0 {
		x *= math.Max(0, 1+lfo*p.LFO.AmpDepth)
	}

	// 非线性失真：tanh 软削波，输出天然有界，极端参数下不发散
	if p.Drive.Drive > 1 {
		d := p.Drive.Drive
		wet := math.Tanh(d*x) / math.Tanh(d)
		x = (1-p.Drive.Mix)*x + p.Drive.Mix*wet
	}

	return sanitize(x * p.Gain)
}

// renderSourceAll 渲染阶段一完整缓冲。
func renderSourceAll(p *Params, total int) []float64 {
	g := newSourceGen(p)
	out := make([]float64, total)
	for i := range out {
		out[i] = g.next(i, total)
	}
	return out
}

// renderShapeAll 对干声缓冲执行阶段二。
func renderShapeAll(p *Params, src []float64) []float64 {
	s := newShaper(p)
	out := make([]float64, len(src))
	for i, x := range src {
		out[i] = s.next(i, len(src), x)
	}
	return out
}

// Renderer 分块流式渲染器。各阶段状态保存在结构体内，
// 包络与 LFO 由绝对采样序号推导（无累积漂移），
// 因此任意块长分块渲染与一次性渲染逐样本一致。
type Renderer struct {
	p     *Params
	total int
	idx   int
	src   *sourceGen
	shp   *shaper
}

// NewRenderer 以给定参数创建渲染器；参数须先通过 Validate。
func NewRenderer(p *Params) *Renderer {
	return &Renderer{
		p:     p,
		total: int(math.Round(p.Duration * float64(p.SampleRate))),
		src:   newSourceGen(p),
		shp:   newShaper(p),
	}
}

// TotalFrames 输出总帧数（单声道采样点数）。
func (r *Renderer) TotalFrames() int { return r.total }

// Render 填充 buf 并返回写入的帧数；到达末尾后返回 0。
func (r *Renderer) Render(buf []float64) int {
	n := 0
	for n < len(buf) && r.idx < r.total {
		buf[n] = r.shp.next(r.idx, r.total, r.src.next(r.idx, r.total))
		r.idx++
		n++
	}
	return n
}

// RenderAll 一次性渲染全部样本。
func RenderAll(p *Params) []float64 {
	r := NewRenderer(p)
	out := make([]float64, r.TotalFrames())
	r.Render(out)
	return out
}
