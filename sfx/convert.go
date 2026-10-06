package sfx

import "math"

// Resample 将单声道浮点样本从 srcRate 重采样到 dstRate（线性插值）。
// 输出帧数为 round(len(src) * dstRate / srcRate)，时长与源一致；
// 采样位置由输出序号直接推导，首尾不缺少、不重复采样，结果确定。
func Resample(src []float64, srcRate, dstRate int) []float64 {
	if srcRate == dstRate {
		out := make([]float64, len(src))
		copy(out, src)
		return out
	}
	if len(src) == 0 {
		return nil
	}
	n := int(math.Round(float64(len(src)) * float64(dstRate) / float64(srcRate)))
	if n < 1 {
		n = 1
	}
	out := make([]float64, n)
	step := float64(srcRate) / float64(dstRate)
	last := len(src) - 1
	for i := 0; i < n; i++ {
		pos := float64(i) * step
		j := int(pos)
		if j >= last {
			out[i] = src[last]
			continue
		}
		frac := pos - float64(j)
		out[i] = src[j]*(1-frac) + src[j+1]*frac
	}
	return out
}

// ConvertChannels 在单/双声道间转换：单->双复制，双->单取平均。
// 双声道样本按 L,R 交错排列。
func ConvertChannels(src []float64, from, to int) []float64 {
	if from == to {
		out := make([]float64, len(src))
		copy(out, src)
		return out
	}
	if from == 1 && to == 2 {
		out := make([]float64, len(src)*2)
		for i, v := range src {
			out[2*i] = v
			out[2*i+1] = v
		}
		return out
	}
	if from == 2 && to == 1 {
		n := len(src) / 2
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			out[i] = (src[2*i] + src[2*i+1]) * 0.5
		}
		return out
	}
	return nil
}

// clipSample 既定策略：超范围样本硬截断到 [-1, 1]。
func clipSample(x float64) float64 {
	return math.Max(-1, math.Min(1, x))
}

// Quantize 将浮点样本按位深量化为整型采样（8/16/24/32 位有符号）。
// 超范围样本按既定策略硬截断，非有限值替换为 0，结果确定。
func Quantize(src []float64, bitDepth int) []int32 {
	bits := uint(bitDepth - 1)
	max := float64(int64(1) << bits)
	out := make([]int32, len(src))
	for i, v := range src {
		v = clipSample(sanitize(v))
		q := math.Round(v * max)
		if q >= max {
			q = max - 1
		}
		if q < -max {
			q = -max
		}
		out[i] = int32(q)
	}
	return out
}
