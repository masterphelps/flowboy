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
		floor, ceiling := fluct.EffectiveRange()
		// sin ranges -1 to 1; map to floor..ceiling
		t := now.UnixNano() + int64(fluct.Phase)
		pos := t % int64(fluct.Period)
		sinVal := math.Sin(2 * math.Pi * float64(pos) / float64(fluct.Period))
		// Map sin [-1,1] to [floor, ceiling]
		mid := (floor + ceiling) / 2
		halfRange := (ceiling - floor) / 2
		multiplier := mid + halfRange*sinVal
		base = base * multiplier
	}

	// Apply jitter: ±5%
	jitter := 1.0 + (rand.Float64()*2-1)*jitterRange
	base *= jitter

	if base < 0 {
		base = 0
	}

	return uint64(base)
}
