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

func TestFluctuationDefaults(t *testing.T) {
	f := DefaultFluctuation()
	if f.Floor != 0.7 {
		t.Errorf("expected floor 0.7, got %f", f.Floor)
	}
	if f.Ceiling != 1.3 {
		t.Errorf("expected ceiling 1.3, got %f", f.Ceiling)
	}
	if f.Period != time.Hour {
		t.Errorf("expected period 1h, got %v", f.Period)
	}
	floor, ceiling := f.EffectiveRange()
	if floor != 0.7 || ceiling != 1.3 {
		t.Errorf("EffectiveRange: expected 0.7/1.3, got %f/%f", floor, ceiling)
	}
}

func TestFluctuationLegacyAmplitude(t *testing.T) {
	f := Fluctuation{Amplitude: 0.3, Period: time.Hour}
	floor, ceiling := f.EffectiveRange()
	if floor != 0.7 || ceiling != 1.3 {
		t.Errorf("legacy amplitude: expected 0.7/1.3, got %f/%f", floor, ceiling)
	}
}

func TestConnectionStyleDefault(t *testing.T) {
	f := NewFlow()
	if f.ConnectionStyle != "persistent" {
		t.Errorf("expected persistent, got %s", f.ConnectionStyle)
	}
}

func TestParseRate(t *testing.T) {
	tests := []struct {
		input   string
		bps     uint64
		wantErr bool
	}{
		{"90Mbps", 90_000_000, false},
		{"1Gbps", 1_000_000_000, false},
		{"500Kbps", 500_000, false},
		{"10GB/day", 925_925, false}, // 10GB / 86400s ~ 925925 bps (bytes, converted)
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
