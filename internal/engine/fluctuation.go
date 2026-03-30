package engine

import (
	"math"
	"math/rand"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
)

const (
	jitterRange = 0.05 // ±5%
)

// fluctuateRate computes bytes for one interval using sine-wave fluctuation.
// perFlow takes priority over global. If both are nil, returns base rate + jitter.
func fluctuateRate(rate config.Rate, interval time.Duration, now time.Time, perFlow, global *config.Fluctuation) uint64 {
	bpi := rate.BytesPerInterval(interval)
	if bpi == 0 {
		return 0
	}

	fluct := perFlow
	if fluct == nil {
		fluct = global
	}

	base := float64(bpi)

	if fluct != nil && fluct.Period > 0 {
		// Use Unix nanoseconds + phase offset, mod period, to get position in cycle
		t := now.UnixNano() + int64(fluct.Phase)
		pos := t % int64(fluct.Period)
		phase := 2 * math.Pi * float64(pos) / float64(fluct.Period)
		base = base + base*fluct.Amplitude*math.Sin(phase)
	}

	// Apply jitter: ±5%
	jitter := 1.0 + (rand.Float64()*2-1)*jitterRange
	base *= jitter

	if base < 0 {
		base = 0
	}

	return uint64(base)
}
