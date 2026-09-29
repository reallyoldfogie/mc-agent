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

type combatPolicyRunner interface {
	RunCombatWithPolicy(context.Context, float64, combat.TargetPolicy) error
}

type creeperLootRunner interface {
	KillCreeperForGunpowder(context.Context) (bool, error)
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

// TestPlayerArmorEquipmentTracking verifies that a combat agent observes armor
// changes made by another player, including the explicit empty-slot update
// sent when the armor is removed.
func (s *CombatFlatSuite) TestPlayerArmorEquipmentTracking() {
	observer, err := s.SpawnWorkingAreaAgentNoCam("CombatArmorObserver", "combat_armor_observer")
	require.NoError(s.T(), err, "spawn armor observer")
	target, err := s.SpawnAgentNearNoCam("CombatArmorTarget", "combat_armor_target", observer.Origin, 3, 0)
	require.NoError(s.T(), err, "spawn armor target")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:diamond_chestplate", target.Name))
	require.NoError(s.T(), err, "give chestplate")
	found, err := waitForAgentHasItem(s.Ctx, target.Agent, "minecraft:diamond_chestplate", 5*time.Second)
	require.NoError(s.T(), err, "wait for chestplate inventory update")
	require.True(s.T(), found, "target should observe the diamond chestplate in its inventory")
	require.NoError(s.T(), target.Agent.Equip(s.Ctx, "minecraft:diamond_chestplate"), "equip chestplate")

	itemRegistry := observer.Agent.GetRegistry("minecraft:item")
	require.NotNil(s.T(), itemRegistry, "item registry should be available")
	chestplateID, ok := itemRegistry.GetIDByName("minecraft:diamond_chestplate")
	require.True(s.T(), ok, "diamond chestplate should be in the item registry")

	deadline := time.Now().Add(5 * time.Second)
	equipped := false
	for time.Now().Before(deadline) {
		if info, found := observer.GetTrackedEntities()[target.Agent.GetEntityID()]; found {
			if item, hasEquipment := info.Equipment[models.EquipmentSlotChest]; hasEquipment && item.Present && item.Count > 0 && item.ItemID == chestplateID {
				equipped = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(s.T(), equipped, "observer never received the target's equipped chestplate")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", target.Name))
	require.NoError(s.T(), err, "remove chestplate")

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, found := observer.GetTrackedEntities()[target.Agent.GetEntityID()]; found {
			item := info.Equipment[models.EquipmentSlotChest]
			if !item.Present || item.Count == 0 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.FailNow(s.T(), "observer never received the target's cleared chest slot")
}

// TestRunCombatPvPOptIn verifies that the normal combat entry point does not
// target players, while the explicit policy entry point can damage a tracked
// player after PvP is enabled by the caller.
func (s *CombatFlatSuite) TestRunCombatPvPOptIn() {
	attacker, err := s.SpawnWorkingAreaAgent("CombatPvPAttacker", "combat_pvp_attacker")
	require.NoError(s.T(), err, "spawn attacker")
	// Keep the target very close. On 1.21.1 the initial near-agent teleport can
	// settle several blocks farther away than requested; one block still leaves
	// enough separation for a real player target while avoiding a reach-boundary
	// failure in the PvP assertion.
	target, err := s.SpawnAgentNearNoCam("CombatPvPTarget", "combat_pvp_target", attacker.Origin, 1, 0)
	require.NoError(s.T(), err, "spawn target")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", attacker.Name))
	require.NoError(s.T(), err, "give PvP sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, attacker, "minecraft:netherite_sword"), "equip PvP sword")

	before, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, target.Name)
	require.NoError(s.T(), err, "read target health before PvP")

	defaultRunner, ok := attacker.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose default combat runner")
	defaultCtx, defaultCancel := context.WithTimeout(s.Ctx, 1200*time.Millisecond)
	err = defaultRunner.RunCombat(defaultCtx, 8, false)
	defaultCancel()
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "default PvE combat should stop on context")
	afterDefault, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, target.Name)
	require.NoError(s.T(), err, "read target health after default combat")
	require.InDelta(s.T(), before, afterDefault, 0.01, "default combat must not target players")

	policyRunner, ok := attacker.Agent.(combatPolicyRunner)
	require.True(s.T(), ok, "agent should expose policy combat runner")
	pvpCtx, pvpCancel := context.WithTimeout(s.Ctx, 4*time.Second)
	defer pvpCancel()
	err = policyRunner.RunCombatWithPolicy(pvpCtx, 8, combat.TargetPolicy{IncludePlayers: true})
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "PvP combat should stop on context")
	afterPvP, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, target.Name)
	require.NoError(s.T(), err, "read target health after PvP")
	require.Less(s.T(), afterPvP, afterDefault, "explicit PvP policy should damage the player target")
}

// TestPlayerDeathRespawnTracking verifies that a player death removes the
// target from combat tracking and that the same player becomes eligible again
// after the target agent's automatic respawn.
func (s *CombatFlatSuite) TestPlayerDeathRespawnTracking() {
	observer, err := s.SpawnWorkingAreaAgentNoCam("CombatDeathObserver", "combat_death_observer")
	require.NoError(s.T(), err, "spawn death observer")
	target, err := s.SpawnAgentNearNoCam("CombatDeathTarget", "combat_death_target", observer.Origin, 3, 0)
	require.NoError(s.T(), err, "spawn death target")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"spawnpoint %s %.0f %.0f %.0f", target.Name, observer.Origin.X, observer.Origin.Y, observer.Origin.Z))
	require.NoError(s.T(), err, "set target spawn point")

	targetID := target.Agent.GetEntityID()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, found := observer.GetTrackedEntities()[targetID]; found && !info.Removed && info.MaxHealth > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	initial, found := observer.GetTrackedEntities()[targetID]
	require.True(s.T(), found, "observer should track the player before death")
	require.False(s.T(), initial.Removed, "player should be active before death")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("kill %s", target.Name))
	require.NoError(s.T(), err, "kill target player")

	deadline = time.Now().Add(5 * time.Second)
	deadObserved := false
	for time.Now().Before(deadline) {
		info, tracked := observer.GetTrackedEntities()[targetID]
		if !tracked || info.Removed || (info.MaxHealth > 0 && info.Health <= 0) {
			deadObserved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(s.T(), deadObserved, "observer should stop treating the dead player as an active target")

	// onDeath schedules Respawn after five seconds. Allow that plus packet
	// propagation and verify that the respawned player is tracked at the same
	// entity ID with a fresh positive-health snapshot.
	deadline = time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if info, tracked := observer.GetTrackedEntities()[targetID]; tracked && !info.Removed && info.Health > 0 {
			require.Greater(s.T(), info.MaxHealth, float32(0), "respawned player should have known max health")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.FailNow(s.T(), "observer never received an active positive-health snapshot after player respawn")
}

// TestRunCombatMultipleTargetsRetreats verifies that the combat loop counts
// multiple active hostile targets and retreats instead of attacking into a
// surround.
func (s *CombatFlatSuite) TestRunCombatMultipleTargetsRetreats() {
	leader, err := s.SpawnWorkingAreaAgent("CombatMultiTargetBot", "combat_multi_target")
	require.NoError(s.T(), err, "spawn multi-target combat agent")

	positions := [][2]float64{{4, 0}, {5, 2}, {5, -2}, {7, 0}}
	for i, position := range positions {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
			`summon minecraft:zombie %.1f %.1f %.1f {Health:100f,NoAI:1b,PersistenceRequired:1b}`,
			leader.Origin.X+position[0], leader.Origin.Y, leader.Origin.Z+position[1]))
		require.NoError(s.T(), err, "spawn hostile target %d", i)
	}
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", leader.Name))
	require.NoError(s.T(), err, "give multi-target sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:netherite_sword"), "equip multi-target sword")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(s.T(), found, "zombie type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var targets []models.TrackedEntityInfo
	for time.Now().Before(deadline) {
		targets = targets[:0]
		for _, entity := range leader.GetTrackedEntities() {
			if entity.EntityType == typeID && !entity.Removed && entity.MaxHealth > 0 {
				targets = append(targets, entity)
			}
		}
		if len(targets) >= len(positions) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.GreaterOrEqual(s.T(), len(targets), len(positions), "all hostile targets should be tracked")

	before, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "multi-target agent position should be initialized")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 2*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 12, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "multi-target combat should stop on test context")

	after, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "multi-target agent position should remain initialized")
	nearestBefore := math.MaxFloat64
	nearestAfter := math.MaxFloat64
	for _, target := range targets {
		nearestBefore = math.Min(nearestBefore, before.DistanceToXZ(models.V3{X: target.X, Z: target.Z}))
		nearestAfter = math.Min(nearestAfter, after.DistanceToXZ(models.V3{X: target.X, Z: target.Z}))
	}
	require.Greater(s.T(), nearestAfter, nearestBefore+0.1, "multi-target pressure should move the agent away from the surround")
}

