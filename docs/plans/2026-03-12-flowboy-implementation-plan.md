# Flowboy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go-based NetFlow v9/IPFIX traffic generator with a Pip-Boy/Vault-Tec CRT aesthetic, providing both a Bubbletea TUI and an embedded web UI with a pixel-art network map.

**Architecture:** Single Go binary, monolithic. Goroutine-per-flow with channel-based communication. YAML config persistence. Embedded web server via `embed.FS`. No frontend framework — vanilla JS + Canvas.

**Tech Stack:** Go 1.22+, Bubbletea, Lip Gloss, gopkg.in/yaml.v3, vanilla JS, Canvas API, Tailwind CSS, WebSocket (gorilla/websocket or nhooyr.io/websocket).

**Design Doc:** `docs/plans/2026-03-12-flowboy-design.md` — read this first for full context on every decision.

---

## Phase 1: Foundation (Config + Data Model)

### Task 1: Initialize Go Module and Project Skeleton

**Files:**
- Create: `go.mod`
- Create: `cmd/flowboy/main.go`
- Create: `internal/config/types.go`
- Create: `internal/config/config.go`
- Create: `configs/flowboy.yaml`

**Step 1: Initialize Go module**

```bash
cd /Users/masterphelps/Documents/flowmaster
go mod init github.com/masterphelps/flowboy
```

**Step 2: Create minimal main.go**

```go
// cmd/flowboy/main.go
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	web := flag.Bool("web", false, "Start web UI")
	both := flag.Bool("both", false, "Start both TUI and web UI")
	headless := flag.Bool("headless", false, "Run headless (no UI)")
	port := flag.Int("port", 8042, "Web UI port")
	config := flag.String("config", "configs/flowboy.yaml", "Config file path")
	flag.Parse()

	fmt.Printf("Flowboy 3000\n")
	fmt.Printf("Mode: web=%v both=%v headless=%v port=%d config=%s\n",
		*web, *both, *headless, *port, *config)
	os.Exit(0)
}
```

**Step 3: Verify it builds and runs**

```bash
go build -o flowboy ./cmd/flowboy && ./flowboy
```

Expected: Prints "Flowboy 3000" and mode info.

**Step 4: Commit**

```bash
git add go.mod cmd/
git commit -m "feat: initialize Go module and CLI entry point"
```

---

### Task 2: Data Model Types

**Files:**
- Create: `internal/config/types.go`
- Create: `internal/config/types_test.go`

**Step 1: Write tests for data model**

```go
// internal/config/types_test.go
package config

import (
	"net"
	"testing"
	"time"
)

func TestMachineSegment(t *testing.T) {
	m := Machine{
		Name: "web-server-01",
		IP:   net.ParseIP("192.168.50.201"),
		Mask: net.CIDRMask(24, 32),
	}
	seg := m.Segment()
	expected := "192.168.50.0/24"
	if seg.String() != expected {
		t.Errorf("expected segment %s, got %s", expected, seg.String())
	}
}

func TestMachineSegmentDifferentMask(t *testing.T) {
	m := Machine{
		Name: "db-server-01",
		IP:   net.ParseIP("10.70.22.45"),
		Mask: net.CIDRMask(16, 32),
	}
	seg := m.Segment()
	expected := "10.70.0.0/16"
	if seg.String() != expected {
		t.Errorf("expected segment %s, got %s", expected, seg.String())
	}
}

func TestFlowDefaults(t *testing.T) {
	f := NewFlow()
	if f.ActiveTimeout != 60*time.Second {
		t.Errorf("expected 60s active timeout, got %v", f.ActiveTimeout)
	}
	if f.InactiveTimeout != 15*time.Second {
		t.Errorf("expected 15s inactive timeout, got %v", f.InactiveTimeout)
	}
	if f.Enabled != true {
		t.Error("expected flow enabled by default")
	}
}

func TestParseRate(t *testing.T) {
	tests := []struct {
		input    string
		bps      uint64
		wantErr  bool
	}{
		{"90Mbps", 90_000_000, false},
		{"1Gbps", 1_000_000_000, false},
		{"500Kbps", 500_000, false},
		{"10GB/day", 925_925, false}, // 10GB / 86400s ≈ 925925 bps (bytes, converted)
		{"badvalue", 0, true},
	}
	for _, tt := range tests {
		rate, err := ParseRate(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseRate(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRate(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if rate.BitsPerSecond != tt.bps {
			t.Errorf("ParseRate(%q) = %d bps, want %d", tt.input, rate.BitsPerSecond, tt.bps)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/ -v
```

Expected: FAIL — types not defined.

**Step 3: Implement types**

