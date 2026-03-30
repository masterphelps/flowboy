package anomaly

import (
	"fmt"
	"sync"
	"time"
)

// Manager tracks active anomalies and computes modifiers for flow runners.
type Manager struct {
	mu      sync.Mutex
	active  map[string]*ActiveAnomaly
	counter int
}

// NewManager creates an anomaly manager.
func NewManager() *Manager {
	return &Manager{
		active: make(map[string]*ActiveAnomaly),
	}
}

// Start activates an anomaly with the given parameters. Returns the anomaly ID.
func (m *Manager) Start(scenario Scenario, duration time.Duration, intensity float64, targets []string, count int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	id := fmt.Sprintf("anomaly-%d", m.counter)

	m.active[id] = &ActiveAnomaly{
		ID:        id,
		Scenario:  scenario,
		StartTime: time.Now(),
		Duration:  duration,
		Intensity: intensity,
		Targets:   targets,
		Count:     count,
	}

	return id, nil
}

// Stop removes a specific anomaly by ID.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.active[id]; !ok {
		return fmt.Errorf("anomaly %q not found", id)
	}
	delete(m.active, id)
	return nil
}

// ClearAll removes all active anomalies.
func (m *Manager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = make(map[string]*ActiveAnomaly)
}

// Active returns a snapshot of all active anomalies, pruning expired ones.
func (m *Manager) Active() []ActiveAnomaly {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Prune expired
	for id, a := range m.active {
		if a.Expired() {
			delete(m.active, id)
		}
	}

	result := make([]ActiveAnomaly, 0, len(m.active))
	for _, a := range m.active {
		result = append(result, *a)
	}
	return result
}

// GetModifiers computes the combined modifier for a given flow and source machine.
// Multiple anomalies compose multiplicatively for rate.
func (m *Manager) GetModifiers(flowName, sourceMachine string) FlowModifier {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod := FlowModifier{RateMultiplier: 1.0}

	for _, a := range m.active {
		if a.Expired() {
			continue
		}

		if !affectsFlow(a, sourceMachine) {
			continue
		}

		switch a.Scenario.Type {
		case BandwidthSpike:
			mod.RateMultiplier *= a.Intensity
		case DataExfil:
			mod.RateMultiplier *= a.Intensity
		case TrafficBlackout:
			mod.RateMultiplier = 0.0
		case DDoSFlood:
			synFlag := uint8(0x02)
			mod.FlagOverride = &synFlag
			mod.RateMultiplier *= a.Intensity
		case PortScan:
			synRst := uint8(0x06)
			mod.FlagOverride = &synRst
		case LateralMovement:
			synAck := uint8(0x12)
			mod.FlagOverride = &synAck
		case RandomChaos:
			mod.RateMultiplier *= a.Intensity
		}
	}

	return mod
}

// affectsFlow returns true if the anomaly targets the given machine.
func affectsFlow(a *ActiveAnomaly, sourceMachine string) bool {
	if len(a.Targets) == 0 {
		return true
	}
	for _, target := range a.Targets {
		if target == sourceMachine {
			return true
		}
	}
	return false
}
