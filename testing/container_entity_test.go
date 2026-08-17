package testing

import (
	"fmt"
	"testing"
	"time"

	semver "github.com/aquasecurity/go-version/pkg/version"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// Minecraft is expected to move from the "1.MINOR.PATCH" versioning scheme
// (through 1.21.11) to a "<year>.<version>.<patch>" scheme afterward (e.g.
// 26.1.0, 27.2.3). All version comparisons below use semver.Parse/Constraints
// (the same library agent/adapters.go::saddleSlotSupported already uses) rather
// than hand-parsing "the second dot-separated component", specifically because
// hand-parsing breaks silently on that scheme change: splitting "26.1.0" on "."
// and reading index 1 gives "1", which a naive `minor >= 21` check reads as an
// *ancient* pre-1.21 version rather than the newest one there is. Real semver
// comparison orders by major first, so "26.1.0" correctly sorts above any
// "1.x.y" threshold without needing to special-case the scheme switch at all —
// verified for 26.1.0 and 27.2.3 against every threshold used in this file.
//
// None of this repo's dependencies support the year-based scheme yet, so nothing
// here has been exercised against a real such server; it only needs to not be
// silently wrong once one becomes available.

// getHorseNBT returns the appropriate NBT format for spawning a tamed horse with saddle
// based on the Minecraft version.
//
// Version ranges:
//   - 1.20.4 - 1.21.4: {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}
//   - 1.21.5+:         {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}
func getHorseNBT(version string) string {
	const oldFormat = `{Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`
	const newFormat = `{Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}`

	if !mcVersionAtLeastSemver(version, ">= "+models.MinSaddleSlotVersion) {
		return oldFormat
	}
	return newFormat
}

// chestBoatEntityID returns the entity type (without the "minecraft:" prefix)
// used to summon and look up an oak chest boat, and any extra NBT needed to
// spawn one, based on the Minecraft version.
//
// Version ranges:
//   - 1.21.1 and earlier: a single "chest_boat" entity type, wood species set
//     via the legacy Type NBT tag (e.g. {Type:"oak"}).
//   - 1.21.2+: per-species entity types (oak_chest_boat, birch_chest_boat, ...)
//     with no Type tag needed.
func chestBoatEntityID(version string) (entityType string, spawnNBT string) {
	if !mcVersionAtLeastSemver(version, ">= 1.21.2") {
		return "chest_boat", `{Type:"oak"}`
	}
	return "oak_chest_boat", ""
}

// mcVersionAtLeastSemver reports whether version satisfies constraint (e.g.
// ">= 1.21.5"), using real semver comparison so the ordering stays correct
// across Minecraft's pending 1.MINOR.PATCH -> year.version.patch scheme change
// (see the package-level comment above). An unparseable version or constraint
// reports false, so callers that gate a feature behind this skip rather than
// risk running against a version that doesn't have it.
func mcVersionAtLeastSemver(version, constraint string) bool {
	parsed, err := semver.Parse(version)
	if err != nil {
		return false
	}
	c, err := semver.NewConstraints(constraint)
	if err != nil {
		return false
	}
	return c.Check(parsed)
}

// mcVersionAtLeast reports whether version is at or above 1.<minMinor>.<minPatch>.
// An unparseable version reports false, so callers that gate a feature behind
// this skip rather than risk running against a version that doesn't have it.
func mcVersionAtLeast(version string, minMinor, minPatch int) bool {
	return mcVersionAtLeastSemver(version, fmt.Sprintf(">= 1.%d.%d", minMinor, minPatch))
}

// getChestedMountNBT returns the NBT for a tamed, saddled, chest-equipped
// donkey or mule. ChestedHorse:1b adds the 15-slot chest inventory that
// distinguishes donkeys/mules from plain horses, which can never carry one.
//
// Version ranges mirror getHorseNBT (see docs/horse-nbt-data.md); donkeys and
// mules share AbstractHorseEntity's saddle handling with horses, so the same
// 1.21.5 split applies:
//   - 1.21.1 - 1.21.4: {Tame:1b,SaddleItem:{...},ChestedHorse:1b}
//   - 1.21.5+:         {Tame:1b,equipment:{saddle:{...}},ChestedHorse:1b}
//
// Unlike getHorseNBT, an unparseable version fails open to the modern format
// here, matching the original behaviour this preserves.
func getChestedMountNBT(version string) string {
	const oldFormat = `{Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1},ChestedHorse:1b}`
	const newFormat = `{Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}},ChestedHorse:1b}`

	parsed, err := semver.Parse(version)
	if err != nil {
		return newFormat
	}
	constraint, err := semver.NewConstraints(">= " + models.MinSaddleSlotVersion)
	if err != nil || constraint.Check(parsed) {
		return newFormat
	}
	return oldFormat
}

// llamaChestNBT is the NBT for a tamed llama with maximum chest capacity
// (Strength:5 -> 15 slots) plus a carpet decoration.
//
// Llamas take no saddle in any version — see SummonLlama in
// testing/vehicles/common_test.go — so unlike the other mounts here there is
// no version split: the same NBT applies across all tested versions.
const llamaChestNBT = `{Tame:1b,ChestedHorse:1b,Strength:5,DecorItem:{id:"minecraft:white_carpet",count:1}}`

// TestVersionGatingSurvivesSchemeChange pins the exact regression the semver
// switch above exists to prevent: a hand-rolled "split on '.', read index 1"
// parser reads "26.1.0" as minor=1 — an ancient pre-1.21 version — because it
// silently discards index 0. That would make every gate in this file (saddle
// NBT format, chest boat entity naming, and any minMinor/minPatch skip in
// testSaddledMountInventoryCache) misclassify the *newest* version as the
// *oldest*. No live server involved; this is pure function logic.
func TestVersionGatingSurvivesSchemeChange(t *testing.T) {
	const futureYearScheme = "26.1.0" // hypothetical post-1.21.11 version

	t.Run("mcVersionAtLeast treats a year-scheme version as newer than any 1.x threshold", func(t *testing.T) {
		require.True(t, mcVersionAtLeast(futureYearScheme, 21, 11),
			"%s should satisfy >= 1.21.11", futureYearScheme)
		require.True(t, mcVersionAtLeast(futureYearScheme, 21, 5),
			"%s should satisfy >= 1.21.5", futureYearScheme)
	})

	t.Run("mcVersionAtLeast still orders old-scheme versions correctly", func(t *testing.T) {
		require.True(t, mcVersionAtLeast("1.21.11", 21, 11))
		require.False(t, mcVersionAtLeast("1.21.10", 21, 11))
		require.False(t, mcVersionAtLeast("1.21.4", 21, 5))
		require.True(t, mcVersionAtLeast("1.21.5", 21, 5))
	})

	t.Run("getHorseNBT uses the modern equipment format on a year-scheme version", func(t *testing.T) {
		require.Contains(t, getHorseNBT(futureYearScheme), "equipment:{saddle:")
		require.Contains(t, getHorseNBT("1.21.4"), "SaddleItem:")
	})

	t.Run("chestBoatEntityID uses per-species naming on a year-scheme version", func(t *testing.T) {
		entityType, spawnNBT := chestBoatEntityID(futureYearScheme)
		require.Equal(t, "oak_chest_boat", entityType)
		require.Empty(t, spawnNBT)

		entityType, spawnNBT = chestBoatEntityID("1.21.1")
		require.Equal(t, "chest_boat", entityType)
		require.Contains(t, spawnNBT, `Type:"oak"`)
	})

	t.Run("getChestedMountNBT uses the modern equipment format on a year-scheme version", func(t *testing.T) {
		require.Contains(t, getChestedMountNBT(futureYearScheme), "equipment:{saddle:")
		require.Contains(t, getChestedMountNBT("1.21.4"), "SaddleItem:")
	})
}

// TestHorseInventoryCache verifies that a horse's saddle/armour inventory
// snapshot is cached after its container is opened and closed.
func TestHorseInventoryCache(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntityWithReplay(t, "entity_horse", tt.MCVersion)
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
			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Look up horse entity type from registry (version-agnostic). Both
			// lookups poll: the registry arrives during configuration, and an RCON
			// summon returns before the spawn packet reaches us.
			horseType := waitForEntityTypeID(t, env, "minecraft:horse", 10*time.Second)
			horseID := waitForNearestEntityByType(t, env, horseType, botX, botY, botZ, 10*time.Second)

			// Teleport near the horse for interaction
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near horse")
			time.Sleep(300 * time.Millisecond)

			// Open horse container
			windowID, err := env.Agent.Agent.OpenEntityContainer(horseID, 10*time.Second)
			require.NoError(t, err, "open horse container")
			t.Logf("Horse container opened with window ID: %d", windowID)

			// Verify it's a HorseContainer
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "horse window should exist")

			// TODO: Import mcscreen package and verify HorseContainer type
			// For now, just verify we got a window
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close horse container")

			// A plain horse has no chest, so its window carries only the saddle and
			// armour slots. Nothing was seeded, so only the snapshot contract is
			// checked: the contents survived the close and are flagged not-live.
			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, horseID, 0, "")

			t.Log("✓ Horse container test passed")
		})
	}
}

