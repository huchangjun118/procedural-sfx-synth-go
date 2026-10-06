package sfx

import (
	"math"
	"testing"
)

// 非整数倍重采样：输出时长与源一致，首尾不缺少、不重复采样。
func TestResampleNonIntegerRatio(t *testing.T) {
	srcRate, dstRate := 44100, 16000
	n := 4410 // 0.1s
	src := make([]float64, n)
	for i := range src {
		src[i] = math.Sin(2 * math.Pi * 440 * float64(i) / float64(srcRate))
	}
	out := Resample(src, srcRate, dstRate)
	wantLen := int(math.Round(float64(n) * float64(dstRate) / float64(srcRate)))
	if len(out) != wantLen {
		t.Fatalf("length %d, want %d (duration must be preserved)", len(out), wantLen)
	}
	// 首样本必须保留
	if out[0] != src[0] {
		t.Fatalf("first sample lost: %v vs %v", out[0], src[0])
	}
	// 末样本不得越界复制出重复尾：最后一个输出对应源末尾
	if math.Abs(out[len(out)-1]-src[len(src)-1]) > 0.2 {
		t.Fatalf("tail mismatch: %v vs %v", out[len(out)-1], src[len(src)-1])
	}
	// 时长一致：输出时长与源时长之差不超过一个输出采样
	srcDur := float64(n) / float64(srcRate)
	outDur := float64(len(out)) / float64(dstRate)
	if math.Abs(srcDur-outDur) > 1.0/float64(dstRate) {
		t.Fatalf("duration drift: %v vs %v", srcDur, outDur)
	}
	// 确定性
	out2 := Resample(src, srcRate, dstRate)
	if !floatsEqual(out, out2) {
		t.Fatal("resample not deterministic")
	}
}

// 同采样率重采样为恒等拷贝。
func TestResampleIdentity(t *testing.T) {
	src := []float64{0.1, -0.5, 0.9}
	out := Resample(src, 44100, 44100)
	if !floatsEqual(src, out) {
		t.Fatal("identity resample altered samples")
	}
}

func TestConvertChannels(t *testing.T) {
	mono := []float64{0.5, -0.5}
	st := ConvertChannels(mono, 1, 2)
	if len(st) != 4 || st[0] != 0.5 || st[1] != 0.5 || st[2] != -0.5 || st[3] != -0.5 {
		t.Fatalf("mono->stereo wrong: %v", st)
	}
	back := ConvertChannels(st, 2, 1)
	if !floatsEqual(mono, back) {
		t.Fatalf("stereo->mono wrong: %v", back)
	}
}

// 量化：超范围样本按既定策略硬截断，非有限值替换为 0。
func TestQuantizeClipPolicy(t *testing.T) {
	src := []float64{0, 0.5, -0.5, 1.5, -1.5, math.NaN(), math.Inf(1)}
	q := Quantize(src, 16)
	max := int32(1 << 15)
	if q[0] != 0 || q[1] != 1<<14 || q[2] != -(1<<14) {
		t.Fatalf("unexpected quantization: %v", q[:3])
	}
	if q[3] != max-1 || q[4] != -max {
		t.Fatalf("clip policy violated: %v %v", q[3], q[4])
	}
	if q[5] != 0 || q[6] != 0 {
		t.Fatalf("non-finite not sanitized: %v %v", q[5], q[6])
	}
	// 各 bit 深度均可量化且不越界
	for _, bd := range []int{8, 16, 24, 32} {
		out := Quantize(src, bd)
		lim := int64(1) << uint(bd-1)
		for i, v := range out {
			if int64(v) >= lim || int64(v) < -lim {
				t.Fatalf("bitDepth %d sample %d out of range: %d", bd, i, v)
			}
		}
	}
}
