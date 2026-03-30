# Traffic Fluctuation, TCP Flags & Anomaly System — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add sine-wave traffic fluctuation, TCP flag support, and a 9-scenario anomaly injection system to flowboy for demo/presentation use.

**Architecture:** Three layers built bottom-up: (1) config types and fluctuation math, (2) TCP flags in templates and engine, (3) anomaly manager with synthetic flows. TUI and Web UI gain an anomaly picker, status bar indicators, and flow highlighting.

**Tech Stack:** Go 1.26+, charmbracelet/bubbletea, gopkg.in/yaml.v3, no new dependencies.

**Scope note — Synthetic Flows:** This plan implements the anomaly manager, modifiers for existing flows, and full TUI/Web UI. Six scenarios (DDoS, port scan, lateral movement, protocol anomaly, beaconing, random chaos) spec synthetic flow creation — temporary flow runners that don't exist in config. This plan's manager infrastructure supports synthetic flows (the `Count` parameter, target selection), but the actual goroutine creation/cleanup is deferred to a Phase 2 follow-up. In Phase 1, these scenarios modify existing flows with rate multipliers and flag overrides, which is sufficient for demo impact (DDoS shows SYN-only flag + rate spike on existing flows, port scan shows SYN+RST flags, etc.). Phase 2 adds the synthetic flow spawning to make these scenarios fully realistic.

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `internal/engine/fluctuation.go` | Sine-wave rate calculation with jitter |
| `internal/engine/fluctuation_test.go` | Tests for fluctuation math |
| `internal/engine/tcpflags.go` | TCP flag constants and connection phase state machine |
| `internal/engine/tcpflags_test.go` | Tests for flag logic |
| `internal/anomaly/types.go` | AnomalyType, Scenario, ActiveAnomaly, FlowModifier types |
| `internal/anomaly/scenarios.go` | 9 predefined scenario definitions |
| `internal/anomaly/manager.go` | AnomalyManager: start/stop/clear anomalies, compute modifiers, manage synthetic flows |
| `internal/anomaly/manager_test.go` | Tests for anomaly manager |
| `internal/tui/anomaly.go` | TUI anomaly picker view and tweak form |

### Modified files
| File | Changes |
|------|---------|
| `internal/config/types.go` | Add `Fluctuation` struct, `ConnectionStyle` type |
| `internal/config/config.go` | Add `Fluctuation` and `ConnectionStyle` to `FlowConfig` and `Config` |
| `internal/config/types_test.go` | Tests for new config fields |
| `internal/engine/templates.go` | Add `FieldTCPFlags` constant, add TCP_FLAGS to `defaultTemplateFields`, add `TCPFlags` to `V9DataRecord`, update `Encode()` |
| `internal/engine/ipfix.go` | Add TCP_FLAGS to `ipfixDefaultElements`, add `TCPFlags` to `IPFIXDataRecord`, update `Encode()` |
| `internal/engine/exporter.go` | Update `decodeRawRecord()` for new field |
| `internal/engine/engine.go` | Add `anomalyMgr` to Engine, integrate fluctuation + TCP flags + anomaly modifiers into `flowRunner.run()`, add anomaly API methods |
| `internal/tui/app.go` | Add `viewAnomaly` mode, `a`/`A` keybindings, anomaly status bar, anomaly messages |
| `internal/tui/flows.go` | Add anomaly color highlighting, synthetic flow display |
| `internal/tui/style.go` | Add anomaly color constants |
| `internal/web/handlers.go` | Add anomaly REST endpoints |
| `internal/web/server.go` | Add anomaly WebSocket broadcast, register anomaly routes |

---

## Task 1: Fluctuation config types

**Files:**
- Modify: `internal/config/types.go:57-100`
- Modify: `internal/config/config.go:30-72`
- Test: `internal/config/types_test.go`

- [ ] **Step 1: Write failing test for Fluctuation struct and defaults**

Add to `internal/config/types_test.go`:

```go
func TestFluctuationDefaults(t *testing.T) {
	f := DefaultFluctuation()
	if f.Amplitude != 0.3 {
		t.Errorf("expected amplitude 0.3, got %f", f.Amplitude)
	}
	if f.Period != time.Hour {
		t.Errorf("expected period 1h, got %v", f.Period)
	}
	if f.Phase != 0 {
		t.Errorf("expected phase 0, got %v", f.Phase)
	}
}

func TestConnectionStyleDefault(t *testing.T) {
	f := NewFlow()
	if f.ConnectionStyle != "persistent" {
		t.Errorf("expected persistent, got %s", f.ConnectionStyle)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestFluctuationDefaults -v`
Expected: FAIL — `DefaultFluctuation` not defined

- [ ] **Step 3: Add Fluctuation struct and ConnectionStyle to types.go**

Add after the `Rate` type in `internal/config/types.go`:

```go
// Fluctuation controls sine-wave rate variation.
type Fluctuation struct {
	Amplitude float64       `yaml:"amplitude"`          // 0.0-1.0, default 0.3
	Period    time.Duration `yaml:"period"`              // default 1h
	Phase     time.Duration `yaml:"phase,omitempty"`     // offset, default 0
}

// DefaultFluctuation returns fluctuation with sensible defaults.
func DefaultFluctuation() Fluctuation {
	return Fluctuation{
		Amplitude: 0.3,
		Period:    time.Hour,
		Phase:     0,
	}
}
```

Add `ConnectionStyle` field to the `Flow` struct:

```go
type Flow struct {
	Name            string        `yaml:"name"`
	SourceName      string        `yaml:"source"`
	SourcePort      uint16        `yaml:"source_port"`
	DestName        string        `yaml:"destination"`
	DestPort        uint16        `yaml:"destination_port"`
	Protocol        string        `yaml:"protocol"`
	Rate            string        `yaml:"rate"`
	AppID           uint32        `yaml:"app_id,omitempty"`
	ActiveTimeout   time.Duration `yaml:"active_timeout"`
	InactiveTimeout time.Duration `yaml:"inactive_timeout"`
	Enabled         bool          `yaml:"enabled"`
	ConnectionStyle string        `yaml:"connection_style,omitempty"` // "persistent" or "transactional"
	Fluctuation     *Fluctuation  `yaml:"fluctuation,omitempty"`
}
```

Update `NewFlow()` to set the default:

```go
func NewFlow() Flow {
	return Flow{
		Protocol:        "TCP",
		ActiveTimeout:   60 * time.Second,
		InactiveTimeout: 15 * time.Second,
		Enabled:         true,
		ConnectionStyle: "persistent",
	}
}
```

- [ ] **Step 4: Add Fluctuation and ConnectionStyle to FlowConfig and Config in config.go**

In `internal/config/config.go`, add fields to `FlowConfig`:

```go
type FlowConfig struct {
	Name            string       `yaml:"name"`
	Source          string       `yaml:"source"`
	SourcePort      uint16       `yaml:"source_port"`
	Destination     string       `yaml:"destination"`
	DestPort        uint16       `yaml:"destination_port"`
	Protocol        string       `yaml:"protocol"`
	Rate            string       `yaml:"rate"`
	AppID           uint32       `yaml:"app_id,omitempty"`
	ActiveTimeout   string       `yaml:"active_timeout,omitempty"`
	InactiveTimeout string       `yaml:"inactive_timeout,omitempty"`
	Enabled         bool         `yaml:"enabled"`
	ConnectionStyle string       `yaml:"connection_style,omitempty"`
	Fluctuation     *Fluctuation `yaml:"fluctuation,omitempty"`
}
```

