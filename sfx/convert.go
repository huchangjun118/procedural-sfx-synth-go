package sfx

import (
	"fmt"
	"math"
)

// Resample converts mono samples from srcRate to dstRate using
// deterministic linear interpolation. The output length is
// round(len(src) * dstRate / srcRate), so the output duration
// matches the source duration even for non-integer ratios: no edge
// samples are dropped or duplicated.
func Resample(src []float64, srcRate, dstRate int) ([]float64, error) {
	if srcRate <= 0 || dstRate <= 0 {
		return nil, &ValidationError{Field: "SampleRate",
			Reason: fmt.Sprintf("rates must be positive, got %d -> %d", srcRate, dstRate)}
	}
	if len(src) == 0 {
		return nil, &ValidationError{Field: "Samples", Reason: "source is empty"}
	}
	if srcRate == dstRate {
		return append([]float64(nil), src...), nil
	}
	n := int(math.Round(float64(len(src)) * float64(dstRate) / float64(srcRate)))
	if n < 1 {
		n = 1
	}
	out := make([]float64, n)
	// Map the output span exactly onto the source span so the first
	// and last source samples are preserved (no dropped or duplicated
	// edge samples).
	step := float64(len(src)-1) / float64(n-1)
	if n == 1 {
		step = 0
	}
	last := len(src) - 1
	for i := range out {
		pos := float64(i) * step
		j := int(pos)
		if j >= last {
			out[i] = src[last]
			continue
		}
		frac := pos - float64(j)
		out[i] = src[j] + frac*(src[j+1]-src[j])
	}
	return out, nil
}

// Quantize converts normalized float samples to the given bit depth
// and back to float. Out-of-range samples are hard-clamped to
// [-1, 1] first (the package clipping policy); the result is
// deterministic and free of NaN/Inf.
func Quantize(src []float64, bits int) ([]float64, error) {
	if bits < 2 || bits > 32 {
		return nil, &ValidationError{Field: "Bits",
			Reason: fmt.Sprintf("bit depth must be in [2, 32], got %d", bits)}
	}
	levels := float64(int64(1) << (bits - 1)) // quantization steps per polarity
	out := make([]float64, len(src))
	for i, x := range src {
		x = sanitize(x)
		x = clamp(x, -1, 1)
		out[i] = math.Round(x*levels) / levels
	}
	return out, nil
}

// ConvertChannels converts between mono (1) and stereo (2) with
// interleaved samples. Mono to stereo duplicates each sample;
// stereo to mono averages each pair. Other channel counts are
// rejected.
func ConvertChannels(src []float64, from, to int) ([]float64, error) {
	for _, c := range []int{from, to} {
		if c != 1 && c != 2 {
			return nil, &ValidationError{Field: "Channels",
				Reason: fmt.Sprintf("only 1 or 2 channels supported, got %d", c)}
		}
	}
	if len(src)%from != 0 {
		return nil, &ValidationError{Field: "Samples",
			Reason: fmt.Sprintf("length %d is not a multiple of %d channels", len(src), from)}
	}
	if from == to {
		return append([]float64(nil), src...), nil
	}
	if from == 1 { // mono -> stereo
		out := make([]float64, 2*len(src))
		for i, x := range src {
			out[2*i] = x
			out[2*i+1] = x
		}
		return out, nil
	}
	// stereo -> mono
	out := make([]float64, len(src)/2)
	for i := range out {
		out[i] = 0.5 * (src[2*i] + src[2*i+1])
	}
	return out, nil
}
