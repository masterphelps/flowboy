//go:build integration

package tests

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

// startUDPListener creates a UDP listener on a random local port and returns
// the connection and the "host:port" address string suitable for a collector.
func startUDPListener(t *testing.T) (*net.UDPConn, string) {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve UDP addr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}
	return conn, conn.LocalAddr().String()
}

// readPackets reads UDP packets from conn until timeout, returning all received packets.
func readPackets(conn *net.UDPConn, timeout time.Duration) [][]byte {
	var packets [][]byte
	buf := make([]byte, 65535)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Timeout is expected; keep trying until deadline.
			continue
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		packets = append(packets, pkt)
	}
	return packets
}

// writeTempConfig writes a YAML config to a temp file and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestFullFlowPipeline(t *testing.T) {
	// 1. Start a local UDP listener (our "collector").
	conn, collectorAddr := startUDPListener(t)
	defer conn.Close()

	// 2. Create a temp YAML config.
	yamlContent := fmt.Sprintf(`
machines:
  - name: server-a
    ip: 10.0.1.10
    mask: 24
  - name: server-b
    ip: 10.0.2.20
    mask: 24
flows:
  - name: web-traffic
    source: server-a
    source_port: 443
    destination: server-b
    destination_port: 8080
    protocol: TCP
    rate: 10Mbps
    active_timeout: 100ms
    enabled: true
collectors:
  - name: local-v9
    address: %s
    version: v9
`, collectorAddr)

	cfgPath := writeTempConfig(t, yamlContent)

	// 3. Load the config.
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// 4. Create engine, add machines, start engine, add flows.
	eng := engine.New()

	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			t.Fatalf("convert machine %s: %v", mc.Name, err)
		}
		eng.AddMachine(m)
	}

	eng.Start()

	for _, fc := range cfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			t.Fatalf("convert flow %s: %v", fc.Name, err)
		}
		if err := eng.AddFlow(f); err != nil {
			t.Fatalf("add flow %s: %v", f.Name, err)
		}
	}

	// 5. Create exporter pointing at the local UDP listener.
	exp, err := engine.NewExporter(eng.Records(), cfg.Collectors)
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}

	// 6. Start exporter.
	exp.Start()

	// 7. Wait for UDP packets with timeout.
	packets := readPackets(conn, 3*time.Second)

	// 8. Validate received packets.
	if len(packets) == 0 {
		t.Fatal("expected at least 1 UDP packet, got 0")
	}
	t.Logf("received %d packets", len(packets))

	// Find a data packet (not just a template). The exporter sends template
	// packets and data packets. We look for a packet with version 9 header.
	var foundValid bool
	for _, pkt := range packets {
		if len(pkt) < 20 {
			continue
		}
		// First 2 bytes should be version 9 (0x00, 0x09).
		if pkt[0] == 0x00 && pkt[1] == 0x09 {
			foundValid = true
			if len(pkt) <= 20 {
				t.Errorf("packet too short: got %d bytes, want > 20", len(pkt))
			}
			break
		}
	}
	if !foundValid {
		t.Fatal("no valid NetFlow v9 packet found (expected version 0x0009 in header)")
	}

	// 9. Stop engine and exporter.
	eng.Stop()
	exp.Stop()

	// 10. Verify engine stopped cleanly.
	if eng.Running() {
		t.Error("engine should not be running after Stop()")
	}
}

func TestIPFIXFlowPipeline(t *testing.T) {
	// 1. Start a local UDP listener (our "collector").
	conn, collectorAddr := startUDPListener(t)
	defer conn.Close()

	// 2. Create config with IPFIX collector.
	yamlContent := fmt.Sprintf(`
machines:
  - name: router-1
    ip: 192.168.1.1
    mask: 24
  - name: router-2
    ip: 192.168.2.1
    mask: 24
flows:
  - name: ipfix-flow
    source: router-1
    source_port: 22
    destination: router-2
    destination_port: 443
    protocol: TCP
    rate: 5Mbps
    active_timeout: 100ms
    enabled: true
collectors:
  - name: ipfix-collector
    address: %s
    version: ipfix
`, collectorAddr)

	cfgPath := writeTempConfig(t, yamlContent)

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	eng := engine.New()

	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			t.Fatalf("convert machine %s: %v", mc.Name, err)
		}
		eng.AddMachine(m)
	}

	eng.Start()

	for _, fc := range cfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			t.Fatalf("convert flow %s: %v", fc.Name, err)
		}
		if err := eng.AddFlow(f); err != nil {
			t.Fatalf("add flow %s: %v", f.Name, err)
		}
	}

	exp, err := engine.NewExporter(eng.Records(), cfg.Collectors)
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}
	exp.Start()

	packets := readPackets(conn, 3*time.Second)

	if len(packets) == 0 {
		t.Fatal("expected at least 1 UDP packet, got 0")
	}
	t.Logf("received %d packets", len(packets))

	// Validate first 2 bytes are version 10 (0x00, 0x0A) for IPFIX.
	var foundValid bool
	for _, pkt := range packets {
		if len(pkt) < 16 {
			continue
		}
		if pkt[0] == 0x00 && pkt[1] == 0x0A {
			foundValid = true
			if len(pkt) <= 20 {
				t.Errorf("IPFIX packet too short: got %d bytes, want > 20", len(pkt))
			}
			break
		}
	}
	if !foundValid {
		t.Fatal("no valid IPFIX packet found (expected version 0x000A in header)")
	}

	eng.Stop()
	exp.Stop()

	if eng.Running() {
		t.Error("engine should not be running after Stop()")
	}
}