Update `ToFlow()` to copy the new fields:

```go
func (fc FlowConfig) ToFlow() (Flow, error) {
	f := NewFlow()
	f.Name = fc.Name
	f.SourceName = fc.Source
	f.SourcePort = fc.SourcePort
	f.DestName = fc.Destination
	f.DestPort = fc.DestPort
	f.Protocol = fc.Protocol
	f.Rate = fc.Rate
	f.AppID = fc.AppID
	f.Enabled = fc.Enabled

	if fc.ConnectionStyle != "" {
		f.ConnectionStyle = fc.ConnectionStyle
	}
	if fc.Fluctuation != nil {
		f.Fluctuation = fc.Fluctuation
	}

	if fc.ActiveTimeout != "" {
		d, err := time.ParseDuration(fc.ActiveTimeout)
		if err != nil {
			return Flow{}, fmt.Errorf("invalid active_timeout for flow %s: %w", fc.Name, err)
		}
		f.ActiveTimeout = d
	}
	if fc.InactiveTimeout != "" {
		d, err := time.ParseDuration(fc.InactiveTimeout)
		if err != nil {
			return Flow{}, fmt.Errorf("invalid inactive_timeout for flow %s: %w", fc.Name, err)
		}
		f.InactiveTimeout = d
	}
	return f, nil
}
```

Add global Fluctuation to `Config`:

```go
type Config struct {
	Machines    []MachineConfig `yaml:"machines"`
	Flows       []FlowConfig    `yaml:"flows"`
	Collectors  []Collector     `yaml:"collectors"`
	Fluctuation *Fluctuation    `yaml:"fluctuation,omitempty"`
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/types.go internal/config/config.go internal/config/types_test.go
git commit -m "feat: add Fluctuation and ConnectionStyle config types"
```

---

## Task 2: Fluctuation math

**Files:**
- Create: `internal/engine/fluctuation.go`
- Create: `internal/engine/fluctuation_test.go`

- [ ] **Step 1: Write failing test for fluctuateRate**

Create `internal/engine/fluctuation_test.go`:

```go
package engine

import (
	"math"
	"testing"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
)

func TestFluctuateRate_NoFluctuation(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 100_000_000}
	interval := 60 * time.Second
	result := fluctuateRate(rate, interval, time.Now(), nil, nil)
	// Without fluctuation, should return base rate (within jitter range ±5%)
	bpi := rate.BytesPerInterval(interval)
	if result < bpi*90/100 || result > bpi*110/100 {
		t.Errorf("expected ~%d bytes, got %d (outside ±10%% tolerance)", bpi, result)
	}
}

func TestFluctuateRate_WithFluctuation(t *testing.T) {
	rate := config.Rate{BitsPerSecond: 100_000_000}
	fluct := &config.Fluctuation{
		Amplitude: 0.5,
		Period:    time.Hour,
		Phase:     0,
	}

	interval := 60 * time.Second

	// At t=0, sin(0)=0, so rate should be near mean
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := fluctuateRate(rate, interval, baseTime, fluct, nil)
	bpi := rate.BytesPerInterval(interval)
	// Allow ±10% for jitter
	if result < bpi*85/100 || result > bpi*115/100 {
		t.Errorf("at sin(0), expected ~%d bytes, got %d", bpi, result)
	}

	// At t=period/4, sin(pi/2)=1, rate should be near mean + amplitude*mean
	quarterTime := baseTime.Add(15 * time.Minute)
	resultHigh := fluctuateRate(rate, interval, quarterTime, fluct, nil)
	expected := uint64(float64(bpi) * 1.5) // mean + 0.5*mean
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
	global := &config.Fluctuation{
		Amplitude: 1.0,
		Period:    time.Hour,
		Phase:     0,
	}
	// With nil per-flow fluctuation, should use global
	interval := 60 * time.Second
	baseTime := time.Date(2026, 1, 1, 0, 15, 0, 0, time.UTC) // quarter period
	result := fluctuateRate(rate, interval, baseTime, nil, global)
	bpi := rate.BytesPerInterval(interval)
	expected := uint64(float64(bpi) * 2.0) // mean + 1.0*mean at sin(pi/2)
	tolerance := uint64(float64(expected) * 0.15)
	if result < expected-tolerance || result > expected+tolerance {
		t.Errorf("with global fluctuation, expected ~%d bytes, got %d", expected, result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestFluctuateRate -v`
Expected: FAIL — `fluctuateRate` not defined

- [ ] **Step 3: Implement fluctuateRate**

Create `internal/engine/fluctuation.go`:

```go
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
		elapsed := now.Add(fluct.Phase).Sub(time.Time{})
		phase := 2 * math.Pi * float64(elapsed.Nanoseconds()) / float64(fluct.Period.Nanoseconds())
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestFluctuateRate -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/fluctuation.go internal/engine/fluctuation_test.go
git commit -m "feat: add sine-wave fluctuation rate calculation"
```

---

## Task 3: TCP flags in NetFlow v9 template

**Files:**
- Modify: `internal/engine/templates.go:17-54`
- Modify: `internal/engine/templates.go:155-229`
- Test: `internal/engine/templates_test.go`

- [ ] **Step 1: Write failing test for TCP_FLAGS in template and data record**

Add to `internal/engine/templates_test.go`:

```go
func TestV9DataRecordHasTCPFlags(t *testing.T) {
	rec := V9DataRecord{
		SrcAddr:   [4]byte{192, 168, 1, 1},
		DstAddr:   [4]byte{10, 0, 0, 1},
		SrcPort:   443,
		DstPort:   5432,
		Protocol:  6,
		Octets:    1000,
		Packets:   10,
		TCPFlags:  0x02, // SYN
		FirstSeen: 100,
		LastSeen:  200,
	}
	encoded := rec.Encode()
	if len(encoded) != dataRecordSize {
		t.Errorf("encoded size %d != dataRecordSize %d", len(encoded), dataRecordSize)
	}

	// Decode and verify TCPFlags survived round-trip
	decoded := decodeRawRecord(encoded)
	if decoded.TCPFlags != 0x02 {
		t.Errorf("expected TCPFlags 0x02, got 0x%02x", decoded.TCPFlags)
	}
}

func TestV9TemplateIncludesTCPFlags(t *testing.T) {
	// Verify field type 6 is in the template
	found := false
	for _, f := range defaultTemplateFields {
		if f.Type == FieldTCPFlags {
			found = true
			if f.Length != 1 {
				t.Errorf("TCP_FLAGS field length should be 1, got %d", f.Length)
			}
			break
		}
	}
	if !found {
		t.Error("TCP_FLAGS (field type 6) not found in defaultTemplateFields")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run "TestV9DataRecordHasTCPFlags|TestV9TemplateIncludesTCPFlags" -v`
Expected: FAIL — `FieldTCPFlags` not defined, `TCPFlags` not a field

- [ ] **Step 3: Add TCP_FLAGS to v9 template and data record**

In `internal/engine/templates.go`, add the constant:

```go
FieldTCPFlags      = 6
```