```go
// internal/config/types.go
package config

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Machine represents a fictitious network device.
type Machine struct {
	Name string    `yaml:"name"`
	IP   net.IP    `yaml:"ip"`
	Mask net.IPMask `yaml:"mask"`
}

// Segment returns the network segment (CIDR) this machine belongs to.
func (m Machine) Segment() net.IPNet {
	network := m.IP.Mask(m.Mask)
	return net.IPNet{IP: network, Mask: m.Mask}
}

// Flow defines a traffic flow between two machines.
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
}

// NewFlow returns a Flow with sensible defaults.
func NewFlow() Flow {
	return Flow{
		Protocol:        "TCP",
		ActiveTimeout:   60 * time.Second,
		InactiveTimeout: 15 * time.Second,
		Enabled:         true,
	}
}

// Collector defines a NetFlow export target.
type Collector struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Version string `yaml:"version"` // "v9" or "ipfix"
}

// Rate represents a parsed traffic rate.
type Rate struct {
	BitsPerSecond uint64
	Original      string
}

var rateRegex = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(Kbps|Mbps|Gbps|[KMGT]B/day)$`)

// ParseRate parses a human-readable rate string into bits per second.
func ParseRate(s string) (Rate, error) {
	s = strings.TrimSpace(s)
	matches := rateRegex.FindStringSubmatch(s)
	if matches == nil {
		return Rate{}, fmt.Errorf("invalid rate format: %q (expected e.g. 90Mbps, 1Gbps, 10GB/day)", s)
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid rate value: %w", err)
	}
	unit := matches[2]
	var bps uint64
	switch unit {
	case "Kbps":
		bps = uint64(value * 1_000)
	case "Mbps":
		bps = uint64(value * 1_000_000)
	case "Gbps":
		bps = uint64(value * 1_000_000_000)
	case "KB/day":
		bps = uint64(value * 1_000 * 8 / 86400)
	case "MB/day":
		bps = uint64(value * 1_000_000 * 8 / 86400)
	case "GB/day":
		bps = uint64(value * 1_000_000_000 * 8 / 86400)
	case "TB/day":
		bps = uint64(value * 1_000_000_000_000 * 8 / 86400)
	}
	return Rate{BitsPerSecond: bps, Original: s}, nil
}

// BytesPerInterval calculates bytes accumulated over a given duration at this rate.
func (r Rate) BytesPerInterval(d time.Duration) uint64 {
	return uint64(float64(r.BitsPerSecond) / 8.0 * d.Seconds())
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/ -v
```

Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/config/types.go internal/config/types_test.go
git commit -m "feat: add data model types (Machine, Flow, Collector, Rate)"
```

---

### Task 3: YAML Config Loading and Persistence

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `configs/flowboy.yaml`

**Step 1: Write tests for config loading**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
machines:
  - name: web-server-01
    ip: 192.168.50.201
    mask: 24
  - name: db-server-01
    ip: 10.70.22.45
    mask: 24

flows:
  - name: web-to-db
    source: web-server-01
    source_port: 46578
    destination: db-server-01
    destination_port: 5432
    protocol: TCP
    rate: 90Mbps
    enabled: true

collectors:
  - name: lab-collector
    address: 10.0.0.5:2055
    version: v9
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.Machines) != 2 {
		t.Errorf("expected 2 machines, got %d", len(cfg.Machines))
	}
	if cfg.Machines[0].Name != "web-server-01" {
		t.Errorf("expected machine name web-server-01, got %s", cfg.Machines[0].Name)
	}
	if len(cfg.Flows) != 1 {
		t.Errorf("expected 1 flow, got %d", len(cfg.Flows))
	}
	if cfg.Flows[0].Rate != "90Mbps" {
		t.Errorf("expected rate 90Mbps, got %s", cfg.Flows[0].Rate)
	}
	if len(cfg.Collectors) != 1 {
		t.Errorf("expected 1 collector, got %d", len(cfg.Collectors))
	}
}

func TestSaveConfig(t *testing.T) {
	cfg := &Config{
		Machines: []MachineConfig{
			{Name: "test-machine", IP: "192.168.1.1", Mask: 24},
		},
	}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "out.yaml")
	err := SaveConfig(cfg, path)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save failed: %v", err)
	}
	if len(loaded.Machines) != 1 {
		t.Errorf("expected 1 machine after round-trip, got %d", len(loaded.Machines))
	}
	if loaded.Machines[0].Name != "test-machine" {
		t.Errorf("expected test-machine, got %s", loaded.Machines[0].Name)
	}
}

