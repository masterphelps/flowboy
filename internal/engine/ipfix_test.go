package engine

import (
	"encoding/binary"
	"testing"
)

func TestIPFIXHeaderVersion(t *testing.T) {
	pkt := NewIPFIXPacket(1)
	data := pkt.Bytes()
	if len(data) < 16 {
		t.Fatalf("packet too short for IPFIX header: %d bytes", len(data))
	}
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 10 {
		t.Errorf("expected IPFIX version 10, got %d", version)
	}
}

func TestIPFIXTemplateEncoding(t *testing.T) {
	tmpl := NewIPFIXTemplate()
	data := tmpl.Encode()
	if len(data) == 0 {
		t.Fatal("encoded IPFIX template is empty")
	}

	// IPFIX header version = 10
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 10 {
		t.Errorf("version: got %d, want 10", version)
	}

	// After the 16-byte IPFIX header, the template set begins.
	// Template Set ID must be 2 per RFC 7011.
	setID := binary.BigEndian.Uint16(data[16:18])
	if setID != 2 {
		t.Errorf("template set ID: got %d, want 2", setID)
	}

	// Set length at offset 18-20.
	setLen := binary.BigEndian.Uint16(data[18:20])
	if setLen == 0 {
		t.Error("template set length is zero")
	}

	// Template ID should be 256 (at offset 20-22).
	tmplID := binary.BigEndian.Uint16(data[20:22])
	if tmplID != 256 {
		t.Errorf("template ID: got %d, want 256", tmplID)
	}

	// Field count = 14 (at offset 22-24, includes TCP_FLAGS).
	fieldCount := binary.BigEndian.Uint16(data[22:24])
	if fieldCount != 14 {
		t.Errorf("field count: got %d, want 14", fieldCount)
	}
}

