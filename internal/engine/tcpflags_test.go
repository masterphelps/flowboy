package engine

import "testing"

func TestFlagsPersistentLifecycle(t *testing.T) {
	cs := newConnState("persistent")

	flags := cs.nextFlags()
	if flags != FlagSYN {
		t.Errorf("first tick persistent: expected SYN (0x02), got 0x%02x", flags)
	}

	flags = cs.nextFlags()
	if flags != FlagACK|FlagPSH {
		t.Errorf("second tick persistent: expected ACK|PSH (0x18), got 0x%02x", flags)
	}

	flags = cs.nextFlags()
	if flags != FlagACK|FlagPSH {
		t.Errorf("third tick persistent: expected ACK|PSH (0x18), got 0x%02x", flags)
	}

	flags = cs.closeFlags()
	if flags != FlagFIN|FlagACK {
		t.Errorf("close persistent: expected FIN|ACK (0x11), got 0x%02x", flags)
	}
}

func TestFlagsTransactionalLifecycle(t *testing.T) {
	cs := newConnState("transactional")

	flags := cs.nextFlags()
	if flags != FlagSYN|FlagACK|FlagPSH|FlagFIN {
		t.Errorf("transactional tick: expected 0x1B, got 0x%02x", flags)
	}

	flags = cs.nextFlags()
	if flags != FlagSYN|FlagACK|FlagPSH|FlagFIN {
		t.Errorf("transactional tick 2: expected 0x1B, got 0x%02x", flags)
	}
}

func TestFlagsDefaultIsPersistent(t *testing.T) {
	cs := newConnState("")
	flags := cs.nextFlags()
	if flags != FlagSYN {
		t.Errorf("default first tick: expected SYN (0x02), got 0x%02x", flags)
	}
}