// TestChestBoatInventoryCache verifies that a chest boat's storage snapshot
// is cached and survives after its container is opened and closed.
func TestChestBoatInventoryCache(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntityWithReplay(t, "entity_chest_boat", tt.MCVersion)
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

			// Summon chest boat (entity type changed from a single chest_boat
			// with a wood-species NBT tag to per-species entity types in 1.21.2)
			chestBoatEntity, chestBoatSpawnNBT := chestBoatEntityID(tt.MCVersion)
			spawnCmd := fmt.Sprintf("summon minecraft:%s %.1f %.1f %.1f", chestBoatEntity, spawnX, spawnY, spawnZ)
			if chestBoatSpawnNBT != "" {
				spawnCmd += " " + chestBoatSpawnNBT
			}
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn chest boat")
			t.Logf("Spawn response: %s", resp)

			// Wait for entity to be tracked
			time.Sleep(500 * time.Millisecond)

			// Verify the chest boat was spawned
			chestBoatSelector := fmt.Sprintf("@e[type=minecraft:%s,limit=1,sort=nearest]", chestBoatEntity)
			verifyCmd := "data get entity " + chestBoatSelector
			verifyResp, err := env.Inst.RCON.Exec(env.Ctx, verifyCmd)
			if err == nil {
				t.Logf("Chest boat NBT data: %s", verifyResp)
			}

			// Get bot position
			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Registry and spawn lookups both poll; see waitForEntityTypeID.
			chestBoatType := waitForEntityTypeID(t, env, "minecraft:"+chestBoatEntity, 10*time.Second)
			boatID := waitForNearestEntityByType(t, env, chestBoatType, botX, botY, botZ, 10*time.Second)

			// Teleport near the chest boat
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near chest boat")
			time.Sleep(300 * time.Millisecond)

			// Seed a known stack so the cached snapshot is checked against a value
			// we chose, rather than against an empty container that a completely
			// broken cache would also satisfy.
			const seededDiamonds = 5
			seedEntityContainerSlot(t, env, chestBoatSelector, 0, "minecraft:diamond", seededDiamonds)

			// Open chest boat container
			windowID, err := env.Agent.Agent.OpenEntityContainer(boatID, 10*time.Second)
			require.NoError(t, err, "open chest boat container")
			t.Logf("Chest boat container opened with window ID: %d", windowID)

			// Verify window exists
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest boat window should exist")
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close chest boat container")

			// The seeded stack must still be readable now the window is gone — that
			// is the whole point of caching it against the entity.
			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, boatID, seededDiamonds, "minecraft:diamond")

			t.Log("✓ Chest boat container test passed")
		})
	}
}

