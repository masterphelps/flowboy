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
		Name:          "test-flow",
		SourceName:    "web-01",
		SourcePort:    46578,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "90Mbps",
		Enabled:       true,
		ActiveTimeout: 1 * time.Second,
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
	records := e.Records()
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
		ActiveTimeout: 100 * time.Millisecond,
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

func TestEngineAddFlowNonExistentSource(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(dst)

	f := config.Flow{
		Name:          "bad-flow",
		SourceName:    "ghost-machine",
		SourcePort:    1234,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "1Mbps",
		Enabled:       true,
		ActiveTimeout: 1 * time.Second,
	}

	err := e.AddFlow(f)
	if err == nil {
		t.Error("expected error when source machine does not exist")
	}
}

func TestEngineAddFlowNonExistentDest(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)

	f := config.Flow{
		Name:          "bad-flow",
		SourceName:    "web-01",
		SourcePort:    1234,
		DestName:      "ghost-dest",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "1Mbps",
		Enabled:       true,
		ActiveTimeout: 1 * time.Second,
	}

	err := e.AddFlow(f)
	if err == nil {
		t.Error("expected error when destination machine does not exist")
	}
}

func TestEngineAddFlowInvalidRate(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)
	e.AddMachine(dst)

	f := config.Flow{
		Name:          "bad-rate-flow",
		SourceName:    "web-01",
		SourcePort:    1234,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "not-a-rate",
		Enabled:       true,
		ActiveTimeout: 1 * time.Second,
	}

	err := e.AddFlow(f)
	if err == nil {
		t.Error("expected error for invalid rate")
	}
}

func TestEngineRemoveFlow(t *testing.T) {
	e := New()
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
		ActiveTimeout: 1 * time.Second,
	}
	e.AddFlow(f)

	if e.FlowCount() != 1 {
		t.Fatalf("expected 1 flow, got %d", e.FlowCount())
	}

	err := e.RemoveFlow("test-flow")
	if err != nil {
		t.Fatalf("RemoveFlow failed: %v", err)
	}
	if e.FlowCount() != 0 {
		t.Errorf("expected 0 flows after removal, got %d", e.FlowCount())
	}
}

func TestEngineRemoveFlowNonExistent(t *testing.T) {
	e := New()
	e.Start()
	defer e.Stop()

	err := e.RemoveFlow("no-such-flow")
	if err == nil {
		t.Error("expected error when removing non-existent flow")
	}
}

func TestEngineFlowStats(t *testing.T) {
	e := New()
	stats := e.Stats()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)
	e.AddMachine(dst)

	f := config.Flow{
		Name:          "stats-flow",
		SourceName:    "web-01",
		SourcePort:    46578,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "1Mbps",
		Enabled:       true,
		ActiveTimeout: 100 * time.Millisecond,
	}
	e.AddFlow(f)

	select {
	case s := <-stats:
		if s.FlowName != "stats-flow" {
			t.Errorf("expected flow name 'stats-flow', got %q", s.FlowName)
		}
		if !s.Active {
			t.Error("expected flow to be active")
		}
		if s.BytesSent == 0 {
			t.Error("expected non-zero bytes sent")
		}
		if s.PacketsSent == 0 {
			t.Error("expected non-zero packets sent")
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for flow stats")
	}
}

func TestEngineDoubleStart(t *testing.T) {
	e := New()
	e.Start()
	e.Start() // should be a no-op
	if !e.Running() {
		t.Error("engine should be running")
	}
	e.Stop()
}

func TestEngineDoubleStop(t *testing.T) {
	e := New()
	e.Start()
	e.Stop()
	e.Stop() // should be a no-op, not panic
	if e.Running() {
		t.Error("engine should not be running")
	}
}

func TestProtocolNumber(t *testing.T) {
	tests := []struct {
		name string
		want uint8
	}{
		{"TCP", 6},
		{"tcp", 6},
		{"UDP", 17},
		{"udp", 17},
		{"ICMP", 1},
		{"icmp", 1},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := protocolNumber(tt.name)
		if got != tt.want {
			t.Errorf("protocolNumber(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestIPTo4(t *testing.T) {
	ip := net.ParseIP("192.168.1.100")
	got := ipTo4(ip)
	want := [4]byte{192, 168, 1, 100}
	if got != want {
		t.Errorf("ipTo4(%v) = %v, want %v", ip, got, want)
	}
}

func TestMaskPrefixLen(t *testing.T) {
	mask := net.CIDRMask(24, 32)
	got := maskPrefixLen(mask)
	if got != 24 {
		t.Errorf("maskPrefixLen(/24) = %d, want 24", got)
	}

	mask16 := net.CIDRMask(16, 32)
	got16 := maskPrefixLen(mask16)
	if got16 != 16 {
		t.Errorf("maskPrefixLen(/16) = %d, want 16", got16)
	}
}

func TestEngineAddFlowDisabled(t *testing.T) {
	e := New()
	records := e.Records()
	e.Start()
	defer e.Stop()

	src := config.Machine{Name: "web-01", IP: net.ParseIP("192.168.50.201"), Mask: net.CIDRMask(24, 32)}
	dst := config.Machine{Name: "db-01", IP: net.ParseIP("10.70.22.45"), Mask: net.CIDRMask(24, 32)}
	e.AddMachine(src)
	e.AddMachine(dst)

	f := config.Flow{
		Name:          "disabled-flow",
		SourceName:    "web-01",
		SourcePort:    46578,
		DestName:      "db-01",
		DestPort:      5432,
		Protocol:      "TCP",
		Rate:          "1Mbps",
		Enabled:       false,
		ActiveTimeout: 100 * time.Millisecond,
	}
	err := e.AddFlow(f)
	if err != nil {
		t.Fatalf("AddFlow failed for disabled flow: %v", err)
	}
	if e.FlowCount() != 1 {
		t.Errorf("expected 1 flow, got %d", e.FlowCount())
	}

	// Disabled flow should not generate records.
	select {
	case <-records:
		t.Error("disabled flow should not generate records")
	case <-time.After(300 * time.Millisecond):
		// expected: no records
	}
}
