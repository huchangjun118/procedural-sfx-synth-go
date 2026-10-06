// Package sfx 提供程序化音效的参数化合成、流式渲染、格式转换与离线烘焙能力。
package sfx

// Waveform 振荡器波形类型。
type Waveform string

const (
	WaveSine     Waveform = "sine"
	WaveSquare   Waveform = "square"
	WaveSaw      Waveform = "saw"
	WaveTriangle Waveform = "triangle"
	WaveNone     Waveform = "none"
)

// NoiseType 噪声源类型。
type NoiseType string

const (
	NoiseNone  NoiseType = "none"
	NoiseWhite NoiseType = "white"
	NoisePink  NoiseType = "pink"
)

// FilterType 滤波器类型。
type FilterType string

const (
	FilterNone     FilterType = "none"
	FilterLowPass  FilterType = "lowpass"
	FilterHighPass FilterType = "highpass"
)

// OscParams 振荡器参数。
type OscParams struct {
	Waveform   Waveform `json:"waveform"`
	Freq       float64  `json:"freq"`       // Hz
	FreqEnd    float64  `json:"freqEnd"`    // Hz，非零时在整个时长内向其线性滑动
	Level      float64  `json:"level"`      // 0..1
	PulseWidth float64  `json:"pulseWidth"` // 仅 square 有效，0<pw<1
}

// NoiseParams 噪声源参数。
type NoiseParams struct {
	Type  NoiseType `json:"type"`
	Level float64   `json:"level"` // 0..1
}

// EnvParams ADSR 包络参数，时间单位秒。
type EnvParams struct {
	Attack  float64 `json:"attack"`
	Decay   float64 `json:"decay"`
	Sustain float64 `json:"sustain"` // 0..1 电平
	Release float64 `json:"release"`
}

// FilterParams 双二阶滤波参数。
type FilterParams struct {
	Type      FilterType `json:"type"`
	Cutoff    float64    `json:"cutoff"`    // Hz
	CutoffEnd float64    `json:"cutoffEnd"` // Hz，非零时随时间滑动
	Resonance float64    `json:"resonance"` // Q，>0
}

// LFOParams 低频调制参数，可调制音高/滤波截止/幅度。
type LFOParams struct {
	Rate        float64  `json:"rate"`        // Hz，0 表示关闭
	Depth       float64  `json:"depth"`       // 0..1
	Waveform    Waveform `json:"waveform"`    // sine / triangle / square
	PitchDepth  float64  `json:"pitchDepth"`  // 半音数
	FilterDepth float64  `json:"filterDepth"` // Hz
	AmpDepth    float64  `json:"ampDepth"`    // 0..1
}

// DriveParams 非线性失真参数。
type DriveParams struct {
	Drive float64 `json:"drive"` // >=1，1 表示旁路
	Mix   float64 `json:"mix"`   // 0..1
}

// Params 一份音效的完整参数描述。
type Params struct {
	Name       string       `json:"name"`
	Duration   float64      `json:"duration"` // 秒
	SampleRate int          `json:"sampleRate"`
	Seed       int64        `json:"seed"`
	Osc        OscParams    `json:"osc"`
	Noise      NoiseParams  `json:"noise"`
	Envelope   EnvParams    `json:"envelope"`
	Filter     FilterParams `json:"filter"`
	LFO        LFOParams    `json:"lfo"`
	Drive      DriveParams  `json:"drive"`
	Gain       float64      `json:"gain"` // 0..1 总增益
}

// Format 目标输出格式。
type Format struct {
	SampleRate int    `json:"sampleRate"`         // 目标采样率
	BitDepth   int    `json:"bitDepth"`           // 8 / 16 / 24 / 32(float)
	Channels   int    `json:"channels"`           // 1 / 2
	ClipMode   string `json:"clipMode,omitempty"` // 超范围样本策略："clip"（默认）
}
