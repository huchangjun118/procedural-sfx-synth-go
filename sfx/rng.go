package sfx

// rng is a deterministic pseudo-random generator (splitmix64).
// It is self-contained so results depend only on the seed, never on
// the Go runtime, process-global state, or call concurrency.
type rng struct {
	state uint64
}

func newRNG(seed int64) *rng {
	return &rng{state: uint64(seed)}
}

// next advances the state and returns the new 64-bit value.
func (r *rng) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// bipolar returns a deterministic value in [-1, 1).
func (r *rng) bipolar() float64 {
	// Use the top 53 bits to build a uniform [0,1) then shift.
	v := float64(r.next()>>11) * (1.0 / (1 << 53))
	return 2*v - 1
}
