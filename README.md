# procedural-sfx-synth-go

程序化音效的参数化合成与离线烘焙库。设计者用一份参数描述（`Params`）即可在本地进程内
离线生成**可复现**的音效，并支持参数微调后的**增量重烘焙**。全程不依赖外部服务与真实音频设备。

```go
p := sfx.Params{ /* 见下文参数模型 */ }
if errs := sfx.Validate(&p); len(errs) > 0 { /* 每个错误都带可定位字段路径 */ }

baker := sfx.NewBaker(nil) // 自带内容缓存
res, err := baker.Bake(&sfx.BakeRequest{
    Params: p,
    Format: sfx.Format{SampleRate: 44100, BitDepth: 16, Channels: 1},
})
// res.WAV 为标准 PCM WAV 字节；res.Hash 为内容哈希
```

## 参数模型

`Params` 描述一次合成的全部输入，渲染管线按以下顺序逐样本处理：

| 阶段 | 参数 | 说明 |
| --- | --- | --- |
| 声源 | `osc` | 振荡器：`waveform`(sine/square/saw/triangle/none)、`freq`→`freqEnd` 线性滑动、`level`、`pulseWidth`（方波） |
| 声源 | `noise` | 噪声源：`type`(white/pink/none)、`level`；白噪声由 splitmix64 PRNG 按 `seed` 生成 |
| 调制 | `lfo` | `rate`/`depth`/`waveform`，可分别调制音高(`pitchDepth` 半音)、滤波截止(`filterDepth` Hz)、幅度(`ampDepth`) |
| 滤波 | `filter` | RBJ 双二阶：`type`(lowpass/highpass/none)、`cutoff`→`cutoffEnd` 滑动、`resonance`(Q) |
| 包络 | `envelope` | ADSR：`attack`/`decay`/`sustain`/`release`（秒） |
| 非线性 | `drive` | tanh 软削波：`drive`≥1、`mix` 干湿比；输出天然有界 |
| 输出 | `gain` | 总增益 0..1 |

`duration`（秒）与 `sampleRate` 决定输出帧数；`seed` 决定全部随机性。

### 校验语义

`Validate` / `ValidateFormat` 返回**全部**错误，每个错误为 `*ValidationError`，
含 `Field`（如 `osc.freq`）与 `Reason`，可定位到具体参数：

- **取值越界**：频率、电平、Q 值、时长等超出合法区间即拒绝；
- **互斥设置**：如 `osc.waveform=none` 但 `osc.level>0`、`drive=0` 但 `mix≠0`、
  滤波关闭却配置 `lfo.filterDepth>0`；
- **缺少必需项**：振荡器与噪声源至少启用一个。

校验只拒绝、**绝不静默裁剪**；非法参数不会进入渲染，因此不会产出爆音结果。

### 确定性

相同 `Params`（含 `seed`）在任何进程、任何批次、任何并发度下渲染结果**逐样本一致**：
随机源为固定算法的 PRNG，包络与 LFO 相位由绝对采样序号推导（无累积漂移），
不读取时钟、不依赖 map 迭代顺序。

## 流式渲染

`NewRenderer` 创建的渲染器支持任意块长分块渲染，且与一次性渲染**逐样本一致**：

```go
r := sfx.NewRenderer(&p)
buf := make([]float64, 512)
for {
    n := r.Render(buf) // 返回 0 表示结束
    ...
}
```

各处理阶段状态（振荡器相位、PRNG、粉红噪声滤波器、双二阶滤波器）保存在渲染器内并
跨块连续；包络/LFO 由绝对采样序号计算，长时间渲染无相位或增益漂移。

## 格式转换

- **重采样** `Resample`：线性插值，输出帧数 `round(N·dstRate/srcRate)`，非整数倍时
  输出时长与源一致，采样位置由输出序号直接推导，首尾不缺少、不重复采样；
- **声道** `ConvertChannels`：单→双复制，双→单取平均；
- **量化** `Quantize`：8/16/24/32 位有符号；超范围样本按既定策略**硬截断**到 [-1,1]，
  非有限值替换为 0；
- **WAV** `EncodeWAV`：标准 PCM（8 位无符号，16/24/32 位小端有符号），纯函数输出。

## 稳定性策略

渲染器最终输出经过安全处理：非有限样本替换为 0，并硬截断到 [-1,1]。
tanh 软削波本身有界；调制产生的中间值（如滤波截止）按既定策略夹取到安全区间。
因此极值参数下输出绝无 NaN/Inf，也不随时间发散。

## 烘焙语义

`Baker` 将渲染管线分为三个缓存阶段，每阶段以**其输入内容的哈希**为键：

1. `src`：声源（振荡器 + 噪声 + 音高调制），键含 `osc`/`noise`/音高 LFO/`seed`/`duration`/`sampleRate`；
2. `shape`：塑形（滤波 + 包络 + 幅度调制 + 失真 + 增益），键含阶段 1 的键与对应参数；
3. `fmt`：格式转换（重采样 + 声道 + 量化 + WAV 编码），键含阶段 2 的键与 `Format`。

- **参数未变化**：三个阶段全部命中缓存，直接复用产物；
- **局部参数变化**：只重建受影响阶段——改包络只重算 2、3，改格式只重算 3；
- 缓存复用与全量重建的结果**逐字节一致**（渲染为纯函数，缓存只存结果不改结果）。

### 并发批量烘焙

```go
results := baker.BakeBatch(reqs, 8) // 任意并发度
```

每个任务相互独立：同一批任务在任意并发度下结果逐字节一致；单个任务失败（如参数非法）
不影响其他任务，失败原因保留在对应下标的 `BatchResult.Err` 中，可定位、可追溯。

## 测试

```sh
go test ./...
```

覆盖：渲染确定性、分块连续性、长时间渲染稳定性、参数校验（越界/互斥/缺项）、
非整数倍重采样边界、量化截断策略、极值非线性稳定性、缓存复用与增量重烘焙、
并发批量一致性与失败隔离。
