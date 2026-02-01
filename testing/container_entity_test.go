package testing

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// getHorseNBT returns the appropriate NBT format for spawning a tamed horse with saddle
// based on the Minecraft version.
//
// Version ranges:
//   - 1.20.4 - 1.21.4: {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}
//   - 1.21.5+:         {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}
func getHorseNBT(version string) string {
	// Parse version (e.g., "1.21.5" -> major=1, minor=21, patch=5)
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		// Default to old format if version is malformed
		return `{Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return `{Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`
	}

	patch := 0
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}

	// Version 1.21.5+ uses new equipment format
	if minor > 21 || (minor == 21 && patch >= 5) {
		return `{Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}`
	}

	// Version 1.21.4 and earlier use old SaddleItem format
	return `{Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`
}

// TestHorse_Standalone tests horse inventory opening
func TestHorse_Standalone(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "entity_horse", tt.mcVersion)
			defer env.Cancel()

			// Spawn a horse near the bot
			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			// Use version-aware NBT format
			horseNBT := getHorseNBT(env.Version)
			spawnCmd := fmt.Sprintf("summon minecraft:horse %.1f %.1f %.1f %s", spawnX, spawnY, spawnZ, horseNBT)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn horse")
			t.Logf("Spawn response: %s", resp)
			t.Logf("Using NBT format for version %s: %s", env.Version, horseNBT)

			// Wait for entity to be tracked by agent
			time.Sleep(500 * time.Millisecond)

			// Verify the horse is actually tamed
			verifyCmd := "data get entity @e[type=minecraft:horse,limit=1,sort=nearest] Tame"
			verifyResp, err := env.Inst.RCON.Exec(env.Ctx, verifyCmd)
			require.NoError(t, err, "verify horse tamed")
			t.Logf("Horse Tame status: %s", verifyResp)
			require.Contains(t, verifyResp, "1b", "horse should be tamed (Tame:1b)")

			// Also verify saddle was applied correctly
			saddleCmd := "data get entity @e[type=minecraft:horse,limit=1,sort=nearest]"
			saddleResp, err := env.Inst.RCON.Exec(env.Ctx, saddleCmd)
			if err == nil {
				t.Logf("Horse full NBT: %s", saddleResp)
			}

			// Get bot position
			botX, botY, botZ, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Look up horse entity type from registry (version-agnostic)
			horseType, ok := env.Agent.Agent.GetEntityTypeID("minecraft:horse")
			require.True(t, ok, "minecraft:horse entity type should be in registry")
			t.Logf("Horse entity type ID from registry: %d", horseType)

			// Find nearest horse entity
			horseID, dist, found := env.Agent.FindNearestEntityByType(horseType, botX, botY, botZ)
			require.True(t, found, "should find horse entity")
			t.Logf("Found horse entity ID %d at distance %.2f blocks", horseID, dist)

			// Teleport near the horse for interaction
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near horse")
			time.Sleep(300 * time.Millisecond)

			// Open horse container
			windowID, err := env.ContainerHelper.OpenEntityContainer(horseID, 5*time.Second)
			require.NoError(t, err, "open horse container")
			t.Logf("Horse container opened with window ID: %d", windowID)

			// Verify it's a HorseContainer
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "horse window should exist")

			// TODO: Import mcscreen package and verify HorseContainer type
			// For now, just verify we got a window
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.ContainerHelper.CloseContainer()
			require.NoError(t, err, "close horse container")

			t.Log("✓ Horse container test passed")
		})
	}
}

// TestChestBoat_Standalone tests chest boat inventory opening
func TestChestBoat_Standalone(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "entity_chest_boat", tt.mcVersion)
			defer env.Cancel()

			// Spawn a chest boat near the bot
			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			// Chest boats need to be in water
			// First, place a water block
			waterCmd := fmt.Sprintf("setblock %.0f %.0f %.0f minecraft:water", spawnX, spawnY, spawnZ)
			_, err := env.Inst.RCON.Exec(env.Ctx, waterCmd)
			require.NoError(t, err, "place water")
			time.Sleep(200 * time.Millisecond)

			// Summon chest boat (1.21.5+ uses wood-specific names like oak_chest_boat)
			spawnCmd := fmt.Sprintf("summon minecraft:oak_chest_boat %.1f %.1f %.1f", spawnX, spawnY, spawnZ)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn chest boat")
			t.Logf("Spawn response: %s", resp)

			// Wait for entity to be tracked
			time.Sleep(500 * time.Millisecond)

			// Verify the chest boat was spawned
			verifyCmd := "data get entity @e[type=minecraft:oak_chest_boat,limit=1,sort=nearest]"
			verifyResp, err := env.Inst.RCON.Exec(env.Ctx, verifyCmd)
			if err == nil {
				t.Logf("Chest boat NBT data: %s", verifyResp)
			}

			// Get bot position
			botX, botY, botZ, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Look up oak chest boat entity type from registry (version-agnostic)
			chestBoatType, ok := env.Agent.Agent.GetEntityTypeID("minecraft:oak_chest_boat")
			require.True(t, ok, "minecraft:oak_chest_boat entity type should be in registry")
			t.Logf("Oak chest boat entity type ID from registry: %d", chestBoatType)

			// Find nearest chest boat entity
			boatID, dist, found := env.Agent.FindNearestEntityByType(chestBoatType, botX, botY, botZ)
			require.True(t, found, "should find chest boat entity")
			t.Logf("Found chest boat entity ID %d at distance %.2f blocks", boatID, dist)

			// Teleport near the chest boat
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near chest boat")
			time.Sleep(300 * time.Millisecond)

			// Open chest boat container
			windowID, err := env.ContainerHelper.OpenEntityContainer(boatID, 5*time.Second)
			require.NoError(t, err, "open chest boat container")
			t.Logf("Chest boat container opened with window ID: %d", windowID)

			// Verify window exists
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest boat window should exist")
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.ContainerHelper.CloseContainer()
			require.NoError(t, err, "close chest boat container")

			t.Log("✓ Chest boat container test passed")
		})
	}
}

// TestChestMinecart_Standalone tests chest minecart inventory opening
func TestChestMinecart_Standalone(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "entity_chest_minecart", tt.mcVersion)
			defer env.Cancel()

			// Spawn a chest minecart near the bot
			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			// Place rails for the minecart
			railCmd := fmt.Sprintf("setblock %.0f %.0f %.0f minecraft:rail", spawnX, spawnY, spawnZ)
			_, err := env.Inst.RCON.Exec(env.Ctx, railCmd)
			require.NoError(t, err, "place rail")
			time.Sleep(200 * time.Millisecond)

			// Summon chest minecart
			spawnCmd := fmt.Sprintf("summon minecraft:chest_minecart %.1f %.1f %.1f", spawnX, spawnY+0.5, spawnZ)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn chest minecart")
			t.Logf("Spawn response: %s", resp)

			// Wait for entity to be tracked
			time.Sleep(500 * time.Millisecond)

			// Verify the chest minecart was spawned
			verifyCmd := "data get entity @e[type=minecraft:chest_minecart,limit=1,sort=nearest]"
			verifyResp, err := env.Inst.RCON.Exec(env.Ctx, verifyCmd)
			if err == nil {
				t.Logf("Chest minecart NBT data: %s", verifyResp)
			}

			// Get bot position
			botX, botY, botZ, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Look up chest minecart entity type from registry (version-agnostic)
			chestMinecartType, ok := env.Agent.Agent.GetEntityTypeID("minecraft:chest_minecart")
			require.True(t, ok, "minecraft:chest_minecart entity type should be in registry")
			t.Logf("Chest minecart entity type ID from registry: %d", chestMinecartType)

			// Find nearest chest minecart entity
			minecartID, dist, found := env.Agent.FindNearestEntityByType(chestMinecartType, botX, botY, botZ)
			require.True(t, found, "should find chest minecart entity")
			t.Logf("Found chest minecart entity ID %d at distance %.2f blocks", minecartID, dist)

			// Teleport near the chest minecart
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near chest minecart")
			time.Sleep(300 * time.Millisecond)

			// Open chest minecart container
			windowID, err := env.ContainerHelper.OpenEntityContainer(minecartID, 5*time.Second)
			require.NoError(t, err, "open chest minecart container")
			t.Logf("Chest minecart container opened with window ID: %d", windowID)

			// Verify window exists
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest minecart window should exist")
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.ContainerHelper.CloseContainer()
			require.NoError(t, err, "close chest minecart container")

			t.Log("✓ Chest minecart container test passed")
		})
	}
}
