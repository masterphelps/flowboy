# Traffic Fluctuation, TCP Flags & Anomaly System

**Date**: 2026-03-30
**Purpose**: Demo/presentation tool — generate realistic-looking traffic with controllable anomalies for showcasing on SIEM/collector dashboards.

---

## 1. Baseline Traffic Fluctuation

Currently every flow emits a flat, constant rate each tick. This changes to a sine-wave model so collector dashboards show organic-looking traffic.

### Rate function

```
rate(t) = mean + (mean * amplitude * sin(2π * t / period)) + jitter
```

- **mean**: The flow's configured rate (e.g., `100Mbps`). This becomes the center line.
- **amplitude**: 0.0–1.0 (default `0.3` = ±30% swing). Configurable per-flow or globally.
- **period**: Duration of one full cycle (default `1h`). Configurable per-flow or globally.
- **phase**: Offset in minutes (default `0`). Per-flow, so not all flows peak simultaneously.
- **jitter**: Random ±5% applied on top of the wave each tick. Not configurable — always present for realism.

### Config

Global defaults in a top-level `fluctuation:` block. Per-flow overrides under each flow entry.

```yaml
fluctuation:
  amplitude: 0.3
  period: 1h

flows:
  - name: web-to-db
    rate: 100Mbps
    fluctuation:
      amplitude: 0.5
      period: 30m
      phase: 15         # offset in minutes
```

If `fluctuation:` is omitted entirely (e.g., old config files), behavior is flat — backward compatible.

### Implementation location

- `internal/engine/engine.go` — `flowRunner.run()` replaces `fr.rate.BytesPerInterval(timeout)` with the sine-wave calculation.
- `internal/config/types.go` — New `Fluctuation` struct, added to `Flow` and top-level `Config`.

---

## 2. TCP Flags

TCP flags are completely absent from the current implementation. Both the NetFlow v9 template and IPFIX elements lack `TCP_FLAGS` (field type 6), and the data record structs have no `TCPFlags` field.

### Template changes

Add `TCP_FLAGS` (field type 6, length 1) to:
- `defaultTemplateFields` in `internal/engine/templates.go`
- `ipfixDefaultElements` in `internal/engine/ipfix.go`

Add `TCPFlags uint8` to `V9DataRecord` and `IPFIXDataRecord`. Update `Encode()` and `decodeRawRecord()` accordingly.

### Flag encoding

Standard NetFlow TCP flag bitmask:

| Flag | Bit | Value |
|------|-----|-------|
| FIN  | 0   | 0x01  |
| SYN  | 1   | 0x02  |
| RST  | 2   | 0x04  |
| PSH  | 3   | 0x08  |
| ACK  | 4   | 0x10  |
| URG  | 5   | 0x20  |

### Connection styles

Each flow gets a `connection_style` field (default: `persistent`):

**`persistent`** — Simulates a long-lived connection (database link, VPN tunnel):
- First record after flow enable: SYN (0x02)
- All subsequent records: ACK+PSH (0x18)
- Record on flow disable/stop: FIN+ACK (0x11)

**`transactional`** — Simulates many short-lived connections (HTTP requests):
- Every tick represents N short connections completing
- Records carry aggregated flags: SYN+ACK+PSH+FIN (0x1B)
- Packet count reflects connection count (each connection = ~10 packets for a typical short HTTP exchange)
- Optional `connections_per_interval` field (default: `rate_bytes_per_interval / 15000`, assuming ~15KB per short connection)

### State tracking

The `flowRunner` gains a `connectionPhase` field:
- `initial` → `established` → `closing`
- `persistent`: transitions `initial→established` after first tick
- `transactional`: every tick is effectively a full cycle, so flags are always the aggregated bitmask

### Config

```yaml
flows:
  - name: web-to-db
    rate: 100Mbps
    connection_style: persistent   # or transactional
```

Default is `persistent` if omitted. Backward compatible with existing configs.

---

## 3. Anomaly System

### Architecture

New package: `internal/anomaly/`

Contains:
- `scenario.go` — Scenario definitions and types
- `manager.go` — AnomalyManager that tracks active anomalies and computes modifiers

The AnomalyManager lives on the engine. It is not persisted to config — anomalies are runtime-only. Restarting flowboy clears all active anomalies.

### Core types

```go
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

type Scenario struct {
    Type        AnomalyType
    Name        string            // Display name: "DDoS Spike"
    Description string            // Short description for TUI/Web
    Duration    time.Duration     // Default duration
    Intensity   float64           // Default rate multiplier
    Targets     []string          // Default target machine/flow names
    Defaults    map[string]any    // Scenario-specific tweakable defaults
}

type ActiveAnomaly struct {
    ID        string              // Unique ID for tracking
    Scenario  Scenario            // Resolved scenario with user tweaks applied
    StartTime time.Time
    Duration  time.Duration
    // Runtime state (synthetic flow runners, etc.)
}

type FlowModifier struct {
    RateMultiplier float64        // 1.0 = no change, 10.0 = 10x, 0.0 = blackout
    FlagOverride   *uint8         // nil = no override, otherwise replaces flags
    Active         bool
}
```

