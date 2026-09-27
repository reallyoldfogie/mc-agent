package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type combatRunner interface {
	RunCombat(context.Context, float64, bool) error
}

type entityAttackRunner interface {
	AttackEntity(context.Context, int32, bool) error
}

type spearRunner interface {
	ExecuteSpearAttack(context.Context, combat.SpearAttackRequest) error
}

type CombatFlatSuite struct {
	VersionWorldSuite
}

func equipCombatItem(ctx context.Context, leader *WorkingAreaAgent, itemName string) error {
	found, err := waitForAgentHasItem(ctx, leader.Agent, itemName, 5*time.Second)
	if err != nil {
		return fmt.Errorf("wait for %s inventory update: %w", itemName, err)
	}
	if !found {
		return fmt.Errorf("%s did not reach the client inventory", itemName)
	}
	equipped, err := leader.Agent.SwitchToItem(ctx, itemName)
	if err != nil {
		return fmt.Errorf("equip %s: %w", itemName, err)
	}
	if !equipped {
		return fmt.Errorf("%s was not found while equipping", itemName)
	}
	return nil
}

func TestCombatFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &CombatFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyNormal
		return s
	})
}

// TestRunCombatMelee validates autonomous target discovery and melee damage
// against a stationary server-spawned target.
func (s *CombatFlatSuite) TestRunCombatMelee() {
	leader, err := s.SpawnWorkingAreaAgent("CombatLoopBot", "combat_loop")
	require.NoError(s.T(), err, "spawn agent")

	spawnX := leader.Origin.X + 3
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn stationary zombie")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", leader.Name))
	require.NoError(s.T(), err, "give sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:netherite_sword"), "equip sword")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var zombieID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			zombieID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), zombieID, "zombie should be tracked")

	before, ok := leader.GetTrackedEntities()[zombieID]
	require.True(s.T(), ok, "zombie health should be tracked")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 4*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 8, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "combat should stop on test context")

	after, stillTracked := leader.GetTrackedEntities()[zombieID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "combat loop should damage the stationary target")
	}
}

// TestAttackEntityCriticalWhileFalling verifies the server-derived critical
// hit path independently of target-selection movement. The agent is
// teleported above a stationary target, allowed to enter the falling phase,
// and then sends the normal entity-interaction attack packet. A netherite
// sword's damage must exceed its fully charged non-critical base damage.
func (s *CombatFlatSuite) TestAttackEntityCriticalWhileFalling() {
	leader, err := s.SpawnWorkingAreaAgent("CombatCriticalBot", "combat_critical")
	require.NoError(s.T(), err, "spawn critical combat agent")

	spawnX := leader.Origin.X + 2
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:100f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn critical target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", leader.Name))
	require.NoError(s.T(), err, "give critical sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:netherite_sword"), "equip critical sword")
	// The server's attack-strength ticker starts below full charge after the
	// agent joins/equips an item. Let it reach full strength before beginning
	// the falling sequence; otherwise the server reports a weak hit regardless
	// of the falling movement state.
	time.Sleep(600 * time.Millisecond)

	targetID := zombieIDForTest(s, leader, "minecraft:zombie")
	before, ok := leader.GetTrackedEntities()[targetID]
	require.True(s.T(), ok, "critical target should have an initial health snapshot")

	// Two blocks gives the client time to report a negative vertical velocity
	// while keeping the target inside the normal interaction reach. Facing the
	// target also removes server-side direction as a confounding variable.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"tp %s %.1f %.1f %.1f facing %.1f %.1f %.1f",
		leader.Name, spawnX, spawnY+2, spawnZ, spawnX, spawnY+1, spawnZ))
	require.NoError(s.T(), err, "teleport agent above critical target")
	time.Sleep(150 * time.Millisecond)

	attacker, ok := leader.Agent.(entityAttackRunner)
	require.True(s.T(), ok, "agent should expose entity attack")
	require.NoError(s.T(), attacker.AttackEntity(s.Ctx, targetID, false), "send falling attack")

	deadline := time.Now().Add(2 * time.Second)
	var after models.TrackedEntityInfo
	for time.Now().Before(deadline) {
		if current, tracked := leader.GetTrackedEntities()[targetID]; tracked {
			after = current
			if current.Health < before.Health {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Less(s.T(), after.Health, before.Health, "falling attack should damage the target")
	require.Greater(s.T(), before.Health-after.Health, float32(8), "falling attack should exceed a fully charged netherite sword base hit")
}

func (s *CombatFlatSuite) TestRunCombatRangedBow() {
	leader, err := s.SpawnWorkingAreaAgent("CombatRangedBot", "combat_ranged")
	require.NoError(s.T(), err, "spawn agent")

	spawnX := leader.Origin.X + 12
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn ranged target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:bow", leader.Name))
	require.NoError(s.T(), err, "give bow")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:arrow 64", leader.Name))
	require.NoError(s.T(), err, "give arrows")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:bow"), "equip bow")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var zombieID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			zombieID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), zombieID, "ranged target should be tracked")

	before, ok := leader.GetTrackedEntities()[zombieID]
	require.True(s.T(), ok, "ranged target health should be tracked")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 8*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 20, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "ranged combat should stop on test context")

	after, stillTracked := leader.GetTrackedEntities()[zombieID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "ranged combat should damage the distant target")
	}
}

