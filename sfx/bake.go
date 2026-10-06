package sfx

import (
	"errors"
	"fmt"
	"sync"
)

// BakeRequest 一次烘焙任务。
type BakeRequest struct {
	Params Params `json:"params"`
	Format Format `json:"format"`
}

// BakeResult 烘焙产物：WAV 字节与内容哈希。
type BakeResult struct {
	WAV  []byte
	Hash string
}

// 阶段缓存键的组成部分：只纳入影响该阶段的参数，
// 使局部参数变化时上游阶段缓存仍可命中。
type lfoPitchKey struct {
	Rate       float64
	Depth      float64
	PitchDepth float64
	Waveform   Waveform
}

type lfoShapeKey struct {
	Rate        float64
	Depth       float64
	FilterDepth float64
	AmpDepth    float64
	Waveform    Waveform
}

// Baker 执行烘焙并维护内容缓存以支持增量重烘焙。
// 渲染管线的每个阶段（声源 / 塑形 / 格式转换）均以其输入内容的
// 哈希为键缓存：参数未变化时整体复用，局部变化时只重建受影响阶段。
type Baker struct {
	cache *Cache
}

// NewBaker 创建烘焙器；cache 为 nil 时使用独立的内存缓存。
func NewBaker(cache *Cache) *Baker {
	if cache == nil {
		cache = NewCache()
	}
	return &Baker{cache: cache}
}

// Cache 返回烘焙器使用的缓存。
func (b *Baker) Cache() *Cache { return b.cache }

func totalFrames(p *Params) int {
	return int(p.Duration*float64(p.SampleRate) + 0.5)
}

// Bake 烘焙单个任务；参数非法时返回可定位的错误，不产出结果。
func (b *Baker) Bake(req *BakeRequest) (*BakeResult, error) {
	p := &req.Params
	if errs := Validate(p); len(errs) > 0 {
		return nil, fmt.Errorf("bake %q rejected: %w", p.Name, errors.Join(errs...))
	}
	if errs := ValidateFormat(&req.Format); len(errs) > 0 {
		return nil, fmt.Errorf("bake %q rejected: %w", p.Name, errors.Join(errs...))
	}
	total := totalFrames(p)

	// 阶段一：声源（振荡器 + 噪声 + 音高调制）
	srcKey := hashParts("src-v1", p.Osc, p.Noise,
		lfoPitchKey{p.LFO.Rate, p.LFO.Depth, p.LFO.PitchDepth, p.LFO.Waveform},
		p.Seed, p.Duration, p.SampleRate)
	var src []float64
	if raw, ok := b.cache.Get(srcKey); ok {
		src = bytesToFloats(raw)
	} else {
		src = renderSourceAll(p, total)
		b.cache.Put(srcKey, floatsToBytes(src))
	}

	// 阶段二：塑形（滤波 + 包络 + 幅度调制 + 失真 + 增益）
	shapeKey := hashParts("shape-v1", srcKey, p.Filter, p.Envelope,
		lfoShapeKey{p.LFO.Rate, p.LFO.Depth, p.LFO.FilterDepth, p.LFO.AmpDepth, p.LFO.Waveform},
		p.Drive, p.Gain, p.Duration, p.SampleRate)
	var shaped []float64
	if raw, ok := b.cache.Get(shapeKey); ok {
		shaped = bytesToFloats(raw)
	} else {
		shaped = renderShapeAll(p, src)
		b.cache.Put(shapeKey, floatsToBytes(shaped))
	}

	// 阶段三：格式转换（重采样 + 声道 + 量化 + WAV 编码）
	fmtKey := hashParts("fmt-v1", shapeKey, req.Format)
	var wav []byte
	if raw, ok := b.cache.Get(fmtKey); ok {
		wav = raw
	} else {
		conv := Resample(shaped, p.SampleRate, req.Format.SampleRate)
		conv = ConvertChannels(conv, 1, req.Format.Channels)
		wav = EncodeWAV(Quantize(conv, req.Format.BitDepth), req.Format)
		b.cache.Put(fmtKey, wav)
	}

	return &BakeResult{WAV: wav, Hash: fmtKey}, nil
}

// BatchResult 批量任务中单个任务的结果。
type BatchResult struct {
	Index  int
	Result *BakeResult
	Err    error
}

// BakeBatch 并发批量烘焙。每个任务相互独立：任意并发度下结果逐字节一致，
// 单个任务失败不影响其他任务，失败原因保留在对应下标的结果中。
// concurrency < 1 时按 1 处理。
func (b *Baker) BakeBatch(reqs []BakeRequest, concurrency int) []BatchResult {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]BatchResult, len(reqs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				res, err := b.Bake(&reqs[i])
				results[i] = BatchResult{Index: i, Result: res, Err: err}
			}
		}()
	}
	for i := range reqs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}
