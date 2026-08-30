package v1_21_1

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	agent_models "github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/clientbound"
)

func TestPlayHandler_ParseLogin_ExtractsGameMode(t *testing.T) {
	pkt := cb.NewLogin()
	pkt.EntityId = pk.Int(42)
	pkt.WorldNames.Set([]pk.String{})
	pkt.WorldState.Gamemode.Value = "creative"

	handler := &playHandler{}
	entityID, gameMode, err := handler.ParseLogin(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseLogin failed: %v", err)
	}
	if entityID != 42 {
		t.Errorf("expected entityID 42, got %d", entityID)
	}
	if gameMode != agent_models.GameModeCreative {
		t.Errorf("expected GameModeCreative, got %v", gameMode)
	}
}

func TestPlayHandler_ParseLogin_DefaultsToSurvival(t *testing.T) {
	pkt := cb.NewLogin()
	pkt.EntityId = pk.Int(7)
	pkt.WorldNames.Set([]pk.String{})
	pkt.WorldState.Gamemode.Value = "survival"

	handler := &playHandler{}
	_, gameMode, err := handler.ParseLogin(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseLogin failed: %v", err)
	}
	if gameMode != agent_models.GameModeSurvival {
		t.Errorf("expected GameModeSurvival, got %v", gameMode)
	}
}

func TestPlayHandler_ParseClientboundAbilities(t *testing.T) {
	pkt := cb.NewAbilities()
	pkt.Flags = pk.Byte(agent_models.AbilitiesFlagFlying | agent_models.AbilitiesFlagAllowFlying)
	pkt.FlyingSpeed = pk.Float(0.05)
	pkt.WalkingSpeed = pk.Float(0.1)

	handler := &playHandler{}
	abilities, err := handler.ParseClientboundAbilities(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseClientboundAbilities failed: %v", err)
	}

	want := agent_models.PlayerAbilities{
		Invulnerable: false,
		Flying:       true,
		AllowFlying:  true,
		CreativeMode: false,
		FlySpeed:     0.05,
		WalkSpeed:    0.1,
	}
	if abilities != want {
		t.Errorf("expected %+v, got %+v", want, abilities)
	}
}
