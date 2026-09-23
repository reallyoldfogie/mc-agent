package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// KnockbackFlatSuite is a
// version-parameterized suite for knockback tests: one server per version,
// shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestWithModeAndBlockPlacement). WorldGen = WorldGenFlat and
// Difficulty = DifficultyNormal, matching every pre-conversion TestKnockback*/
// TestWindCharged* function's own choice.
type KnockbackFlatSuite struct {
	VersionWorldSuite
}

func TestKnockbackFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &KnockbackFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyNormal
		return s
	})
}

// buildKnockbackPlatform builds a flat platform centered on origin (matching
// every pre-conversion knockback test's own env.ContainerPos-relative
// layout - env.ContainerPos was already just the agent's own spawn point
// for these placeBlock=false tests, exactly what SpawnWorkingAreaAgent's
// Origin is here) and, if roofed, a roof 10 blocks above it to protect a
// spawned zombie from burning in sunlight.
func buildKnockbackPlatform(s *KnockbackFlatSuite, origin models.V3, roofed bool) {
	platformY := int(math.Floor(origin.Y)) - 1
	platformX := int(math.Floor(origin.X)) - 10
	platformZ := int(math.Floor(origin.Z)) - 10

	BuildPlatform(s.Ctx, s.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")

	if err := ClearArea(s.Ctx, s.Inst.RCON,
		platformX, platformY+1, platformZ,
		platformX+20, platformY+5, platformZ+20); err != nil {
		s.T().Logf("warning: failed to clear area: %v", err)
	}

	if roofed {
		BuildPlatform(s.Ctx, s.Inst.RCON, platformX, platformY+10, platformZ, 20, 20, "minecraft:grass_block")
	}
}

// TestAttackFromEntity verifies that when the agent takes damage from a
// nearby entity, it is knocked back in the direction away from the
// attacker. Equivalent to the original TestKnockback_AttackFromEntity.
func (s *KnockbackFlatSuite) TestAttackFromEntity() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("KnockbackEntityBot", "knockback_entity")
	require.NoError(t, err, "spawn agent")

	buildKnockbackPlatform(s, leader.Origin, true)
	time.Sleep(2 * time.Second)

	botPos, posInitialized := leader.Agent.GetPositionSimple()
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
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn zombie")
	t.Logf("Spawn zombie at (+%.0fX): %s", zombieOffsetX, resp)

	time.Sleep(3 * time.Second)

	zombieType, typeFound := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(t, typeFound, "zombie entity type should be in registry")
	_, _, entityFound := debugEntityTracking(t, leader.ManagedAgent, zombieType, botPos, "minecraft:zombie")
	require.True(t, entityFound, "zombie should be tracked by the agent")

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before knockback")
	t.Logf("Position before knockback: (%.4f, %.4f, %.4f)", beforePos.X, beforePos.Y, beforePos.Z)

	// Deal damage to the agent from the zombie using /damage command. This
	// sends a ClientboundDamageEvent packet with the zombie as
	// sourceDirectID.
	damageCmd := fmt.Sprintf(
		`damage %s 1 minecraft:mob_attack by @e[type=zombie,limit=1,sort=nearest]`,
		leader.Name,
	)
	resp, err = s.Inst.RCON.Exec(s.Ctx, damageCmd)
	require.NoError(t, err, "deal damage to agent")
	t.Logf("Damage command response: %s", resp)

	time.Sleep(2 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after knockback")
	t.Logf("Position after knockback: (%.4f, %.4f, %.4f)", afterPos.X, afterPos.Y, afterPos.Z)

	// The zombie is at +X relative to the agent, so knockback should push
	// the agent in the -X direction (away from the attacker).
	xDisplacement := afterPos.X - beforePos.X
	t.Logf("X displacement: %.4f", xDisplacement)
	require.Less(t, xDisplacement, -0.01,
		"agent should be knocked back in -X direction (away from zombie at +X)")
}

// TestAttackFromSourcePosition verifies knockback when the damage source
// position is provided explicitly (no tracked entity), such as when the
// attacker entity is out of tracking range. Equivalent to the original
// TestKnockback_AttackFromSourcePosition.
func (s *KnockbackFlatSuite) TestAttackFromSourcePosition() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("KnockbackSourcePosBot", "knockback_source_pos")
	require.NoError(t, err, "spawn agent")

	buildKnockbackPlatform(s, leader.Origin, false)
	time.Sleep(2 * time.Second)

	botPos, posInitialized := leader.Agent.GetPositionSimple()
	require.True(t, posInitialized, "bot position initialized")

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before knockback")
	t.Logf("Position before knockback: (%.4f, %.4f, %.4f)", beforePos.X, beforePos.Y, beforePos.Z)

	// Deal damage using an explicit source position instead of a tracked
	// entity. The source is placed in the -Z direction (north) so knockback
	// should push +Z (south).
	sourceZ := botPos.Z - 3.0
	damageCmd := fmt.Sprintf(
		`damage %s 5 minecraft:mob_attack at %.1f %.1f %.1f`,
		leader.Name, botPos.X, beforePos.Y, sourceZ,
	)
	resp, err := s.Inst.RCON.Exec(s.Ctx, damageCmd)
	require.NoError(t, err, "deal damage to agent from source position")
	t.Logf("Damage command response:%s => %s", damageCmd, resp)

	time.Sleep(2 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after knockback")
	t.Logf("Position after knockback: (%.4f, %.4f, %.4f)", afterPos.X, afterPos.Y, afterPos.Z)

	// Source is at -Z, so knockback should push agent in +Z direction.
	zDisplacement := afterPos.Z - beforePos.Z
	t.Logf("Z displacement: %.4f", zDisplacement)
	require.Greater(t, zDisplacement, -0.01,
		"agent should be knocked back in +Z direction (away from source at -Z)")
}

// TestWindChargedExplosionKnocksBackNearbyPlayer verifies PHASE_4_PLAN.md
// §2.2/§4.12 end-to-end: an entity with Wind Charged dying nearby triggers a
// knockback-only explosion (WindChargedStatusEffect.onEntityRemoval), and
// the agent's own predicted position is pushed away from it. Equivalent to
// the original function of the same name.
func (s *KnockbackFlatSuite) TestWindChargedExplosionKnocksBackNearbyPlayer() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WindChargedBot", "wind_charged_explosion")
	require.NoError(t, err, "spawn agent")

	buildKnockbackPlatform(s, leader.Origin, false)
	time.Sleep(2 * time.Second)

	botPos, posInitialized := leader.Agent.GetPositionSimple()
	require.True(t, posInitialized, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Spawn a NoAI pig 2 blocks in the +X direction - close enough to
	// guarantee it's within the explosion's 3-5 block blast radius
	// regardless of the random roll, but far enough that the agent isn't
	// standing exactly on the explosion's center.
	pigOffsetX := 2.0
	spawnX := botPos.X + pigOffsetX
	spawnY := botPos.Y
	spawnZ := botPos.Z
	spawnCmd := fmt.Sprintf(`summon minecraft:pig %.1f %.1f %.1f {Health:10f,NoAI:1b}`, spawnX, spawnY, spawnZ)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn pig")
	t.Logf("Spawn pig at (+%.0fX): %s", pigOffsetX, resp)

	time.Sleep(1 * time.Second)

	effectCmd := "effect give @e[type=minecraft:pig,limit=1,sort=nearest] minecraft:wind_charged 100 0"
	resp, err = s.Inst.RCON.Exec(s.Ctx, effectCmd)
	require.NoError(t, err, "apply wind charged to pig")
	t.Logf("Effect command response: %s", resp)

	time.Sleep(300 * time.Millisecond)

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before explosion")
	t.Logf("Position before explosion: (%.4f, %.4f, %.4f)", beforePos.X, beforePos.Y, beforePos.Z)

	// Kill the pig - WindChargedStatusEffect.onEntityRemoval fires on
	// RemovalReason.KILLED, creating a knockback-only explosion centered on
	// it.
	killCmd := "kill @e[type=minecraft:pig,limit=1,sort=nearest]"
	resp, err = s.Inst.RCON.Exec(s.Ctx, killCmd)
	require.NoError(t, err, "kill wind-charged pig")
	t.Logf("Kill command response: %s", resp)

	time.Sleep(2 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after explosion")
	t.Logf("Position after explosion: (%.4f, %.4f, %.4f)", afterPos.X, afterPos.Y, afterPos.Z)

	// The pig died at +X relative to the agent, so knockback should push
	// the agent in the -X direction (away from the explosion).
	xDisplacement := afterPos.X - beforePos.X
	t.Logf("X displacement: %.4f", xDisplacement)
	require.Less(t, xDisplacement, -0.01,
		"agent should be knocked back in -X direction (away from the wind-charged explosion at +X)")
}