func TestAutoSegments(t *testing.T) {
	cfg := &Config{
		Machines: []MachineConfig{
			{Name: "web-01", IP: "192.168.50.201", Mask: 24},
			{Name: "app-01", IP: "192.168.50.100", Mask: 24},
			{Name: "db-01", IP: "10.70.22.45", Mask: 24},
		},
	}
	segments := cfg.BuildSegments()
	if len(segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(segments))
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/ -v -run "TestLoad|TestSave|TestAuto"
```

Expected: FAIL — Config types not defined.

**Step 3: Implement config loading**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// MachineConfig is the YAML-friendly representation of a Machine.
type MachineConfig struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
	Mask int    `yaml:"mask"`
}

// ToMachine converts a MachineConfig to a Machine with parsed net types.
func (mc MachineConfig) ToMachine() (Machine, error) {
	ip := net.ParseIP(mc.IP)
	if ip == nil {
		return Machine{}, fmt.Errorf("invalid IP for machine %s: %s", mc.Name, mc.IP)
	}
	mask := net.CIDRMask(mc.Mask, 32)
	return Machine{Name: mc.Name, IP: ip, Mask: mask}, nil
}

// FlowConfig is the YAML-friendly representation of a Flow.
type FlowConfig struct {
	Name            string `yaml:"name"`
	Source          string `yaml:"source"`
	SourcePort      uint16 `yaml:"source_port"`
	Destination     string `yaml:"destination"`
	DestPort        uint16 `yaml:"destination_port"`
	Protocol        string `yaml:"protocol"`
	Rate            string `yaml:"rate"`
	AppID           uint32 `yaml:"app_id,omitempty"`
	ActiveTimeout   string `yaml:"active_timeout,omitempty"`
	InactiveTimeout string `yaml:"inactive_timeout,omitempty"`
	Enabled         bool   `yaml:"enabled"`
}

// ToFlow converts a FlowConfig to a Flow with parsed durations.
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

// Segment groups machines by network prefix.
type Segment struct {
	CIDR     net.IPNet
	Machines []Machine
}

// Config is the top-level YAML configuration.
type Config struct {
	Machines   []MachineConfig   `yaml:"machines"`
	Flows      []FlowConfig      `yaml:"flows"`
	Collectors []Collector        `yaml:"collectors"`
}

// LoadConfig reads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the config to a YAML file.
func SaveConfig(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// BuildSegments auto-groups machines by their subnet.
func (cfg *Config) BuildSegments() []Segment {
	segMap := make(map[string]*Segment)
	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			continue
		}
		seg := m.Segment()
		key := seg.String()
		if s, ok := segMap[key]; ok {
			s.Machines = append(s.Machines, m)
		} else {
			segMap[key] = &Segment{
				CIDR:     seg,
				Machines: []Machine{m},
			}
		}
	}
	segments := make([]Segment, 0, len(segMap))
	for _, s := range segMap {
		segments = append(segments, *s)
	}
	return segments
}
```

**Step 4: Add yaml dependency and run tests**

```bash
go get gopkg.in/yaml.v3
go test ./internal/config/ -v -run "TestLoad|TestSave|TestAuto"
```

Expected: All PASS.

**Step 5: Create default config file**

```yaml
# configs/flowboy.yaml
# Flowboy 3000 Configuration

machines:
  - name: web-server-01
    ip: 192.168.50.201
    mask: 24
  - name: db-server-01
    ip: 10.70.22.45
    mask: 24

flows:
  - name: web-to-db
    source: web-server-01
    source_port: 46578
    destination: db-server-01
    destination_port: 5432
    protocol: TCP
    rate: 90Mbps
    enabled: true

collectors:
  - name: lab-collector
    address: 10.0.0.5:2055
    version: v9
```

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go configs/flowboy.yaml go.mod go.sum
git commit -m "feat: add YAML config loading, persistence, and auto-segmentation"
```

---

## Phase 2: NetFlow Engine

### Task 4: NetFlow v9 Template and Packet Encoding

**Files:**
- Create: `internal/engine/templates.go`
- Create: `internal/engine/templates_test.go`

**Step 1: Write tests for v9 packet encoding**

```go
// internal/engine/templates_test.go
package engine

import (
	"testing"
)

func TestV9TemplateEncoding(t *testing.T) {
	tmpl := NewV9Template()
	data := tmpl.Encode()
	if len(data) == 0 {
		t.Fatal("encoded template is empty")
	}
	// v9 header: version (2 bytes) = 9
	version := uint16(data[0])<<8 | uint16(data[1])
	if version != 9 {
		t.Errorf("expected version 9, got %d", version)
	}
}

func TestV9DataRecordEncoding(t *testing.T) {
	rec := V9DataRecord{
		SrcAddr:    [4]byte{192, 168, 50, 201},
		DstAddr:    [4]byte{10, 70, 22, 45},
		SrcPort:    46578,
		DstPort:    5432,
		Protocol:   6, // TCP
		Octets:     675_000_000,
		Packets:    450_000,
		FirstSeen:  1000,
		LastSeen:   61000,
		AppID:      0,
	}
	data := rec.Encode()
	if len(data) == 0 {
		t.Fatal("encoded data record is empty")
	}
}

func TestV9PacketAssembly(t *testing.T) {
	pkt := NewV9Packet(1, 1000)
	rec := V9DataRecord{
		SrcAddr:  [4]byte{192, 168, 50, 201},
		DstAddr:  [4]byte{10, 70, 22, 45},
		SrcPort:  46578,
		DstPort:  5432,
		Protocol: 6,
		Octets:   675_000_000,
		Packets:  450_000,
	}
	pkt.AddDataRecord(rec)
	data := pkt.Bytes()
	if len(data) < 20 {
		t.Errorf("packet too short: %d bytes", len(data))
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/engine/ -v
```

Expected: FAIL — types not defined.

**Step 3: Implement v9 encoding**

Implement the NetFlow v9 packet format per RFC 3954. The key structures:
- V9 Header (20 bytes): version, count, sysUptime, unixSecs, sequence, sourceID
- Template FlowSet: flowset ID 0, template ID, field count, field definitions
- Data FlowSet: template ID, data records

The template should include these fields at minimum:
- IN_BYTES (1), IN_PKTS (2), PROTOCOL (4), SRC_TOS (5)
- L4_SRC_PORT (7), IPV4_SRC_ADDR (8), SRC_MASK (9)
- L4_DST_PORT (11), IPV4_DST_ADDR (12), DST_MASK (13)
- LAST_SWITCHED (21), FIRST_SWITCHED (22)
- APPLICATION_ID (95) — for NBAR app ID support

Create: `internal/engine/templates.go` with full v9 header, template flowset, data flowset encoding per the RFC. Use `encoding/binary` with BigEndian byte order (network byte order).

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/engine/ -v
```

Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/engine/templates.go internal/engine/templates_test.go
git commit -m "feat: add NetFlow v9 template and packet encoding"
```

---

### Task 5: IPFIX Template and Packet Encoding

**Files:**
- Create: `internal/engine/ipfix.go`
- Create: `internal/engine/ipfix_test.go`

**Step 1: Write tests for IPFIX encoding**

Similar structure to v9 but per RFC 7011:
- IPFIX header: version 10, length, export time, sequence, observation domain ID
- Template Set (ID 2): template ID, field count, information element IDs + lengths
- Data Set: template ID, data records

Test that:
- Version field is 10
- Template encodes correctly
- Data records encode correctly
- AppID (IANA IE 95) is included when non-zero

**Step 2: Implement IPFIX encoding**

Create `internal/engine/ipfix.go` following RFC 7011 format. Same field set as v9 but using IPFIX information element IDs and encoding rules.

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```bash
git add internal/engine/ipfix.go internal/engine/ipfix_test.go
git commit -m "feat: add IPFIX template and packet encoding"
```

---

### Task 6: Flow Engine Core

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`

**Step 1: Write tests for engine lifecycle**

```go
// internal/engine/engine_test.go
package engine

import (
	"net"
	"testing"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
)

func TestEngineStartStop(t *testing.T) {
	e := New()
	e.Start()
	if !e.Running() {
		t.Error("engine should be running after Start")
	}
	e.Stop()
	if e.Running() {
		t.Error("engine should not be running after Stop")
	}
}

func TestEngineAddFlow(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}

	f := config.Flow{
		Name:       "test-flow",
		SourceName: "web-01",
		SourcePort: 46578,
		DestName:   "db-01",
		DestPort:   5432,
		Protocol:   "TCP",
		Rate:       "90Mbps",
		Enabled:    true,
		ActiveTimeout: 1 * time.Second, // short for testing
	}

	e.AddMachine(src)
	e.AddMachine(dst)
	err := e.AddFlow(f)
	if err != nil {
		t.Fatalf("AddFlow failed: %v", err)
	}
	if e.FlowCount() != 1 {
		t.Errorf("expected 1 flow, got %d", e.FlowCount())
	}
}

func TestEngineFlowGeneratesRecords(t *testing.T) {
	e := New()
	records := e.Records() // channel of encoded records
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)
	e.AddMachine(dst)

	f := config.Flow{
		Name:          "test-flow",
		SourceName:    "web-01",
		SourcePort:    46578,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "1Mbps",
		Enabled:       true,
		ActiveTimeout: 100 * time.Millisecond, // very short for testing
	}
	e.AddFlow(f)

	select {
	case rec := <-records:
		if len(rec) == 0 {
			t.Error("received empty record")
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for flow record")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/engine/ -v -run "TestEngine"
```

**Step 3: Implement engine**

```go
// internal/engine/engine.go
package engine

import (
	"sync"

	"github.com/masterphelps/flowboy/internal/config"
)

// Engine manages flow goroutines and record generation.
type Engine struct {
	mu        sync.RWMutex
	machines  map[string]config.Machine
	flows     map[string]*flowRunner
	records   chan []byte
	stats     chan FlowStats
	running   bool
	stopCh    chan struct{}
}

// FlowStats carries real-time stats from a flow goroutine to the UI.
type FlowStats struct {
	FlowName    string
	BytesSent   uint64
	PacketsSent uint64
	Active      bool
}

// New creates a new Engine.
func New() *Engine {
	return &Engine{
		machines: make(map[string]config.Machine),
		flows:    make(map[string]*flowRunner),
		records:  make(chan []byte, 1000),
		stats:    make(chan FlowStats, 1000),
		stopCh:   make(chan struct{}),
	}
}

func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = true
	// Start all enabled flows
	for _, fr := range e.flows {
		if fr.flow.Enabled {
			go fr.run(e.records, e.stats, e.stopCh)
		}
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		close(e.stopCh)
		e.running = false
		e.stopCh = make(chan struct{}) // reset for potential restart
	}
}

func (e *Engine) Running() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

func (e *Engine) Records() <-chan []byte {
	return e.records
}

func (e *Engine) Stats() <-chan FlowStats {
	return e.stats
}

func (e *Engine) AddMachine(m config.Machine) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.machines[m.Name] = m
}

func (e *Engine) AddFlow(f config.Flow) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	src, ok := e.machines[f.SourceName]
	if !ok {
		return &MachineNotFoundError{f.SourceName}
	}
	dst, ok := e.machines[f.DestName]
	if !ok {
		return &MachineNotFoundError{f.DestName}
	}

	rate, err := config.ParseRate(f.Rate)
	if err != nil {
		return err
	}

	fr := &flowRunner{
		flow: f,
		src:  src,
		dst:  dst,
		rate: rate,
	}
	e.flows[f.Name] = fr

	if e.running && f.Enabled {
		go fr.run(e.records, e.stats, e.stopCh)
	}
	return nil
}

func (e *Engine) FlowCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.flows)
}

type MachineNotFoundError struct {
	Name string
}

func (e *MachineNotFoundError) Error() string {
	return "machine not found: " + e.Name
}
```

Then implement `flowRunner` in a separate file or within engine.go:

```go
// flowRunner manages a single flow's goroutine.
type flowRunner struct {
	flow config.Flow
	src  config.Machine
	dst  config.Machine
	rate config.Rate
}

func (fr *flowRunner) run(records chan<- []byte, stats chan<- FlowStats, stop <-chan struct{}) {
	ticker := time.NewTicker(fr.flow.ActiveTimeout)
	defer ticker.Stop()

	var totalBytes, totalPackets uint64
	for {
		select {
		case <-ticker.C:
			bytes := fr.rate.BytesPerInterval(fr.flow.ActiveTimeout)
			packets := bytes / 1500 // approximate MTU-sized packets
			if packets == 0 {
				packets = 1
			}
			totalBytes += bytes
			totalPackets += packets

			// Build v9 data record (IPFIX handled by exporter based on collector version)
			rec := V9DataRecord{
				SrcAddr:  ipTo4(fr.src.IP),
				DstAddr:  ipTo4(fr.dst.IP),
				SrcPort:  fr.flow.SourcePort,
				DstPort:  fr.flow.DestPort,
				Protocol: protocolNumber(fr.flow.Protocol),
				Octets:   uint32(bytes),
				Packets:  uint32(packets),
				AppID:    fr.flow.AppID,
			}
			records <- rec.Encode()

			stats <- FlowStats{
				FlowName:    fr.flow.Name,
				BytesSent:   totalBytes,
				PacketsSent: totalPackets,
				Active:      true,
			}
		case <-stop:
			return
		}
	}
}
```

Add helper functions `ipTo4` and `protocolNumber` in the same file.

**Step 4: Run tests**

```bash
go test ./internal/engine/ -v -run "TestEngine" -timeout 10s
```

Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat: add flow engine with goroutine-per-flow and channel-based records"
```

---

### Task 7: UDP Exporter with Fan-Out

**Files:**
- Create: `internal/engine/exporter.go`
- Create: `internal/engine/exporter_test.go`

**Step 1: Write tests for exporter**

Test that:
- Exporter reads from the records channel
- Sends UDP datagrams to multiple collector addresses
- Handles v9 vs IPFIX per collector
- Template records are sent periodically (every N data exports)
- Use a local UDP listener in tests to capture packets

**Step 2: Implement exporter**

The exporter goroutine:
1. Reads encoded records from the engine's records channel
2. Wraps them in v9 or IPFIX packets with proper headers
3. Sends template records every `templateInterval` exports (default every 10)
4. Fans out to all configured collectors via UDP
5. Tracks per-collector stats (packets sent, errors)

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```bash
git add internal/engine/exporter.go internal/engine/exporter_test.go
git commit -m "feat: add UDP exporter with multi-collector fan-out"
```

---

### Task 8: Wire Engine to CLI Entry Point

**Files:**
- Modify: `cmd/flowboy/main.go`

**Step 1: Update main.go to load config, start engine, and run in headless mode**

Wire up: parse flags → load YAML config → build machines/flows/collectors → start engine → start exporter → wait for signal (SIGINT/SIGTERM) → graceful shutdown.

This gives us a working headless flow generator before any UI work begins.

**Step 2: Manual integration test**

```bash
go build -o flowboy ./cmd/flowboy
./flowboy --headless --config configs/flowboy.yaml
```

Run `tcpdump` or a real collector to verify packets are arriving:

```bash
# In another terminal:
tcpdump -i lo0 -n udp port 2055
```

**Step 3: Commit**

```bash
git add cmd/flowboy/main.go
git commit -m "feat: wire engine to CLI with headless mode"
```

---

## Phase 3: TUI

### Task 9: Lip Gloss Pip-Boy Theme

**Files:**
- Create: `internal/tui/style.go`

**Step 1: Define the Pip-Boy color palette and styles**

```go
// internal/tui/style.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Pip-Boy color palette (from CodePen reference)
	colorGreen     = lipgloss.Color("#8df776")
	colorDimGreen  = lipgloss.Color("#172f18")
	colorBlack     = lipgloss.Color("#000000")
	colorPanel     = lipgloss.Color("#272b2a")
	colorAccent    = lipgloss.Color("#d8c99e")
	colorBorder    = lipgloss.Color("#333333")
	colorBright    = lipgloss.Color("#7ff12a")

	// Panel styles
	panelStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorGreen).
		Background(colorBlack).
		Foreground(colorGreen).
		Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Uppercase(true).
		MarginBottom(1)

	activeItemStyle = lipgloss.NewStyle().
		Foreground(colorBright).
		Bold(true)

	dimItemStyle = lipgloss.NewStyle().
		Foreground(colorDimGreen)

	statusBarStyle = lipgloss.NewStyle().
		Background(colorPanel).
		Foreground(colorGreen).
		Padding(0, 1).
		Width(80)

	titleStyle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Uppercase(true).
		Align(lipgloss.Center).
		MarginBottom(1)
)
```

**Step 2: Add bubbletea + lipgloss dependencies**

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

**Step 3: Commit**

```bash
git add internal/tui/style.go go.mod go.sum
git commit -m "feat: add Pip-Boy Lip Gloss theme"
```

---

### Task 10: TUI Main App Model

**Files:**
- Create: `internal/tui/app.go`

**Step 1: Implement the main Bubbletea model**

The main model composes sub-models:
- `machineList` (left panel)
- `flowList` (right panel)
- `collectorBar` (bottom)
- Tab state for STATS/MAP/CONFIG views

Handles window resize, keyboard navigation (tab between panels, j/k or arrows within panels, N/E/D for CRUD), and tick messages for animations.

The title bar renders "F L O W B O Y  3000" with the titleStyle.

**Step 2: Verify it runs**

```bash
go build -o flowboy ./cmd/flowboy && ./flowboy
```

Should show the Pip-Boy themed TUI frame with placeholder content.

**Step 3: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat: add TUI main app model with Pip-Boy layout"
```

---

### Task 11: TUI Machine List Panel

**Files:**
- Create: `internal/tui/machines.go`

Implement the machine list panel:
- Displays machines with name + IP/mask
- Arrow keys / j/k to navigate
- N to create new machine (inline form)
- E to edit selected machine
- D to delete (with confirmation)
- Selecting a machine filters the flows panel to show only its flows
- Alternating dim/normal rows for scan line effect

**Commit after working.**

---

### Task 12: TUI Active Flows Panel

**Files:**
- Create: `internal/tui/flows.go`

Implement the flows panel:
- Shows flow name, source:port → dest:port, protocol
- Throughput progress bar (██████░░░░)
- Oscilloscope waveform animation (~∿∿~ cycling on tick)
- N to create new flow, E to edit, D to delete
- Start/Stop individual flows

**Commit after working.**

---

### Task 13: TUI Collector Status Bar

**Files:**
- Create: `internal/tui/collectors.go`

Bottom bar showing:
- Each collector with address, connection state (● OK / ● ERR), packet count
- N to add, D to remove

**Commit after working.**

---

### Task 14: Wire TUI to Engine

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `cmd/flowboy/main.go`

Connect the TUI to the engine:
- Engine stats channel → TUI update messages
- TUI CRUD actions → engine API calls
- Config persistence on every change

**Commit after working.**

---

## Phase 4: Web UI

### Task 15: HTTP Server + WebSocket

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/handlers.go`
- Create: `internal/web/server_test.go`

**Step 1: Write tests for REST API**

Test endpoints:
- `GET /api/machines` — list machines
- `POST /api/machines` — create machine
- `DELETE /api/machines/:name` — delete machine
- `GET /api/flows` — list flows with stats
- `POST /api/flows` — create flow
- `PUT /api/flows/:name` — update flow
- `DELETE /api/flows/:name` — delete flow
- `POST /api/flows/:name/start` — start flow
- `POST /api/flows/:name/stop` — stop flow
- `GET /api/collectors` — list collectors
- `POST /api/collectors` — add collector
- `DELETE /api/collectors/:name` — remove collector
- `GET /api/engine/status` — engine state
- `POST /api/engine/start` — start all
- `POST /api/engine/stop` — stop all
- `WS /ws` — WebSocket for real-time stats

**Step 2: Implement server and handlers**

Use Go's `net/http` standard library (no framework). `embed.FS` for static files. WebSocket via `nhooyr.io/websocket` or `gorilla/websocket`.

The WebSocket broadcasts `FlowStats` messages from the engine as JSON.

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```bash
git add internal/web/ go.mod go.sum
git commit -m "feat: add HTTP server, REST API, and WebSocket for web UI"
```

---

### Task 16: Web UI HTML + CRT Shell

**Files:**
- Create: `internal/web/static/index.html`
- Create: `internal/web/static/css/pipboy.css`
- Create: `internal/web/static/css/app.css`

**Step 1: Build the CRT bezel shell**

`index.html` — single page with:
- Outer bezel div with "FLOWBOY 3000" header
- Three-tier grid layout (machines+collectors / flows / map)
- Status bar at bottom
- Scan line overlay div
- Screen reflection overlay div

`pipboy.css` — adapted from CodePen reference:
- `#8df776` green phosphor on `#000` background
- Droid Sans / monospace font, uppercase, bold
- Scan line animation (`@keyframes scan`)
- Screen reflection overlay (gradient, low opacity)
- CRT glow effects (`text-shadow`, `box-shadow`)
- Selection color `lightgreen`

`app.css` — Tailwind (via CDN) + custom:
- Three-tier grid layout
- Panel styles with green borders
- Responsive scaling

**Step 2: Verify bezel renders**

```bash
go build -o flowboy ./cmd/flowboy && ./flowboy --web
```

Open `http://localhost:8042` — should see empty CRT-styled dashboard.

**Step 3: Commit**

```bash
git add internal/web/static/
git commit -m "feat: add web UI CRT shell with Pip-Boy theme"
```

---

### Task 17: Web UI Dashboard Panels (Machines, Flows, Collectors)

**Files:**
- Create: `internal/web/static/js/app.js`
- Create: `internal/web/static/js/ws.js`

**Step 1: Implement app.js**

Vanilla JS SPA:
- Fetches machines, flows, collectors from REST API
- Renders into the three-tier layout panels
- CRUD modals/forms for add/edit/delete
- WebSocket connection for live flow stats updates
- Progress bars + oscilloscope waveform text animations
- Click handlers for bidirectional highlighting (click machine → highlight its flows, etc.)

**Step 2: Implement ws.js**

WebSocket client:
- Connects to `ws://localhost:8042/ws`
- Parses incoming FlowStats JSON
- Updates flow throughput bars and packet counters in real-time
- Reconnects on disconnect

**Step 3: Verify panels work with live data**

Start flowboy with some config, open the web UI, create/edit/delete machines and flows through the UI.

**Step 4: Commit**

```bash
git add internal/web/static/js/
git commit -m "feat: add web UI dashboard panels with live WebSocket updates"
```

---

### Task 18: Pixel-Art Network Map (Canvas)

**Files:**
- Create: `internal/web/static/js/map.js`

This is the crown jewel. Implement a Canvas-based pixel-art network map:

**Step 1: Pixel grid renderer**

- Set up Canvas with `image-rendering: pixelated`
- Define a pixel grid size (e.g., 4px per grid cell)
- Disable anti-aliasing: `ctx.imageSmoothingEnabled = false`
- All drawing snaps to grid coordinates

**Step 2: Segment and node rendering**

- Read segments from `/api/machines` (server derives segments from machine subnets)
- Draw segment boundaries as dotted pixel borders
- Label segments with CIDR notation in pixel font
- Draw machine nodes as 8-bit pixel-art terminal icons (chunky green squares with label)

**Step 3: Auto-layout algorithm**

- Segments arranged in a grid or force-directed layout
- Machines placed within their segment bounds
- Flow lines are pixelated stepped paths (horizontal then vertical, like circuit traces)

**Step 4: Animated packet sprites**

- Each active flow spawns pixel sprites at the source node
- Sprites travel along the flow line toward destination
- Sprite density proportional to throughput (bits per second)
- Sprite types: TCP = solid square ▪, UDP = hollow square ▫, ICMP = diamond ◆
- Animation runs on `requestAnimationFrame`

**Step 5: Flow start/stop effects**

- Flow start: "power up" pixel burst animation at source node
- Idle flows: dim dashed pixel line, no sprites
- Flow stop: sprites drain and line dims

**Step 6: Interactions**

- Click a node → highlight in machine list + filter flows panel
- Click a flow line → highlight in flow panel
- Hover shows tooltip with flow details
- Bidirectional: selecting in panels above highlights on map

**Step 7: CRT overlay**

- Scan line animation layer on top of Canvas
- Green phosphor glow around bright pixels (`box-shadow` or canvas post-processing)

**Step 8: Verify the map works**

Configure 3+ machines across 2+ subnets, create flows between them, start the engine, and watch pixel packets fly.

**Step 9: Commit**

```bash
git add internal/web/static/js/map.js
git commit -m "feat: add pixel-art Space Invaders network map with animated packets"
```

---

## Phase 5: Integration + Polish

### Task 19: Wire Web UI to Engine

**Files:**
- Modify: `cmd/flowboy/main.go`
- Modify: `internal/web/server.go`

Connect the web server to the engine instance:
- REST handlers operate on the shared engine
- WebSocket broadcasts engine stats
- Config persistence on changes
- Support `--web`, `--both`, `--port` flags

**Commit after working.**

---

### Task 20: Embed Static Files

**Files:**
- Modify: `internal/web/server.go`

Use `//go:embed static/*` to bundle all web assets into the binary. Verify single-binary deployment works:

```bash
go build -o flowboy ./cmd/flowboy
# Move binary somewhere with no static/ directory
cp flowboy /tmp/
/tmp/flowboy --web
```

Open `http://localhost:8042` — should work with embedded assets.

**Commit after working.**

---

### Task 21: End-to-End Integration Test

**Files:**
- Create: `tests/integration_test.go`

Write an integration test that:
1. Loads a test YAML config
2. Starts the engine
3. Starts a local UDP listener (fake collector)
4. Waits for NetFlow packets
5. Validates packet structure (v9 header, correct source/dest IPs)
6. Stops the engine
7. Verifies graceful shutdown

```bash
go test ./tests/ -v -tags integration -timeout 30s
```

**Commit after working.**

---

### Task 22: Polish and README

**Files:**
- Create: `README.md` (only because this is a new project that needs install/usage docs)
- Modify: any files needing cleanup

- Add a README with: project description, build instructions, usage examples, screenshots placeholder, config file reference
- Verify all CLI modes work: `--tui`, `--web`, `--both`, `--headless`
- Verify config round-trip: start → create machines/flows via UI → stop → restart → data persisted

**Final commit:**

```bash
git add -A
git commit -m "docs: add README and polish for initial release"
```

---

## Task Dependency Graph

```
Task 1 (skeleton)
  └→ Task 2 (types)
      └→ Task 3 (config)
          └→ Task 4 (v9 encoding)
              └→ Task 5 (IPFIX encoding)
                  └→ Task 6 (engine core)
                      └→ Task 7 (exporter)
                          └→ Task 8 (CLI wiring)
                              ├→ Task 9 (TUI theme)
                              │   └→ Task 10 (TUI app)
                              │       ├→ Task 11 (TUI machines)
                              │       ├→ Task 12 (TUI flows)
                              │       └→ Task 13 (TUI collectors)
                              │           └→ Task 14 (TUI ↔ engine)
                              └→ Task 15 (web server)
                                  └→ Task 16 (web CRT shell)
                                      └→ Task 17 (web panels)
                                          └→ Task 18 (pixel map) ★
                                              └→ Task 19 (web ↔ engine)
                                                  └→ Task 20 (embed)
                                                      └→ Task 21 (integration test)
                                                          └→ Task 22 (polish)
```

**Note:** Tasks 9-14 (TUI) and Tasks 15-18 (Web UI) can be done in parallel once Task 8 is complete.

**Estimated commits:** 22