func (s *CombatFlatSuite) TestRunCombatRangedCrossbow() {
	leader, err := s.SpawnWorkingAreaAgent("CombatCrossbowBot", "combat_crossbow")
	require.NoError(s.T(), err, "spawn agent")

	spawnX := leader.Origin.X + 12
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn crossbow target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:crossbow", leader.Name))
	require.NoError(s.T(), err, "give crossbow")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:arrow 64", leader.Name))
	require.NoError(s.T(), err, "give arrows")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:crossbow"), "equip crossbow")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var zombieID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			zombieID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), zombieID, "crossbow target should be tracked")

	before, ok := leader.GetTrackedEntities()[zombieID]
	require.True(s.T(), ok, "crossbow target health should be tracked")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 10*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 20, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "crossbow combat should stop on test context")

	after, stillTracked := leader.GetTrackedEntities()[zombieID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "crossbow combat should damage the distant target")
	}
}

// TestRunCombatShieldBlocksSkeleton verifies the end-to-end shield path:
// inventory synchronization, off-hand shield detection, use-item/release
// packets, and server-side projectile mitigation. This deliberately uses an
// active skeleton rather than /damage because shield blocking only applies to
// an actual directional projectile threat.
func (s *CombatFlatSuite) TestRunCombatShieldBlocksSkeleton() {
	leader, err := s.SpawnWorkingAreaAgent("CombatShieldBot", "combat_shield")
	require.NoError(s.T(), err, "spawn shield combat agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"item replace entity %s weapon.offhand with minecraft:shield",
		leader.Name))
	require.NoError(s.T(), err, "equip shield in off-hand")

	// Confirm the server-side inventory update reached the agent before combat
	// starts; otherwise a successful run could merely mean the shield policy
	// never considered a shield available.
	hasShield, err := waitForAgentHasItem(s.Ctx, leader.Agent, "minecraft:shield", 5*time.Second)
	require.NoError(s.T(), err, "wait for shield inventory update")
	require.True(s.T(), hasShield, "agent should observe the off-hand shield")

	// Spawn the threat only after the shield is present in the server and
	// visible to the agent. This prevents setup damage from being mistaken for
	// a failure of the blocking behavior under test.
	spawnX := leader.Origin.X + 8
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:skeleton %.1f %.1f %.1f {PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn skeleton")

	zombieID := zombieIDForTest(s, leader, "minecraft:skeleton")
	require.NotZero(s.T(), zombieID, "skeleton should be tracked")

	before, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(s.T(), err, "read health before shield combat")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 6*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 12, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "shield combat should stop on test context")
	after, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(s.T(), err, "read health after shield combat")
	require.GreaterOrEqual(s.T(), after, before-0.01, "shield should block skeleton arrow damage")
}

func (s *CombatFlatSuite) TestRunCombatMovingTarget() {
	leader, err := s.SpawnWorkingAreaAgent("CombatMovingBot", "combat_moving")
	require.NoError(s.T(), err, "spawn agent")

	spawnX := leader.Origin.X + 12
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn moving target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:bow", leader.Name))
	require.NoError(s.T(), err, "give bow")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:arrow 64", leader.Name))
	require.NoError(s.T(), err, "give arrows")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:bow"), "equip bow")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var zombieID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			zombieID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), zombieID, "moving target should be tracked")
	before, ok := leader.GetTrackedEntities()[zombieID]
	require.True(s.T(), ok, "moving target should have an initial snapshot")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 6*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 20, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "moving-target combat should stop on test context")
	after, stillTracked := leader.GetTrackedEntities()[zombieID]
	if stillTracked && !after.Removed {
		require.NotEqual(s.T(), before.X, after.X, "moving target X should change")
	}
}

