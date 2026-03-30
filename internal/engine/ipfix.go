package engine

import (
	"bytes"
	"encoding/binary"
	"time"
)

// IPFIX constants per RFC 7011.
const (
	IPFIXVersion      = 10
	IPFIXTemplateSetID = 2
	IPFIXTemplateID    = 256
)

// ipfixInformationElement defines a single Information Element in an IPFIX template.
type ipfixInformationElement struct {
	ID     uint16
	Length uint16
}

// ipfixDefaultElements is the ordered list of Information Elements our
// template advertises.  These mirror the v9 default template fields so
// the engine can produce either format from the same flow data.
var ipfixDefaultElements = []ipfixInformationElement{
	{FieldINBytes, 4},       // octetDeltaCount (1)
	{FieldINPkts, 4},        // packetDeltaCount (2)
	{FieldProtocol, 1},      // protocolIdentifier (4)
	{FieldSrcTOS, 1},        // ipClassOfService (5)
	{FieldTCPFlags, 1},      // tcpControlBits (6)
	{FieldL4SrcPort, 2},     // sourceTransportPort (7)
	{FieldIPv4SrcAddr, 4},   // sourceIPv4Address (8)
	{FieldSrcMask, 1},       // sourceIPv4PrefixLength (9)
	{FieldL4DstPort, 2},     // destinationTransportPort (11)
	{FieldIPv4DstAddr, 4},   // destinationIPv4Address (12)
	{FieldDstMask, 1},       // destinationIPv4PrefixLength (13)
	{FieldLastSwitched, 4},  // flowEndSysUpTime (21)
	{FieldFirstSwitched, 4}, // flowStartSysUpTime (22)
	{FieldApplicationID, 4}, // applicationId (95)
}

// ipfixDataRecordSize is the total byte size of a single IPFIX data record
// matching the default template.
var ipfixDataRecordSize int

func init() {
	for _, e := range ipfixDefaultElements {
		ipfixDataRecordSize += int(e.Length)
	}
}

// ---------- IPFIX Message Header (16 bytes) ----------

// ipfixHeader is the 16-byte IPFIX message header per RFC 7011 section 3.1.
type ipfixHeader struct {
	Version             uint16
	Length              uint16
	ExportTime          uint32
	SequenceNumber      uint32
	ObservationDomainID uint32
}

func (h *ipfixHeader) encode(buf *bytes.Buffer) {
	binary.Write(buf, binary.BigEndian, h)
}

// ---------- IPFIX Template ----------

// IPFIXTemplate represents an IPFIX template set wrapped in a full
// IPFIX message (header + template set).  Collectors must receive the
// template before they can decode data sets.
type IPFIXTemplate struct {
	ObservationDomainID uint32
	SequenceNumber      uint32
	ExportTime          uint32
}

// NewIPFIXTemplate returns an IPFIX template ready to encode.
func NewIPFIXTemplate() *IPFIXTemplate {
	return &IPFIXTemplate{}
}

