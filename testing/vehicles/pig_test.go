package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestPigMounting verifies that the agent can mount and dismount a saddled pig.
func TestPigMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "PigMountBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport PigMountBot 100 1 100")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon saddled pig
			pigID, err := helper.SummonPig(ctx, x+2, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Mount
			err = helper.MountEntity(ctx, pigID)
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

// TestPigMovement verifies that a mounted pig responds to throttle inputs with proper velocity and steering.
func TestPigMovement(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "PigMoveBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport PigMoveBot 200 1 200")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon and mount pig
			pigID, err := helper.SummonPig(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, pigID)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Enter manual mode for controlled movement
			err = helper.EnterManualMode()
			require.NoError(t, err)

			// Move forward
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			// Stop
			helper.SetManualThrottle(0, 0)
			time.Sleep(1 * time.Second)

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

			// Pig should move (base speed ~0.25)
			require.Greater(t, displacement, 1.0)

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestPigSpeedAttribute verifies pig speed handling and attribute reading.
func TestPigSpeedAttribute(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "PigSpeedBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport PigSpeedBot 300 1 300")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon and mount pig
			pigID, err := helper.SummonPig(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, pigID)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Try to get pig's movement speed attribute
			pigAgent := helper.ManagedAgent.Agent
			getter, ok := pigAgent.(models.MountedEntityPositionGetter)
			if !ok {
				t.Skip("Agent does not implement MountedEntityPositionGetter")
			}

			speedAttr, found := getter.GetEntityAttribute(pigID, "generic.movement_speed")

			if found {
				t.Logf("Pig movement_speed attribute: %.4f", speedAttr)
				require.Greater(t, speedAttr, 0.1)
			} else {
				t.Logf("Pig does not have generic.movement_speed attribute")
			}

			// Verify movement
			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			finalPos, _ := pigAgent.GetPositionSimple()
			movement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

			require.Greater(t, movement, 2.0)
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

// TestPigCarrotBoost verifies that carrot-on-a-stick provides a speed boost.
func TestPigCarrotBoost(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "PigCarrotBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport PigCarrotBot 400 1 400")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x1, y1, z1 := pos.X, pos.Y, pos.Z

			// Test movement WITHOUT carrot
			pigID1, err := helper.SummonPig(ctx, x1, y1, z1)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, pigID1)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			pos1, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementWithoutCarrot := GetDistance(pos1.X, pos1.Y, pos1.Z, x1, y1, z1)

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Teleport to new location
			x2, y2, z2 := 400.0, 1.0, 500.0
			_, err = helper.Instance.RCON.Exec(ctx, "teleport PigCarrotBot 400 1 500")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Test movement WITH carrot
			pigID2, err := helper.SummonPig(ctx, x2, y2, z2)
			require.NoError(t, err)

			// Give carrot and ensure it's in hand
			_, _ = helper.Instance.RCON.Exec(ctx, "give PigCarrotBot carrot_on_a_stick")
			time.Sleep(500 * time.Millisecond)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, pigID2)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			err = helper.EnterManualMode()
			require.NoError(t, err)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			pos2, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			movementWithCarrot := GetDistance(pos2.X, pos2.Y, pos2.Z, x2, y2, z2)

			// Movement with carrot should be faster
			actualRatio := movementWithCarrot / movementWithoutCarrot

			t.Logf("Movement without carrot: %.2f blocks", movementWithoutCarrot)
			t.Logf("Movement with carrot: %.2f blocks", movementWithCarrot)
			t.Logf("Speed ratio (with/without): %.2f", actualRatio)

			require.Greater(t, actualRatio, 1.0, "carrot should increase movement speed")

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}