### The 9 scenarios

| # | Scenario | Type | Affects | Rate Effect | Flag Behavior | Synthetic Flows |
|---|----------|------|---------|-------------|---------------|-----------------|
| 1 | DDoS Flood | `ddos` | Target machine | Default 20x connections | SYN only (0x02) | 50 new src→target flows (default) |
| 2 | Port Scan | `port_scan` | Target machine | 1Kbps per port | SYN→RST (0x06) | 1 src scanning 100 sequential dst ports (default) |
| 3 | Lateral Movement | `lateral_movement` | Machine pairs | Normal rate | SYN→ACK (0x12) | 3 new flows between machines that have no configured flows |
| 4 | Data Exfiltration | `data_exfiltration` | Source machine | Default 10x outbound | ACK+PSH (0x18) | None — existing outbound flows from source spike |
| 5 | Bandwidth Spike | `bandwidth_spike` | All flows | Default 5x all | Unchanged | None — existing flows scale |
| 6 | Traffic Blackout | `traffic_blackout` | All or selected | →0 | FIN (0x01) | None — existing flows drop to zero |
| 7 | Protocol Anomaly | `protocol_anomaly` | Selected flows | Normal rate | Unchanged | 1 new UDP flow mirroring each selected TCP flow's src/dst |
| 8 | Beaconing | `beaconing` | Source machine | 1Kbps, every 30s | SYN→ACK→FIN cycle per beacon | 1 new flow, beacon interval configurable (default 30s) |
| 9 | Random Chaos | `random_chaos` | Random subset (50% of machines) | 0.5x–5x random per flow | Random valid flags | Creates 1-5 random flows, kills 0-2 existing, reshuffles every 10s |

### Tweakable parameters

When a user selects a scenario, these are shown pre-filled with defaults:

- **Duration** (all scenarios): How long the anomaly runs. Defaults: DDoS 60s, port scan 30s, lateral movement 120s, exfil 90s, bandwidth spike 60s, blackout 30s, protocol anomaly 60s, beaconing 300s, random chaos 60s.
- **Intensity / multiplier** (volume-based: DDoS, exfil, bandwidth spike): Rate multiplier. See scenario table for defaults.
- **Target machines or flows** (connection/pattern-based): Which machines/flows are affected. Default: all machines for global scenarios, first machine for targeted scenarios.
- **Connection count** (DDoS, port scan): How many synthetic connections/ports. Default: DDoS 50 flows, port scan 100 ports.

### Stacking

Multiple anomalies can run simultaneously. When multiple modifiers apply to the same flow:
- Rate multipliers compose multiplicatively (bandwidth spike 5x + exfil 10x = 50x on that flow)
- Flag overrides: last-started anomaly wins (most recent takes precedence)
- "Clear all" kills every active anomaly instantly and removes all synthetic flows

### Synthetic flows

Some scenarios create temporary flows that don't exist in the config:

- On anomaly start: manager creates synthetic `flowRunner` goroutines
- Synthetic flow names are prefixed and generated (e.g., `[A] ddos-syn-10.0.1.5-00001`)
- Synthetic flows use existing machines from the config as sources/targets
- On anomaly end or clear-all: manager stops and removes all synthetic runners
- Synthetic flows emit `FlowStats` on the same stats channel — TUI/Web see them naturally
- Synthetic flows are **not** persisted to config

---

## 4. Engine Integration

### Modified flow runner tick

The `flowRunner.run()` loop changes from flat rate to:

1. **Compute base rate** from sine wave: `baseRate = fluctuate(configuredRate, time.Now())`
2. **Query anomaly manager**: `modifier = manager.GetModifiers(flowName, machineName)`
3. **Apply rate multiplier**: `finalRate = baseRate * modifier.RateMultiplier`
4. **Resolve TCP flags**: If `modifier.FlagOverride != nil`, use it. Otherwise, use connection-style phase logic.
5. **Encode record** with `TCPFlags` field populated
6. **Send** on records channel as before

### Anomaly manager on the engine

```go
type Engine struct {
    // ... existing fields
    anomalyMgr *anomaly.Manager
}
```

- `engine.StartAnomaly(scenario Scenario) (string, error)` — starts an anomaly, returns its ID
- `engine.StopAnomaly(id string)` — stops a specific anomaly
- `engine.ClearAnomalies()` — stops all active anomalies
- `engine.ActiveAnomalies() []ActiveAnomaly` — list for TUI/Web display

