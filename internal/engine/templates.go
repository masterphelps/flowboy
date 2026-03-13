package engine

import (
	"bytes"
	"encoding/binary"
	"time"
)

// NetFlow v9 constants per RFC 3954.
const (
	V9Version        = 9
	TemplateFlowSetID = 0
	FirstTemplateID   = 256
)

// NetFlow v9 field type IDs per RFC 3954 / IANA assignments.
const (
	FieldINBytes       = 1
	FieldINPkts        = 2
	FieldProtocol      = 4
	FieldSrcTOS        = 5
	FieldL4SrcPort     = 7
	FieldIPv4SrcAddr   = 8
	FieldSrcMask       = 9
	FieldL4DstPort     = 11
	FieldIPv4DstAddr   = 12
	FieldDstMask       = 13
	FieldLastSwitched  = 21
	FieldFirstSwitched = 22
	FieldApplicationID = 95
)

// templateField defines a single field in a v9 template.
type templateField struct {
	Type   uint16
	Length uint16
}

// defaultTemplateFields is the ordered list of fields our template advertises.
var defaultTemplateFields = []templateField{
	{FieldINBytes, 4},
	{FieldINPkts, 4},
	{FieldProtocol, 1},
	{FieldSrcTOS, 1},
	{FieldL4SrcPort, 2},
	{FieldIPv4SrcAddr, 4},
	{FieldSrcMask, 1},
	{FieldL4DstPort, 2},
	{FieldIPv4DstAddr, 4},
	{FieldDstMask, 1},
	{FieldLastSwitched, 4},
	{FieldFirstSwitched, 4},
	{FieldApplicationID, 4},
}

// dataRecordSize is the total byte size of a single data record matching
// the default template (sum of all field lengths).
var dataRecordSize int

func init() {
	for _, f := range defaultTemplateFields {
		dataRecordSize += int(f.Length)
	}
}

// ---------- V9 Header ----------

// v9Header is the 20-byte NetFlow v9 packet header.
type v9Header struct {
	Version   uint16
	Count     uint16
	SysUptime uint32
	UnixSecs  uint32
	Sequence  uint32
	SourceID  uint32
}

func (h *v9Header) encode(buf *bytes.Buffer) {
	binary.Write(buf, binary.BigEndian, h)
}

// ---------- Template FlowSet ----------

// V9Template represents a NetFlow v9 template FlowSet wrapped in a full
// v9 packet (header + template flowset).  Collectors must receive the
// template before they can decode data flowsets.
type V9Template struct {
	SourceID  uint32
	SysUptime uint32
	Sequence  uint32
}

// NewV9Template returns a template ready to encode.
func NewV9Template() *V9Template {
	return &V9Template{}
}

