package engine

import (
	"encoding/binary"
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
		SrcAddr:   [4]byte{192, 168, 50, 201},
		DstAddr:   [4]byte{10, 70, 22, 45},
		SrcPort:   46578,
		DstPort:   5432,
		Protocol:  6, // TCP
		Octets:    675_000_000,
		Packets:   450_000,
		FirstSeen: 1000,
		LastSeen:  61000,
		AppID:     0,
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

func TestV9TemplateHeaderFields(t *testing.T) {
	tmpl := &V9Template{
		SourceID:  42,
		SysUptime: 5000,
		Sequence:  7,
	}
	data := tmpl.Encode()

	// Header is 20 bytes.
	if len(data) < 20 {
		t.Fatalf("packet too short for header: %d bytes", len(data))
	}

	version := binary.BigEndian.Uint16(data[0:2])
	count := binary.BigEndian.Uint16(data[2:4])
	sysUptime := binary.BigEndian.Uint32(data[4:8])
	seq := binary.BigEndian.Uint32(data[12:16])
	srcID := binary.BigEndian.Uint32(data[16:20])

	if version != 9 {
		t.Errorf("version: got %d, want 9", version)
	}
	if count != 1 {
		t.Errorf("count: got %d, want 1", count)
	}
	if sysUptime != 5000 {
		t.Errorf("sysUptime: got %d, want 5000", sysUptime)
	}
	if seq != 7 {
		t.Errorf("sequence: got %d, want 7", seq)
	}
	if srcID != 42 {
		t.Errorf("sourceID: got %d, want 42", srcID)
	}

	// After header: template flowset ID = 0
	tsID := binary.BigEndian.Uint16(data[20:22])
	if tsID != 0 {
		t.Errorf("template flowset ID: got %d, want 0", tsID)
	}

	// Template ID should be 256
	// Flowset length is at offset 22-24, then template ID at 24-26
	tmplID := binary.BigEndian.Uint16(data[24:26])
	if tmplID != 256 {
		t.Errorf("template ID: got %d, want 256", tmplID)
	}

	// Field count = 14 fields in our default template (including TCP_FLAGS)
	fieldCount := binary.BigEndian.Uint16(data[26:28])
	if fieldCount != 14 {
		t.Errorf("field count: got %d, want 14", fieldCount)
	}
}

func TestV9DataRecordByteValues(t *testing.T) {
	rec := V9DataRecord{
		SrcAddr:   [4]byte{10, 0, 0, 1},
		DstAddr:   [4]byte{10, 0, 0, 2},
		SrcPort:   80,
		DstPort:   12345,
		Protocol:  17, // UDP
		SrcTOS:    0,
		SrcMask:   24,
		DstMask:   16,
		Octets:    1000,
		Packets:   10,
		FirstSeen: 100,
		LastSeen:  200,
		AppID:     99,
	}
	data := rec.Encode()

	// Verify exact field values at known offsets.
	// IN_BYTES at offset 0
	if got := binary.BigEndian.Uint32(data[0:4]); got != 1000 {
		t.Errorf("IN_BYTES: got %d, want 1000", got)
	}
	// IN_PKTS at offset 4
	if got := binary.BigEndian.Uint32(data[4:8]); got != 10 {
		t.Errorf("IN_PKTS: got %d, want 10", got)
	}
	// PROTOCOL at offset 8
	if data[8] != 17 {
		t.Errorf("PROTOCOL: got %d, want 17", data[8])
	}
	// SRC_TOS at offset 9
	if data[9] != 0 {
		t.Errorf("SRC_TOS: got %d, want 0", data[9])
	}
	// TCP_FLAGS at offset 10
	if data[10] != 0 {
		t.Errorf("TCP_FLAGS: got %d, want 0", data[10])
	}
	// L4_SRC_PORT at offset 11
	if got := binary.BigEndian.Uint16(data[11:13]); got != 80 {
		t.Errorf("L4_SRC_PORT: got %d, want 80", got)
	}
	// IPV4_SRC_ADDR at offset 13
	if data[13] != 10 || data[14] != 0 || data[15] != 0 || data[16] != 1 {
		t.Errorf("IPV4_SRC_ADDR: got %v, want [10 0 0 1]", data[13:17])
	}
	// SRC_MASK at offset 17
	if data[17] != 24 {
		t.Errorf("SRC_MASK: got %d, want 24", data[17])
	}
	// L4_DST_PORT at offset 18
	if got := binary.BigEndian.Uint16(data[18:20]); got != 12345 {
		t.Errorf("L4_DST_PORT: got %d, want 12345", got)
	}
	// IPV4_DST_ADDR at offset 20
	if data[20] != 10 || data[21] != 0 || data[22] != 0 || data[23] != 2 {
		t.Errorf("IPV4_DST_ADDR: got %v, want [10 0 0 2]", data[20:24])
	}
	// DST_MASK at offset 24
	if data[24] != 16 {
		t.Errorf("DST_MASK: got %d, want 16", data[24])
	}
	// LAST_SWITCHED at offset 25
	if got := binary.BigEndian.Uint32(data[25:29]); got != 200 {
		t.Errorf("LAST_SWITCHED: got %d, want 200", got)
	}
	// FIRST_SWITCHED at offset 29
	if got := binary.BigEndian.Uint32(data[29:33]); got != 100 {
		t.Errorf("FIRST_SWITCHED: got %d, want 100", got)
	}
	// APPLICATION_ID at offset 33
	if got := binary.BigEndian.Uint32(data[33:37]); got != 99 {
		t.Errorf("APPLICATION_ID: got %d, want 99", got)
	}

	// Total record size should be 37 bytes (was 36, +1 for TCP_FLAGS).
	if len(data) != 37 {
		t.Errorf("record size: got %d, want 37", len(data))
	}
}

func TestV9PacketMultipleRecords(t *testing.T) {
	pkt := NewV9Packet(100, 2000)
	pkt.SetSequence(42)

	for i := 0; i < 5; i++ {
		rec := V9DataRecord{
			SrcAddr:  [4]byte{192, 168, 1, byte(i)},
			DstAddr:  [4]byte{10, 0, 0, 1},
			SrcPort:  uint16(10000 + i),
			DstPort:  443,
			Protocol: 6,
			Octets:   uint32(1000 * (i + 1)),
			Packets:  uint32(10 * (i + 1)),
		}
		pkt.AddDataRecord(rec)
	}

	data := pkt.Bytes()

	// Check header.
	version := binary.BigEndian.Uint16(data[0:2])
	count := binary.BigEndian.Uint16(data[2:4])
	seq := binary.BigEndian.Uint32(data[12:16])
	srcID := binary.BigEndian.Uint32(data[16:20])

	if version != 9 {
		t.Errorf("version: got %d, want 9", version)
	}
	// count = 1 template + 5 data records = 6
	if count != 6 {
		t.Errorf("count: got %d, want 6", count)
	}
	if seq != 42 {
		t.Errorf("sequence: got %d, want 42", seq)
	}
	if srcID != 100 {
		t.Errorf("sourceID: got %d, want 100", srcID)
	}

	// Verify data flowset ID references our template (256).
	// After header (20) + template flowset, we should find data flowset.
	tsLen := binary.BigEndian.Uint16(data[22:24])
	dataFSOffset := 20 + int(tsLen)
	dataFSID := binary.BigEndian.Uint16(data[dataFSOffset : dataFSOffset+2])
	if dataFSID != 256 {
		t.Errorf("data flowset ID: got %d, want 256", dataFSID)
	}
}