// TestChestMinecartInventoryCache verifies that a chest minecart's storage
// snapshot is cached and survives after its container is opened and closed.
func TestChestMinecartInventoryCache(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntityWithReplay(t, "entity_chest_minecart", tt.MCVersion)
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
			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Registry and spawn lookups both poll; see waitForEntityTypeID.
			chestMinecartType := waitForEntityTypeID(t, env, "minecraft:chest_minecart", 10*time.Second)
			minecartID := waitForNearestEntityByType(t, env, chestMinecartType, botX, botY, botZ, 10*time.Second)

			// Teleport near the chest minecart
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near chest minecart")
			time.Sleep(300 * time.Millisecond)

			// Seed a distinct count from the chest boat test so a snapshot leaking
			// between entities would be obvious rather than coincidentally passing.
			const seededEmeralds = 7
			seedEntityContainerSlot(t, env,
				"@e[type=minecraft:chest_minecart,limit=1,sort=nearest]",
				0, "minecraft:emerald", seededEmeralds)

			// Open chest minecart container
			windowID, err := env.Agent.Agent.OpenEntityContainer(minecartID, 10*time.Second)
			require.NoError(t, err, "open chest minecart container")
			t.Logf("Chest minecart container opened with window ID: %d", windowID)

			// Verify window exists
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest minecart window should exist")
			t.Logf("Screen type: %T", screen)

			// Close container
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close chest minecart container")

			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, minecartID, seededEmeralds, "minecraft:emerald")

			t.Log("✓ Chest minecart container test passed")
		})
	}
}

