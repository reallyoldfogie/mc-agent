package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestStriderMounting verifies that the agent can mount and dismount a strider.
// The player must hold warped_fungus_on_a_stick for the server to treat them as
// the controlling passenger.
func TestStriderMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StriderMountBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport StriderMountBot 100 1 100")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Give fungus BEFORE mounting — required for controlling passenger
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon saddled strider
			striderID, err := helper.SummonStrider(ctx, x+2, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Mount
			err = helper.MountEntity(ctx, striderID)
			require.NoError(t, err)

			// Wait for mounted
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err)

			// Wait for dismounted
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestStriderMovementOnLand verifies that a mounted strider moves on land (cold/shivering).
// The strider should move forward when throttle is applied, though slower than on lava
// due to cold speed penalties.
func TestStriderMovementOnLand(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StriderLandBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport StriderLandBot 200 1 200")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Equip fungus for strider control
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon and mount strider on land (flat world = stone, strider will be cold)
			striderID, err := helper.SummonStrider(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			// Move forward on land
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			// Stop
			helper.SetManualThrottle(0, 0)
			time.Sleep(1 * time.Second)

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

			t.Logf("Strider on-land displacement: %.2f blocks in 2 seconds", displacement)

			// Strider should move on land, even if slowly (cold/shivering)
			require.Greater(t, displacement, 0.5, "strider should move on land with fungus equipped")

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestStriderMovementOnLava verifies that a mounted strider moves on lava and floats
// without sinking. Lava is the strider's natural habitat — it should move faster
// than on land and maintain a stable Y position.
func TestStriderMovementOnLava(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StriderLavaBot")
			defer cleanup()

			// Build a large lava pool (21x21 blocks, radius 10)
			lavaCenterX, lavaCenterY, lavaCenterZ := 400, 1, 400
			err := helper.BuildLavaPool(ctx, lavaCenterX, lavaCenterY, lavaCenterZ, 10)
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Teleport agent to lava pool center
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport StriderLavaBot %d %d %d", lavaCenterX, lavaCenterY+1, lavaCenterZ))
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Equip fungus for strider control
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, _, z := pos.X, pos.Y, pos.Z

			// Summon strider on lava
			striderID, err := helper.SummonStrider(ctx, float64(lavaCenterX), float64(lavaCenterY+1), float64(lavaCenterZ))
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Record position after mounting (let strider settle on lava surface)
			time.Sleep(500 * time.Millisecond)
			mountedPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			initialY := mountedPos.Y

			err = helper.EnterManualMode()
			require.NoError(t, err)

			// Move forward on lava
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			finalY := finalPos.Y

			// Strider should NOT sink — Y should stay roughly stable
			yDelta := initialY - finalY
			require.Less(t, yDelta, 0.5, "strider should float on lava and not sink")

			// Strider should move horizontally
			xzDisplacement := GetDistance(finalPos.X, 0, finalPos.Z, x, 0, z)
			t.Logf("Strider on-lava XZ displacement: %.2f blocks, Y delta: %.2f", xzDisplacement, yDelta)
			require.Greater(t, xzDisplacement, 0.5, "strider should move horizontally on lava")

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestStriderLavaVsLandSpeed verifies that a strider moves significantly faster on
// lava (warm) than on land (cold/shivering). In Java, the cold strider has:
//   - SUFFOCATING_MODIFIER (-0.34 multiplied to base speed)
//   - Cold speed multiplier (0.35 vs warm 0.55)
//
// Combined, the cold strider should be roughly 2-3x slower than the warm strider.
func TestStriderLavaVsLandSpeed(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StriderSpdBot")
			defer cleanup()

			// Equip fungus once — it persists across mount/dismount
			err := helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			// ── Test 1: Movement on LAND (cold) ──
			_, err = helper.Instance.RCON.Exec(ctx, "teleport StriderSpdBot 300 1 300")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			landStart, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			striderLand, err := helper.SummonStrider(ctx, landStart.X, landStart.Y, landStart.Z)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderLand)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(3 * time.Second)

			landEnd, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementOnLand := GetDistance(landEnd.X, 0, landEnd.Z, landStart.X, 0, landStart.Z)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// ── Test 2: Movement on LAVA (warm) ──
			// Teleport into the lava area FIRST so the server loads the target
			// chunk before we fill it. Filling an unloaded chunk returns
			// "That position is not loaded" with a nil RCON error, which would
			// silently leave the strider on bare ground and make this "lava"
			// phase identical to the land phase.
			lavaCenterX, lavaCenterZ := 500, 500
			teleportLavaCmd := fmt.Sprintf("teleport StriderSpdBot %d 1 %d", lavaCenterX, lavaCenterZ)
			teleportResp, err := helper.Instance.RCON.Exec(ctx, teleportLavaCmd)
			require.NoError(t, err)
			require.NoError(t, rconResponseError(teleportLavaCmd, teleportResp))
			time.Sleep(500 * time.Millisecond)

			lavaStart, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			// Fill lava at the block directly beneath the strider's resting
			// position so computeRidingLavaPhysics sees lava below it and the
			// strider is warm. Mirrors TestStriderWarmBlockColdState, which fills
			// at y-1 relative to the settled agent position.
			err = helper.BuildLavaPool(ctx, int(lavaStart.X), int(lavaStart.Y)-1, int(lavaStart.Z), 10)
			require.NoError(t, err)
			time.Sleep(1 * time.Second)

			// Re-equip fungus in case hotbar shifted
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			striderLava, err := helper.SummonStrider(ctx, lavaStart.X, lavaStart.Y, lavaStart.Z)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderLava)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(3 * time.Second)

			lavaEnd, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementOnLava := GetDistance(lavaEnd.X, 0, lavaEnd.Z, lavaStart.X, 0, lavaStart.Z)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// ── Compare speeds ──
			t.Logf("Movement on land (cold):  %.2f blocks in 3 seconds", movementOnLand)
			t.Logf("Movement on lava (warm):  %.2f blocks in 3 seconds", movementOnLava)

			if movementOnLand > 0 {
				speedRatio := movementOnLava / movementOnLand
				t.Logf("Lava/Land speed ratio: %.2f", speedRatio)
				// Warm strider should be meaningfully faster than cold strider
				require.Greater(t, speedRatio, 1.3, "strider should be significantly faster on lava than on land")
			} else {
				// If land movement is zero (server rejected), lava movement should still work
				require.Greater(t, movementOnLava, 1.0, "strider should move on lava even if land movement failed")
			}
		})
	}
}