The manager runs a background goroutine that expires anomalies when their duration elapses.

### Stats pipeline

No changes to the stats channel or message types. Synthetic flows emit `FlowStats` like normal flows. The `FlowStats` struct gains no new fields — the flow name prefix `[A]` is sufficient for TUI/Web to identify anomaly-generated flows.

---

## 5. TUI Integration

### Triggering anomalies

| Key | Context | Action |
|-----|---------|--------|
| `a` | Normal mode | Open anomaly scenario picker |
| `A` (shift-a) | Normal mode | Clear all active anomalies |

**Scenario picker**: A new view (same pattern as config panel or network map — a view mode, not a persistent panel). Shows a list of 9 scenarios with name + one-line description. `j/k` to navigate, `Enter` to select.

**Tweak form**: After selecting a scenario, shows a form pre-filled with defaults (same form pattern as add/edit machine/flow). `Tab` between fields, `Enter` to confirm and fire. `Esc` to cancel.

### Active anomaly visibility

**Status bar**: When anomalies are active, the status bar shows:
```
ANOMALY: DDoS Spike (45s) | Beaconing (2m 15s)
```
Countdowns update each tick. When no anomalies are active, this line is absent.

**Flow highlighting**: Flows affected by an active anomaly get:
- Distinct color: red for attack-type (DDoS, port scan, lateral movement), yellow for volume-type (exfil, bandwidth spike, blackout), cyan for pattern-type (protocol, beaconing, chaos)
- Prefix marker: `!` before the flow name
- Synthetic flows show `[A]` prefix and use the same color coding

### New message types

- `AnomalyStartedMsg{ID, Scenario}` — triggers status bar update and flow re-render
- `AnomalyEndedMsg{ID}` — triggers cleanup
- `AnomalyClearedMsg` — all cleared

---

## 6. Web UI Integration

### Controls

- **"Introduce Anomaly" button** in the control area → opens a modal with scenario cards
- Click a scenario card → shows tweak form with pre-filled defaults
- "Confirm" fires the anomaly, "Cancel" dismisses
- **"Clear All Anomalies" button** — visible when any anomaly is active

### Active anomaly display

- Header banner showing active anomalies with countdowns (mirrors TUI status bar)
- Flow table rows highlighted with color coding matching TUI scheme
- Synthetic flows appear in the flow table with `[A]` prefix

### WebSocket messages

New broadcast message types:

```json
{"type": "anomaly_started", "data": {"id": "...", "scenario": "ddos", "name": "DDoS Spike", "duration": 60, "targets": [...]}}
{"type": "anomaly_ended", "data": {"id": "..."}}
{"type": "anomaly_cleared", "data": {}}
```

### REST endpoints

- `POST /api/anomaly/start` — body: `{scenario, duration, intensity, targets, ...}`
- `POST /api/anomaly/stop` — body: `{id}`
- `POST /api/anomaly/clear` — clears all
- `GET /api/anomaly/active` — list active anomalies
- `GET /api/anomaly/scenarios` — list available scenarios with defaults

---

## 7. File Changes Summary

### New files
- `internal/anomaly/scenario.go` — Scenario type, 9 predefined scenarios
- `internal/anomaly/manager.go` — AnomalyManager, modifier computation, synthetic flow lifecycle

### Modified files
- `internal/config/types.go` — `Fluctuation` struct, `ConnectionStyle` field on `Flow`, top-level `Fluctuation` on `Config`
- `internal/engine/engine.go` — Sine-wave rate calc, anomaly manager integration, TCP flag phase tracking
- `internal/engine/templates.go` — Add `TCP_FLAGS` field type 6 to v9 template
- `internal/engine/ipfix.go` — Add `TCP_FLAGS` to IPFIX elements
- `internal/engine/exporter.go` — Handle `TCPFlags` in encode/decode
- `internal/tui/app.go` — Anomaly view mode, keybindings `a`/`A`, status bar anomaly display, anomaly messages
- `internal/tui/anomaly.go` — New file: anomaly scenario picker and tweak form (sub-model)
- `internal/tui/flows.go` — Flow highlighting for anomaly-affected flows, synthetic flow display
- `internal/tui/style.go` — New color constants for anomaly types (red/yellow/cyan)
- `internal/web/handlers.go` — Anomaly REST endpoints
- `internal/web/server.go` — Anomaly WebSocket broadcast
- `internal/web/static/` — Anomaly modal, banner, flow highlighting in Web GUI

### Unchanged
- Config YAML schema gains optional fields only — all existing configs remain valid
- Anomalies are runtime-only — no YAML persistence
