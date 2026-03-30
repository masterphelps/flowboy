package anomaly

import (
	"testing"
	"time"
)

func TestManagerStartAnomaly(t *testing.T) {
	m := NewManager()
	scenario := AllScenarios()[0] // DDoS Flood
	id, err := m.Start(scenario, 10*time.Second, 20.0, nil, 50)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}
	active := m.Active()
	if len(active) != 1 {
		t.Fatalf("expected 1 active anomaly, got %d", len(active))
	}
	if active[0].ID != id {
		t.Errorf("expected ID %s, got %s", id, active[0].ID)
	}
}

func TestManagerStopAnomaly(t *testing.T) {
	m := NewManager()
	scenario := AllScenarios()[0]
	id, _ := m.Start(scenario, 10*time.Second, 20.0, nil, 50)
	err := m.Stop(id)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if len(m.Active()) != 0 {
		t.Error("expected 0 active anomalies after stop")
	}
}

func TestManagerClearAll(t *testing.T) {
	m := NewManager()
	m.Start(AllScenarios()[0], 10*time.Second, 20.0, nil, 50)
	m.Start(AllScenarios()[4], 10*time.Second, 5.0, nil, 0)
	if len(m.Active()) != 2 {
		t.Fatalf("expected 2 active, got %d", len(m.Active()))
	}
	m.ClearAll()
	if len(m.Active()) != 0 {
		t.Error("expected 0 after ClearAll")
	}
}

func TestManagerGetModifiers_BandwidthSpike(t *testing.T) {
	m := NewManager()
	m.Start(AllScenarios()[4], 60*time.Second, 5.0, nil, 0) // Bandwidth Spike
	mod := m.GetModifiers("any-flow", "any-machine")
	if mod.RateMultiplier != 5.0 {
		t.Errorf("expected rate multiplier 5.0, got %f", mod.RateMultiplier)
	}
}

func TestManagerGetModifiers_NoAnomaly(t *testing.T) {
	m := NewManager()
	mod := m.GetModifiers("any-flow", "any-machine")
	if mod.RateMultiplier != 1.0 {
		t.Errorf("expected rate multiplier 1.0, got %f", mod.RateMultiplier)
	}
}

func TestManagerGetModifiers_BlackoutZero(t *testing.T) {
	m := NewManager()
	m.Start(AllScenarios()[5], 30*time.Second, 0.0, nil, 0) // Traffic Blackout
	mod := m.GetModifiers("any-flow", "any-machine")
	if mod.RateMultiplier != 0.0 {
		t.Errorf("expected rate multiplier 0.0, got %f", mod.RateMultiplier)
	}
}

func TestManagerGetModifiers_TargetedExfil(t *testing.T) {
	m := NewManager()
	m.Start(AllScenarios()[3], 60*time.Second, 10.0, []string{"web-01"}, 0) // Data Exfiltration

	mod := m.GetModifiers("some-flow", "web-01")
	if mod.RateMultiplier != 10.0 {
		t.Errorf("expected 10.0 for targeted machine, got %f", mod.RateMultiplier)
	}

	mod2 := m.GetModifiers("other-flow", "db-01")
	if mod2.RateMultiplier != 1.0 {
		t.Errorf("expected 1.0 for non-targeted machine, got %f", mod2.RateMultiplier)
	}
}

func TestManagerStacking(t *testing.T) {
	m := NewManager()
	m.Start(AllScenarios()[4], 60*time.Second, 5.0, nil, 0)                        // bandwidth spike
	m.Start(AllScenarios()[3], 60*time.Second, 10.0, []string{"web-01"}, 0) // exfil

	mod := m.GetModifiers("some-flow", "web-01")
	if mod.RateMultiplier != 50.0 {
		t.Errorf("expected stacked multiplier 50.0, got %f", mod.RateMultiplier)
	}
}
