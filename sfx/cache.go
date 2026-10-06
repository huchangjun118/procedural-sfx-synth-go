package sfx

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sync"
)

// SegmentSamples is the fixed segment length used for incremental
// caching. A parameter change at time t invalidates only segments
// whose relevant-parameter hash changed, i.e. segments at or after t.
const SegmentSamples = 4096

// rendererState is a full snapshot of the streaming state; restoring
// it makes the renderer continue exactly as if it had rendered all
// preceding samples itself.
type rendererState struct {
	pos                int
	oscPhase           float64
	lfoPhase           float64
	noiseState         uint64
	bx1, bx2, by1, by2 float64
}

func (r *Renderer) snapshot() rendererState {
	return rendererState{
		pos: r.pos, oscPhase: r.oscPhase, lfoPhase: r.lfoPhase,
		noiseState: r.noise.state,
		bx1:        r.bx1, bx2: r.bx2, by1: r.by1, by2: r.by2,
	}
}

func (r *Renderer) restore(s rendererState) {
	r.pos, r.oscPhase, r.lfoPhase = s.pos, s.oscPhase, s.lfoPhase
	r.noise.state = s.noiseState
	r.bx1, r.bx2, r.by1, r.by2 = s.bx1, s.bx2, s.by1, s.by2
}

type cacheEntry struct {
	samples []float64
	after   rendererState
}

// Cache stores baked segments keyed by content hash. Segments whose
// relevant parameters are unchanged are reused verbatim, so a cached
// bake is byte-identical to a full re-render.
type Cache struct {
	mu      sync.Mutex
	entries map[uint64]cacheEntry
	hits    int
	misses  int
}

// NewCache creates an empty content cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[uint64]cacheEntry)}
}

// Stats reports cache hits and misses since creation.
func (c *Cache) Stats() (hits, misses int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

// Bake renders params, reusing cached segments where possible. The
// result is byte-identical to a full Render of the same params.
func (c *Cache) Bake(p *Params) ([]float64, error) {
	r, err := NewRenderer(p)
	if err != nil {
		return nil, err
	}
	total := r.TotalSamples()
	out := make([]float64, 0, total)
	for start := 0; start < total; start += SegmentSamples {
		end := start + SegmentSamples
		if end > total {
			end = total
		}
		key := segmentKey(p, start)
		c.mu.Lock()
		entry, ok := c.entries[key]
		if ok {
			c.hits++
		} else {
			c.misses++
		}
		c.mu.Unlock()

		if ok {
			out = append(out, entry.samples...)
			r.restore(entry.after)
			continue
		}
		buf := make([]float64, end-start)
		r.RenderBlock(buf)
		entry = cacheEntry{samples: buf, after: r.snapshot()}
		c.mu.Lock()
		c.entries[key] = entry
		c.mu.Unlock()
		out = append(out, buf...)
	}
	return out, nil
}

// segmentKey hashes the static parameters plus the automation
// breakpoints relevant to the segment starting at sample `start`:
// all points before the segment end, plus the first point at or
// after it (it influences interpolation inside the segment).
func segmentKey(p *Params, start int) uint64 {
	sr := float64(p.SampleRate)
	segEndSec := float64(start+SegmentSamples) / sr
	h := fnv.New64a()
	var b [8]byte
	putF := func(v float64) { binary.LittleEndian.PutUint64(b[:], math.Float64bits(v)); h.Write(b[:]) }
	putI := func(v int64) { binary.LittleEndian.PutUint64(b[:], uint64(v)); h.Write(b[:]) }

	putI(p.Seed)
	putI(int64(p.SampleRate))
	putF(p.DurationSec)
	putI(int64(p.Oscillator.Waveform))
	putF(p.Oscillator.FreqHz)
	putF(p.Oscillator.Gain)
	putI(boolInt(p.Noise.Enabled))
	putF(p.Noise.Gain)
	putF(p.Envelope.Attack)
	putF(p.Envelope.Decay)
	putF(p.Envelope.Sustain)
	putF(p.Envelope.Release)
	putI(int64(p.Filter.Type))
	putF(p.Filter.CutoffHz)
	putF(p.Filter.Resonance)
	putI(int64(p.LFO.Target))
	putI(int64(p.LFO.Waveform))
	putF(p.LFO.FreqHz)
	putF(p.LFO.Depth)
	putF(p.Drive.Amount)
	putF(p.Drive.Mix)
	putAutomation := func(bps []Breakpoint) {
		n := 0
		for _, bp := range bps {
			if bp.TimeSec < segEndSec {
				n++
			}
		}
		count := n
		if n < len(bps) {
			count++ // the first point at/after segEndSec is relevant too
		}
		putI(int64(count))
		for i := 0; i < count; i++ {
			putF(bps[i].TimeSec)
			putF(bps[i].Value)
		}
	}
	putAutomation(p.GainAutomation)
	putAutomation(p.CutoffAutomation)
	putI(int64(start))
	return h.Sum64()
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
