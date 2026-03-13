package engine

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/masterphelps/flowboy/internal/config"
)

// Exporter reads encoded data records from a channel, wraps them in proper
// v9 or IPFIX packets with headers, and sends UDP datagrams to multiple
// collectors simultaneously.
type Exporter struct {
	collectors       []CollectorConn
	records          <-chan []byte
	stats            map[string]*ExporterStats
	templateInterval int // send template every N data exports
	sourceID         uint32
	sequence         uint32
	stopCh           chan struct{}
	mu               sync.RWMutex
}

// CollectorConn pairs a collector configuration with its UDP connection.
type CollectorConn struct {
	collector config.Collector
	conn      *net.UDPConn
}

// ExporterStats tracks per-collector export statistics.
type ExporterStats struct {
	PacketsSent uint64
	BytesSent   uint64
	Errors      uint64
}

// NewExporter dials UDP to each collector and returns an Exporter ready to start.
func NewExporter(records <-chan []byte, collectors []config.Collector) (*Exporter, error) {
	exp := &Exporter{
		records:          records,
		stats:            make(map[string]*ExporterStats),
		templateInterval: 10,
		sourceID:         1,
		stopCh:           make(chan struct{}),
	}

	for _, c := range collectors {
		raddr, err := net.ResolveUDPAddr("udp", c.Address)
		if err != nil {
			// Close any already-opened connections.
			for _, cc := range exp.collectors {
				cc.conn.Close()
			}
			return nil, fmt.Errorf("resolve collector %q address %q: %w", c.Name, c.Address, err)
		}
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			for _, cc := range exp.collectors {
				cc.conn.Close()
			}
			return nil, fmt.Errorf("dial collector %q at %q: %w", c.Name, c.Address, err)
		}
		exp.collectors = append(exp.collectors, CollectorConn{
			collector: c,
			conn:      conn,
		})
		exp.stats[c.Name] = &ExporterStats{}
	}

	return exp, nil
}

// Start launches the exporter goroutine that reads records and fans out
// UDP packets to all collectors.
func (e *Exporter) Start() {
	go e.run()
}

// Stop signals the exporter goroutine to stop and closes all UDP connections.
func (e *Exporter) Stop() {
	close(e.stopCh)
	for _, cc := range e.collectors {
		cc.conn.Close()
	}
}

// GetStats returns a snapshot of per-collector export statistics.
func (e *Exporter) GetStats() map[string]*ExporterStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]*ExporterStats)
	for name, s := range e.stats {
		result[name] = &ExporterStats{
			PacketsSent: atomic.LoadUint64(&s.PacketsSent),
			BytesSent:   atomic.LoadUint64(&s.BytesSent),
			Errors:      atomic.LoadUint64(&s.Errors),
		}
	}
	return result
}

// run is the main exporter loop.
func (e *Exporter) run() {
	var exportCount uint32

	for {
		select {
		case <-e.stopCh:
			return
		case recordData, ok := <-e.records:
			if !ok {
				return
			}

			e.sequence++
			exportCount++

			// Determine if we need to send a template this round.
			sendTemplate := exportCount == 1 || (exportCount%uint32(e.templateInterval)) == 1

			// Send to each collector.
			for i := range e.collectors {
				cc := &e.collectors[i]
				st := e.stats[cc.collector.Name]

				if sendTemplate {
					tmplData := e.buildTemplatePacket(cc.collector.Version)
					e.sendPacket(cc, st, tmplData)
				}

				pktData := e.buildDataPacket(cc.collector.Version, recordData)
				e.sendPacket(cc, st, pktData)
			}
		}
	}
}

// buildTemplatePacket builds a template-only packet for the given version.
func (e *Exporter) buildTemplatePacket(version string) []byte {
	switch version {
	case "ipfix":
		tmpl := NewIPFIXTemplate()
		tmpl.ObservationDomainID = e.sourceID
		tmpl.SequenceNumber = e.sequence
		return tmpl.Encode()
	default: // "v9"
		tmpl := NewV9Template()
		tmpl.SourceID = e.sourceID
		tmpl.Sequence = e.sequence
		return tmpl.Encode()
	}
}

// buildDataPacket wraps raw record bytes in a proper v9 or IPFIX data packet.
// The raw record bytes are the output of V9DataRecord.Encode(), which has the
// same byte layout as IPFIXDataRecord.Encode().
func (e *Exporter) buildDataPacket(version string, recordData []byte) []byte {
	switch version {
	case "ipfix":
		rec := decodeRawRecord(recordData)
		pkt := NewIPFIXPacket(e.sourceID)
		pkt.SetSequence(e.sequence)
		pkt.AddDataRecord(IPFIXDataRecord(rec))
		return pkt.Bytes()
	default: // "v9"
		rec := decodeRawRecord(recordData)
		pkt := NewV9Packet(e.sourceID, 0)
		pkt.SetSequence(e.sequence)
		pkt.AddDataRecord(rec)
		return pkt.Bytes()
	}
}

// decodeRawRecord reverses V9DataRecord.Encode() to recover the struct fields.
// The byte layout is identical for both V9DataRecord and IPFIXDataRecord.
func decodeRawRecord(data []byte) V9DataRecord {
	if len(data) < dataRecordSize {
		return V9DataRecord{}
	}

	var rec V9DataRecord
	offset := 0

	// IN_BYTES (4)
	rec.Octets = beUint32(data[offset:])
	offset += 4

	// IN_PKTS (4)
	rec.Packets = beUint32(data[offset:])
	offset += 4

	// PROTOCOL (1)
	rec.Protocol = data[offset]
	offset++

	// SRC_TOS (1)
	rec.SrcTOS = data[offset]
	offset++

	// L4_SRC_PORT (2)
	rec.SrcPort = beUint16(data[offset:])
	offset += 2

	// IPV4_SRC_ADDR (4)
	copy(rec.SrcAddr[:], data[offset:offset+4])
	offset += 4

	// SRC_MASK (1)
	rec.SrcMask = data[offset]
	offset++

	// L4_DST_PORT (2)
	rec.DstPort = beUint16(data[offset:])
	offset += 2

	// IPV4_DST_ADDR (4)
	copy(rec.DstAddr[:], data[offset:offset+4])
	offset += 4

	// DST_MASK (1)
	rec.DstMask = data[offset]
	offset++

	// LAST_SWITCHED (4)
	rec.LastSeen = beUint32(data[offset:])
	offset += 4

	// FIRST_SWITCHED (4)
	rec.FirstSeen = beUint32(data[offset:])
	offset += 4

	// APPLICATION_ID (4)
	rec.AppID = beUint32(data[offset:])

	return rec
}

func beUint16(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

func beUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// sendPacket writes data to a collector's UDP connection and updates stats.
func (e *Exporter) sendPacket(cc *CollectorConn, st *ExporterStats, data []byte) {
	n, err := cc.conn.Write(data)
	if err != nil {
		atomic.AddUint64(&st.Errors, 1)
		return
	}
	atomic.AddUint64(&st.PacketsSent, 1)
	atomic.AddUint64(&st.BytesSent, uint64(n))
}
