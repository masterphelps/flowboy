package engine

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
)

// newTestUDPListener creates a local UDP listener on a random port.
func newTestUDPListener(t *testing.T) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve UDP addr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}
	return conn
}

// readUDPPacket reads a single UDP packet from the connection with a timeout.
func readUDPPacket(conn *net.UDPConn, timeout time.Duration) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 65535)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func TestExporterSendsUDPPackets(t *testing.T) {
	listener := newTestUDPListener(t)
	defer listener.Close()

	records := make(chan []byte, 16)

	collectors := []config.Collector{
		{Name: "test-collector", Address: listener.LocalAddr().String(), Version: "v9"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.Start()
	defer exp.Stop()

	// Send a data record through the channel.
	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   1000,
		Packets:  10,
	}
	records <- rec.Encode()

	// Read a UDP packet from the listener.
	data, err := readUDPPacket(listener, 3*time.Second)
	if err != nil {
		t.Fatalf("failed to receive UDP packet: %v", err)
	}

	// The first record triggers a template send (interval=1 on first record).
	// We may receive a template packet first, then a data packet, or both in one packet.
	// At minimum we should receive something valid.
	if len(data) < 20 {
		t.Fatalf("packet too short: %d bytes", len(data))
	}

	// Check version 9 header.
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 9 {
		t.Errorf("expected version 9, got %d", version)
	}
}

func TestExporterFanOutMultipleCollectors(t *testing.T) {
	listener1 := newTestUDPListener(t)
	defer listener1.Close()
	listener2 := newTestUDPListener(t)
	defer listener2.Close()
	listener3 := newTestUDPListener(t)
	defer listener3.Close()

	records := make(chan []byte, 16)

	collectors := []config.Collector{
		{Name: "collector-1", Address: listener1.LocalAddr().String(), Version: "v9"},
		{Name: "collector-2", Address: listener2.LocalAddr().String(), Version: "v9"},
		{Name: "collector-3", Address: listener3.LocalAddr().String(), Version: "v9"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.Start()
	defer exp.Stop()

	// Send a data record.
	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   5000,
		Packets:  50,
	}
	records <- rec.Encode()

	// All three listeners should receive packets.
	var wg sync.WaitGroup
	errs := make([]error, 3)
	listeners := []*net.UDPConn{listener1, listener2, listener3}

	for i, l := range listeners {
		wg.Add(1)
		go func(idx int, conn *net.UDPConn) {
			defer wg.Done()
			// May receive template packet and/or data packet.
			_, err := readUDPPacket(conn, 3*time.Second)
			errs[idx] = err
		}(i, l)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("collector-%d did not receive packet: %v", i+1, err)
		}
	}
}

func TestExporterTemplateSentPeriodically(t *testing.T) {
	listener := newTestUDPListener(t)
	defer listener.Close()

	records := make(chan []byte, 64)

	collectors := []config.Collector{
		{Name: "collector", Address: listener.LocalAddr().String(), Version: "v9"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	// Send template every 3 data records.
	exp.templateInterval = 3
	exp.Start()
	defer exp.Stop()

	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   1000,
		Packets:  10,
	}

	// Send enough records to trigger multiple template sends.
	// With interval=3, templates should be sent at records 1, 4, 7, etc.
	numRecords := 7
	for i := 0; i < numRecords; i++ {
		records <- rec.Encode()
	}

	// Collect all packets received within a reasonable timeout.
	var packets [][]byte
	for {
		data, err := readUDPPacket(listener, 2*time.Second)
		if err != nil {
			break
		}
		packets = append(packets, data)
	}

	if len(packets) == 0 {
		t.Fatal("received no packets")
	}

	// Count how many packets contain a template flowset (FlowSet ID = 0).
	templateCount := 0
	for _, pkt := range packets {
		if len(pkt) < 24 {
			continue
		}
		// After the 20-byte v9 header, the first flowset starts.
		flowsetID := binary.BigEndian.Uint16(pkt[20:22])
		if flowsetID == 0 {
			templateCount++
		}
	}

	// We sent 7 records with interval=3, so templates at records 1, 4, 7 = 3 templates.
	// Template packets are separate from data packets, so we expect at least 3 template packets.
	if templateCount < 3 {
		t.Errorf("expected at least 3 template packets, got %d (out of %d total packets)", templateCount, len(packets))
	}
}

func TestExporterV9VersionHeader(t *testing.T) {
	listener := newTestUDPListener(t)
	defer listener.Close()

	records := make(chan []byte, 16)

	collectors := []config.Collector{
		{Name: "v9-collector", Address: listener.LocalAddr().String(), Version: "v9"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.Start()
	defer exp.Stop()

	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   1000,
		Packets:  10,
	}
	records <- rec.Encode()

	// Collect all packets (template + data).
	var allPackets [][]byte
	for {
		data, err := readUDPPacket(listener, 3*time.Second)
		if err != nil {
			break
		}
		allPackets = append(allPackets, data)
	}

	if len(allPackets) == 0 {
		t.Fatal("received no packets")
	}

	for i, pkt := range allPackets {
		if len(pkt) < 2 {
			t.Errorf("packet %d too short", i)
			continue
		}
		version := binary.BigEndian.Uint16(pkt[0:2])
		if version != 9 {
			t.Errorf("packet %d: expected v9 header (version 9), got %d", i, version)
		}
	}
}

func TestExporterIPFIXVersionHeader(t *testing.T) {
	listener := newTestUDPListener(t)
	defer listener.Close()

	records := make(chan []byte, 16)

	collectors := []config.Collector{
		{Name: "ipfix-collector", Address: listener.LocalAddr().String(), Version: "ipfix"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.Start()
	defer exp.Stop()

	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   1000,
		Packets:  10,
	}
	records <- rec.Encode()

	// Collect all packets.
	var allPackets [][]byte
	for {
		data, err := readUDPPacket(listener, 3*time.Second)
		if err != nil {
			break
		}
		allPackets = append(allPackets, data)
	}

	if len(allPackets) == 0 {
		t.Fatal("received no packets")
	}

	for i, pkt := range allPackets {
		if len(pkt) < 2 {
			t.Errorf("packet %d too short", i)
			continue
		}
		version := binary.BigEndian.Uint16(pkt[0:2])
		if version != 10 {
			t.Errorf("packet %d: expected IPFIX header (version 10), got %d", i, version)
		}
	}
}

func TestExporterPerCollectorStats(t *testing.T) {
	listener1 := newTestUDPListener(t)
	defer listener1.Close()
	listener2 := newTestUDPListener(t)
	defer listener2.Close()

	records := make(chan []byte, 64)

	collectors := []config.Collector{
		{Name: "stats-v9", Address: listener1.LocalAddr().String(), Version: "v9"},
		{Name: "stats-ipfix", Address: listener2.LocalAddr().String(), Version: "ipfix"},
	}

	exp, err := NewExporter(records, collectors)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.Start()
	defer exp.Stop()

	rec := V9DataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   1000,
		Packets:  10,
	}

	// Send 3 records.
	for i := 0; i < 3; i++ {
		records <- rec.Encode()
	}

	// Drain packets from both listeners so sends aren't blocked.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			if _, err := readUDPPacket(listener1, 2*time.Second); err != nil {
				break
			}
		}
	}()
	go func() {
		for {
			if _, err := readUDPPacket(listener2, 2*time.Second); err != nil {
				return
			}
		}
	}()

	<-drainDone

	stats := exp.GetStats()

	// Both collectors should have stats entries.
	s1, ok := stats["stats-v9"]
	if !ok {
		t.Fatal("missing stats for stats-v9")
	}
	s2, ok := stats["stats-ipfix"]
	if !ok {
		t.Fatal("missing stats for stats-ipfix")
	}

	// Each collector should have sent at least 3 packets (could be more with templates).
	if s1.PacketsSent == 0 {
		t.Errorf("stats-v9: expected non-zero PacketsSent, got %d", s1.PacketsSent)
	}
	if s1.BytesSent == 0 {
		t.Errorf("stats-v9: expected non-zero BytesSent, got %d", s1.BytesSent)
	}
	if s2.PacketsSent == 0 {
		t.Errorf("stats-ipfix: expected non-zero PacketsSent, got %d", s2.PacketsSent)
	}
	if s2.BytesSent == 0 {
		t.Errorf("stats-ipfix: expected non-zero BytesSent, got %d", s2.BytesSent)
	}
}