// testSaddledMountInventoryCache exercises the horse-style saddle-only
// inventory contract (open -> verify window -> close -> cached snapshot
// survives) for an AbstractHorseEntity subtype that takes a saddle but never
// a chest: camel, camel husk, skeleton horse, zombie horse.
//
// minMinor/minPatch gate the entity to versions at or above 1.<minMinor>.<minPatch>
// (e.g. camel husk needs 1.21.11+); pass 0, 0 for no gate.
//
// TestHorseInventoryCache stays as its own standalone function rather than
// routing through this helper, since it predates it and is the most-referenced
// example of the pattern.
func testSaddledMountInventoryCache(t *testing.T, entityType, testDirName string, minMinor, minPatch int) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if minMinor > 0 && !mcVersionAtLeast(tt.MCVersion, minMinor, minPatch) {
				t.Skipf("%s requires Minecraft 1.%d.%d+, got %s", entityType, minMinor, minPatch, tt.MCVersion)
			}

			env := setupStandaloneTestForEntityWithReplay(t, testDirName, tt.MCVersion)
			defer env.Cancel()

			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			mountNBT := getHorseNBT(env.Version)
			spawnCmd := fmt.Sprintf("summon minecraft:%s %.1f %.1f %.1f %s", entityType, spawnX, spawnY, spawnZ, mountNBT)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn "+entityType)
			t.Logf("Spawn response: %s", resp)

			time.Sleep(500 * time.Millisecond)

			selector := fmt.Sprintf("@e[type=minecraft:%s,limit=1,sort=nearest]", entityType)
			tameCmd := "data get entity " + selector + " Tame"
			tameResp, err := env.Inst.RCON.Exec(env.Ctx, tameCmd)
			require.NoError(t, err, "verify "+entityType+" tamed")
			t.Logf("%s Tame status: %s", entityType, tameResp)
			require.Contains(t, tameResp, "1b", entityType+" should be tamed (Tame:1b)")

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			// Registry and spawn lookups both poll; see waitForEntityTypeID.
			mountType := waitForEntityTypeID(t, env, "minecraft:"+entityType, 10*time.Second)
			mountID := waitForNearestEntityByType(t, env, mountType, botX, botY, botZ, 10*time.Second)

			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near "+entityType)
			time.Sleep(300 * time.Millisecond)

			windowID, err := env.Agent.Agent.OpenEntityContainer(mountID, 10*time.Second)
			require.NoError(t, err, "open "+entityType+" container")
			t.Logf("%s container opened with window ID: %d", entityType, windowID)

			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, entityType+" window should exist")
			t.Logf("Screen type: %T", screen)

			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, entityType+" container close")

			// No chest on these mobs, so the window carries only the saddle
			// slot. Nothing was seeded, so only the snapshot contract is
			// checked (see TestHorseInventoryCache for the seeded-content
			// variant, used on entities that carry a real chest).
			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, mountID, 0, "")

			t.Logf("✓ %s container test passed", entityType)
		})
	}
}

