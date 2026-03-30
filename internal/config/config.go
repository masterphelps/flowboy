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
	Enabled         bool         `yaml:"enabled"`
	ConnectionStyle string       `yaml:"connection_style,omitempty"`
	Fluctuation     *Fluctuation `yaml:"fluctuation,omitempty"`
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

// Segment groups machines by network prefix.
type Segment struct {
	CIDR     net.IPNet
	Machines []Machine
}

// Config is the top-level YAML configuration.
type Config struct {
	Machines    []MachineConfig `yaml:"machines"`
	Flows       []FlowConfig    `yaml:"flows"`
	Collectors  []Collector     `yaml:"collectors"`
	Fluctuation *Fluctuation    `yaml:"fluctuation,omitempty"`
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

// isRFC1918 returns true if ip is in a private address range.
func isRFC1918(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// 10.0.0.0/8
	if ip4[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	return false
}

// BuildSegments auto-groups machines by their subnet.
// Non-RFC1918 addresses are grouped into a single "PUBLIC" segment.
func (cfg *Config) BuildSegments() []Segment {
	segMap := make(map[string]*Segment)
	var publicMachines []Machine

	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			continue
		}

		if isRFC1918(m.IP) {
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
		} else {
			publicMachines = append(publicMachines, m)
		}
	}

	segments := make([]Segment, 0, len(segMap)+1)
	for _, s := range segMap {
		segments = append(segments, *s)
	}

	if len(publicMachines) > 0 {
		_, publicNet, _ := net.ParseCIDR("0.0.0.0/0")
		segments = append(segments, Segment{
			CIDR:     *publicNet,
			Machines: publicMachines,
		})
	}

	return segments
}