// TestStriderSpeedAttribute verifies that the strider's movement_speed attribute
// is within the expected vanilla range (base 0.175).
func TestStriderSpeedAttribute(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StrdrSpeedBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport StrdrSpeedBot 600 1 600")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Equip fungus for control
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			striderID, err := helper.SummonStrider(ctx, x, y, z)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Check movement_speed attribute
			striderAgent := helper.ManagedAgent.Agent
			getter, ok := striderAgent.(models.MountedEntityPositionGetter)
			if !ok {
				t.Skip("Agent does not implement MountedEntityPositionGetter")
			}

			speedAttr, found := getter.GetEntityAttribute(striderID, "generic.movement_speed")
			if found {
				t.Logf("Strider movement_speed attribute: %.4f", speedAttr)
				// Strider base speed should be around 0.175 (vanilla default)
				require.Greater(t, speedAttr, 0.05)
				require.Less(t, speedAttr, 0.3)
			} else {
				t.Logf("Strider movement_speed attribute not found (server may not provide it for summoned entities)")
			}

			// Verify movement works
			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			finalPos, _ := striderAgent.GetPositionSimple()
			movement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

			require.Greater(t, movement, 0.5)
			require.Less(t, movement, 15.0)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestStriderWarmBlockColdState verifies that a strider is warm when standing on
// lava and cold when on other blocks (stone). Mirrors Java StriderEntity.tick()
// where bl = blockState.isIn(STRIDER_WARM_BLOCKS) || fluidHeight(LAVA) > 0.
// STRIDER_WARM_BLOCKS contains only lava in vanilla (not nylium).
func TestStriderWarmBlockColdState(t *testing.T) {
	t.Parallel()
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StriderWarmBot")
			defer cleanup()

			// Use the same ground level as the other land tests (y=1)
			_, err := helper.Instance.RCON.Exec(ctx, "teleport StriderWarmBot 800 1 800")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			_, err = helper.Instance.RCON.Exec(ctx, "teleport StriderWarmBoCam 810 1 810")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			_, err = helper.Instance.RCON.Exec(ctx, "/effect give @a fire_resistance infinite 0 false")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Equip fungus for control
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Replace the ground block with lava (flat-world ground is at y-1)
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:lava",
				int64(x-10), int64(y)-1, int64(z-10), int64(x+10), int64(y)-1, int64(z+10),
			))
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			striderID, err := helper.SummonStrider(ctx, x, y, z)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			lavaPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			lavaMovement := GetDistance(lavaPos.X, 0, lavaPos.Z, x, 0, z)
			t.Logf("Movement on lava (warm): %.2f blocks", lavaMovement)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Kill strider and replace lava with stone for cold comparison
			killResponse, err := helper.Instance.RCON.Exec(ctx, "kill @e[type=minecraft:strider]")
			t.Logf("Kill strider response: %s", killResponse)
			require.NoError(t, err)
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:stone",
				int64(x-10), int64(y)-1, int64(z-10), int64(x+10), int64(y)-1, int64(z+10),
			))

			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Get current position for second strider (agent has moved during first mount)
			currentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			cx, cy, cz := currentPos.X, currentPos.Y, currentPos.Z

			striderID2, err := helper.SummonStrider(ctx, cx, cy, cz)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Re-equip fungus in case hotbar shifted during dismount
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Look at strider before mounting (required for mount to work)
			striderEntity := helper.GetTrackedEntity(striderID2)
			require.NotNil(t, striderEntity)
			err = helper.ManagedAgent.Agent.LookAt(ctx, striderEntity.X, striderEntity.Y, striderEntity.Z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID2)
			require.NoError(t, err)
			err = helper.WaitForMountedWithDiagnostics(ctx, 5*time.Second, striderID2)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			stonePos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			stoneMovement := GetDistance(stonePos.X, 0, stonePos.Z, cx, 0, cz)
			t.Logf("Movement on stone (cold): %.2f blocks", stoneMovement)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			time.Sleep(5 * time.Second)

			// Warm strider on lava should move faster than cold strider on stone
			if stoneMovement > 0 {
				ratio := lavaMovement / stoneMovement
				t.Logf("Lava/Stone speed ratio: %.2f", ratio)
				t.Logf("Movement on lava: %.2f, Movement on stone: %.2f", lavaMovement, stoneMovement)
				t.Logf("Starting positions: Lava (%.2f, %.2f, %.2f), Stone (%.2f, %.2f, %.2f)", x, y, z, cx, cy, cz)
				t.Logf("Ending Position: (%s)", stonePos.String())
				require.Greater(t, ratio, 1.0, "strider should be faster on warm lava than on cold stone")
			} else {
				require.Greater(t, lavaMovement, 0.5, "strider should move on lava")
			}
		})
	}
}