// TestCamelInventoryCache verifies that a camel's saddle inventory snapshot
// is cached after its container is opened and closed. Camels take a saddle
// but, unlike donkeys/mules/llamas, can never carry a chest.
func TestCamelInventoryCache(t *testing.T) {
	testSaddledMountInventoryCache(t, "camel", "entity_camel", 0, 0)
}

// TestCamelHuskInventoryCache verifies the same contract as
// TestCamelInventoryCache for camel husks, which were added in Minecraft
// 1.21.11 and are skipped on earlier versions.
func TestCamelHuskInventoryCache(t *testing.T) {
	testSaddledMountInventoryCache(t, "camel_husk", "entity_camel_husk", 21, 11)
}

// TestSkeletonHorseInventoryCache verifies that a skeleton horse's saddle
// inventory snapshot is cached after its container is opened and closed.
func TestSkeletonHorseInventoryCache(t *testing.T) {
	testSaddledMountInventoryCache(t, "skeleton_horse", "entity_skeleton_horse", 0, 0)
}

// TestZombieHorseInventoryCache verifies that a zombie horse's saddle
// inventory snapshot is cached after its container is opened and closed.
func TestZombieHorseInventoryCache(t *testing.T) {
	testSaddledMountInventoryCache(t, "zombie_horse", "entity_zombie_horse", 0, 0)
}

// testChestedMountInventoryCache exercises the donkey/mule-style inventory
// contract: a saddle slot plus a 15-slot chest (ChestedHorse:1b), which is
// the capability that actually distinguishes these two from a plain horse.
func testChestedMountInventoryCache(t *testing.T, entityType, testDirName string) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntityWithReplay(t, testDirName, tt.MCVersion)
			defer env.Cancel()

			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			mountNBT := getChestedMountNBT(env.Version)
			spawnCmd := fmt.Sprintf("summon minecraft:%s %.1f %.1f %.1f %s", entityType, spawnX, spawnY, spawnZ, mountNBT)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn "+entityType)
			t.Logf("Spawn response: %s", resp)

			time.Sleep(500 * time.Millisecond)

			selector := fmt.Sprintf("@e[type=minecraft:%s,limit=1,sort=nearest]", entityType)

			tameCmd := "data get entity " + selector + " Tame"
			tameResp, err := env.Inst.RCON.Exec(env.Ctx, tameCmd)
			require.NoError(t, err, "verify "+entityType+" tamed")
			t.Logf("%s Tame status: %s", entityType, tameResp)
			require.Contains(t, tameResp, "1b", entityType+" should be tamed (Tame:1b)")

			// This is the assertion that actually confirms the chest attached —
			// if ChestedHorse turns out to be the wrong tag for a given version,
			// this fails immediately here instead of surfacing later as a
			// confusing container-open or slot-count mismatch.
			chestCmd := "data get entity " + selector + " ChestedHorse"
			chestResp, err := env.Inst.RCON.Exec(env.Ctx, chestCmd)
			require.NoError(t, err, "verify "+entityType+" has chest equipped")
			t.Logf("%s ChestedHorse status: %s", entityType, chestResp)
			require.Contains(t, chestResp, "1b", entityType+" should have a chest equipped (ChestedHorse:1b)")

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			mountType := waitForEntityTypeID(t, env, "minecraft:"+entityType, 10*time.Second)
			mountID := waitForNearestEntityByType(t, env, mountType, botX, botY, botZ, 10*time.Second)

			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near "+entityType)
			time.Sleep(300 * time.Millisecond)

			windowID, err := env.Agent.Agent.OpenEntityContainer(mountID, 10*time.Second)
			require.NoError(t, err, "open "+entityType+" container")
			t.Logf("%s container opened with window ID: %d", entityType, windowID)

			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, entityType+" window should exist")
			t.Logf("Screen type: %T", screen)

			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, entityType+" container close")

			// The chest slots aren't seeded here: the exact container.N index
			// for a chested donkey/mule's chest (as opposed to the saddle slot
			// at index 0) hasn't been confirmed against a live server. This
			// checks only the open/close/cache round-trip, same as the plain
			// (unchested) horse case; the slot dump assertEntityInventoryCachedAfterClose
			// logs on the first real run is what to check that index against
			// before adding a seeded-content assertion here.
			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, mountID, 0, "")

			t.Logf("✓ %s container test passed", entityType)
		})
	}
}