Add to `defaultTemplateFields` (after `FieldSrcTOS`):

```go
{FieldTCPFlags, 1},
```

Add `TCPFlags uint8` to `V9DataRecord`:

```go
type V9DataRecord struct {
	SrcAddr   [4]byte
	DstAddr   [4]byte
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	SrcTOS    uint8
	TCPFlags  uint8
	SrcMask   uint8
	DstMask   uint8
	Octets    uint32
	Packets   uint32
	FirstSeen uint32
	LastSeen  uint32
	AppID     uint32
}
```

Update `V9DataRecord.Encode()` — add after `SRC_TOS`:

```go
// TCP_FLAGS (1)
buf[offset] = r.TCPFlags
offset++
```

- [ ] **Step 4: Update decodeRawRecord in exporter.go**

In `internal/engine/exporter.go`, `decodeRawRecord()` — add after `SRC_TOS` decode:

```go
// TCP_FLAGS (1)
rec.TCPFlags = data[offset]
offset++
```

- [ ] **Step 5: Run all engine tests to verify they pass**

Run: `go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/engine/templates.go internal/engine/exporter.go
git commit -m "feat: add TCP_FLAGS field to NetFlow v9 template and data record"
```

---

## Task 4: TCP flags in IPFIX template

**Files:**
- Modify: `internal/engine/ipfix.go:25-39`
- Modify: `internal/engine/ipfix.go:147-221`
- Test: `internal/engine/ipfix_test.go`

- [ ] **Step 1: Write failing test for TCP_FLAGS in IPFIX**

Add to `internal/engine/ipfix_test.go`:

```go
func TestIPFIXDataRecordHasTCPFlags(t *testing.T) {
	rec := IPFIXDataRecord{
		SrcAddr:   [4]byte{192, 168, 1, 1},
		DstAddr:   [4]byte{10, 0, 0, 1},
		SrcPort:   443,
		DstPort:   5432,
		Protocol:  6,
		TCPFlags:  0x18, // ACK+PSH
		Octets:    5000,
		Packets:   50,
		FirstSeen: 100,
		LastSeen:  200,
	}
	encoded := rec.Encode()
	if len(encoded) != ipfixDataRecordSize {
		t.Errorf("encoded size %d != ipfixDataRecordSize %d", len(encoded), ipfixDataRecordSize)
	}
}

func TestIPFIXTemplateIncludesTCPFlags(t *testing.T) {
	found := false
	for _, e := range ipfixDefaultElements {
		if e.ID == FieldTCPFlags {
			found = true
			if e.Length != 1 {
				t.Errorf("TCP_FLAGS element length should be 1, got %d", e.Length)
			}
			break
		}
	}
	if !found {
		t.Error("TCP_FLAGS (element ID 6) not found in ipfixDefaultElements")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run "TestIPFIXDataRecordHasTCPFlags|TestIPFIXTemplateIncludesTCPFlags" -v`
Expected: FAIL — `TCPFlags` not a field on `IPFIXDataRecord`

- [ ] **Step 3: Add TCP_FLAGS to IPFIX template and data record**

In `internal/engine/ipfix.go`, add `{FieldTCPFlags, 1}` to `ipfixDefaultElements` after `FieldSrcTOS`:

```go
{FieldTCPFlags, 1},       // tcpControlBits (6)
```

Add `TCPFlags uint8` to `IPFIXDataRecord` (same position as V9DataRecord):

```go
type IPFIXDataRecord struct {
	SrcAddr   [4]byte
	DstAddr   [4]byte
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	SrcTOS    uint8
	TCPFlags  uint8
	SrcMask   uint8
	DstMask   uint8
	Octets    uint32
	Packets   uint32
	FirstSeen uint32
	LastSeen  uint32
	AppID     uint32
}
```

Update `IPFIXDataRecord.Encode()` — add after `ipClassOfService`:

```go
// tcpControlBits (1)
buf[offset] = r.TCPFlags
offset++
```

- [ ] **Step 4: Run all engine tests**

Run: `go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/ipfix.go
git commit -m "feat: add TCP_FLAGS field to IPFIX template and data record"
```

---

## Task 5: TCP flag state machine

**Files:**
- Create: `internal/engine/tcpflags.go`
- Create: `internal/engine/tcpflags_test.go`

- [ ] **Step 1: Write failing tests for TCP flag logic**

Create `internal/engine/tcpflags_test.go`:

```go
package engine

import "testing"

func TestFlagsPersistentLifecycle(t *testing.T) {
	cs := newConnState("persistent")

	// First tick: SYN
	flags := cs.nextFlags()
	if flags != FlagSYN {
		t.Errorf("first tick persistent: expected SYN (0x02), got 0x%02x", flags)
	}

	// Subsequent ticks: ACK+PSH
	flags = cs.nextFlags()
	if flags != FlagACK|FlagPSH {
		t.Errorf("second tick persistent: expected ACK|PSH (0x18), got 0x%02x", flags)
	}

	flags = cs.nextFlags()
	if flags != FlagACK|FlagPSH {
		t.Errorf("third tick persistent: expected ACK|PSH (0x18), got 0x%02x", flags)
	}

	// Close: FIN+ACK
	flags = cs.closeFlags()
	if flags != FlagFIN|FlagACK {
		t.Errorf("close persistent: expected FIN|ACK (0x11), got 0x%02x", flags)
	}
}

func TestFlagsTransactionalLifecycle(t *testing.T) {
	cs := newConnState("transactional")

	// Every tick: aggregated SYN+ACK+PSH+FIN
	flags := cs.nextFlags()
	if flags != FlagSYN|FlagACK|FlagPSH|FlagFIN {
		t.Errorf("transactional tick: expected 0x1B, got 0x%02x", flags)
	}

	// Second tick: same
	flags = cs.nextFlags()
	if flags != FlagSYN|FlagACK|FlagPSH|FlagFIN {
		t.Errorf("transactional tick 2: expected 0x1B, got 0x%02x", flags)
	}
}

func TestFlagsDefaultIsPersistent(t *testing.T) {
	cs := newConnState("")
	flags := cs.nextFlags()
	if flags != FlagSYN {
		t.Errorf("default first tick: expected SYN (0x02), got 0x%02x", flags)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestFlags -v`
Expected: FAIL — `newConnState` not defined

- [ ] **Step 3: Implement TCP flag state machine**

Create `internal/engine/tcpflags.go`:

```go
package engine

// TCP flag bitmask constants per RFC 793.
const (
	FlagFIN uint8 = 0x01
	FlagSYN uint8 = 0x02
	FlagRST uint8 = 0x04
	FlagPSH uint8 = 0x08
	FlagACK uint8 = 0x10
	FlagURG uint8 = 0x20
)

type connPhase int

const (
	phaseInitial connPhase = iota
	phaseEstablished
)

// connState tracks the TCP flag lifecycle for a flow.
type connState struct {
	style string    // "persistent" or "transactional"
	phase connPhase
}

// newConnState creates a connection state for the given style.
// Empty string defaults to "persistent".
func newConnState(style string) *connState {
	if style == "" {
		style = "persistent"
	}
	return &connState{style: style, phase: phaseInitial}
}

// nextFlags returns the TCP flags for the next record emission.
func (cs *connState) nextFlags() uint8 {
	switch cs.style {
	case "transactional":
		return FlagSYN | FlagACK | FlagPSH | FlagFIN
	default: // persistent
		if cs.phase == phaseInitial {
			cs.phase = phaseEstablished
			return FlagSYN
		}
		return FlagACK | FlagPSH
	}
}

// closeFlags returns the TCP flags for flow shutdown.
func (cs *connState) closeFlags() uint8 {
	return FlagFIN | FlagACK
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestFlags -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/tcpflags.go internal/engine/tcpflags_test.go
git commit -m "feat: add TCP flag state machine for persistent and transactional flows"
```

