package sfx

import (
	"bytes"
	"testing"
)

func baseRequest() BakeRequest {
	return BakeRequest{
		Params: *baseParams(),
		Format: Format{SampleRate: 44100, BitDepth: 16, Channels: 1},
	}
}

// 参数未变化时复用缓存，结果与全量重建逐字节一致。
func TestBakeCacheReuseIdentical(t *testing.T) {
	b := NewBaker(nil)
	req := baseRequest()
	r1, err := b.Bake(&req)
	if err != nil {
		t.Fatal(err)
	}
	entriesAfterFirst := b.Cache().Len()
	r2, err := b.Bake(&req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r1.WAV, r2.WAV) || r1.Hash != r2.Hash {
		t.Fatal("cached bake differs from fresh bake")
	}
	if b.Cache().Len() != entriesAfterFirst {
		t.Fatal("unchanged params should not create new cache entries")
	}
	// 全新烘焙器（冷缓存）全量重建，结果必须逐字节一致
	fresh := NewBaker(nil)
	r3, err := fresh.Bake(&req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r1.WAV, r3.WAV) {
		t.Fatal("cold-cache rebuild differs from cached result")
	}
}

// 局部参数变化时只重建受影响的阶段。
func TestIncrementalRebake(t *testing.T) {
	b := NewBaker(nil)
	req := baseRequest()
	if _, err := b.Bake(&req); err != nil {
		t.Fatal(err)
	}
	base := b.Cache().Len()

	// 仅改包络：声源阶段应复用，只新增塑形与格式两个条目
	req.Params.Envelope.Decay = 0.3
	if _, err := b.Bake(&req); err != nil {
		t.Fatal(err)
	}
	if got := b.Cache().Len() - base; got != 2 {
		t.Fatalf("envelope change rebuilt %d stages, want 2", got)
	}

	// 仅改格式：只新增格式一个条目
	before := b.Cache().Len()
	req.Format.BitDepth = 24
	if _, err := b.Bake(&req); err != nil {
		t.Fatal(err)
	}
	if got := b.Cache().Len() - before; got != 1 {
		t.Fatalf("format change rebuilt %d stages, want 1", got)
	}

	// 改声源：三个阶段全部重建
	before = b.Cache().Len()
	req.Params.Osc.Freq = 440
	if _, err := b.Bake(&req); err != nil {
		t.Fatal(err)
	}
	if got := b.Cache().Len() - before; got != 3 {
		t.Fatalf("source change rebuilt %d stages, want 3", got)
	}
}

// 并发批量烘焙：任意并发度下结果逐字节一致。
func TestBatchConcurrencyDeterministic(t *testing.T) {
	var reqs []BakeRequest
	for i := 0; i < 12; i++ {
		r := baseRequest()
		r.Params.Seed = int64(i)
		r.Params.Osc.Freq = 200 + float64(i)*50
		reqs = append(reqs, r)
	}
	var reference []BatchResult
	for _, conc := range []int{1, 4, 16} {
		b := NewBaker(nil)
		got := b.BakeBatch(reqs, conc)
		if len(got) != len(reqs) {
			t.Fatalf("conc %d: got %d results", conc, len(got))
		}
		for i, res := range got {
			if res.Index != i {
				t.Fatalf("conc %d: result %d has index %d", conc, i, res.Index)
			}
			if res.Err != nil {
				t.Fatalf("conc %d task %d failed: %v", conc, i, res.Err)
			}
		}
		if reference == nil {
			reference = got
			continue
		}
		for i := range got {
			if !bytes.Equal(reference[i].Result.WAV, got[i].Result.WAV) {
				t.Fatalf("conc %d: task %d differs from sequential result", conc, i)
			}
		}
	}
}

// 单个任务失败不影响其他任务，失败原因可定位。
func TestBatchFailureIsolation(t *testing.T) {
	var reqs []BakeRequest
	for i := 0; i < 5; i++ {
		reqs = append(reqs, baseRequest())
	}
	reqs[2].Params.Osc.Freq = -1 // 非法：必须被拒绝
	b := NewBaker(nil)
	results := b.BakeBatch(reqs, 4)
	for i, res := range results {
		if i == 2 {
			if res.Err == nil {
				t.Fatal("invalid task should fail")
			}
			if res.Result != nil {
				t.Fatal("failed task should not produce result")
			}
			want := "osc.freq"
			if got := res.Err.Error(); !bytes.Contains([]byte(got), []byte(want)) {
				t.Fatalf("error not locatable, want %q in %q", want, got)
			}
			continue
		}
		if res.Err != nil || res.Result == nil {
			t.Fatalf("task %d affected by sibling failure: %v", i, res.Err)
		}
	}
}

// 烘焙产物是合法 WAV 且内容随参数变化。
func TestBakeProducesDistinctContent(t *testing.T) {
	b := NewBaker(nil)
	var hashes []string
	for i := 0; i < 3; i++ {
		req := baseRequest()
		req.Params.Osc.Freq = 220 * float64(i+1)
		res, err := b.Bake(&req)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(res.WAV[:4], []byte("RIFF")) {
			t.Fatal("output is not a WAV file")
		}
		hashes = append(hashes, res.Hash)
	}
	seen := map[string]bool{}
	for _, h := range hashes {
		if seen[h] {
			t.Fatal("different params produced identical hash")
		}
		seen[h] = true
	}
}

// 非法格式必须被拒绝且原因可定位。
func TestBakeRejectsBadFormat(t *testing.T) {
	b := NewBaker(nil)
	req := baseRequest()
	req.Format.BitDepth = 12
	_, err := b.Bake(&req)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("format.bitDepth")) {
		t.Fatalf("expected locatable format error, got %v", err)
	}
}
