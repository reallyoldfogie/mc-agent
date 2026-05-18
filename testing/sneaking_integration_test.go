package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSneaking_EdgePrevention tests that the bot cannot walk off block edges while sneaking
func TestSneaking_EdgePrevention(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"SneakBot_EdgePrevention",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)

			// Enable/disable cam agent
			// agCfg.EnableCamAgent = true
			agCfg.EnableCamAgent = false
			agCfg.EnableReplay = true

			ag, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if ag != nil {
					_ = ag.Stop(stopCtx) // Handles agent + cam agent cleanup
				}
			}()
			time.Sleep(2 * time.Second)

			// Get bot position
			botX, botY, botZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")

			platformX := int(math.Floor(botX))
			platformY := int(math.Floor(botY)) - 1
			platformZ := int(math.Floor(botZ))

			// create a pit around the platform to test edge prevention
			fillCmd := fmt.Sprintf(`fill %d %d %d %d %d %d minecraft:air`, platformX-20, platformY, platformZ-20, platformX+20, platformY-5, platformZ+20)
			fillResponse, err := inst.RCON.Exec(ctx, fillCmd)
			t.Logf("%s => %s", fillCmd, fillResponse)
			require.NoError(t, err)

			// Build a 3x3 platform
			for x := platformX - 1; x <= platformX+1; x++ {
				for z := platformZ - 1; z <= platformZ+1; z++ {
					_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:grass_block`, x, platformY, z))
					require.NoError(t, err)
				}
			}

			time.Sleep(3 * time.Second)

			// Teleport bot to center of platform
			tp := fmt.Sprintf(`teleport %s %.1f %.1f %.1f`, ag.Name, float64(platformX)+0.5, float64(platformY+1)+0.5, float64(platformZ)+0.5)
			_, err = inst.RCON.Exec(ctx, tp)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			initialX, initialY, initialZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok, "bot position should be available")

			// Start sneaking
			err = ag.Agent.StartSneaking()
			require.NoError(t, err, "StartSneaking error")

			// Move forward while sneaking to test edge prevention
			err = ag.Agent.MoveForward(ctx, 3.0)
			if err != nil { // expected, since the edge prevention should stop movement to the full 3 blocks
				t.Logf("MoveForward error: %v", err)
			}

			// give time to move
			time.Sleep(6 * time.Second)

			// Check bot position - should not have walked off edge, but should have still moved
			finalX, finalY, finalZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok, "bot position should be available")

			initialV3 := models.V3{X: initialX, Y: initialY, Z: initialZ}
			finalV3 := models.V3{X: finalX, Y: finalY, Z: finalZ}
			distanceMoved := initialV3.DistanceTo(finalV3)
			ag.Agent.SendChat(fmt.Sprintf("Movement test: Moved from %.2f %.2f %.2f to %.2f %.2f %.2f (%.2f blocks)", initialX, initialY, initialZ, finalX, finalY, finalZ, distanceMoved))

			assert.NotEqual(t, initialV3, finalV3)

			// Bot should still be on platform (within reasonable bounds)
			assert.GreaterOrEqual(t, finalY, float64(platformY)+0.5,
				"bot should still be on platform level after sneaking movement attempt")
		})
	}
}

// TestSneaking_MovementAllowed tests that bot can move normally when not near edges
func TestSneaking_MovementAllowed(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"SneakBot_Movement",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)
			ag, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)
			defer func() {
				if ag != nil && ag.BotClient() != nil {
					_ = ag.BotClient().Close()
				}
			}()

			time.Sleep(2 * time.Second)

			// Get bot position
			botX, botY, botZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")

			platformX := int(math.Floor(botX))
			platformY := int(math.Floor(botY)) - 1
			platformZ := int(math.Floor(botZ))

			fillCmd := fmt.Sprintf(`fill %d %d %d %d %d %d minecraft:air`, platformX-20, platformY, platformZ-20, platformX+20, platformY-5, platformZ+20)
			fillResponse, err := inst.RCON.Exec(ctx, fillCmd)
			t.Logf("%s => %s", fillCmd, fillResponse)
			require.NoError(t, err)

			// Build a large platform (10x10) so bot is far from edges
			for x := platformX - 5; x <= platformX+5; x++ {
				for z := platformZ - 5; z <= platformZ+5; z++ {
					_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:grass_block`, x, platformY, z))
					require.NoError(t, err)
				}
			}

			time.Sleep(500 * time.Millisecond)

			// Teleport bot to center
			tp := fmt.Sprintf(`teleport %s %.1f %.1f %.1f`, ag.Name, float64(platformX)+0.5, float64(platformY+1)+0.5, float64(platformZ)+0.5)
			_, err = inst.RCON.Exec(ctx, tp)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			initialX, initialY, initialZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok)

			// Start sneaking and move
			err = ag.Agent.StartSneaking()
			require.NoError(t, err)

			err = ag.Agent.MoveForward(ctx, 3.0)
			require.NoError(t, err, "bot should be able to move when not near edge")

			// give time to move
			time.Sleep(5 * time.Second)

			// Check bot moved
			finalX, finalY, finalZ, ok := ag.Agent.GetPositionSimple()
			require.True(t, ok)

			initialV3 := models.V3{X: initialX, Y: initialY, Z: initialZ}
			finalV3 := models.V3{X: finalX, Y: finalY, Z: finalZ}
			distanceMoved := initialV3.DistanceTo(finalV3)
			ag.Agent.SendChat(fmt.Sprintf("Movement test: Moved from %.2f %.2f %.2f to %.2f %.2f %.2f (%.2f blocks)", initialX, initialY, initialZ, finalX, finalY, finalZ, distanceMoved))

			assert.NotEqual(t, initialV3, finalV3)

			// Should have moved a measurable distance
			assert.InDeltaf(t, distanceMoved, 3.0, .15,
				"bot should move at least 3 blocks when sneaking and not near edge")
			assert.GreaterOrEqual(t, finalY, float64(platformY)+0.5,
				"bot should still be on platform")
		})
	}
}