---

## Task 6: Integrate fluctuation and TCP flags into engine

**Files:**
- Modify: `internal/engine/engine.go:32-39` (flowRunner struct)
- Modify: `internal/engine/engine.go:204-270` (flowRunner.run)
- Test: `internal/engine/engine_test.go`

- [ ] **Step 1: Write failing test for TCP flags in flow records**

Add to `internal/engine/engine_test.go`:

```go
func TestEngineFlowRecordHasTCPFlags(t *testing.T) {
	e := New()
	records := e.Records()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)
	e.AddMachine(dst)

	f := config.Flow{
		Name:            "tcp-flag-flow",
		SourceName:      "web-01",
		SourcePort:      443,
		DestName:        "db-01",
		DestPort:        5432,
		Protocol:        "TCP",
		Rate:            "1Mbps",
		Enabled:         true,
		ConnectionStyle: "persistent",
		ActiveTimeout:   100 * time.Millisecond,
	}
	e.AddFlow(f)

	select {
	case rec := <-records:
		decoded := decodeRawRecord(rec)
		// First record should have SYN flag
		if decoded.TCPFlags != 0x02 {
			t.Errorf("first record: expected SYN (0x02), got 0x%02x", decoded.TCPFlags)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for flow record")
	}

	// Second record should have ACK+PSH
	select {
	case rec := <-records:
		decoded := decodeRawRecord(rec)
		if decoded.TCPFlags != 0x18 {
			t.Errorf("second record: expected ACK|PSH (0x18), got 0x%02x", decoded.TCPFlags)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for second record")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestEngineFlowRecordHasTCPFlags -v`
Expected: FAIL — `TCPFlags` is always 0

- [ ] **Step 3: Add connState and globalFluctuation to flowRunner, update run()**

In `internal/engine/engine.go`, update the `flowRunner` struct:

```go
type flowRunner struct {
	flow      config.Flow
	src       config.Machine
	dst       config.Machine
	rate      config.Rate
	stopCh    chan struct{}
	connState *connState
}
```

Update `AddFlow()` to initialize `connState`:

```go
fr := &flowRunner{
	flow:      f,
	src:       src,
	dst:       dst,
	rate:      rate,
	stopCh:    make(chan struct{}),
	connState: newConnState(f.ConnectionStyle),
}
```

Add a `globalFluctuation` field to `Engine`:

```go
type Engine struct {
	mu                sync.RWMutex
	machines          map[string]config.Machine
	flows             map[string]*flowRunner
	records           chan []byte
	stats             chan FlowStats
	running           bool
	stopCh            chan struct{}
	globalFluctuation *config.Fluctuation
}
```

Add a setter:

```go
// SetGlobalFluctuation sets the global fluctuation defaults.
func (e *Engine) SetGlobalFluctuation(f *config.Fluctuation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.globalFluctuation = f
}
```

Update `flowRunner.run()` signature to accept global fluctuation, and update the tick body:

```go
func (fr *flowRunner) run(records chan<- []byte, stats chan<- FlowStats, engineStop <-chan struct{}, globalFluct *config.Fluctuation) {
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

			select {
			case records <- encoded:
			default:
			}

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
```

Update all call sites of `fr.run()` in `Start()` and `AddFlow()` to pass `e.globalFluctuation`:

```go
go fr.run(e.records, e.stats, e.stopCh, e.globalFluctuation)
```

- [ ] **Step 4: Run all engine tests**

Run: `go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/engine.go
git commit -m "feat: integrate fluctuation and TCP flags into flow runner"
```

---

## Task 7: Anomaly types and scenarios

**Files:**
- Create: `internal/anomaly/types.go`
- Create: `internal/anomaly/scenarios.go`

- [ ] **Step 1: Create anomaly types**

Create `internal/anomaly/types.go`:

```go
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
	Type            AnomalyType
	Category        AnomalyCategory
	Name            string
	Description     string
	DefaultDuration time.Duration
	DefaultIntensity float64  // rate multiplier
	DefaultTargets  []string // empty = all machines
	DefaultCount    int      // synthetic flow/port count
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
```

- [ ] **Step 2: Create scenario definitions**

Create `internal/anomaly/scenarios.go`:

```go
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
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/anomaly/`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/anomaly/types.go internal/anomaly/scenarios.go
git commit -m "feat: add anomaly types and 9 predefined scenario definitions"
```

---

## Task 8: Anomaly manager

**Files:**
- Create: `internal/anomaly/manager.go`
- Create: `internal/anomaly/manager_test.go`

- [ ] **Step 1: Write failing tests for anomaly manager**

Create `internal/anomaly/manager_test.go`:

```go
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
	s1 := AllScenarios()[0]
	s2 := AllScenarios()[4] // Bandwidth Spike
	m.Start(s1, 10*time.Second, 20.0, nil, 50)
	m.Start(s2, 10*time.Second, 5.0, nil, 0)

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
	scenario := AllScenarios()[4] // Bandwidth Spike — affects all flows
	m.Start(scenario, 60*time.Second, 5.0, nil, 0)

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
	scenario := AllScenarios()[5] // Traffic Blackout
	m.Start(scenario, 30*time.Second, 0.0, nil, 0)

	mod := m.GetModifiers("any-flow", "any-machine")
	if mod.RateMultiplier != 0.0 {
		t.Errorf("expected rate multiplier 0.0, got %f", mod.RateMultiplier)
	}
}

func TestManagerGetModifiers_TargetedExfil(t *testing.T) {
	m := NewManager()
	scenario := AllScenarios()[3] // Data Exfiltration
	m.Start(scenario, 60*time.Second, 10.0, []string{"web-01"}, 0)

	// Flow from targeted machine should get multiplied
	mod := m.GetModifiers("some-flow", "web-01")
	if mod.RateMultiplier != 10.0 {
		t.Errorf("expected 10.0 for targeted machine, got %f", mod.RateMultiplier)
	}

	// Flow from non-targeted machine should be unaffected
	mod2 := m.GetModifiers("other-flow", "db-01")
	if mod2.RateMultiplier != 1.0 {
		t.Errorf("expected 1.0 for non-targeted machine, got %f", mod2.RateMultiplier)
	}
}

