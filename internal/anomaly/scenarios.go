package anomaly

import "time"

// AllScenarios returns the 9 predefined anomaly scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		{
			Type:             DDoSFlood,
			Category:         CategoryAttack,
			Name:             "DDoS Flood",
			Description:      "Massive SYN flood targeting a machine",
			DefaultDuration:  60 * time.Second,
			DefaultIntensity: 20.0,
			DefaultCount:     50,
		},
		{
			Type:             PortScan,
			Category:         CategoryAttack,
			Name:             "Port Scan",
			Description:      "Sequential port scan against a target",
			DefaultDuration:  30 * time.Second,
			DefaultIntensity: 1.0,
			DefaultCount:     100,
		},
		{
			Type:             LateralMovement,
			Category:         CategoryAttack,
			Name:             "Lateral Movement",
			Description:      "New connections between unusual machine pairs",
			DefaultDuration:  120 * time.Second,
			DefaultIntensity: 1.0,
			DefaultCount:     3,
		},
		{
			Type:             DataExfil,
			Category:         CategoryVolume,
			Name:             "Data Exfiltration",
			Description:      "Sudden outbound data spike from a machine",
			DefaultDuration:  90 * time.Second,
			DefaultIntensity: 10.0,
			DefaultCount:     0,
		},
		{
			Type:             BandwidthSpike,
			Category:         CategoryVolume,
			Name:             "Bandwidth Spike",
			Description:      "All flows ramp up simultaneously",
			DefaultDuration:  60 * time.Second,
			DefaultIntensity: 5.0,
			DefaultCount:     0,
		},
		{
			Type:             TrafficBlackout,
			Category:         CategoryVolume,
			Name:             "Traffic Blackout",
			Description:      "Flows drop to near-zero (outage simulation)",
			DefaultDuration:  30 * time.Second,
			DefaultIntensity: 0.0,
			DefaultCount:     0,
		},
		{
			Type:             ProtocolAnomaly,
			Category:         CategoryPattern,
			Name:             "Protocol Anomaly",
			Description:      "Unexpected UDP flows on normally TCP-only pairs",
			DefaultDuration:  60 * time.Second,
			DefaultIntensity: 1.0,
			DefaultCount:     1,
		},
		{
			Type:             Beaconing,
			Category:         CategoryPattern,
			Name:             "Beaconing",
			Description:      "Regular low-volume periodic connections (C2 pattern)",
			DefaultDuration:  300 * time.Second,
			DefaultIntensity: 1.0,
			DefaultCount:     1,
		},
		{
			Type:             RandomChaos,
			Category:         CategoryPattern,
			Name:             "Random Chaos",
			Description:      "Random flows appear and disappear unpredictably",
			DefaultDuration:  60 * time.Second,
			DefaultIntensity: 1.0,
			DefaultCount:     5,
		},
	}
}