func TestIPFIXDataRecordEncoding(t *testing.T) {
	rec := IPFIXDataRecord{
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

	// Total record size should be 37 bytes (was 36, +1 for TCP_FLAGS).
	if len(data) != 37 {
		t.Fatalf("record size: got %d, want 37", len(data))
	}

	// Verify exact field values at known offsets.
	// octetDeltaCount at offset 0
	if got := binary.BigEndian.Uint32(data[0:4]); got != 1000 {
		t.Errorf("octetDeltaCount: got %d, want 1000", got)
	}
	// packetDeltaCount at offset 4
	if got := binary.BigEndian.Uint32(data[4:8]); got != 10 {
		t.Errorf("packetDeltaCount: got %d, want 10", got)
	}
	// protocolIdentifier at offset 8
	if data[8] != 17 {
		t.Errorf("protocolIdentifier: got %d, want 17", data[8])
	}
	// ipClassOfService at offset 9
	if data[9] != 0 {
		t.Errorf("ipClassOfService: got %d, want 0", data[9])
	}
	// tcpControlBits at offset 10
	if data[10] != 0 {
		t.Errorf("tcpControlBits: got %d, want 0", data[10])
	}
	// sourceTransportPort at offset 11
	if got := binary.BigEndian.Uint16(data[11:13]); got != 80 {
		t.Errorf("sourceTransportPort: got %d, want 80", got)
	}
	// sourceIPv4Address at offset 13
	if data[13] != 10 || data[14] != 0 || data[15] != 0 || data[16] != 1 {
		t.Errorf("sourceIPv4Address: got %v, want [10 0 0 1]", data[13:17])
	}
	// sourceIPv4PrefixLength at offset 17
	if data[17] != 24 {
		t.Errorf("sourceIPv4PrefixLength: got %d, want 24", data[17])
	}
	// destinationTransportPort at offset 18
	if got := binary.BigEndian.Uint16(data[18:20]); got != 12345 {
		t.Errorf("destinationTransportPort: got %d, want 12345", got)
	}
	// destinationIPv4Address at offset 20
	if data[20] != 10 || data[21] != 0 || data[22] != 0 || data[23] != 2 {
		t.Errorf("destinationIPv4Address: got %v, want [10 0 0 2]", data[20:24])
	}
	// destinationIPv4PrefixLength at offset 24
	if data[24] != 16 {
		t.Errorf("destinationIPv4PrefixLength: got %d, want 16", data[24])
	}
	// flowEndSysUpTime at offset 25
	if got := binary.BigEndian.Uint32(data[25:29]); got != 200 {
		t.Errorf("flowEndSysUpTime: got %d, want 200", got)
	}
	// flowStartSysUpTime at offset 29
	if got := binary.BigEndian.Uint32(data[29:33]); got != 100 {
		t.Errorf("flowStartSysUpTime: got %d, want 100", got)
	}
	// applicationId at offset 33
	if got := binary.BigEndian.Uint32(data[33:37]); got != 99 {
		t.Errorf("applicationId: got %d, want 99", got)
	}
}

func TestIPFIXAppIDIncluded(t *testing.T) {
	rec := IPFIXDataRecord{
		SrcAddr:  [4]byte{172, 16, 0, 1},
		DstAddr:  [4]byte{172, 16, 0, 2},
		SrcPort:  443,
		DstPort:  54321,
		Protocol: 6,
		Octets:   5000,
		Packets:  50,
		AppID:    42,
	}
	data := rec.Encode()

	// applicationId (IANA IE 95) is at offset 33 (shifted +1 by TCP_FLAGS).
	appID := binary.BigEndian.Uint32(data[33:37])
	if appID != 42 {
		t.Errorf("applicationId: got %d, want 42", appID)
	}
}

func TestIPFIXPacketAssembly(t *testing.T) {
	pkt := NewIPFIXPacket(100)
	pkt.SetSequence(7)

	for i := 0; i < 3; i++ {
		rec := IPFIXDataRecord{
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

	// Minimum size: 16 (header) + template set + data set
	if len(data) < 16 {
		t.Fatalf("packet too short: %d bytes", len(data))
	}

	// Verify it's a valid IPFIX message.
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 10 {
		t.Errorf("version: got %d, want 10", version)
	}

	// Length field should match actual packet length.
	length := binary.BigEndian.Uint16(data[2:4])
	if int(length) != len(data) {
		t.Errorf("length field: got %d, actual %d", length, len(data))
	}

	// Observation Domain ID
	obsDomainID := binary.BigEndian.Uint32(data[12:16])
	if obsDomainID != 100 {
		t.Errorf("observation domain ID: got %d, want 100", obsDomainID)
	}

	// After header (16 bytes), template set starts with Set ID = 2.
	setID := binary.BigEndian.Uint16(data[16:18])
	if setID != 2 {
		t.Errorf("template set ID: got %d, want 2", setID)
	}

	// Find data set: skip past template set.
	tsLen := binary.BigEndian.Uint16(data[18:20])
	dataSetOffset := 16 + int(tsLen)
	if dataSetOffset >= len(data) {
		t.Fatalf("data set offset %d out of bounds (packet len %d)", dataSetOffset, len(data))
	}

	// Data set ID should be 256 (our template ID).
	dataSetID := binary.BigEndian.Uint16(data[dataSetOffset : dataSetOffset+2])
	if dataSetID != 256 {
		t.Errorf("data set ID: got %d, want 256", dataSetID)
	}
}

func TestIPFIXHeaderByteLevel(t *testing.T) {
	pkt := NewIPFIXPacket(42)
	pkt.SetSequence(123)
	pkt.SetExportTime(1700000000)

	rec := IPFIXDataRecord{
		SrcAddr:  [4]byte{10, 0, 0, 1},
		DstAddr:  [4]byte{10, 0, 0, 2},
		SrcPort:  80,
		DstPort:  443,
		Protocol: 6,
		Octets:   500,
		Packets:  5,
	}
	pkt.AddDataRecord(rec)

	data := pkt.Bytes()

	// Version (offset 0-1) = 10
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 10 {
		t.Errorf("version: got %d, want 10", version)
	}

	// Length (offset 2-3) = total message length
	length := binary.BigEndian.Uint16(data[2:4])
	if int(length) != len(data) {
		t.Errorf("length: got %d, want %d", length, len(data))
	}

	// Export Time (offset 4-7) = 1700000000
	exportTime := binary.BigEndian.Uint32(data[4:8])
	if exportTime != 1700000000 {
		t.Errorf("export time: got %d, want 1700000000", exportTime)
	}

	// Sequence Number (offset 8-11) = 123
	seq := binary.BigEndian.Uint32(data[8:12])
	if seq != 123 {
		t.Errorf("sequence number: got %d, want 123", seq)
	}

	// Observation Domain ID (offset 12-15) = 42
	obsDomainID := binary.BigEndian.Uint32(data[12:16])
	if obsDomainID != 42 {
		t.Errorf("observation domain ID: got %d, want 42", obsDomainID)
	}
}