// TestRunCombatCreeperMaintainsDistance verifies that the hazardous-target
// movement filter replaces an exposed approach with evasive movement.
func (s *CombatFlatSuite) TestRunCombatCreeperMaintainsDistance() {
	leader, err := s.SpawnWorkingAreaAgent("CombatCreeperBot", "combat_creeper")
	require.NoError(s.T(), err, "spawn creeper combat agent")

	spawnX := leader.Origin.X + 8
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:creeper %.1f %.1f %.1f {NoAI:1b,PersistenceRequired:1b}`, spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn stationary creeper")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", leader.Name))
	require.NoError(s.T(), err, "give creeper sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:netherite_sword"), "equip creeper sword")

	typeID, found := leader.Agent.GetEntityTypeID("minecraft:creeper")
	require.True(s.T(), found, "creeper type should be registered")
	deadline := time.Now().Add(5 * time.Second)
	var targetID int32
	for time.Now().Before(deadline) {
		if id, _, ok := leader.Agent.FindNearestEntityByType(typeID, leader.Origin.X, leader.Origin.Y, leader.Origin.Z, false); ok {
			targetID = id
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotZero(s.T(), targetID, "creeper should be tracked")
	target, ok := leader.GetTrackedEntities()[targetID]
	require.True(s.T(), ok, "creeper snapshot should be available")
	before, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "creeper combat agent position should be initialized")
	initialDistance := before.DistanceToXZ(models.V3{X: target.X, Z: target.Z})

	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 2*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 12, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "creeper combat should stop on test context")

	after, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "creeper combat agent position should remain initialized")
	finalDistance := after.DistanceToXZ(models.V3{X: target.X, Z: target.Z})
	require.GreaterOrEqual(s.T(), finalDistance, initialDistance-0.5,
		"hazardous creeper approach should preserve distance: initial=%.2f final=%.2f", initialDistance, finalDistance)
}

// TestRunCombatCreeperKillsAndCollectsGunpowder verifies the complete
// strike-and-disengage encounter. The agent must close to melee reach, land
// hits, retreat far enough to reset the fuse, and repeat until the creeper
// dies; it then walks to the dropped gunpowder and lets vanilla pickup collect
// it.
func (s *CombatFlatSuite) TestRunCombatCreeperKillsAndCollectsGunpowder() {
	leader, err := s.SpawnWorkingAreaAgent("CombatCreeperLootBot", "combat_creeper_loot")
	require.NoError(s.T(), err, "spawn creeper loot agent")

	spawnX := leader.Origin.X + 6
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:creeper %.1f %.1f %.1f {Health:20f,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn creeper")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:netherite_sword", leader.Name))
	require.NoError(s.T(), err, "give creeper sword")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:netherite_sword"), "equip creeper sword")

	runner, ok := leader.Agent.(creeperLootRunner)
	require.True(s.T(), ok, "agent should expose KillCreeperForGunpowder")
	collected, err := runner.KillCreeperForGunpowder(s.Ctx)
	require.NoError(s.T(), err, "complete creeper encounter")

	health, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(s.T(), err, "check agent health after creeper encounter")
	require.Greater(s.T(), health, float32(0), "agent must survive the creeper encounter")
	if collected {
		collected, err = waitForInventoryItem(s.Ctx, s.Inst.RCON, leader.Name, "minecraft:gunpowder", 5*time.Second)
		require.NoError(s.T(), err, "verify collected gunpowder")
		require.True(s.T(), collected, "action reported gunpowder collection")
	}
}

// TestRunCombatRetreatsAtCriticalHealth verifies that a known low-health agent
// stops attacking and moves away from a visible target. The target is NoAI so
// any displacement is attributable to the combat retreat policy.
func (s *CombatFlatSuite) TestRunCombatRetreatsAtCriticalHealth() {
	leader, err := s.SpawnWorkingAreaAgent("CombatRetreatBot", "combat_retreat")
	require.NoError(s.T(), err, "spawn retreat combat agent")

	spawnX := leader.Origin.X + 4
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:100f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn retreat target")
	targetID := zombieIDForTest(s, leader, "minecraft:zombie")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("damage %s 17 minecraft:generic", leader.Name))
	require.NoError(s.T(), err, "reduce agent health")
	healthProvider, ok := leader.Agent.(models.HealthProvider)
	require.True(s.T(), ok, "agent should expose health observations")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		health, _, _, known := healthProvider.Health()
		if known && health <= 4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	health, _, _, known := healthProvider.Health()
	require.True(s.T(), known, "agent health should be synchronized")
	require.LessOrEqual(s.T(), health, float32(4), "agent should be at critical health")

	before, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "agent position should be initialized")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 2*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 8, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "retreat combat should stop on test context")

	after, initialized := leader.Agent.GetPositionSimple()
	require.True(s.T(), initialized, "agent position should remain initialized")
	tracked, trackedOK := leader.GetTrackedEntities()[targetID]
	require.True(s.T(), trackedOK, "retreat target should remain tracked")
	beforeDistance := before.DistanceToXZ(models.V3{X: tracked.X, Z: tracked.Z})
	afterDistance := after.DistanceToXZ(models.V3{X: tracked.X, Z: tracked.Z})
	require.Greater(s.T(), afterDistance, beforeDistance+0.1,
		"critical-health combat should move away from the target")
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

// TestRunCombatRangedTrident verifies the final fallback in autonomous ranged
// dispatch: when no bow or crossbow is available, a held trident is selected
// and thrown at a distant target.
func (s *CombatFlatSuite) TestRunCombatRangedTrident() {
	leader, err := s.SpawnWorkingAreaAgent("CombatTridentBot", "combat_trident")
	require.NoError(s.T(), err, "spawn trident combat agent")

	spawnX := leader.Origin.X + 12
	// Keep the target's feet one block above the flat-world origin. At Y=0,
	// the trident solver can correctly report the ground block as intersecting
	// the final downward trajectory on older protocol versions, even though the
	// server-side mob is otherwise a valid target.
	spawnY := leader.Origin.Y + 1
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn trident target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:trident", leader.Name))
	require.NoError(s.T(), err, "give trident")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:trident"), "equip trident")

	targetID := zombieIDForTest(s, leader, "minecraft:zombie")
	before, ok := leader.GetTrackedEntities()[targetID]
	require.True(s.T(), ok, "trident target should be tracked")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 10*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 20, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "trident combat should stop on test context")
	after, stillTracked := leader.GetTrackedEntities()[targetID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "autonomous trident throw should damage the target")
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

// TestRunCombatSpearJab1211 verifies that autonomous melee dispatch detects a
// held 1.21.11+ spear and routes it through the spear executor instead of
// treating it as an untyped AttackEntity weapon.
func (s *CombatFlatSuite) TestRunCombatSpearJab1211() {
	if !combat.SpearSupportedVersion(s.Version) {
		s.T().Skipf("spears require Minecraft Java 1.21.11+: %s", s.Version)
	}
	leader, err := s.SpawnWorkingAreaAgent("CombatSpearLoopBot", "combat_spear_loop")
	require.NoError(s.T(), err, "spawn autonomous spear agent")
	spawnX := leader.Origin.X + 3
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,NoAI:1b,PersistenceRequired:1b}`,
		spawnX, spawnY, spawnZ))
	require.NoError(s.T(), err, "spawn autonomous spear target")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:iron_spear", leader.Name))
	require.NoError(s.T(), err, "give autonomous spear")
	require.NoError(s.T(), equipCombatItem(s.Ctx, leader, "minecraft:iron_spear"), "equip autonomous spear")

	targetID := zombieIDForTest(s, leader, "minecraft:zombie")
	before, ok := leader.GetTrackedEntities()[targetID]
	require.True(s.T(), ok, "autonomous spear target should be tracked")
	runner, ok := leader.Agent.(combatRunner)
	require.True(s.T(), ok, "agent should expose RunCombat")
	ctx, cancel := context.WithTimeout(s.Ctx, 3*time.Second)
	defer cancel()
	err = runner.RunCombat(ctx, 8, false)
	require.ErrorIs(s.T(), err, context.DeadlineExceeded, "autonomous spear combat should stop on test context")
	after, stillTracked := leader.GetTrackedEntities()[targetID]
	if stillTracked && !after.Removed {
		require.Less(s.T(), after.Health, before.Health, "autonomous spear Jab should damage the target")
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