// Encode serializes a complete IPFIX message containing just the template
// set.  This is the message you send to a collector so it learns the
// Information Element layout.
func (t *IPFIXTemplate) Encode() []byte {
	var buf bytes.Buffer

	// Build the template set payload first so we know its length.
	var tsBuf bytes.Buffer

	// Template ID
	binary.Write(&tsBuf, binary.BigEndian, uint16(IPFIXTemplateID))
	// Field count
	binary.Write(&tsBuf, binary.BigEndian, uint16(len(ipfixDefaultElements)))
	// Information Element specifiers
	for _, e := range ipfixDefaultElements {
		binary.Write(&tsBuf, binary.BigEndian, e.ID)
		binary.Write(&tsBuf, binary.BigEndian, e.Length)
	}

	tsPayload := tsBuf.Bytes()

	// Set header: Set ID (2 = template) + Length (header 4 bytes + payload).
	// Length must be padded to a 4-byte boundary per RFC 7011 section 3.3.1.
	setLen := 4 + len(tsPayload)
	padding := (4 - (setLen % 4)) % 4
	setLen += padding

	// Total message length: 16-byte header + template set.
	totalLen := 16 + setLen

	exportTime := t.ExportTime
	if exportTime == 0 {
		exportTime = uint32(time.Now().Unix())
	}

	// Write IPFIX message header.
	hdr := ipfixHeader{
		Version:             IPFIXVersion,
		Length:              uint16(totalLen),
		ExportTime:          exportTime,
		SequenceNumber:      t.SequenceNumber,
		ObservationDomainID: t.ObservationDomainID,
	}
	hdr.encode(&buf)

	// Write template set header.
	binary.Write(&buf, binary.BigEndian, uint16(IPFIXTemplateSetID))
	binary.Write(&buf, binary.BigEndian, uint16(setLen))

	// Write template payload.
	buf.Write(tsPayload)

	// Write padding bytes.
	for i := 0; i < padding; i++ {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// ---------- IPFIX Data Record ----------

// IPFIXDataRecord holds the fields for a single IPFIX data record
// matching our default template.  The field names mirror V9DataRecord
// so the engine can populate either from the same flow data.
type IPFIXDataRecord struct {
	SrcAddr   [4]byte
	DstAddr   [4]byte
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	SrcTOS    uint8
	TCPFlags  uint8
	SrcMask   uint8
	DstMask   uint8
	Octets    uint32
	Packets   uint32
	FirstSeen uint32
	LastSeen  uint32
	AppID     uint32
}

// Encode serializes the data record in the exact Information Element order
// defined by the default template.  All multi-byte values are big-endian.
func (r *IPFIXDataRecord) Encode() []byte {
	buf := make([]byte, ipfixDataRecordSize)
	offset := 0

	// octetDeltaCount (4)
	binary.BigEndian.PutUint32(buf[offset:], r.Octets)
	offset += 4

	// packetDeltaCount (4)
	binary.BigEndian.PutUint32(buf[offset:], r.Packets)
	offset += 4

	// protocolIdentifier (1)
	buf[offset] = r.Protocol
	offset++

	// ipClassOfService (1)
	buf[offset] = r.SrcTOS
	offset++

	// tcpControlBits (1)
	buf[offset] = r.TCPFlags
	offset++

	// sourceTransportPort (2)
	binary.BigEndian.PutUint16(buf[offset:], r.SrcPort)
	offset += 2

	// sourceIPv4Address (4)
	copy(buf[offset:], r.SrcAddr[:])
	offset += 4

	// sourceIPv4PrefixLength (1)
	buf[offset] = r.SrcMask
	offset++

	// destinationTransportPort (2)
	binary.BigEndian.PutUint16(buf[offset:], r.DstPort)
	offset += 2

	// destinationIPv4Address (4)
	copy(buf[offset:], r.DstAddr[:])
	offset += 4

	// destinationIPv4PrefixLength (1)
	buf[offset] = r.DstMask
	offset++

	// flowEndSysUpTime (4)
	binary.BigEndian.PutUint32(buf[offset:], r.LastSeen)
	offset += 4

	// flowStartSysUpTime (4)
	binary.BigEndian.PutUint32(buf[offset:], r.FirstSeen)
	offset += 4

	// applicationId (4)
	binary.BigEndian.PutUint32(buf[offset:], r.AppID)

	return buf
}

// ---------- IPFIX Packet ----------

// IPFIXPacket assembles a complete IPFIX message containing a template
// set followed by a data set.
type IPFIXPacket struct {
	observationDomainID uint32
	sequenceNumber      uint32
	exportTime          uint32
	records             []IPFIXDataRecord
}

// NewIPFIXPacket creates a new IPFIX message builder for the given
// Observation Domain ID.
func NewIPFIXPacket(observationDomainID uint32) *IPFIXPacket {
	return &IPFIXPacket{
		observationDomainID: observationDomainID,
	}
}

// SetSequence sets the cumulative data record sequence number.
func (p *IPFIXPacket) SetSequence(seq uint32) {
	p.sequenceNumber = seq
}

// SetExportTime sets the export timestamp (seconds since epoch).
// If not called, the current time is used.
func (p *IPFIXPacket) SetExportTime(t uint32) {
	p.exportTime = t
}

// AddDataRecord appends a flow data record to the message.
func (p *IPFIXPacket) AddDataRecord(rec IPFIXDataRecord) {
	p.records = append(p.records, rec)
}

// Bytes serializes the full IPFIX message: header + template set + data set.
// Including the template in every message ensures collectors can always
// decode the data even if they missed an earlier template.
func (p *IPFIXPacket) Bytes() []byte {
	var buf bytes.Buffer

	// --- Build template set ---
	var tsBuf bytes.Buffer
	binary.Write(&tsBuf, binary.BigEndian, uint16(IPFIXTemplateID))
	binary.Write(&tsBuf, binary.BigEndian, uint16(len(ipfixDefaultElements)))
	for _, e := range ipfixDefaultElements {
		binary.Write(&tsBuf, binary.BigEndian, e.ID)
		binary.Write(&tsBuf, binary.BigEndian, e.Length)
	}
	tsPayload := tsBuf.Bytes()
	tsSetLen := 4 + len(tsPayload)
	tsPadding := (4 - (tsSetLen % 4)) % 4
	tsSetLen += tsPadding

	// --- Build data set ---
	var dsBuf bytes.Buffer
	for i := range p.records {
		dsBuf.Write(p.records[i].Encode())
	}
	dsPayload := dsBuf.Bytes()
	dsSetLen := 4 + len(dsPayload)
	dsPadding := (4 - (dsSetLen % 4)) % 4
	dsSetLen += dsPadding

	// Total message length: 16-byte header + template set + data set (if any).
	totalLen := 16 + tsSetLen
	if len(p.records) > 0 {
		totalLen += dsSetLen
	}

	exportTime := p.exportTime
	if exportTime == 0 {
		exportTime = uint32(time.Now().Unix())
	}

	// --- Write IPFIX message header ---
	hdr := ipfixHeader{
		Version:             IPFIXVersion,
		Length:              uint16(totalLen),
		ExportTime:          exportTime,
		SequenceNumber:      p.sequenceNumber,
		ObservationDomainID: p.observationDomainID,
	}
	hdr.encode(&buf)

	// --- Write template set ---
	binary.Write(&buf, binary.BigEndian, uint16(IPFIXTemplateSetID))
	binary.Write(&buf, binary.BigEndian, uint16(tsSetLen))
	buf.Write(tsPayload)
	for i := 0; i < tsPadding; i++ {
		buf.WriteByte(0)
	}

	// --- Write data set ---
	if len(p.records) > 0 {
		binary.Write(&buf, binary.BigEndian, uint16(IPFIXTemplateID))
		binary.Write(&buf, binary.BigEndian, uint16(dsSetLen))
		buf.Write(dsPayload)
		for i := 0; i < dsPadding; i++ {
			buf.WriteByte(0)
		}
	}

	return buf.Bytes()
}
