package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestKnockback_AttackFromEntity verifies that when the agent takes damage
// from a nearby entity, it is knocked back in the direction away from the attacker.
func TestKnockback_AttackFromEntity(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "knockback_entity", "survival", false, tt.MCVersion, DifficultyNormal, true)
			defer env.Cancel()

			ctx := context.Background()

			platformY := int(math.Floor(env.ContainerPos.Y)) - 1
			platformX := int(math.Floor(env.ContainerPos.X)) - 10
			platformZ := int(math.Floor(env.ContainerPos.Z)) - 10

			// Build a flat platform for the agent to stand on
			BuildPlatform(ctx, env.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")

			// Clear area above the platform so nothing blocks movement
			if err := ClearArea(ctx, env.Inst.RCON,
				platformX, platformY+1, platformZ,
				platformX+20, platformY+5, platformZ+20); err != nil {
				t.Logf("warning: failed to clear area: %v", err)
			}

			// Build a roof to protect the zombie from burning in sunlight
			BuildPlatform(ctx, env.Inst.RCON, platformX, platformY+10, platformZ, 20, 20, "minecraft:grass_block")

			time.Sleep(2 * time.Second)

			// Get the bot's current position
			botPos, posInitialized := env.Agent.Agent.GetPositionSimple()
			require.True(t, posInitialized, "bot position initialized")
			t.Logf("Bot position: %s", botPos)

			// Spawn a NoAI zombie 5 blocks in the +X direction from the agent.
			// NoAI prevents it from wandering so the knockback direction is predictable.
			zombieOffsetX := 5.0
			spawnX := botPos.X + zombieOffsetX
			spawnY := botPos.Y
			spawnZ := botPos.Z
			spawnCmd := fmt.Sprintf(
				`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,CanPickUpLoot:1b,ArmorItems:[{},{},{},{id:"minecraft:leather_helmet",Count:1b}]}`,
				spawnX, spawnY, spawnZ,
			)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn zombie")
			t.Logf("Spawn zombie at (+%.0fX): %s", zombieOffsetX, resp)

			// Wait for the entity to be sent to the client and tracked
			time.Sleep(3 * time.Second)

			// Verify the zombie is tracked
			zombieType, typeFound := env.Agent.Agent.GetEntityTypeID("minecraft:zombie")
			require.True(t, typeFound, "zombie entity type should be in registry")
			_, _, entityFound := debugEntityTracking(t, env.Agent, zombieType, botPos, "minecraft:zombie")
			require.True(t, entityFound, "zombie should be tracked by the agent")

			// Record the agent's position before taking damage
			beforePos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position before knockback")
			t.Logf("Position before knockback: (%.4f, %.4f, %.4f)", beforePos.X, beforePos.Y, beforePos.Z)

			// Deal damage to the agent from the zombie using /damage command.
			// This sends a ClientboundDamageEvent packet with the zombie as sourceDirectID.
			damageCmd := fmt.Sprintf(
				`damage %s 1 minecraft:mob_attack by @e[type=zombie,limit=1,sort=nearest]`,
				env.BotName,
			)
			resp, err = env.Inst.RCON.Exec(env.Ctx, damageCmd)
			require.NoError(t, err, "deal damage to agent")
			t.Logf("Damage command response: %s", resp)

			// Wait for knockback physics to take effect
			time.Sleep(2 * time.Second)

			// Record the agent's position after knockback
			afterPos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position after knockback")
			t.Logf("Position after knockback: (%.4f, %.4f, %.4f)", afterPos.X, afterPos.Y, afterPos.Z)

			// The zombie is at +X relative to the agent, so knockback should push
			// the agent in the -X direction (away from the attacker).
			xDisplacement := afterPos.X - beforePos.X
			t.Logf("X displacement: %.4f", xDisplacement)
			require.Less(t, xDisplacement, -0.01,
				"agent should be knocked back in -X direction (away from zombie at +X)")
		})
	}
}

// TestKnockback_AttackFromSourcePosition verifies knockback when the damage
// source position is provided explicitly (no tracked entity), such as when
// the attacker entity is out of tracking range.
func TestKnockback_AttackFromSourcePosition(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "knockback_source_pos", "survival", false, tt.MCVersion, DifficultyNormal, true)
			defer env.Cancel()

			ctx := context.Background()

			platformY := int(math.Floor(env.ContainerPos.Y)) - 1
			platformX := int(math.Floor(env.ContainerPos.X)) - 10
			platformZ := int(math.Floor(env.ContainerPos.Z)) - 10

			// Build a flat platform for the agent to stand on
			BuildPlatform(ctx, env.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")

			// Clear area above the platform
			if err := ClearArea(ctx, env.Inst.RCON,
				platformX, platformY+1, platformZ,
				platformX+20, platformY+5, platformZ+20); err != nil {
				t.Logf("warning: failed to clear area: %v", err)
			}

			time.Sleep(2 * time.Second)

			// Get the bot's current position
			botPos, posInitialized := env.Agent.Agent.GetPositionSimple()
			require.True(t, posInitialized, "bot position initialized")

			// Record the agent's position before taking damage
			beforePos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position before knockback")
			t.Logf("Position before knockback: (%.4f, %.4f, %.4f)", beforePos.X, beforePos.Y, beforePos.Z)

			// Deal damage using an explicit source position instead of a tracked entity.
			// The source is placed in the -Z direction (north) so knockback should push +Z (south).
			sourceZ := botPos.Z - 3.0
			damageCmd := fmt.Sprintf(
				`damage %s 5 minecraft:mob_attack at %.1f %.1f %.1f`,
				env.BotName, botPos.X, beforePos.Y, sourceZ,
			)
			resp, err := env.Inst.RCON.Exec(env.Ctx, damageCmd)
			require.NoError(t, err, "deal damage to agent from source position")
			t.Logf("Damage command response:%s => %s", damageCmd, resp)

			// Wait for knockback physics to take effect
			time.Sleep(2 * time.Second)

			// Record the agent's position after knockback
			afterPos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position after knockback")
			t.Logf("Position after knockback: (%.4f, %.4f, %.4f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Source is at -Z, so knockback should push agent in +Z direction
			zDisplacement := afterPos.Z - beforePos.Z
			t.Logf("Z displacement: %.4f", zDisplacement)
			require.Greater(t, zDisplacement, -0.01,
				"agent should be knocked back in +Z direction (away from source at -Z)")
		})
	}
}
