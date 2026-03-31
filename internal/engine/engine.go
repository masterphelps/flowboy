package engine

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/masterphelps/flowboy/internal/anomaly"
	"github.com/masterphelps/flowboy/internal/config"
)

// FlowStats holds real-time statistics for a single flow.
type FlowStats struct {
	FlowName    string
	BytesSent   uint64
	PacketsSent uint64
	Active      bool
}

// Engine manages flow goroutines and record generation.
type Engine struct {
	mu       sync.RWMutex
	machines map[string]config.Machine
	flows    map[string]*flowRunner
	records  chan []byte    // encoded NetFlow v9 data records
	stats    chan FlowStats // real-time stats for UI
	running           bool
	stopCh            chan struct{}
	globalFluctuation *config.Fluctuation
	anomalyMgr        *anomaly.Manager
}

// flowRunner manages a single flow's goroutine.
type flowRunner struct {
	flow      config.Flow
	src       config.Machine
	dst       config.Machine
	rate      config.Rate
	stopCh    chan struct{}
	connState *connState
}

// New creates a new Engine instance.
func New() *Engine {
	return &Engine{
		machines:   make(map[string]config.Machine),
		flows:      make(map[string]*flowRunner),
		records:    make(chan []byte, 4096),
		stats:      make(chan FlowStats, 256),
		anomalyMgr: anomaly.NewManager(),
	}
}

// Start begins the engine. If already running, this is a no-op.
func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return
	}
	e.running = true
	e.stopCh = make(chan struct{})

	// Start goroutines for any flows already added while engine was stopped.
	for _, fr := range e.flows {
		if fr.flow.Enabled {
			go fr.run(e.records, e.stats, e.stopCh, e.globalFluctuation, e.anomalyMgr)
		}
	}
}

// Stop shuts down the engine and all running flows.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}
	e.running = false
	close(e.stopCh)
}

// Running reports whether the engine is currently running.
func (e *Engine) Running() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// AddMachine registers a machine with the engine.
func (e *Engine) AddMachine(m config.Machine) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.machines[m.Name] = m
}

// RemoveMachine removes a machine by name.
func (e *Engine) RemoveMachine(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.machines, name)
}

// UpdateMachine replaces a machine entry. If the old name differs from the
// new config's Name, the old entry is removed.
func (e *Engine) UpdateMachine(oldName string, m config.Machine) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if oldName != m.Name {
		delete(e.machines, oldName)
	}
	e.machines[m.Name] = m
}

// Machines returns a snapshot of all registered machines.
func (e *Engine) Machines() []config.Machine {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]config.Machine, 0, len(e.machines))
	for _, m := range e.machines {
		result = append(result, m)
	}
	return result
}

// AddFlow validates machine references, parses the rate, and starts the
// flow's goroutine if the engine is currently running.
func (e *Engine) AddFlow(f config.Flow) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	src, ok := e.machines[f.SourceName]
	if !ok {
		return fmt.Errorf("source machine %q not found", f.SourceName)
	}
	dst, ok := e.machines[f.DestName]
	if !ok {
		return fmt.Errorf("destination machine %q not found", f.DestName)
	}

	rate, err := config.ParseRate(f.Rate)
	if err != nil {
		return fmt.Errorf("invalid rate for flow %q: %w", f.Name, err)
	}

	fr := &flowRunner{
		flow:      f,
		src:       src,
		dst:       dst,
		rate:      rate,
		stopCh:    make(chan struct{}),
		connState: newConnState(f.ConnectionStyle),
	}

	e.flows[f.Name] = fr

	if e.running && f.Enabled {
		go fr.run(e.records, e.stats, e.stopCh, e.globalFluctuation, e.anomalyMgr)
	}

	return nil
}

// RemoveFlow stops and removes a flow by name.
func (e *Engine) RemoveFlow(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	fr, ok := e.flows[name]
	if !ok {
		return fmt.Errorf("flow %q not found", name)
	}
	close(fr.stopCh)
	delete(e.flows, name)
	return nil
}

// FlowCount returns the number of flows registered in the engine.
func (e *Engine) FlowCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.flows)
}

// Flows returns a snapshot of all registered flows.
func (e *Engine) Flows() []config.Flow {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]config.Flow, 0, len(e.flows))
	for _, fr := range e.flows {
		result = append(result, fr.flow)
	}
	return result
}

// Records returns a read-only channel of encoded NetFlow data records.
func (e *Engine) Records() <-chan []byte {
	return e.records
}