func TestManagerStacking(t *testing.T) {
	m := NewManager()
	// Bandwidth spike 5x + exfil 10x on same machine = 50x
	m.Start(AllScenarios()[4], 60*time.Second, 5.0, nil, 0)      // bandwidth spike
	m.Start(AllScenarios()[3], 60*time.Second, 10.0, []string{"web-01"}, 0) // exfil

	mod := m.GetModifiers("some-flow", "web-01")
	if mod.RateMultiplier != 50.0 {
		t.Errorf("expected stacked multiplier 50.0, got %f", mod.RateMultiplier)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/anomaly/ -v`
Expected: FAIL — `NewManager` not defined

- [ ] **Step 3: Implement anomaly manager**

Create `internal/anomaly/manager.go`:

```go
package anomaly

import (
	"fmt"
	"sync"
	"time"
)

// Manager tracks active anomalies and computes modifiers for flow runners.
type Manager struct {
	mu      sync.RWMutex
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

// Active returns a snapshot of all active anomalies.
func (m *Manager) Active() []ActiveAnomaly {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Prune expired while we're here
	m.mu.RUnlock()
	m.mu.Lock()
	for id, a := range m.active {
		if a.Expired() {
			delete(m.active, id)
		}
	}
	m.mu.Unlock()
	m.mu.RLock()

	result := make([]ActiveAnomaly, 0, len(m.active))
	for _, a := range m.active {
		result = append(result, *a)
	}
	return result
}

// GetModifiers computes the combined modifier for a given flow and source machine.
// Multiple anomalies compose multiplicatively for rate.
func (m *Manager) GetModifiers(flowName, sourceMachine string) FlowModifier {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mod := FlowModifier{RateMultiplier: 1.0}

	for _, a := range m.active {
		if a.Expired() {
			continue
		}

		if !m.affectsFlow(a, flowName, sourceMachine) {
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
		case Beaconing:
			// Beaconing creates synthetic flows, doesn't modify existing
		case ProtocolAnomaly:
			// Protocol anomaly creates synthetic flows, doesn't modify existing
		case RandomChaos:
			mod.RateMultiplier *= a.Intensity
		}
	}

	return mod
}

// affectsFlow returns true if the anomaly targets the given flow/machine.
func (m *Manager) affectsFlow(a *ActiveAnomaly, flowName, sourceMachine string) bool {
	// Global anomalies (no targets) affect everything
	if len(a.Targets) == 0 {
		return true
	}

	// Targeted anomalies only affect matching machines
	for _, target := range a.Targets {
		if target == sourceMachine {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/anomaly/ -v`
Expected: All PASS

Note: The `Active()` method has a lock upgrade issue (RLock -> RUnlock -> Lock -> Unlock -> RLock). If tests fail on race, refactor to use a single Lock throughout. Run with: `go test -race ./internal/anomaly/ -v`

- [ ] **Step 5: Fix Active() lock pattern if needed**

Replace `Active()` with a cleaner implementation:

```go
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
```

- [ ] **Step 6: Run tests with race detector**

Run: `go test -race ./internal/anomaly/ -v`
Expected: All PASS, no races

- [ ] **Step 7: Commit**

```bash
git add internal/anomaly/manager.go internal/anomaly/manager_test.go
git commit -m "feat: implement anomaly manager with stacking and targeted modifiers"
```

---

## Task 9: Wire anomaly manager into engine

**Files:**
- Modify: `internal/engine/engine.go`
- Test: `internal/engine/engine_test.go`

- [ ] **Step 1: Write failing test for engine anomaly API**

Add to `internal/engine/engine_test.go`:

```go
import "github.com/masterphelps/flowboy/internal/anomaly"

func TestEngineAnomalyStartStop(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	scenarios := anomaly.AllScenarios()
	id, err := e.StartAnomaly(scenarios[4], 10*time.Second, 5.0, nil, 0) // bandwidth spike
	if err != nil {
		t.Fatalf("StartAnomaly failed: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	active := e.ActiveAnomalies()
	if len(active) != 1 {
		t.Fatalf("expected 1 active anomaly, got %d", len(active))
	}

	err = e.StopAnomaly(id)
	if err != nil {
		t.Fatalf("StopAnomaly failed: %v", err)
	}
	if len(e.ActiveAnomalies()) != 0 {
		t.Error("expected 0 active after stop")
	}
}

func TestEngineClearAnomalies(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	scenarios := anomaly.AllScenarios()
	e.StartAnomaly(scenarios[0], 10*time.Second, 20.0, nil, 50)
	e.StartAnomaly(scenarios[4], 10*time.Second, 5.0, nil, 0)

	e.ClearAnomalies()
	if len(e.ActiveAnomalies()) != 0 {
		t.Error("expected 0 after ClearAnomalies")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run "TestEngineAnomaly" -v`
Expected: FAIL — `StartAnomaly` not defined

- [ ] **Step 3: Add anomaly methods to Engine**

In `internal/engine/engine.go`, add import and initialize manager in `New()`:

```go
import "github.com/masterphelps/flowboy/internal/anomaly"
```

Update `Engine` struct:

```go
type Engine struct {
	mu                sync.RWMutex
	machines          map[string]config.Machine
	flows             map[string]*flowRunner
	records           chan []byte
	stats             chan FlowStats
	running           bool
	stopCh            chan struct{}
	globalFluctuation *config.Fluctuation
	anomalyMgr        *anomaly.Manager
}
```

Update `New()`:

```go
func New() *Engine {
	return &Engine{
		machines:   make(map[string]config.Machine),
		flows:      make(map[string]*flowRunner),
		records:    make(chan []byte, 4096),
		stats:      make(chan FlowStats, 256),
		anomalyMgr: anomaly.NewManager(),
	}
}
```

Add API methods:

```go
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
```

- [ ] **Step 4: Integrate anomaly modifiers into flowRunner.run()**

Update the `run()` method to accept the anomaly manager and apply modifiers:

```go
func (fr *flowRunner) run(records chan<- []byte, stats chan<- FlowStats, engineStop <-chan struct{}, globalFluct *config.Fluctuation, anomalyMgr *anomaly.Manager) {
```

In the tick body, after computing `octets` via `fluctuateRate`, add:

```go
// Apply anomaly modifiers
if anomalyMgr != nil {
	mod := anomalyMgr.GetModifiers(fr.flow.Name, fr.flow.SourceName)
	octets = uint64(float64(octets) * mod.RateMultiplier)
	if mod.FlagOverride != nil {
		tcpFlags = *mod.FlagOverride
	}
}
```

Move `tcpFlags := fr.connState.nextFlags()` above the anomaly modifier block so it can be overridden.

Update all `fr.run()` call sites to pass `e.anomalyMgr`:

```go
go fr.run(e.records, e.stats, e.stopCh, e.globalFluctuation, e.anomalyMgr)
```

- [ ] **Step 5: Run all engine tests**

Run: `go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat: wire anomaly manager into engine with modifier integration"
```

---

## Task 10: TUI anomaly colors and styles

**Files:**
- Modify: `internal/tui/style.go`

- [ ] **Step 1: Add anomaly color constants**

In `internal/tui/style.go`, add:

```go
// Anomaly colors
colorAnomalyAttack  = lipgloss.Color("#ff4444") // red
colorAnomalyVolume  = lipgloss.Color("#ffaa00") // yellow/amber
colorAnomalyPattern = lipgloss.Color("#00cccc") // cyan
colorAnomalyBanner  = lipgloss.Color("#ff6600") // orange for status bar
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/tui/`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/tui/style.go
git commit -m "feat: add anomaly color constants to TUI styles"
```

---

## Task 11: TUI anomaly picker and tweak form

**Files:**
- Create: `internal/tui/anomaly.go`

- [ ] **Step 1: Create the anomaly panel sub-model**

Create `internal/tui/anomaly.go`:

```go
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/anomaly"
)

type anomalyMode int

const (
	anomalyPicking anomalyMode = iota
	anomalyTweaking
)

// AnomalyStartMsg is emitted when the user confirms an anomaly.
type AnomalyStartMsg struct {
	Scenario  anomaly.Scenario
	Duration  time.Duration
	Intensity float64
	Targets   []string
	Count     int
}

// AnomalyClearMsg is emitted when the user requests clearing all anomalies.
type AnomalyClearMsg struct{}

// AnomalyPanel handles the anomaly scenario picker and tweak form.
type AnomalyPanel struct {
	scenarios     []anomaly.Scenario
	cursor        int
	mode          anomalyMode
	width         int
	height        int

	// Tweak form fields
	durationInput  textinput.Model
	intensityInput textinput.Model
	targetsInput   textinput.Model
	countInput     textinput.Model
	formFocus      int
	selected       anomaly.Scenario
}

// NewAnomalyPanel creates an anomaly panel with all 9 scenarios.
func NewAnomalyPanel() AnomalyPanel {
	di := textinput.New()
	di.Placeholder = "60s"
	di.CharLimit = 10
	di.Width = 10

	ii := textinput.New()
	ii.Placeholder = "5.0"
	ii.CharLimit = 6
	ii.Width = 8

	ti := textinput.New()
	ti.Placeholder = "all (comma-separated)"
	ti.CharLimit = 80
	ti.Width = 30

	ci := textinput.New()
	ci.Placeholder = "50"
	ci.CharLimit = 5
	ci.Width = 6

	return AnomalyPanel{
		scenarios:      anomaly.AllScenarios(),
		durationInput:  di,
		intensityInput: ii,
		targetsInput:   ti,
		countInput:     ci,
	}
}

// SetSize updates the available dimensions.
func (p *AnomalyPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Update handles key messages.
func (p *AnomalyPanel) Update(msg tea.KeyMsg) tea.Cmd {
	switch p.mode {
	case anomalyPicking:
		return p.updatePicking(msg)
	case anomalyTweaking:
		return p.updateTweaking(msg)
	}
	return nil
}

func (p *AnomalyPanel) updatePicking(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.scenarios)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		p.enterTweakMode()
	case "esc":
		return func() tea.Msg { return nil } // signal to exit view
	}
	return nil
}

func (p *AnomalyPanel) enterTweakMode() {
	s := p.scenarios[p.cursor]
	p.selected = s
	p.mode = anomalyTweaking
	p.formFocus = 0

	p.durationInput.SetValue(s.DefaultDuration.String())
	p.intensityInput.SetValue(fmt.Sprintf("%.1f", s.DefaultIntensity))
	p.targetsInput.SetValue("")
	p.countInput.SetValue(strconv.Itoa(s.DefaultCount))

	p.durationInput.Focus()
	p.intensityInput.Blur()
	p.targetsInput.Blur()
	p.countInput.Blur()
}

const anomalyFormFieldCount = 4

func (p *AnomalyPanel) updateTweaking(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = anomalyPicking
		return nil
	case "tab", "down":
		p.formFocus = (p.formFocus + 1) % anomalyFormFieldCount
		p.focusField()
		return nil
	case "shift+tab", "up":
		p.formFocus = (p.formFocus + anomalyFormFieldCount - 1) % anomalyFormFieldCount
		p.focusField()
		return nil
	case "enter":
		return p.confirmAnomaly()
	}

	p.updateFocusedInput(msg)
	return nil
}

func (p *AnomalyPanel) focusField() {
	p.durationInput.Blur()
	p.intensityInput.Blur()
	p.targetsInput.Blur()
	p.countInput.Blur()
	switch p.formFocus {
	case 0:
		p.durationInput.Focus()
	case 1:
		p.intensityInput.Focus()
	case 2:
		p.targetsInput.Focus()
	case 3:
		p.countInput.Focus()
	}
}

func (p *AnomalyPanel) updateFocusedInput(msg tea.KeyMsg) {
	switch p.formFocus {
	case 0:
		m, _ := p.durationInput.Update(msg)
		p.durationInput = m
	case 1:
		m, _ := p.intensityInput.Update(msg)
		p.intensityInput = m
	case 2:
		m, _ := p.targetsInput.Update(msg)
		p.targetsInput = m
	case 3:
		m, _ := p.countInput.Update(msg)
		p.countInput = m
	}
}

func (p *AnomalyPanel) confirmAnomaly() tea.Cmd {
	dur, err := time.ParseDuration(strings.TrimSpace(p.durationInput.Value()))
	if err != nil {
		dur = p.selected.DefaultDuration
	}

	intensity, err := strconv.ParseFloat(strings.TrimSpace(p.intensityInput.Value()), 64)
	if err != nil {
		intensity = p.selected.DefaultIntensity
	}

	var targets []string
	raw := strings.TrimSpace(p.targetsInput.Value())
	if raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				targets = append(targets, t)
			}
		}
	}

	count, err := strconv.Atoi(strings.TrimSpace(p.countInput.Value()))
	if err != nil {
		count = p.selected.DefaultCount
	}

	p.mode = anomalyPicking
	msg := AnomalyStartMsg{
		Scenario:  p.selected,
		Duration:  dur,
		Intensity: intensity,
		Targets:   targets,
		Count:     count,
	}
	return func() tea.Msg { return msg }
}

// View renders the anomaly panel.
func (p *AnomalyPanel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("INTRODUCE ANOMALY"))
	b.WriteString("\n")

	switch p.mode {
	case anomalyPicking:
		b.WriteString(p.renderPicker())
	case anomalyTweaking:
		b.WriteString(p.renderTweakForm())
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).
		Render("  Enter: select  Esc: back"))

	return b.String()
}

func (p *AnomalyPanel) renderPicker() string {
	var b strings.Builder
	for i, s := range p.scenarios {
		prefix := "  "
		if i == p.cursor {
			prefix = "\u25b8 "
		}

		var color lipgloss.Color
		switch s.Category {
		case anomaly.CategoryAttack:
			color = colorAnomalyAttack
		case anomaly.CategoryVolume:
			color = colorAnomalyVolume
		case anomaly.CategoryPattern:
			color = colorAnomalyPattern
		}

		nameStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(colorAccent)

		line := fmt.Sprintf("%s%s  %s",
			prefix,
			nameStyle.Render(s.Name),
			descStyle.Render(s.Description))

		if i == p.cursor {
			b.WriteString(activeItemStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (p *AnomalyPanel) renderTweakForm() string {
	var b strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(colorAnomalyBanner).Bold(true)
	b.WriteString(nameStyle.Render(p.selected.Name))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render(p.selected.Description))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		view  string
	}{
		{"Duration:  ", p.durationInput.View()},
		{"Intensity: ", p.intensityInput.View()},
		{"Targets:   ", p.targetsInput.View()},
		{"Count:     ", p.countInput.View()},
	}

	for i, f := range fields {
		prefix := "  "
		if i == p.formFocus {
			prefix = "\u25b8 "
		}
		b.WriteString(prefix + f.label + f.view + "\n")
	}

	b.WriteString("\n  Enter: fire  Esc: back  Tab: next field\n")
	return b.String()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/tui/`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/tui/anomaly.go
git commit -m "feat: add TUI anomaly scenario picker and tweak form"
```

---

## Task 12: Wire anomaly into TUI app

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/flows.go`

- [ ] **Step 1: Add viewAnomaly mode and anomaly panel to Model**

In `internal/tui/app.go`, add to `viewMode`:

```go
const (
	viewDashboard viewMode = iota
	viewMap
	viewConfig
	viewAnomaly
)
```

Add `anomalyPanel` to `Model` struct:

```go
type Model struct {
	// ... existing fields
	anomalyPanel AnomalyPanel
}
```

Initialize in `NewModel`:

```go
ap := NewAnomalyPanel()
```

And add `anomalyPanel: ap` to the return struct.

- [ ] **Step 2: Add keybindings and message handling**

In the global normal-mode key handling section (after the `"f"` case), add:

```go
case "a":
	m.view = viewAnomaly
	return m, nil
case "A":
	if m.engine != nil {
		m.engine.ClearAnomalies()
	}
	return m, nil
```

Add `viewAnomaly` handling in the key dispatch (similar to `viewConfig`):

```go
if m.view == viewAnomaly {
	switch msg.String() {
	case "esc":
		m.view = viewDashboard
		return m, nil
	case "q":
		m.view = viewDashboard
		return m, nil
	}
	cmd := m.anomalyPanel.Update(msg)
	return m, cmd
}
```

Add message handler for `AnomalyStartMsg`:

```go
case AnomalyStartMsg:
	if m.engine != nil {
		m.engine.StartAnomaly(msg.Scenario, msg.Duration, msg.Intensity, msg.Targets, msg.Count)
	}
	m.view = viewDashboard
```

- [ ] **Step 3: Add anomaly rendering to View**

Add a case in the `View()` switch:

```go
case viewAnomaly:
	return m.renderAnomalyView(title)
```

Add the render method:

```go
func (m Model) renderAnomalyView(title string) string {
	anomalyStyle := panelStyle.Width(m.width - 2).Height(m.height - 6).
		BorderForeground(colorAnomalyBanner)
	content := m.anomalyPanel.View()
	return lipgloss.JoinVertical(lipgloss.Left, title, anomalyStyle.Render(content),
		m.renderNavHint())
}
```

Update `updatePanelSizes` to include `viewAnomaly`:

```go
case viewAnomaly:
	m.anomalyPanel.SetSize(m.width-4, m.height-6)
```

- [ ] **Step 4: Add anomaly status to the status bar**

In `renderStatusBar()`, after the engine status line, add:

```go
// Anomaly status
if m.engine != nil {
	active := m.engine.ActiveAnomalies()
	if len(active) > 0 {
		var parts []string
		for _, a := range active {
			remaining := a.Remaining()
			parts = append(parts, fmt.Sprintf("%s (%s)", a.Scenario.Name, remaining.Truncate(time.Second)))
		}
		anomalyLine := "  ANOMALY: " + strings.Join(parts, " | ")
		content += "\n" + lipgloss.NewStyle().Foreground(colorAnomalyBanner).Bold(true).Render(anomalyLine)
	}
}
```

Add `"strings"` to the import if not already present. Add the anomaly import:

```go
import "github.com/masterphelps/flowboy/internal/anomaly"
```

(Note: The `anomaly` import is needed indirectly through `engine.ActiveAnomalies()` return type.)

- [ ] **Step 5: Update nav hint to include anomaly key**

In `renderNavHint()`:

```go
return lipgloss.NewStyle().Foreground(colorAccent).
	Render("  [Esc] Dashboard  [M] Map  [F] File  [A] Anomaly  [Q] Quit")
```

- [ ] **Step 6: Add flow highlighting for anomaly-affected flows**

In `internal/tui/flows.go`, update `renderFlowRow` to check for anomaly markers. Add a method to detect if a flow is anomaly-affected. In `renderList()`, after building the line, check the flow name prefix:

```go
func (p *FlowPanel) isAnomalyFlow(f FlowDisplay) bool {
	return strings.HasPrefix(f.Name, "[A]")
}
```

And in `renderList()`, update the color logic:

```go
for i, f := range visible {
	line := p.renderFlowRow(f, i)
	if p.isAnomalyFlow(f) {
		// Synthetic anomaly flow — use anomaly color
		b.WriteString(lipgloss.NewStyle().Foreground(colorAnomalyAttack).Render("! " + line))
	} else if i == p.cursor {
		b.WriteString(activeItemStyle.Render("\u25b8 " + line))
	} else if i%2 == 0 {
		b.WriteString(dimItemStyle.Render("  " + line))
	} else {
		b.WriteString("  " + line)
	}
	b.WriteString("\n")
}
```

- [ ] **Step 7: Build and verify**

Run: `go build ./cmd/flowboy/`
Expected: Success

- [ ] **Step 8: Commit**

```bash
git add internal/tui/app.go internal/tui/flows.go
git commit -m "feat: wire anomaly system into TUI with picker, status bar, and flow highlighting"
```

---

## Task 13: Web API anomaly endpoints

**Files:**
- Modify: `internal/web/handlers.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Add anomaly REST endpoints to handlers.go**

Add to `internal/web/handlers.go`:

```go
// ---------- Anomalies ----------

type anomalyStartRequest struct {
	Scenario  string   `json:"scenario"`
	Duration  string   `json:"duration"`
	Intensity float64  `json:"intensity"`
	Targets   []string `json:"targets"`
	Count     int      `json:"count"`
}

type anomalyResponse struct {
	ID        string  `json:"id"`
	Scenario  string  `json:"scenario"`
	Name      string  `json:"name"`
	Duration  string  `json:"duration"`
	Intensity float64 `json:"intensity"`
	Remaining string  `json:"remaining"`
}

func (s *Server) handleAnomalyScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scenarios := anomaly.AllScenarios()
	type scenarioResp struct {
		Type        string  `json:"type"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Duration    string  `json:"default_duration"`
		Intensity   float64 `json:"default_intensity"`
		Count       int     `json:"default_count"`
	}
	resp := make([]scenarioResp, len(scenarios))
	for i, sc := range scenarios {
		resp[i] = scenarioResp{
			Type:        string(sc.Type),
			Name:        sc.Name,
			Description: sc.Description,
			Duration:    sc.DefaultDuration.String(),
			Intensity:   sc.DefaultIntensity,
			Count:       sc.DefaultCount,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAnomalyActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	active := s.engine.ActiveAnomalies()
	resp := make([]anomalyResponse, len(active))
	for i, a := range active {
		resp[i] = anomalyResponse{
			ID:        a.ID,
			Scenario:  string(a.Scenario.Type),
			Name:      a.Scenario.Name,
			Duration:  a.Duration.String(),
			Intensity: a.Intensity,
			Remaining: a.Remaining().Truncate(time.Second).String(),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAnomalyStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req anomalyStartRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Find matching scenario
	var scenario anomaly.Scenario
	found := false
	for _, sc := range anomaly.AllScenarios() {
		if string(sc.Type) == req.Scenario {
			scenario = sc
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "unknown scenario: "+req.Scenario, http.StatusBadRequest)
		return
	}

	dur := scenario.DefaultDuration
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err == nil {
			dur = d
		}
	}

	intensity := scenario.DefaultIntensity
	if req.Intensity > 0 {
		intensity = req.Intensity
	}

	count := scenario.DefaultCount
	if req.Count > 0 {
		count = req.Count
	}

	id, err := s.engine.StartAnomaly(scenario, dur, intensity, req.Targets, count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast anomaly started
	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_started",
		"data": map[string]any{
			"id": id, "scenario": req.Scenario, "name": scenario.Name,
			"duration": dur.String(), "targets": req.Targets,
		},
	})
	s.broadcast(data)

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "started"})
}

func (s *Server) handleAnomalyStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.StopAnomaly(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_ended",
		"data": map[string]string{"id": req.ID},
	})
	s.broadcast(data)

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleAnomalyClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.engine.ClearAnomalies()

	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_cleared",
		"data": map[string]any{},
	})
	s.broadcast(data)

	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
```

Add `"encoding/json"` and `"github.com/masterphelps/flowboy/internal/anomaly"` to the imports.

- [ ] **Step 2: Register routes in server.go**

In `internal/web/server.go`, add to `routes()`:

```go
s.mux.HandleFunc("/api/anomaly/scenarios", s.cors(s.handleAnomalyScenarios))
s.mux.HandleFunc("/api/anomaly/active", s.cors(s.handleAnomalyActive))
s.mux.HandleFunc("/api/anomaly/start", s.cors(s.handleAnomalyStart))
s.mux.HandleFunc("/api/anomaly/stop", s.cors(s.handleAnomalyStop))
s.mux.HandleFunc("/api/anomaly/clear", s.cors(s.handleAnomalyClear))
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/flowboy/`
Expected: Success

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers.go internal/web/server.go
git commit -m "feat: add anomaly REST endpoints and WebSocket broadcast"
```

---

## Task 14: Config persistence for new fields

**Files:**
- Modify: `internal/tui/app.go:572-617` (saveConfig)
- Modify: `internal/tui/flows.go:34-47` (FlowDisplay)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for config round-trip with new fields**

Add to `internal/config/config_test.go`:

```go
func TestConfigRoundTripWithFluctuation(t *testing.T) {
	cfg := &Config{
		Fluctuation: &Fluctuation{
			Amplitude: 0.5,
			Period:    30 * time.Minute,
		},
		Machines: []MachineConfig{
			{Name: "test", IP: "192.168.1.1", Mask: 24},
		},
		Flows: []FlowConfig{
			{
				Name:            "test-flow",
				Source:          "test",
				SourcePort:      443,
				Destination:     "test",
				DestPort:        5432,
				Protocol:        "TCP",
				Rate:            "100Mbps",
				Enabled:         true,
				ConnectionStyle: "transactional",
				Fluctuation:     &Fluctuation{Amplitude: 0.8, Period: time.Hour},
			},
		},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yaml")

	err := SaveConfig(cfg, path)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Fluctuation == nil {
		t.Fatal("expected global fluctuation")
	}
	if loaded.Fluctuation.Amplitude != 0.5 {
		t.Errorf("global amplitude: expected 0.5, got %f", loaded.Fluctuation.Amplitude)
	}
	if loaded.Flows[0].ConnectionStyle != "transactional" {
		t.Errorf("connection_style: expected transactional, got %s", loaded.Flows[0].ConnectionStyle)
	}
	if loaded.Flows[0].Fluctuation == nil {
		t.Fatal("expected per-flow fluctuation")
	}
	if loaded.Flows[0].Fluctuation.Amplitude != 0.8 {
		t.Errorf("per-flow amplitude: expected 0.8, got %f", loaded.Flows[0].Fluctuation.Amplitude)
	}
}
```

Add `"time"` to the import.

- [ ] **Step 2: Run test**

Run: `go test ./internal/config/ -run TestConfigRoundTripWithFluctuation -v`
Expected: PASS (the YAML fields are already defined — this verifies the round-trip)

- [ ] **Step 3: Add ConnectionStyle to FlowDisplay and saveConfig**

In `internal/tui/flows.go`, add to `FlowDisplay`:

```go
type FlowDisplay struct {
	Name            string
	Source          string
	SrcPort         uint16
	Dest            string
	DstPort         uint16
	Protocol        string
	Rate            string
	AppID           uint32
	BytesSent       uint64
	PacketsSent     uint64
	Active          bool
	Enabled         bool
	ConnectionStyle string
}
```

In `SetFlows`, copy the field:

```go
ConnectionStyle: f.ConnectionStyle,
```

In `internal/tui/app.go`, update `saveConfig()` to include `ConnectionStyle` in the `FlowConfig`:

```go
m.cfg.Flows[i] = config.FlowConfig{
	Name:            fd.Name,
	Source:          fd.Source,
	SourcePort:      fd.SrcPort,
	Destination:     fd.Dest,
	DestPort:        fd.DstPort,
	Protocol:        fd.Protocol,
	Rate:            fd.Rate,
	AppID:           fd.AppID,
	Enabled:         fd.Enabled,
	ConnectionStyle: fd.ConnectionStyle,
}
```

Also update the `FlowToggleMsg` handler to pass `ConnectionStyle`:

In the `FlowToggleMsg` case, add `f.ConnectionStyle = msg.Flow.ConnectionStyle` after setting other fields.

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config_test.go internal/tui/app.go internal/tui/flows.go
git commit -m "feat: persist ConnectionStyle through config and TUI"
```

---

## Task 15: Wire global fluctuation from config into engine at startup

**Files:**
- Modify: `cmd/flowboy/main.go`

- [ ] **Step 1: Read main.go to see current wiring**

Read `cmd/flowboy/main.go` to understand the startup sequence.

- [ ] **Step 2: Add global fluctuation wiring**

After `eng := engine.New()` and before `eng.Start()`, add:

```go
if cfg.Fluctuation != nil {
	eng.SetGlobalFluctuation(cfg.Fluctuation)
}
```

- [ ] **Step 3: Build and verify**

Run: `go build -o flowboy ./cmd/flowboy/`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add cmd/flowboy/main.go
git commit -m "feat: wire global fluctuation config into engine at startup"
```

---

## Task 16: End-to-end manual verification

- [ ] **Step 1: Build**

Run: `go build -o flowboy ./cmd/flowboy/`

- [ ] **Step 2: Run TUI mode and verify**

Run: `./flowboy -config configs/acme-manufacturing.yaml`

Verify:
- Traffic fluctuates (watch flow stats changing non-uniformly)
- Press `a` → anomaly scenario picker appears
- Select DDoS Flood → tweak form with defaults
- Press Enter → anomaly fires, status bar shows "ANOMALY: DDoS Flood (59s)"
- Press `A` → anomalies clear
- Press `q` to exit

- [ ] **Step 3: Run web mode and verify**

Run: `./flowboy -web -config configs/acme-manufacturing.yaml`

Verify:
- `GET /api/anomaly/scenarios` returns 9 scenarios
- `POST /api/anomaly/start` with `{"scenario":"bandwidth_spike","duration":"30s","intensity":5.0}` starts an anomaly
- `GET /api/anomaly/active` shows the active anomaly
- `POST /api/anomaly/clear` clears all

- [ ] **Step 4: Run full test suite one final time**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: cleanup and verify traffic fluctuation + anomaly system"
```
