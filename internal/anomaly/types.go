package anomaly

import "time"

// AnomalyType identifies a class of anomaly.
type AnomalyType string

const (
	DDoSFlood       AnomalyType = "ddos"
	PortScan        AnomalyType = "port_scan"
	LateralMovement AnomalyType = "lateral_movement"
	DataExfil       AnomalyType = "data_exfiltration"
	BandwidthSpike  AnomalyType = "bandwidth_spike"
	TrafficBlackout AnomalyType = "traffic_blackout"
	ProtocolAnomaly AnomalyType = "protocol_anomaly"
	Beaconing       AnomalyType = "beaconing"
	RandomChaos     AnomalyType = "random_chaos"
)

// AnomalyCategory groups anomalies for color-coding in the TUI.
type AnomalyCategory string

const (
	CategoryAttack  AnomalyCategory = "attack"  // red: DDoS, port scan, lateral movement
	CategoryVolume  AnomalyCategory = "volume"  // yellow: exfil, bandwidth spike, blackout
	CategoryPattern AnomalyCategory = "pattern" // cyan: protocol, beaconing, chaos
)

// Scenario defines a predefined anomaly template with tweakable defaults.
type Scenario struct {
	Type             AnomalyType
	Category         AnomalyCategory
	Name             string
	Description      string
	DefaultDuration  time.Duration
	DefaultIntensity float64 // rate multiplier
	DefaultTargets   []string
	DefaultCount     int // synthetic flow/port count
}

// ActiveAnomaly is a running anomaly instance.
type ActiveAnomaly struct {
	ID        string
	Scenario  Scenario
	StartTime time.Time
	Duration  time.Duration
	Intensity float64
	Targets   []string
	Count     int
}

// Remaining returns how long until the anomaly expires.
func (a *ActiveAnomaly) Remaining() time.Duration {
	elapsed := time.Since(a.StartTime)
	rem := a.Duration - elapsed
	if rem < 0 {
		return 0
	}
	return rem
}

// Expired returns true if the anomaly's duration has elapsed.
func (a *ActiveAnomaly) Expired() bool {
	return time.Since(a.StartTime) >= a.Duration
}

// FlowModifier describes how an anomaly modifies a flow's output.
type FlowModifier struct {
	RateMultiplier float64 // 1.0 = no change
	FlagOverride   *uint8  // nil = no override
}
