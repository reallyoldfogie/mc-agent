package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestBoatSteering verifies that the agent can mount a boat and steer it using manual inputs
func TestBoatSteering(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion)
			defer cleanup()

			// Teleport agent to a location with water
			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("%s => %s", cmd, resp)

			_, err = helper.Instance.RCON.Exec(ctx, "teleport VehicleBot -1 1 -1")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond) // Wait for position update

			// Get agent's initial position
			x1, y1, z1, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Summon a boat at the agent's location
			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak") // oak boat in water
			require.NoError(t, err, "summon boat")

			time.Sleep(500 * time.Millisecond) // Wait for boat to spawn

			t.Logf("Mounting boat from %.2f %.2f %.2f", x1, y1, z1)

			// Mount the boat
			err = helper.MountEntity(ctx, boatEntityID)
			require.NoError(t, err, "mount boat")

			// Wait for mount confirmation
			err = helper.WaitForMounted(ctx, 15*time.Second)
			require.NoError(t, err, "agent should be mounted")

			t.Logf("agent mounted successfully")

			x1, y1, z1, _ = helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("Updated agent position after mounting %.2f %.2f %.2f", x1, y1, z1)

			// Enter manual mode
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			t.Logf("Move Forward - setting throttle (0, 1.0)")
			// Set throttle to move forward
			helper.SetManualThrottle(0, 1.0) // positive Z = forward

			// Wait for movement (boat should move forward)
			time.Sleep(2 * time.Second)

			// Get agent's position after moving forward
			x2, y2, z2, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f after move forward", x2, y2, z2)

			// Verify agent moved forward (positive Z direction)
			distance := GetDistance(x1, y1, z1, x2, y2, z2)
			require.Greater(t, distance, 0.5, "agent should have moved forward at least 0.5 blocks")
			require.Greater(t, z2, z1, "agent should have moved in positive Z direction")

			// Test lateral movement (steer right)
			initialX := x2

			t.Logf("Steer right - setting throttle 1.0 .5")

			// Stop forward movement and steer right
			helper.SetManualThrottle(1.0, 0.5) // positive X = right, positive Z = forward

			time.Sleep(5 * time.Second)

			x3, _, z3, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f after steer right", x3, y2, z3)

			// Verify agent moved right (positive X direction)
			require.Greater(t, x3, initialX, "agent should have moved in positive X direction")

			t.Logf("Move backwards - setting throttle to 0, -1.0")

			// Test backward movement
			helper.SetManualThrottle(0, -1.0) // negative Z = backward

			time.Sleep(5 * time.Second)

			x4, _, z4, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f after move backwards", x4, y2, z4)

			// Verify agent moved backward
			require.Less(t, z4, z3, "agent should have moved backward")

			t.Logf("Idle - setting throttle 0, 0")

			// Test stop
			helper.SetManualThrottle(0, 0)

			time.Sleep(5 * time.Second)

			x5, _, z5, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f after idle", x5, y2, z5)

			// Position should be approximately the same (no movement)
			distance = GetDistance(x4, 0, z4, x5, 0, z5)
			require.Less(t, distance, 0.1, "agent should have minimal movement when throttle is zero")

			// Exit manual mode
			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount vehicle")

			// Verify dismount
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be dismounted")
		})
	}
}