// TestDonkeyInventoryCache verifies that a donkey's saddle+chest inventory
// snapshot is cached after its container is opened and closed. This is the
// capability TestHorseInventoryCache structurally can't cover, since plain
// horses can never carry a chest.
func TestDonkeyInventoryCache(t *testing.T) {
	testChestedMountInventoryCache(t, "donkey", "entity_donkey")
}

// TestMuleInventoryCache verifies the same contract as TestDonkeyInventoryCache
// for mules.
func TestMuleInventoryCache(t *testing.T) {
	testChestedMountInventoryCache(t, "mule", "entity_mule")
}

// TestLlamaInventoryCache verifies that a llama's decoration/storage
// inventory snapshot is cached after its container is opened and closed.
//
// Llamas take no saddle in any version (see SummonLlama in
// testing/vehicles/common_test.go); their inventory is a carpet decoration
// slot plus a strength-based chest (3-15 slots, here maxed at Strength:5).
// Unlike TestDonkeyInventoryCache/TestMuleInventoryCache this doesn't share
// testChestedMountInventoryCache, since llamas have no saddle slot and a
// fixed (non-version-split) spawn NBT.
func TestLlamaInventoryCache(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntityWithReplay(t, "entity_llama_chest", tt.MCVersion)
			defer env.Cancel()

			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z

			spawnCmd := fmt.Sprintf("summon minecraft:llama %.1f %.1f %.1f %s", spawnX, spawnY, spawnZ, llamaChestNBT)
			resp, err := env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn llama")
			t.Logf("Spawn response: %s => %s", spawnCmd, resp)

			time.Sleep(500 * time.Millisecond)

			selector := "@e[type=minecraft:llama,limit=1,sort=nearest]"

			tameCmd := "data get entity " + selector + " Tame"
			tameResp, err := env.Inst.RCON.Exec(env.Ctx, tameCmd)
			require.NoError(t, err, "verify llama tamed")
			t.Logf("Llama Tame status: %s", tameResp)
			require.Contains(t, tameResp, "1b", "llama should be tamed (Tame:1b)")

			// Confirms the chest attached — see the comment in
			// testChestedMountInventoryCache on why this check exists.
			chestCmd := "data get entity " + selector + " ChestedHorse"
			chestResp, err := env.Inst.RCON.Exec(env.Ctx, chestCmd)
			require.NoError(t, err, "verify llama has chest equipped")
			t.Logf("Llama ChestedHorse status: %s", chestResp)
			require.Contains(t, chestResp, "1b", "llama should have a chest equipped (ChestedHorse:1b)")

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
			require.True(t, ok, "bot position initialized")
			t.Logf("Bot position: (%.1f, %.1f, %.1f)", botX, botY, botZ)

			llamaType := waitForEntityTypeID(t, env, "minecraft:llama", 10*time.Second)
			llamaID := waitForNearestEntityByType(t, env, llamaType, botX, botY, botZ, 10*time.Second)

			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, spawnX-2, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, tpCmd)
			require.NoError(t, err, "teleport near llama")
			time.Sleep(300 * time.Millisecond)

			windowID, err := env.Agent.Agent.OpenEntityContainer(llamaID, 10*time.Second)
			require.NoError(t, err, "open llama container")
			t.Logf("Llama container opened with window ID: %d", windowID)

			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "llama window should exist")
			t.Logf("Screen type: %T", screen)

			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close llama container")

			// As with the chested donkey/mule tests, the exact container.N
			// index for the decoration and chest slots hasn't been confirmed
			// against a live server, so this checks only the open/close/cache
			// round-trip; see testChestedMountInventoryCache for why.
			assertEntityInventoryCachedAfterClose(t, env.Agent.Agent, llamaID, 0, "")

			t.Log("✓ Llama container test passed")
		})
	}
}