func TestMultiCollectorFanOut(t *testing.T) {
	// 1. Set up 2 local UDP listeners.
	conn1, addr1 := startUDPListener(t)
	defer conn1.Close()
	conn2, addr2 := startUDPListener(t)
	defer conn2.Close()

	// 2. Create config with both collectors.
	yamlContent := fmt.Sprintf(`
machines:
  - name: host-x
    ip: 172.16.0.10
    mask: 16
  - name: host-y
    ip: 172.16.0.20
    mask: 16
flows:
  - name: fanout-flow
    source: host-x
    source_port: 80
    destination: host-y
    destination_port: 9090
    protocol: UDP
    rate: 1Mbps
    active_timeout: 100ms
    enabled: true
collectors:
  - name: collector-1
    address: %s
    version: v9
  - name: collector-2
    address: %s
    version: v9
`, addr1, addr2)

	cfgPath := writeTempConfig(t, yamlContent)

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	eng := engine.New()

	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			t.Fatalf("convert machine %s: %v", mc.Name, err)
		}
		eng.AddMachine(m)
	}

	eng.Start()

	for _, fc := range cfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			t.Fatalf("convert flow %s: %v", fc.Name, err)
		}
		if err := eng.AddFlow(f); err != nil {
			t.Fatalf("add flow %s: %v", f.Name, err)
		}
	}

	exp, err := engine.NewExporter(eng.Records(), cfg.Collectors)
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}
	exp.Start()

	// 3. Read from both listeners concurrently.
	type result struct {
		packets [][]byte
	}
	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)

	go func() {
		ch1 <- result{packets: readPackets(conn1, 3*time.Second)}
	}()
	go func() {
		ch2 <- result{packets: readPackets(conn2, 3*time.Second)}
	}()

	r1 := <-ch1
	r2 := <-ch2

	// 4. Verify BOTH listeners received packets.
	if len(r1.packets) == 0 {
		t.Error("collector-1 received 0 packets, expected at least 1")
	} else {
		t.Logf("collector-1 received %d packets", len(r1.packets))
	}

	if len(r2.packets) == 0 {
		t.Error("collector-2 received 0 packets, expected at least 1")
	} else {
		t.Logf("collector-2 received %d packets", len(r2.packets))
	}

	eng.Stop()
	exp.Stop()
}

func TestConfigPersistence(t *testing.T) {
	// 1. Create an initial config and write it out.
	initialYAML := `
machines:
  - name: original-host
    ip: 10.10.10.1
    mask: 24
flows: []
collectors: []
`
	cfgPath := writeTempConfig(t, initialYAML)

	// 2. Load config.
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(cfg.Machines) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(cfg.Machines))
	}

	// 3. Create engine, add machines from config.
	eng := engine.New()
	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			t.Fatalf("convert machine: %v", err)
		}
		eng.AddMachine(m)
	}

	// 4. Add a new machine via engine.
	newMachine := config.Machine{
		Name: "added-host",
		IP:   net.ParseIP("10.10.10.2"),
		Mask: net.CIDRMask(24, 32),
	}
	eng.AddMachine(newMachine)

	// 5. Update config with engine's machines, then save.
	machines := eng.Machines()
	cfg.Machines = make([]config.MachineConfig, 0, len(machines))
	for _, m := range machines {
		ones, _ := m.Mask.Size()
		cfg.Machines = append(cfg.Machines, config.MachineConfig{
			Name: m.Name,
			IP:   m.IP.String(),
			Mask: ones,
		})
	}

	savePath := filepath.Join(t.TempDir(), "saved_config.yaml")
	if err := config.SaveConfig(cfg, savePath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// 6. Reload config from saved file.
	reloaded, err := config.LoadConfig(savePath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	// 7. Verify the new machine is present.
	if len(reloaded.Machines) != 2 {
		t.Fatalf("expected 2 machines after reload, got %d", len(reloaded.Machines))
	}

	found := false
	for _, mc := range reloaded.Machines {
		if mc.Name == "added-host" && mc.IP == "10.10.10.2" && mc.Mask == 24 {
			found = true
			break
		}
	}
	if !found {
		t.Error("reloaded config does not contain the added machine 'added-host'")
	}

	// Verify original machine is still there too.
	foundOriginal := false
	for _, mc := range reloaded.Machines {
		if mc.Name == "original-host" && mc.IP == "10.10.10.1" && mc.Mask == 24 {
			foundOriginal = true
			break
		}
	}
	if !foundOriginal {
		t.Error("reloaded config is missing the original machine 'original-host'")
	}
}
