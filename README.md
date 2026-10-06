# procedural-sfx-synth-go

程序化音效合成与烘焙库：设计者用一份参数描述即可在本地进程内离线生成可复现的音效，支持参数微调后的增量重烘焙。不依赖外部服务与真实音频设备。

## 快速开始

```go
p := &sfx.Params{
    Seed:        42,
    SampleRate:  44100,
    DurationSec: 0.5,
    Oscillator:  sfx.Oscillator{Waveform: sfx.WaveSaw, FreqHz: 220, Gain: 0.8},
    Noise:       sfx.Noise{Enabled: true, Gain: 0.2},
    Envelope:    sfx.Envelope{Attack: 0.01, Decay: 0.1, Sustain: 0.7, Release: 0.1},
    Filter:      sfx.Filter{Type: sfx.FilterLowPass, CutoffHz: 3000, Resonance: 2.0},
    LFO:         sfx.LFO{Target: sfx.ModCutoff, Waveform: sfx.WaveSine, FreqHz: 5, Depth: 0.5},
Drive:       sfx.Drive{Amount: 3, Mix: 0.5},
}
samples, err := sfx.Render(p) // []float64, 单声道, 规范化浮点
```

## 参数模型

`Params` 是音效的完整描述，处理链顺序固定为：

**源（振荡器 + 噪声）→ 包络/增益 → 滤波 → 非线性驱动 → 输出保护**

| 字段 | 说明 | 约束 |
|---|---|---|
| `Seed` | 噪声源的确定性随机种子 | 任意 int64 |
| `SampleRate` | 采样率 Hz | 8000..192000 |
| `DurationSec` | 时长秒 `DurationSec * SampleRate` 四舍五入为总采样数 | 0 < x ≤ 3600 |
| `Oscillator` | 周期波形源（正弦/方波/锯齿/三角），频率与增益 | Freq ∈ (0, Nyquist]，Gain ∈ [0,4] 且至少一个源可发声| 
| `Noise`  `Enabled` 与 `Gain` 必须同时设置或同时为零 | `Gain` ∈ (0,4] |  
| `Envelope` ADSR, Attack/Decay/Release 为秒, Sustain 为电平 | Sustain ∈ [0,1] | 
| `Filter` 双二阶滤波（低通/高通），cutoff 可随 LFO/自动化变化 | Cutoff ∈ (0, Nyquist), Q ∈ [0.5,100] | 
| `LFO` 低频调制器，目标为 Freq/Gain/Cutoff | Freq ∈ (0,100], Depth ∈ (0,1] | 
| `Drive` tanh 波形整形, `Amount` 为驱动量, `Mix` 干湿比 | Amount ∈ [0,100], Mix ∈ (0,1] | 
| `GainAutomation` / `CutoffAutomation` | 折点线性插值自动化 | 时间有序且在时长内 |

### 校验语义

`Validate` 在渲染前执行，发现以下情况即拒绝并返回 `*ValidationError`（含可定位的字段路径与原因）：取值越界、互斥字段冲突（如 `Noise.Enabled=false` 但 `Gain>0` 、 `FilterNone` 但设置了 Cutoff 、 `ModNone` 但设置了 LFO 参数）以及缺少必需项（无任何可发声源）。**绝不静默裁剪参数**。

## 渲染语义

### 确定性
相同参数与随机种子产生逐样本一致的输出。随机源使用内置 splitmix64 发生器，不依赖 Go 运行时或进程全局状态，跨进程重启与不同批次执行结果不变。

### 分块流式渲染
`NewRenderer` + `RenderBlock` 支持任意块长的流式渲染。所有处理阶段（振荡器相位、LFO 相位、噪声 RNG、滤波器历史、包络时间）的状态在块间连续携带，任意分块结果与一次性渲染逐样本一致；相位用取模保持在 [0,1) 内，长时渲染无相位/增益漂移。

### 调制与自动化
LFO 可对频率、增益、滤波截止频率进行乘性调制；自动化折点在时间轴上线性插值。调制后的动态参数按既定策略钳制到稳定域（如 cutoff ∈ [1, 0.999*Nyquist]）；静态越界参数则在校验阶段直接拒绝。

### 非线性稳定性
驱动级使用有界 tanh 整形，极值参数下不会发散；最终输出经过 `sanitize` 保护：非有限值（NaN/Inf）替换为 0，输出钳制到 [-4,4]。

## 格式转换

`Resample`：线性插值重采样，输出长度 = `round(len*dstRate/srcRate)`，非整数倍时输出时长与源一致；输出区间精确映射到源区间，首尾样本不缺失、不重复。

`Quantize`：位深量化，越界样本按既定策略硬钳制到 [-1,1]，NaN/Inf 替换为 0。

`ConvertChannels`：单声道 ↔ 立体声（复制 / 平均），其它声道数被拒绝。

## 增量重烘焙与缓存

`Cache.Bake` 将渲染分为固定 4096 样本的段，每段以「静态参数 + 该段时间范围内相关自动化折点 + 段索引」的内容哈希为键缓存样本与段末渲染器状态。参数未变化时全部复用；局部参数变化（如修改某个自动化折点）只重建受影响时间段之后的段。缓存复用与全量重建的结果逐字节一致。

## 并发批量烘焙

`BakeBatch(tasks, workers)` 以任意并发度执行批量烘焙：每个任务独立渲染，结果按任务顺序返回，任意并发度下逐字节一致；单个任务失败不影响其他任务，错误以 `*BakeError`（含任务 ID）返回，可定位可追溯。

## 测试

```sh
go test ./...        # 单元测试
go test -race ./...  # 含竞态检测
```

覆盖：确定性、分块连续性、重采样边界、非线性稳定性、并发一致性、缓存复用。