// StartAnomaly activates an anomaly scenario.
func (e *Engine) StartAnomaly(scenario anomaly.Scenario, duration time.Duration, intensity float64, targets []string, count int) (string, error) {
	return e.anomalyMgr.Start(scenario, duration, intensity, targets, count)
}

// StopAnomaly stops a specific anomaly by ID.
func (e *Engine) StopAnomaly(id string) error {
	return e.anomalyMgr.Stop(id)
}

// ClearAnomalies stops all active anomalies.
func (e *Engine) ClearAnomalies() {
	e.anomalyMgr.ClearAll()
}

// ActiveAnomalies returns all currently active anomalies.
func (e *Engine) ActiveAnomalies() []anomaly.ActiveAnomaly {
	return e.anomalyMgr.Active()
}

// AnomalyManager returns the anomaly manager for direct access.
func (e *Engine) AnomalyManager() *anomaly.Manager {
	return e.anomalyMgr
}

// SetGlobalFluctuation sets the global fluctuation defaults.
func (e *Engine) SetGlobalFluctuation(f *config.Fluctuation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.globalFluctuation = f
}

// GlobalFluctuation returns the current global fluctuation settings.
func (e *Engine) GlobalFluctuation() *config.Fluctuation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.globalFluctuation
}

// Stats returns a read-only channel of flow statistics.
func (e *Engine) Stats() <-chan FlowStats {
	return e.stats
}

// run is the main loop for a single flow's goroutine. It generates NetFlow v9
// data records at the flow's ActiveTimeout interval.
func (fr *flowRunner) run(records chan<- []byte, stats chan<- FlowStats, engineStop <-chan struct{}, globalFluct *config.Fluctuation, anomalyMgr *anomaly.Manager) {
	timeout := fr.flow.ActiveTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ticker := time.NewTicker(timeout)
	defer ticker.Stop()

	var totalBytes uint64
	var totalPackets uint64

	for {
		select {
		case <-fr.stopCh:
			return
		case <-engineStop:
			return
		case <-ticker.C:
			now := time.Now()
			octets := fluctuateRate(fr.rate, timeout, now, fr.flow.Fluctuation, globalFluct)
			packets := octets / 1500
			if packets == 0 {
				packets = 1
			}

			totalBytes += octets
			totalPackets += packets

			tcpFlags := fr.connState.nextFlags()

			// Apply anomaly modifiers
			if anomalyMgr != nil {
				mod := anomalyMgr.GetModifiers(fr.flow.Name, fr.flow.SourceName)
				octets = uint64(float64(octets) * mod.RateMultiplier)
				if mod.FlagOverride != nil {
					tcpFlags = *mod.FlagOverride
				}
			}

			rec := V9DataRecord{
				SrcAddr:   ipTo4(fr.src.IP),
				DstAddr:   ipTo4(fr.dst.IP),
				SrcPort:   fr.flow.SourcePort,
				DstPort:   fr.flow.DestPort,
				Protocol:  protocolNumber(fr.flow.Protocol),
				SrcMask:   maskPrefixLen(fr.src.Mask),
				DstMask:   maskPrefixLen(fr.dst.Mask),
				Octets:    uint32(octets),
				Packets:   uint32(packets),
				TCPFlags:  tcpFlags,
				FirstSeen: uint32(now.Add(-timeout).UnixMilli() & 0xFFFFFFFF),
				LastSeen:  uint32(now.UnixMilli() & 0xFFFFFFFF),
				AppID:     fr.flow.AppID,
			}

			encoded := rec.Encode()

			// Non-blocking send to records channel.
			select {
			case records <- encoded:
			default:
				// Drop if channel full — back-pressure.
			}

			// Non-blocking send to stats channel.
			select {
			case stats <- FlowStats{
				FlowName:    fr.flow.Name,
				BytesSent:   totalBytes,
				PacketsSent: totalPackets,
				Active:      true,
			}:
			default:
			}
		}
	}
}

// ipTo4 converts a net.IP to a 4-byte array.
func ipTo4(ip net.IP) [4]byte {
	var addr [4]byte
	v4 := ip.To4()
	if v4 != nil {
		copy(addr[:], v4)
	}
	return addr
}

// protocolNumber converts a protocol name to its IANA number.
func protocolNumber(name string) uint8 {
	switch strings.ToUpper(name) {
	case "TCP":
		return 6
	case "UDP":
		return 17
	case "ICMP":
		return 1
	default:
		return 0
	}
}

// maskPrefixLen returns the prefix length (e.g. 24) from a net.IPMask.
func maskPrefixLen(mask net.IPMask) uint8 {
	ones, _ := mask.Size()
	return uint8(ones)
}