// Encode serializes a complete v9 packet containing just the template
// flowset.  This is the packet you send to a collector so it learns
// the field layout.
func (t *V9Template) Encode() []byte {
	var buf bytes.Buffer

	// Build the template flowset payload first so we know its length.
	var tsBuf bytes.Buffer

	// Template ID
	binary.Write(&tsBuf, binary.BigEndian, uint16(FirstTemplateID))
	// Field count
	binary.Write(&tsBuf, binary.BigEndian, uint16(len(defaultTemplateFields)))
	// Field definitions
	for _, f := range defaultTemplateFields {
		binary.Write(&tsBuf, binary.BigEndian, f.Type)
		binary.Write(&tsBuf, binary.BigEndian, f.Length)
	}

	tsPayload := tsBuf.Bytes()

	// FlowSet header: ID (0 = template) + length (header 4 bytes + payload).
	// Length must be padded to a 4-byte boundary per RFC 3954 section 5.3.
	flowsetLen := 4 + len(tsPayload)
	padding := (4 - (flowsetLen % 4)) % 4
	flowsetLen += padding

	// Write v9 header.  Count = 1 (one template record inside).
	hdr := v9Header{
		Version:   V9Version,
		Count:     1,
		SysUptime: t.SysUptime,
		UnixSecs:  uint32(time.Now().Unix()),
		Sequence:  t.Sequence,
		SourceID:  t.SourceID,
	}
	hdr.encode(&buf)

	// Write template flowset header.
	binary.Write(&buf, binary.BigEndian, uint16(TemplateFlowSetID))
	binary.Write(&buf, binary.BigEndian, uint16(flowsetLen))

	// Write template payload.
	buf.Write(tsPayload)

	// Write padding bytes.
	for i := 0; i < padding; i++ {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// ---------- V9 Data Record ----------

// V9DataRecord holds the fields for a single NetFlow v9 data record
// matching our default template.
type V9DataRecord struct {
	SrcAddr   [4]byte
	DstAddr   [4]byte
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	SrcTOS    uint8
	SrcMask   uint8
	DstMask   uint8
	Octets    uint32
	Packets   uint32
	FirstSeen uint32
	LastSeen  uint32
	AppID     uint32
}

// Encode serializes the data record in the exact field order defined by
// the default template.  All multi-byte values are big-endian.
func (r *V9DataRecord) Encode() []byte {
	buf := make([]byte, dataRecordSize)
	offset := 0

	// IN_BYTES (4)
	binary.BigEndian.PutUint32(buf[offset:], r.Octets)
	offset += 4

	// IN_PKTS (4)
	binary.BigEndian.PutUint32(buf[offset:], r.Packets)
	offset += 4

	// PROTOCOL (1)
	buf[offset] = r.Protocol
	offset++

	// SRC_TOS (1)
	buf[offset] = r.SrcTOS
	offset++

	// L4_SRC_PORT (2)
	binary.BigEndian.PutUint16(buf[offset:], r.SrcPort)
	offset += 2

	// IPV4_SRC_ADDR (4)
	copy(buf[offset:], r.SrcAddr[:])
	offset += 4

	// SRC_MASK (1)
	buf[offset] = r.SrcMask
	offset++

	// L4_DST_PORT (2)
	binary.BigEndian.PutUint16(buf[offset:], r.DstPort)
	offset += 2

	// IPV4_DST_ADDR (4)
	copy(buf[offset:], r.DstAddr[:])
	offset += 4

	// DST_MASK (1)
	buf[offset] = r.DstMask
	offset++

	// LAST_SWITCHED (4)
	binary.BigEndian.PutUint32(buf[offset:], r.LastSeen)
	offset += 4

	// FIRST_SWITCHED (4)
	binary.BigEndian.PutUint32(buf[offset:], r.FirstSeen)
	offset += 4

	// APPLICATION_ID (4)
	binary.BigEndian.PutUint32(buf[offset:], r.AppID)

	return buf
}

// ---------- V9 Packet ----------

// V9Packet assembles a complete NetFlow v9 export packet containing a
// template flowset followed by a data flowset.
type V9Packet struct {
	sourceID  uint32
	sysUptime uint32
	sequence  uint32
	records   []V9DataRecord
}

// NewV9Packet creates a new v9 packet builder.
func NewV9Packet(sourceID uint32, sysUptime uint32) *V9Packet {
	return &V9Packet{
		sourceID:  sourceID,
		sysUptime: sysUptime,
	}
}

// SetSequence sets the packet sequence number.
func (p *V9Packet) SetSequence(seq uint32) {
	p.sequence = seq
}

// AddDataRecord appends a flow data record to the packet.
func (p *V9Packet) AddDataRecord(rec V9DataRecord) {
	p.records = append(p.records, rec)
}

// Bytes serializes the full v9 packet: header + template flowset + data
// flowset.  Including the template in every packet ensures collectors
// can always decode the data even if they missed an earlier template.
func (p *V9Packet) Bytes() []byte {
	var buf bytes.Buffer

	// --- Build template flowset ---
	var tsBuf bytes.Buffer
	binary.Write(&tsBuf, binary.BigEndian, uint16(FirstTemplateID))
	binary.Write(&tsBuf, binary.BigEndian, uint16(len(defaultTemplateFields)))
	for _, f := range defaultTemplateFields {
		binary.Write(&tsBuf, binary.BigEndian, f.Type)
		binary.Write(&tsBuf, binary.BigEndian, f.Length)
	}
	tsPayload := tsBuf.Bytes()
	tsFlowsetLen := 4 + len(tsPayload)
	tsPadding := (4 - (tsFlowsetLen % 4)) % 4
	tsFlowsetLen += tsPadding

	// --- Build data flowset ---
	var dsBuf bytes.Buffer
	for i := range p.records {
		dsBuf.Write(p.records[i].Encode())
	}
	dsPayload := dsBuf.Bytes()
	dsFlowsetLen := 4 + len(dsPayload)
	dsPadding := (4 - (dsFlowsetLen % 4)) % 4
	dsFlowsetLen += dsPadding

	// Count = number of template records (1) + number of data records.
	// Per RFC 3954, Count is "the total number of records in the Export
	// Packet, which is the sum of Options Template, Template, and Data
	// records".
	count := uint16(1 + len(p.records))

	// --- Write header ---
	hdr := v9Header{
		Version:   V9Version,
		Count:     count,
		SysUptime: p.sysUptime,
		UnixSecs:  uint32(time.Now().Unix()),
		Sequence:  p.sequence,
		SourceID:  p.sourceID,
	}
	hdr.encode(&buf)

	// --- Write template flowset ---
	binary.Write(&buf, binary.BigEndian, uint16(TemplateFlowSetID))
	binary.Write(&buf, binary.BigEndian, uint16(tsFlowsetLen))
	buf.Write(tsPayload)
	for i := 0; i < tsPadding; i++ {
		buf.WriteByte(0)
	}

	// --- Write data flowset ---
	if len(p.records) > 0 {
		binary.Write(&buf, binary.BigEndian, uint16(FirstTemplateID))
		binary.Write(&buf, binary.BigEndian, uint16(dsFlowsetLen))
		buf.Write(dsPayload)
		for i := 0; i < dsPadding; i++ {
			buf.WriteByte(0)
		}
	}

	return buf.Bytes()
}
