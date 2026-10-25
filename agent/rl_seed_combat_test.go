package agent

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestVerifyPlayerNearPosition_RequiresRCON(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)
	_, err = a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{}, 10)
	if err == nil {
		t.Fatal("want an error when RCON is not configured")
	}
}

func TestVerifyPlayerNearPosition_WithinRadius(t *testing.T) {
	rc := &execOnlyRCON{response: "SomePlayer has the following entity data: [1.0d, 64.0d, 1.0d]"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{X: 0, Y: 64, Z: 0}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v", err)
	}
	if !near {
		t.Fatalf("near = false, want true (distance ~%.2f is within radius 5)", 1.0*1.414)
	}
}

func TestVerifyPlayerNearPosition_OutsideRadius(t *testing.T) {
	rc := &execOnlyRCON{response: "SomePlayer has the following entity data: [100.0d, 64.0d, 100.0d]"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{X: 0, Y: 64, Z: 0}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v", err)
	}
	if near {
		t.Fatal("near = true, want false (player is 100+ blocks away)")
	}
}

func TestVerifyPlayerNearPosition_NotLocatableIsNotAnError(t *testing.T) {
	rc := &execOnlyRCON{response: "No entity was found"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "GhostPlayer", models.V3{}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v, want nil (not-locatable is a normal outcome, not a transport error)", err)
	}
	if near {
		t.Fatal("near = true, want false for an unlocatable player")
	}
}
