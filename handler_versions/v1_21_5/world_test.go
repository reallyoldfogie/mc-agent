package v1_21_5

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
)

// TestWorldHandler_ParseSetTickingState covers the ClientboundSetTickingState
// parser added so onSetTickingState (agent/handlers.go) can keep this
// agent's physics executor in sync with a server tick rate changed via
// the vanilla /tick command (added 1.20.5).
func TestWorldHandler_ParseSetTickingState(t *testing.T) {
	pkt := cb.NewSetTickingState()
	pkt.TickRate = pk.Float(40.0)
	pkt.IsFrozen = pk.Boolean(false)

	handler := &worldHandler{}
	tickRate, isFrozen, err := handler.ParseSetTickingState(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseSetTickingState failed: %v", err)
	}
	if tickRate != 40.0 {
		t.Errorf("tickRate = %v, want 40.0", tickRate)
	}
	if isFrozen {
		t.Errorf("isFrozen = true, want false")
	}
}

// TestWorldHandler_ParseSetTickingState_Frozen covers the /tick freeze
// case (isFrozen=true), separately from the rate itself.
func TestWorldHandler_ParseSetTickingState_Frozen(t *testing.T) {
	pkt := cb.NewSetTickingState()
	pkt.TickRate = pk.Float(20.0)
	pkt.IsFrozen = pk.Boolean(true)

	handler := &worldHandler{}
	tickRate, isFrozen, err := handler.ParseSetTickingState(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseSetTickingState failed: %v", err)
	}
	if tickRate != 20.0 {
		t.Errorf("tickRate = %v, want 20.0", tickRate)
	}
	if !isFrozen {
		t.Errorf("isFrozen = false, want true")
	}
}
