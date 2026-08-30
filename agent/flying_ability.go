package agent

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/models"
)

// SetFlying requests the flying ability be toggled. See decompiled
// ServerPlayNetworkHandler.onUpdatePlayerAbilities: the server never echoes
// an Abilities packet back in response to the serverbound one - it silently
// updates its own state, gated by AllowFlying - so a real client (and this
// one) optimistically updates its own tracked Flying state immediately
// after sending rather than waiting for confirmation.
//
// TODO(PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4 step 3): this only
// updates ability state/the wire packet so far - it does not yet switch the
// physics engine into a flying movement mode.
func (a *agent) SetFlying(ctx context.Context, flying bool) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	abilities, ok := a.GetPlayerAbilities()
	if flying && (!ok || !abilities.AllowFlying) {
		return fmt.Errorf("server has not granted this player permission to fly")
	}

	var flags byte
	if flying {
		flags |= models.AbilitiesFlagFlying
	}
	if err := a.versionHandler.Play().Movement().SendPlayerAbilities(a.client.Conn(), flags); err != nil {
		return fmt.Errorf("send player abilities: %w", err)
	}

	abilities.Flying = flying
	a.setPlayerAbilities(abilities)

	return nil
}