// TestStriderControllability verifies that a strider is only controllable when
// the rider holds warped_fungus_on_a_stick. In vanilla Java, getControllingPassenger()
// returns the player only when the player is holding the fungus item AND the strider
// has a saddle. Without the fungus, the strider ignores player movement input.
func TestStriderControllability(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "StrdrCtrlBot")
			defer cleanup()

			// ── Test WITHOUT fungus ──
			startPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			striderID1, err := helper.SummonStrider(ctx, startPos.X, startPos.Y, startPos.Z)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID1)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			pos1, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementWithoutFungus := pos1.DistanceTo(startPos)

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// ── Test WITH fungus ──
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport StrdrCtrlBot %d 1 %d", int64(startPos.X), int64(startPos.Z)))
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Give and equip fungus
			err = helper.GiveAndEquipWarpedFungus(ctx)
			require.NoError(t, err)

			startPos2, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x2, y2, z2 := startPos2.X, startPos2.Y, startPos2.Z

			striderID2, err := helper.SummonStrider(ctx, x2, y2, z2)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, striderID2)
			require.NoError(t, err)
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			pos2, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementWithFungus := GetDistance(pos2.X, pos2.Y, pos2.Z, x2, y2, z2)

			t.Logf("Movement without fungus: %.2f blocks", movementWithoutFungus)
			t.Logf("Movement with fungus: %.2f blocks", movementWithFungus)

			if movementWithoutFungus > 0 {
				actualRatio := movementWithFungus / movementWithoutFungus
				t.Logf("Speed ratio (with/without): %.2f", actualRatio)
				require.Greater(t, actualRatio, 1.0, "fungus should enable or increase movement")
			} else {
				// Without fungus the strider was not controllable at all (expected)
				require.Greater(t, movementWithFungus, 0.5, "strider should move when fungus is equipped")
			}

			err = helper.ExitManualMode()
			require.NoError(t, err)
			err = helper.DismountEntity()
			require.NoError(t, err)
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}