// TestHorseSteering verifies that the agent can mount a horse and steer it using manual inputs
func TestHorseSteering(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion)
			defer cleanup()

			// Teleport agent to ground level (flat world ground is at y=0/1)
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 0 1 0")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			// Get agent's initial position
			x1, y1, z1, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Summon a tamed and saddled horse
			horseEntityID, err := helper.SummonHorse(ctx, x1+1, y1, z1)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			t.Logf("Mounting horse from %.2f %.2f %.2f", x1, y1, z1)

			// Mount the horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			// Wait for mount confirmation
			err = helper.WaitForMounted(ctx, 25*time.Second)
			require.NoError(t, err, "agent should be mounted")

			t.Logf("agent mounted successfully")

			x1, y1, z1, _ = helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("Updated agent position after mounting %.2f %.2f %.2f", x1, y1, z1)

			// Enter manual mode
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			t.Logf("Setting throttle (0, 1.0)")
			// Set throttle to move forward
			helper.SetManualThrottle(0, 1.0) // positive Z = forward

			// Wait for movement (horse should move forward)
			time.Sleep(2 * time.Second)

			// Get agent's position after moving forward
			x2, y2, z2, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f", x2, y2, z2)

			// Verify agent moved forward (positive Z direction)
			distance := GetDistance(x1, y1, z1, x2, y2, z2)
			require.Greater(t, distance, 0.5, "agent should have moved forward at least 0.5 blocks")
			require.Greater(t, z2, z1, "agent should have moved in positive Z direction")

			// Test lateral movement (steer right)
			initialX := x2

			t.Logf("Setting throttle 1.0 .5")

			// Stop forward movement and steer right
			helper.SetManualThrottle(1.0, 0.5) // positive X = right, positive Z = forward

			time.Sleep(5 * time.Second)

			x3, _, z3, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f", x3, y2, z3)

			// Verify agent moved right (positive X direction)
			require.Greater(t, x3, initialX, "agent should have moved in positive X direction")

			t.Logf("Setting throttle to 0, -1.0")

			// Test backward movement
			helper.SetManualThrottle(0, -1.0) // negative Z = backward

			time.Sleep(5 * time.Second)

			x4, _, z4, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f", x4, y2, z4)

			// Verify agent moved backward
			require.Less(t, z4, z3, "agent should have moved backward")

			t.Logf("Setting throttle 0, 0")

			// Test stop
			helper.SetManualThrottle(0, 0)

			time.Sleep(5 * time.Second)

			x5, _, z5, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			t.Logf("New Position: %.2f %.2f %.2f", x5, y2, z5)

			// Position should be approximately the same (no movement)
			distance = GetDistance(x4, 0, z4, x5, 0, z5)
			require.Less(t, distance, 0.1, "agent should have minimal movement when throttle is zero")

			// Exit manual mode
			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount vehicle")

			// Verify dismount
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be dismounted")
		})
	}
}

// TestSteering validates steering input system for mounted vehicles
func TestVehicleSteeringInputs(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion)
			defer cleanup()

			// Teleport agent to a location with water
			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("%s => %s", cmd, resp)

			_, err = helper.Instance.RCON.Exec(ctx, "teleport VehicleBot -1 1 -1")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond) // Wait for position update

			// Get agent's initial position
			_, _, _, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Summon a boat at the agent's location
			// Use hardcoded coordinates: agent is at (25.5, 50.5, 25.5), boat should be at (25.5, 50.0, 25.5)
			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak") // oak boat in water
			require.NoError(t, err, "summon boat")

			time.Sleep(500 * time.Millisecond) // Wait for boat to spawn

			// Mount and enter manual mode
			err = helper.MountEntity(ctx, boatEntityID)
			require.NoError(t, err, "mount boat")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			// Test multiple throttle combinations
			testCases := []struct {
				name      string
				throttleX float64
				throttleZ float64
				duration  time.Duration
			}{
				{"forward", 0, 1.0, 1 * time.Second},
				{"backward", 0, -1.0, 1 * time.Second},
				{"right", 1.0, 0, 1 * time.Second},
				{"left", -1.0, 0, 1 * time.Second},
				{"forward-right", 0.7, 0.7, 1 * time.Second},
				{"forward-left", -0.7, 0.7, 1 * time.Second},
				{"stop", 0, 0, 500 * time.Millisecond},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					px, _, pz, _ := helper.ManagedAgent.Agent.GetPositionSimple()
					helper.SetManualThrottle(tc.throttleX, tc.throttleZ)
					time.Sleep(tc.duration)
					x2, _, z2, _ := helper.ManagedAgent.Agent.GetPositionSimple()

					// For non-zero throttle, verify movement occurred
					if tc.throttleX != 0 || tc.throttleZ != 0 {
						distance := GetDistance(px, 0, pz, x2, 0, z2)
						require.Greater(t, distance, 0.2, "throttle %v,%v should cause movement", tc.throttleX, tc.throttleZ)
					}
				})
			}

			// Exit manual mode and dismount
			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")

			err = helper.DismountEntity()
			require.NoError(t, err, "dismount")
		})
	}
}
