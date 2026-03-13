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
