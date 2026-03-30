package engine

import (
	"testing"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
)

func TestFluctuateRate_NoFluctuation(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 100_000_000}
	interval := 60 * time.Second
	result := fluctuateRate(rate, interval, time.Now(), nil, nil)
	bpi := rate.BytesPerInterval(interval)
	// Without fluctuation, should return base rate within jitter range ±5%
	if result < bpi*90/100 || result > bpi*110/100 {
		t.Errorf("expected ~%d bytes, got %d (outside ±10%% tolerance)", bpi, result)
	}
}

func TestFluctuateRate_WithFluctuation(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 100_000_000}
	period := time.Hour
	fluct := &config.Fluctuation{
		Amplitude: 0.5,
		Period:    period,
		Phase:     0,
	}
	interval := 60 * time.Second

	// Pick a base time that lands exactly at the start of a period cycle
	// (Unix time mod period == 0)
	epoch := time.Unix(0, 0)
	periodNs := int64(period)
	nowNs := time.Now().UnixNano()
	alignedNs := nowNs - (nowNs % periodNs)
	baseTime := epoch.Add(time.Duration(alignedNs))

	// At cycle start: sin(0)=0, rate should be near mean
	result := fluctuateRate(rate, interval, baseTime, fluct, nil)
	bpi := rate.BytesPerInterval(interval)
	if result < bpi*85/100 || result > bpi*115/100 {
		t.Errorf("at sin(0), expected ~%d bytes, got %d", bpi, result)
	}

	// At t=period/4: sin(pi/2)=1, rate should be near mean * 1.5
	quarterTime := baseTime.Add(period / 4)
	resultHigh := fluctuateRate(rate, interval, quarterTime, fluct, nil)
	expected := uint64(float64(bpi) * 1.5)
	tolerance := uint64(float64(expected) * 0.15)
	if resultHigh < expected-tolerance || resultHigh > expected+tolerance {
		t.Errorf("at sin(pi/2), expected ~%d bytes, got %d", expected, resultHigh)
	}
}

func TestFluctuateRate_ZeroRate(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 0}
	result := fluctuateRate(rate, 60*time.Second, time.Now(), nil, nil)
	if result != 0 {
		t.Errorf("expected 0 for zero rate, got %d", result)
	}
}

func TestFluctuateRate_GlobalFallback(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 100_000_000}
	period := time.Hour
	global := &config.Fluctuation{
		Amplitude: 1.0,
		Period:    period,
		Phase:     0,
	}
	interval := 60 * time.Second

	// Align to period boundary, then add period/4 for sin(pi/2)=1
	epoch := time.Unix(0, 0)
	periodNs := int64(period)
	nowNs := time.Now().UnixNano()
	alignedNs := nowNs - (nowNs % periodNs)
	quarterTime := epoch.Add(time.Duration(alignedNs) + period/4)

	result := fluctuateRate(rate, interval, quarterTime, nil, global)
	bpi := rate.BytesPerInterval(interval)
	expected := uint64(float64(bpi) * 2.0) // mean + 1.0*mean at sin(pi/2)
	tolerance := uint64(float64(expected) * 0.15)
	if result < expected-tolerance || result > expected+tolerance {
		t.Errorf("with global fluctuation, expected ~%d bytes, got %d", expected, result)
	}
}
