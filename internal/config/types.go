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
	Name string     `yaml:"name"`
	IP   net.IP     `yaml:"ip"`
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
	ConnectionStyle string        `yaml:"connection_style,omitempty"` // "persistent" or "transactional"
	Fluctuation     *Fluctuation  `yaml:"fluctuation,omitempty"`
}

// NewFlow returns a Flow with sensible defaults.
func NewFlow() Flow {
	return Flow{
		Protocol:        "TCP",
		ActiveTimeout:   60 * time.Second,
		InactiveTimeout: 15 * time.Second,
		Enabled:         true,
		ConnectionStyle: "persistent",
	}
}

// Collector defines a NetFlow export target.
type Collector struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Version string `yaml:"version"` // "v9" or "ipfix"
}

// Fluctuation controls sine-wave rate variation.
type Fluctuation struct {
	Amplitude float64       `yaml:"amplitude"`      // 0.0-1.0, default 0.3
	Period    time.Duration `yaml:"period"`          // default 1h
	Phase     time.Duration `yaml:"phase,omitempty"` // offset, default 0
}

// DefaultFluctuation returns fluctuation with sensible defaults.
func DefaultFluctuation() Fluctuation {
	return Fluctuation{
		Amplitude: 0.3,
		Period:    time.Hour,
		Phase:     0,
	}
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