func (s *CombatFlatSuite) TestRunCombatObstructedTarget() {
	leader, err := s.SpawnWorkingAreaAgent("CombatObstructedBot", "combat_obstructed")
	require.NoError(s.T(), err, "spawn agent")

	spawnX := leader.Origin.X + 12
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn obstructed target")
	wallX := int(math.Round(leader.Origin.X + 6))
	wallY := int(math.Floor(leader.Origin.Y))
	wallZ := int(math.Round(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:stone",
		wallX, wallY, wallZ-3, wallX, wallY+2, wallZ+3))
	require.NoError(s.T(), err, "build obstruction")
	_, err = WaitForBlockState(s.Ctx, leader.ManagedAgent, models.V3{
		X: float64(wallX), Y: float64(wallY + 1), Z: float64(wallZ),
	}, "minecraft:stone", 5*time.Second)
	require.NoError(s.T(), err, "wait for obstruction to reach the client world")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:bow", leader.Name))
	require.NoError(s.T(), err, "give bow")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:arrow 64", leader.Name))
	require.NoError(s.T(), err, "give arrows")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:bow"), "equip bow")

	visible, err := leader.Agent.HasLineOfSight(s.Ctx, spawnX, spawnY, spawnZ)
	require.NoError(s.T(), err, "check obstruction line of sight")
	require.False(s.T(), visible, "stone wall should block target line of sight")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 4*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 20, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "obstructed combat should stop on test context")
}

func (s *CombatFlatSuite) TestSpearJab1211() {
	if !combat.SpearSupportedVersion(s.Version) {
		s.T().Skipf("spears require Minecraft Java 1.21.11+: %s", s.Version)
	}
	leader, err := s.SpawnWorkingAreaAgent("CombatSpearJabBot", "combat_spear_jab")
	require.NoError(s.T(), err, "spawn agent")
	spawnX := leader.Origin.X + 3
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn spear target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:iron_spear", leader.Name))
	require.NoError(s.T(), err, "give spear")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var zombieID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			zombieID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), zombieID, "spear target should be tracked")
	before, ok := leader.GetTrackedEntities()[zombieID]
	require.True(s.T(), ok, "spear target health should be tracked")
	runner, ok := leader.Agent.(spearRunner)
	require.True(s.T(), ok, "agent should expose spear execution")
	err = runner.ExecuteSpearAttack(s.Ctx, combat.SpearAttackRequest{
		TargetID: zombieID, ItemName: "minecraft:iron_spear", Mode: combat.SpearJab,
		Distance: 3, MinReach: 1, MaxReach: 6,
	})
	require.NoError(s.T(), err, "spear Jab")
	time.Sleep(750 * time.Millisecond)
	after, stillTracked := leader.GetTrackedEntities()[zombieID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "spear Jab should damage the target")
	}
}

func (s *CombatFlatSuite) TestSpearCharge1211() {
	if !combat.SpearSupportedVersion(s.Version) {
		s.T().Skipf("spears require Minecraft Java 1.21.11+: %s", s.Version)
	}
	leader, err := s.SpawnWorkingAreaAgent("CombatSpearChargeBot", "combat_spear_charge")
	require.NoError(s.T(), err, "spawn agent")
	spawnX := leader.Origin.X + 4
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn charge target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:iron_spear", leader.Name))
	require.NoError(s.T(), err, "give spear")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:iron_spear"), "equip spear")
	runner, ok := leader.Agent.(spearRunner)
	require.True(s.T(), ok, "agent should expose spear execution")
	err = runner.ExecuteSpearAttack(s.Ctx, combat.SpearAttackRequest{
		TargetID: zombieIDForTest(s, leader, "minecraft:zombie"), ItemName: "minecraft:iron_spear", Mode: combat.SpearCharge,
		Distance: 4, MinReach: 1, MaxReach: 6,
		TargetPosition: models.V3{X: spawnX, Y: spawnY, Z: spawnZ},
		ViewAlignment:  1, MinAlignment: 0.5, RelativeSpeed: 1, MinSpeed: 0,
		HoldDuration: 250 * time.Millisecond,
		Profile:      combat.SpearChargeProfile{EngagedDuration: 100 * time.Millisecond, TiredDuration: 100 * time.Millisecond},
	})
	require.NoError(s.T(), err, "spear Charge packet execution")
}

func zombieIDForTest(s *CombatFlatSuite, leader *WorkingAreaAgent, typeName string) int32 {
	typeID, found := leader.Agent.GetEntityTypeID(typeName)
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			return id
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Fail(s.T(), "zombie should be tracked")
	return 0
}
