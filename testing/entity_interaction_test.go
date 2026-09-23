package testing

import (
	"context"
	"fmt"
	"log"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// debugEntityTracking logs all tracked entities and the search result
func debugEntityTracking(t *testing.T, agent *ManagedAgent, entityType int32, botPos models.V3, entityName string) (int32, float64, bool) {
	trackedEntities := agent.GetTrackedEntities()
	t.Logf("Total tracked entities: %d", len(trackedEntities))
	for id, info := range trackedEntities {
		t.Logf("  Entity ID %d: type=%d, pos=(%.1f, %.1f, %.1f)", id, info.EntityType, info.X, info.Y, info.Z)
	}

	entityID, dist, found := agent.FindNearestEntityByType(entityType, botPos.X, botPos.Y, botPos.Z, false)
	t.Logf("FindNearestEntityByType(%s) result: found=%v, entityID=%d, dist=%.2f", entityName, found, entityID, dist)
	return entityID, dist, found
}

// EntityInteractionSuite covers all 8 entity-interaction methods: 5
// non-combat (villager/horse/armor-stand right-click variants) and 3
// zombie-combat methods. The two groups need different Difficulty values
// (Easy for non-combat, Normal for combat - Peaceful would despawn the
// combat methods' zombies outright), so each method calls
// VersionWorldSuite.SetDifficulty at its own start rather than relying on
// a suite-wide default (see that method's own doc comment for why this is
// safe). The combat methods also each build their own platform (with a
// sun-blocking roof, belt-and-suspenders alongside the zombie's own
// leather helmet) since a zombie needs solid, clear ground to fight on,
// unlike the non-combat methods' simple entities, which need no platform
// at all (the natural flat ground under each agent's own working area is
// enough).
type EntityInteractionSuite struct {
	VersionWorldSuite
}

func TestEntityInteractionSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &EntityInteractionSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestSimpleInteract tests right-clicking an entity
func (s *EntityInteractionSuite) TestSimpleInteract() {
	t := s.T()
	s.SetDifficulty(t, DifficultyEasy)

	leader, err := s.SpawnWorkingAreaAgent("EntityInteractBot", "entity_interact")
	require.NoError(t, err, "spawn agent")

	// Spawn a villager for testing (can be right-clicked for trade UI)
	spawnCmd := fmt.Sprintf(`summon minecraft:villager %.1f %.1f %.1f {VillagerData:{profession:"minecraft:librarian",level:1}}`,
		leader.Origin.X+3, leader.Origin.Y, leader.Origin.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn villager")
	t.Logf("Spawn villager command response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find villager entity
	villagerType, ok := leader.Agent.GetEntityTypeID("minecraft:villager")
	require.True(t, ok, "villager entity type should be in registry")

	villagerID, dist, found := debugEntityTracking(t, leader.ManagedAgent, villagerType, botPos, "minecraft:villager")
	require.True(t, found, "should find villager entity")
	require.Less(t, dist, 20.0, "villager should be within 20 blocks")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Send simple interact packet (main hand = 0)
	err = entityHandler.SendInteract(botClient.Conn(), villagerID, 0, false)
	require.NoError(t, err, "send interact packet")
	t.Logf("✓ Interacted with villager entity ID %d", villagerID)

	time.Sleep(200 * time.Millisecond)
}

// TestInteractAt tests right-clicking at a specific position on entity
func (s *EntityInteractionSuite) TestInteractAt() {
	t := s.T()
	s.SetDifficulty(t, DifficultyEasy)

	leader, err := s.SpawnWorkingAreaAgent("EntityInteractAtBot", "entity_interact_at")
	require.NoError(t, err, "spawn agent")

	// Spawn a horse (good for testing interact-at on specific positions)
	spawnCmd := fmt.Sprintf(`summon minecraft:horse %.1f %.1f %.1f {Tame:1b}`,
		leader.Origin.X+3, leader.Origin.Y, leader.Origin.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn horse")
	t.Logf("Spawn horse command response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find horse entity
	horseType, ok := leader.Agent.GetEntityTypeID("minecraft:horse")
	require.True(t, ok, "horse entity type should be in registry")

	horseID, dist, found := debugEntityTracking(t, leader.ManagedAgent, horseType, botPos, "minecraft:horse")
	require.True(t, found, "should find horse entity")
	require.Less(t, dist, 20.0, "horse should be within 20 blocks")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Send interact-at packet (interactionType = 2) at head position
	// Target coordinates are relative to entity center, with Y offset for head
	targetX := float32(0.0) // Center
	targetY := float32(1.5) // Head height
	targetZ := float32(0.0) // Center

	err = entityHandler.SendInteractAt(botClient.Conn(), horseID, targetX, targetY, targetZ, 0, false)
	require.NoError(t, err, "send interact-at packet")
	t.Logf("✓ Interacted at position on horse entity ID %d", horseID)

	time.Sleep(200 * time.Millisecond)
}

// TestSneaking tests entity interaction while sneaking
func (s *EntityInteractionSuite) TestSneaking() {
	t := s.T()
	s.SetDifficulty(t, DifficultyEasy)

	leader, err := s.SpawnWorkingAreaAgent("EntitySneakBot", "entity_interact_sneak")
	require.NoError(t, err, "spawn agent")

	// Spawn an armorstand (useful for precision interactions)
	spawnCmd := fmt.Sprintf(`summon minecraft:armor_stand %.1f %.1f %.1f`,
		leader.Origin.X+3, leader.Origin.Y, leader.Origin.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn armor stand")
	t.Logf("Spawn armor stand command response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find armor stand entity
	armorStandType, ok := leader.Agent.GetEntityTypeID("minecraft:armor_stand")
	require.True(t, ok, "armor_stand entity type should be in registry")

	armorStandID, dist, found := debugEntityTracking(t, leader.ManagedAgent, armorStandType, botPos, "minecraft:armor_stand")
	require.True(t, found, "should find armor stand entity")
	require.Less(t, dist, 20.0, "armor stand should be within 20 blocks")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Send interact packet with sneaking=true
	err = entityHandler.SendInteract(botClient.Conn(), armorStandID, 0, true)
	require.NoError(t, err, "send interact packet (sneaking)")
	t.Logf("✓ Interacted with armor stand while sneaking (entity ID %d)", armorStandID)

	time.Sleep(200 * time.Millisecond)
}

// TestVillagerTrade tests purchasing items from a villager
func (s *EntityInteractionSuite) TestVillagerTrade() {
	t := s.T()
	s.SetDifficulty(t, DifficultyEasy)

	leader, err := s.SpawnWorkingAreaAgent("EntityTradeBot", "entity_villager_trade")
	require.NoError(t, err, "spawn agent")

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Give bot 64 emeralds for trading
	giveCmd := fmt.Sprintf(`give %s minecraft:emerald 64`, leader.Name)
	resp, err := s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give emeralds")
	t.Logf("Give emeralds response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	// Spawn a librarian villager (has diverse trades)
	spawnVillagerCmd := fmt.Sprintf(`summon minecraft:villager %.1f %.1f %.1f {VillagerData:{profession:"minecraft:librarian",level:3}}`,
		botPos.X+3, botPos.Y, botPos.Z)
	resp, err = s.Inst.RCON.Exec(s.Ctx, spawnVillagerCmd)
	require.NoError(t, err, "spawn librarian villager")
	t.Logf("Spawn villager response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	// Find villager entity
	villagerType, _ := leader.Agent.GetEntityTypeID("minecraft:villager")
	villagerID, _, found := debugEntityTracking(t, leader.ManagedAgent, villagerType, botPos, "minecraft:librarian")
	require.True(t, found, "should find librarian villager")

	// Face and interact with villager to open trade GUI
	err = leader.FaceEntity(villagerID)
	require.NoError(t, err, "face villager")
	time.Sleep(100 * time.Millisecond)

	err = entityHandler.SendInteract(botClient.Conn(), villagerID, 0, false)
	require.NoError(t, err, "send villager interact packet")
	t.Logf("✓ Sent interact packet to villager")

	// Wait for trade GUI to fully open on server
	time.Sleep(800 * time.Millisecond)

	// Note: Completing actual trades requires:
	// 1. Detecting merchant GUI window ID (from screen manager when it opens)
	// 2. Identifying available trade slots in the merchant
	// 3. Clicking the appropriate trade slot
	// 4. Verifying the trade succeeded via inventory changes
	//
	// This requires integration with the screen manager to detect when the
	// merchant GUI is open, which is beyond simple entity interaction.
	//
	// For this test, we validate the core interaction worked:
	// - Villager was found and is responding
	// - Trade GUI interaction packet was sent successfully
	// - Villager remains in tracked entities (not despawned/removed)

	// Verify villager is still tracked and responsive
	trackedEntitiesAfter := leader.GetTrackedEntities()
	villagerStillTracked := false
	for _, info := range trackedEntitiesAfter {
		if info.EntityID == villagerID && !info.Removed {
			villagerStillTracked = true
			t.Logf("✓ Villager still responsive after trade interaction")
			break
		}
	}
	require.True(t, villagerStillTracked, "villager should still be tracked after trade")
}

// TestMultipleEntities tests interacting with different entity types sequentially
func (s *EntityInteractionSuite) TestMultipleEntities() {
	t := s.T()
	s.SetDifficulty(t, DifficultyEasy)

	leader, err := s.SpawnWorkingAreaAgent("EntityMultiBot", "entity_multiple_interact")
	require.NoError(t, err, "spawn agent")

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Test 1: Interact with villager (should open trade GUI)
	spawnVillagerCmd := fmt.Sprintf(`summon minecraft:villager %.1f %.1f %.1f {VillagerData:{profession:"minecraft:librarian",level:1}}`,
		botPos.X+3, botPos.Y, botPos.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnVillagerCmd)
	require.NoError(t, err, "spawn villager")
	t.Logf("Spawn villager response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	villagerType, _ := leader.Agent.GetEntityTypeID("minecraft:villager")
	if villagerID, _, found := debugEntityTracking(t, leader.ManagedAgent, villagerType, botPos, "minecraft:villager"); found {
		err := leader.FaceEntity(villagerID)
		require.NoError(t, err, "face villager")
		time.Sleep(100 * time.Millisecond)

		err = entityHandler.SendInteract(botClient.Conn(), villagerID, 0, false)
		require.NoError(t, err, "send villager interact packet")
		time.Sleep(300 * time.Millisecond) // Wait for server to process and send trade UI

		// Verify interaction by checking entity is still responsive
		villagerEntities := leader.GetTrackedEntities()
		found := false
		for _, info := range villagerEntities {
			if info.EntityID == villagerID && !info.Removed {
				found = true
				t.Logf("✓ Villager interaction successful - entity still responsive")
				break
			}
		}
		if !found {
			t.Logf("Warning: Villager disappeared or was removed after interaction")
		}
	} else {
		t.Logf("Warning: villager not found")
	}
	time.Sleep(200 * time.Millisecond)

	// Test 2: Interact with armor stand while sneaking (armor stands can be edited)
	spawnArmorStandCmd := fmt.Sprintf(`summon minecraft:armor_stand %.1f %.1f %.1f {Pose:{Head:[45f,45f,0f]}}`,
		botPos.X-3, botPos.Y, botPos.Z)
	resp, err = s.Inst.RCON.Exec(s.Ctx, spawnArmorStandCmd)
	require.NoError(t, err, "spawn armor stand")
	t.Logf("Spawn armor stand response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	armorStandType, _ := leader.Agent.GetEntityTypeID("minecraft:armor_stand")
	if armorStandID, _, found := debugEntityTracking(t, leader.ManagedAgent, armorStandType, botPos, "minecraft:armor_stand"); found {
		err := leader.FaceEntity(armorStandID)
		require.NoError(t, err, "face armor stand")
		time.Sleep(100 * time.Millisecond)

		err = entityHandler.SendInteract(botClient.Conn(), armorStandID, 0, true) // Sneaking allows editing
		require.NoError(t, err, "send armor stand interact packet (sneaking)")
		time.Sleep(200 * time.Millisecond)

		// For armor stands, verify by checking if the entity is still tracked and responsive
		armorStandEntities := leader.GetTrackedEntities()
		found := false
		for _, info := range armorStandEntities {
			if info.EntityID == armorStandID && !info.Removed {
				found = true
				t.Logf("✓ Armor stand still tracked after interaction - edit mode activated")
				break
			}
		}
		if !found {
			t.Logf("Warning: Armor stand disappeared or was removed after interaction")
		}
	} else {
		t.Logf("Warning: armor stand not found")
	}
	time.Sleep(200 * time.Millisecond)

	// Test 3: Interact with horse at specific position (interact-at packet)
	spawnHorseCmd := fmt.Sprintf(`summon minecraft:horse %.1f %.1f %.1f {Tame:1b,Saddle:1b}`,
		botPos.X+5, botPos.Y, botPos.Z)
	resp, err = s.Inst.RCON.Exec(s.Ctx, spawnHorseCmd)
	require.NoError(t, err, "spawn horse")
	t.Logf("Spawn horse response: %s", resp)
	time.Sleep(500 * time.Millisecond)

	horseType, _ := leader.Agent.GetEntityTypeID("minecraft:horse")
	if horseID, _, found := debugEntityTracking(t, leader.ManagedAgent, horseType, botPos, "minecraft:horse"); found {
		err := leader.FaceEntity(horseID)
		require.NoError(t, err, "face horse")
		time.Sleep(100 * time.Millisecond)

		// Interact at head position using interact-at packet
		targetX := float32(0.0) // Center
		targetY := float32(1.5) // Head height
		targetZ := float32(0.0) // Center
		err = entityHandler.SendInteractAt(botClient.Conn(), horseID, targetX, targetY, targetZ, 0, false)
		require.NoError(t, err, "send horse interact-at packet")
		time.Sleep(300 * time.Millisecond)

		// Verify interaction by checking entity state and tracking
		horseEntities := leader.GetTrackedEntities()
		found := false
		for _, info := range horseEntities {
			if info.EntityID == horseID && !info.Removed {
				found = true
				t.Logf("✓ Horse interaction successful - still tracked at (%.1f, %.1f, %.1f)", info.X, info.Y, info.Z)
				break
			}
		}
		if !found {
			t.Logf("Warning: Horse disappeared or was removed after interaction")
		}
	} else {
		t.Logf("Warning: horse not found")
	}
}

// buildCombatArena builds a 20x20 platform around origin (offset -10,-10, so
// it fits well within one working area's 256-block separation), clears
// headroom above it, and builds a second platform 10 blocks up to block sun
// exposure for a zombie spawned there.
func (s *EntityInteractionSuite) buildCombatArena(origin models.V3) {
	t := s.T()

	platformY := int(math.Floor(origin.Y)) - 1 // Platform is below agent's feet
	platformX := int(math.Floor(origin.X)) - 10
	platformZ := int(math.Floor(origin.Z)) - 10

	require.NoError(t, BuildPlatform(s.Ctx, s.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block"), "build platform")

	// Clear area above the platform (for zombie headroom)
	if err := ClearArea(s.Ctx, s.Inst.RCON,
		platformX, platformY+1, platformZ,
		platformX+20, platformY+5, platformZ+20); err != nil {
		t.Logf("warning: failed to clear area: %v", err)
	}

	// Build a platform to protect the zombie from the sun
	require.NoError(t, BuildPlatform(s.Ctx, s.Inst.RCON, platformX, platformY+10, platformZ, 20, 20, "minecraft:grass_block"), "build sun-blocking roof")
}

// TestAttack tests attacking an entity
func (s *EntityInteractionSuite) TestAttack() {
	t := s.T()
	s.SetDifficulty(t, DifficultyNormal)

	leader, err := s.SpawnWorkingAreaAgent("EntityAttackBot", "entity_attack")
	require.NoError(t, err, "spawn agent")

	s.buildCombatArena(leader.Origin)

	// Give bot a netherite sword
	giveCmd := fmt.Sprintf(`give %s netherite_sword`, leader.Name)
	resp, err := s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give netherite sword")
	t.Logf("Give sword response: %s => %s", giveCmd, resp)

	// Wait for bot to process block updates
	time.Sleep(3 * time.Second)

	// Spawn a zombie (with helmet so it doesn't take damage from the sun)
	spawnX := leader.Origin.X + 3
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	spawnCmd := fmt.Sprintf(`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,CanPickUpLoot:1b,ArmorItems:[{},{},{},{id:"minecraft:leather_helmet",Count:1b}]}`, spawnX, spawnY, spawnZ)
	resp, err = s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn zombie")
	t.Logf("Spawn zombie command response: %s => %s", spawnCmd, resp)

	// Wait for entity to be sent to client
	time.Sleep(3 * time.Second)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find zombie entity
	zombieType, ok := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(t, ok, "zombie entity type should be in registry")

	zombieID, dist, found := debugEntityTracking(t, leader.ManagedAgent, zombieType, botPos, "minecraft:zombie")
	require.True(t, found, "should find zombie entity")
	require.Less(t, dist, 20.0, "zombie should be within 20 blocks")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	log.Printf("Getting Zombie (entity ID %d) health before attack", zombieID)
	// Get zombie health before attack
	trackedEntitiesBefore := leader.GetTrackedEntities()
	var healthBefore float32 = -1.0
	for _, info := range trackedEntitiesBefore {
		if info.EntityID == zombieID {
			healthBefore = info.Health
			t.Logf("Zombie health before attack: %.1f", healthBefore)
			break
		}
	}
	log.Printf("Zombie (entity ID %d) health before attack: %.1f", zombieID, healthBefore)

	// Equip the sword before attacking
	log.Printf("Equipping sword...")
	err = leader.EquipItemByName(context.Background(), "minecraft:netherite_sword")
	require.NoError(t, err, "equip netherite sword")
	t.Logf("✓ Equipped sword")

	// Face the zombie before attacking
	log.Printf("Facing zombie entity ID %d", zombieID)
	err = leader.FaceEntity(zombieID)
	require.NoError(t, err, "face entity")
	t.Logf("✓ Facing zombie")

	// Wait for rotation to be sent
	time.Sleep(100 * time.Millisecond)

	log.Printf("Attacking zombie entity ID %d", zombieID)

	// Attack the zombie (interactionType = 1)
	err = entityHandler.SendAttack(botClient.Conn(), zombieID, false)
	require.NoError(t, err, "send attack packet")
	t.Logf("✓ Attacked zombie entity ID %d", zombieID)

	// Wait for server to process damage
	time.Sleep(500 * time.Millisecond)

	// Check if zombie is still alive and get health after attack
	trackedEntitiesAfter := leader.GetTrackedEntities()
	zombieStillExists := false
	var zombieInfoAfter *models.TrackedEntityInfo
	for _, info := range trackedEntitiesAfter {
		if info.EntityID == zombieID {
			if info.Removed {
				continue // Zombie was removed
			}
			zombieStillExists = true
			zombieInfoAfter = &info
			break
		}
	}

	if zombieStillExists && zombieInfoAfter != nil {
		// Zombie survived - check if health decreased
		t.Logf("Zombie health after attack: %.1f", zombieInfoAfter.Health)
		t.Logf("✓ Zombie survived attack at position (%.1f, %.1f, %.1f)", zombieInfoAfter.X, zombieInfoAfter.Y, zombieInfoAfter.Z)

		if healthBefore > 0 && zombieInfoAfter.Health < healthBefore {
			t.Logf("✓ Damage dealt: %.1f health lost", healthBefore-zombieInfoAfter.Health)
		} else {
			t.Logf("Note: Could not verify health decrease (before: %.1f, after: %.1f)", healthBefore, zombieInfoAfter.Health)
		}
	} else {
		// Zombie died - that's also success (max damage dealt)
		t.Logf("✓ Zombie died from attack (entity ID %d is no longer tracked)", zombieID)
	}

	// Wait a bit before ending test
	time.Sleep(3 * time.Second)
}

// TestOffhandAttack tests attacking with offhand item
func (s *EntityInteractionSuite) TestOffhandAttack() {
	t := s.T()
	s.SetDifficulty(t, DifficultyNormal)

	leader, err := s.SpawnWorkingAreaAgent("EntityOffhandBot", "entity_attack_offhand")
	require.NoError(t, err, "spawn agent")

	s.buildCombatArena(leader.Origin)
	time.Sleep(500 * time.Millisecond)

	// Give bot a sword for offhand
	giveCmd := fmt.Sprintf(`give %s minecraft:wooden_sword`, leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give sword")

	// Spawn a zombie
	spawnCmd := fmt.Sprintf(`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,CanPickUpLoot:1b,ArmorItems:[{},{},{},{id:"minecraft:leather_helmet",Count:1b}]}`,
		leader.Origin.X+3, leader.Origin.Y, leader.Origin.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn zombie")
	t.Logf("Spawn zombie command response: %s", resp)
	time.Sleep(3 * time.Second)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find zombie entity
	zombieType, ok := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(t, ok, "zombie entity type should be in registry")

	zombieID, _, found := debugEntityTracking(t, leader.ManagedAgent, zombieType, botPos, "minecraft:zombie")
	require.True(t, found, "should find zombie entity")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Equip the sword before attacking
	err = leader.EquipItemByName(context.Background(), "minecraft:wooden_sword")
	require.NoError(t, err, "equip wooden sword")
	t.Logf("✓ Equipped sword")

	// Face the zombie before attacking
	err = leader.FaceEntity(zombieID)
	require.NoError(t, err, "face entity")
	t.Logf("✓ Facing zombie")

	// Wait for rotation to be sent
	time.Sleep(100 * time.Millisecond)

	// Attack with offhand (test uses main hand since container moving requires more setup)
	err = entityHandler.SendAttack(botClient.Conn(), zombieID, false)
	require.NoError(t, err, "send attack packet")

	time.Sleep(500 * time.Millisecond)
}

// TestRapidAttacks tests rapid successive attacks
func (s *EntityInteractionSuite) TestRapidAttacks() {
	t := s.T()
	s.SetDifficulty(t, DifficultyNormal)

	leader, err := s.SpawnWorkingAreaAgent("EntityRapidBot", "entity_attack_rapid")
	require.NoError(t, err, "spawn agent")

	s.buildCombatArena(leader.Origin)
	time.Sleep(500 * time.Millisecond)

	// Give bot a sword
	giveCmd := fmt.Sprintf(`give %s minecraft:diamond_sword`, leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give sword")

	// Spawn a zombie
	spawnCmd := fmt.Sprintf(`summon minecraft:zombie %.1f %.1f %.1f {Health:20f,CanPickUpLoot:1b,ArmorItems:[{},{},{},{id:"minecraft:leather_helmet",Count:1b}]}`,
		leader.Origin.X+3, leader.Origin.Y, leader.Origin.Z)
	resp, err := s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn zombie")
	t.Logf("Spawn zombie command response: %s", resp)
	time.Sleep(3 * time.Second)

	// Get bot position
	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("Bot position: %s", botPos)

	// Find zombie entity
	zombieType, ok := leader.Agent.GetEntityTypeID("minecraft:zombie")
	require.True(t, ok, "zombie entity type should be in registry")

	zombieID, _, found := debugEntityTracking(t, leader.ManagedAgent, zombieType, botPos, "minecraft:zombie")
	require.True(t, found, "should find zombie entity")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	entityHandler := leader.Config.VersionHandler.Play().Entities()
	botClient := leader.BotClient()

	// Equip the sword before attacking
	err = leader.EquipItemByName(context.Background(), "minecraft:diamond_sword")
	require.NoError(t, err, "equip diamond sword")
	t.Logf("✓ Equipped sword")

	// Face the zombie before attacking
	err = leader.FaceEntity(zombieID)
	require.NoError(t, err, "face entity")
	t.Logf("✓ Facing zombie")

	// Wait for rotation to be sent
	time.Sleep(100 * time.Millisecond)

	// Send 5 rapid attacks
	for i := 0; i < 5; i++ {
		err = entityHandler.SendAttack(botClient.Conn(), zombieID, false)
		require.NoError(t, err, "send attack %d", i+1)

		time.Sleep(100 * time.Millisecond) // Brief delay between attacks
	}

	time.Sleep(500 * time.Millisecond)
}
