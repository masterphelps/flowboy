package engine

// TCP flag bitmask constants per RFC 793.
const (
	FlagFIN uint8 = 0x01
	FlagSYN uint8 = 0x02
	FlagRST uint8 = 0x04
	FlagPSH uint8 = 0x08
	FlagACK uint8 = 0x10
	FlagURG uint8 = 0x20
)

type connPhase int

const (
	phaseInitial connPhase = iota
	phaseEstablished
)

// connState tracks the TCP flag lifecycle for a flow.
type connState struct {
	style string // "persistent" or "transactional"
	phase connPhase
}

// newConnState creates a connection state for the given style.
// Empty string defaults to "persistent".
func newConnState(style string) *connState {
	if style == "" {
		style = "persistent"
	}
	return &connState{style: style, phase: phaseInitial}
}

// nextFlags returns the TCP flags for the next record emission.
func (cs *connState) nextFlags() uint8 {
	switch cs.style {
	case "transactional":
		return FlagSYN | FlagACK | FlagPSH | FlagFIN
	default: // persistent
		if cs.phase == phaseInitial {
			cs.phase = phaseEstablished
			return FlagSYN
		}
		return FlagACK | FlagPSH
	}
}

// closeFlags returns the TCP flags for flow shutdown.
func (cs *connState) closeFlags() uint8 {
	return FlagFIN | FlagACK
}
