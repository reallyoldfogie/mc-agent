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

// TestSwimming_FallIntoWaterNoFallDamage verifies that an agent dropped from a
// lethal height into a deep water pool survives without taking fall damage.
// This validates the fall distance reset when entering water.
func TestSwimming_FallIntoWaterNoFallDamage(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "swimming_fall", "survival", false, tt.MCVersion, DifficultyNormal, true)
			defer env.Cancel()

			ctx := context.Background()

			baseY := int(math.Floor(env.ContainerPos.Y)) - 1
			baseX := int(math.Floor(env.ContainerPos.X)) - 5
			baseZ := int(math.Floor(env.ContainerPos.Z)) - 5

			poolWidth := 10
			poolDepth := 5
			dropHeight := 30

			// Build a stone floor under the pool
			err := BuildPlatform(ctx, env.Inst.RCON, baseX, baseY, baseZ, poolWidth, poolWidth, "minecraft:stone")
			require.NoError(t, err, "build pool floor")

			// Fill pool with water (5 blocks deep)
			for waterY := baseY + 1; waterY <= baseY+poolDepth; waterY++ {
				fillCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					baseX, waterY, baseZ, baseX+poolWidth-1, waterY, baseZ+poolWidth-1)
				_, err := env.Inst.RCON.Exec(ctx, fillCmd)
				require.NoError(t, err, "fill water layer at Y=%d", waterY)
			}

			// Clear air above the pool so the agent can fall freely
			clearTopY := baseY + poolDepth + dropHeight + 5
			if err := ClearArea(ctx, env.Inst.RCON,
				baseX, baseY+poolDepth+1, baseZ,
				baseX+poolWidth-1, clearTopY, baseZ+poolWidth-1); err != nil {
				t.Logf("warning: failed to clear area above pool: %v", err)
			}

			// Wait for world to settle
			time.Sleep(2 * time.Second)

			// Ensure the agent starts with full health
			healCmd := fmt.Sprintf("effect give %s minecraft:instant_health 1 10", env.BotName)
			_, _ = env.Inst.RCON.Exec(ctx, healCmd)
			time.Sleep(500 * time.Millisecond)

			healthBefore, err := GetPlayerHealth(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get health before drop")
			t.Logf("Health before drop: %.1f", healthBefore)
			require.Equal(t, float32(20.0), healthBefore, "agent should start with full health")

			// Teleport agent above the center of the pool at lethal fall height
			centerX := float64(baseX) + float64(poolWidth)/2.0
			centerZ := float64(baseZ) + float64(poolWidth)/2.0
			dropY := float64(baseY + poolDepth + dropHeight)
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, centerX, dropY, centerZ)
			resp, err := env.Inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent above pool")
			t.Logf("Teleported to (%.1f, %.1f, %.1f): %s", centerX, dropY, centerZ, resp)

			// Wait for the agent to fall into the water and settle
			// At ~30 blocks drop height, this takes several seconds
			time.Sleep(8 * time.Second)

			// Verify the agent's position is inside/near the water pool (they fell down)
			afterPos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position after fall")
			t.Logf("Position after fall: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			assert.Less(t, afterPos.Y, dropY-5.0,
				"agent should have fallen from the drop height")

			// Verify the agent survived (health should still be full)
			healthAfter, err := GetPlayerHealth(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get health after fall")
			t.Logf("Health after fall into water: %.1f", healthAfter)

			assert.Equal(t, float32(20.0), healthAfter,
				"agent should not take fall damage when landing in water")
		})
	}
}

// TestSwimming_SwimToSurface verifies that an agent submerged in water can swim
// upward toward the surface. The agent is placed at the bottom of a deep pool,
// then given an upward movement target. The physics engine should apply swim-up
// velocity (via jump input in water) to move the agent upward.
func TestSwimming_SwimToSurface(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "swimming_surface", "survival", false, tt.MCVersion, DifficultyNormal, true)
			defer env.Cancel()

			ctx := context.Background()

			baseY := int(math.Floor(env.ContainerPos.Y)) - 1
			baseX := int(math.Floor(env.ContainerPos.X)) - 5
			baseZ := int(math.Floor(env.ContainerPos.Z)) - 5

			poolWidth := 10
			poolDepth := 8

			// Build a stone floor under the pool
			err := BuildPlatform(ctx, env.Inst.RCON, baseX, baseY, baseZ, poolWidth, poolWidth, "minecraft:stone")
			require.NoError(t, err, "build pool floor")

			// Fill pool with water
			for waterY := baseY + 1; waterY <= baseY+poolDepth; waterY++ {
				fillCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					baseX, waterY, baseZ, baseX+poolWidth-1, waterY, baseZ+poolWidth-1)
				_, err := env.Inst.RCON.Exec(ctx, fillCmd)
				require.NoError(t, err, "fill water layer at Y=%d", waterY)
			}

			// Clear air above the pool
			if err := ClearArea(ctx, env.Inst.RCON,
				baseX, baseY+poolDepth+1, baseZ,
				baseX+poolWidth-1, baseY+poolDepth+10, baseZ+poolWidth-1); err != nil {
				t.Logf("warning: failed to clear area above pool: %v", err)
			}

			// Wait for world to settle
			time.Sleep(2 * time.Second)

			// Teleport agent to bottom of pool
			centerX := float64(baseX) + float64(poolWidth)/2.0
			centerZ := float64(baseZ) + float64(poolWidth)/2.0
			bottomY := float64(baseY + 1)
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, centerX, bottomY, centerZ)
			resp, err := env.Inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to bottom of pool")
			t.Logf("Teleported to pool bottom (%.1f, %.1f, %.1f): %s", centerX, bottomY, centerZ, resp)

			// Wait for physics to settle and water detection
			time.Sleep(2 * time.Second)

			// Record position at bottom
			beforePos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position before swimming")
			t.Logf("Position before swimming: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

			// Move the agent upward — MoveUp uses position updates to swim up
			surfaceY := float64(baseY + poolDepth + 1)
			err = env.Agent.Agent.MoveUp(ctx, surfaceY-beforePos.Y)
			if err != nil {
				t.Logf("MoveUp returned error (may be expected): %v", err)
			}

			// Wait for movement to complete
			time.Sleep(3 * time.Second)

			// Record position after swimming
			afterPos, err := GetPlayerPosition(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get position after swimming")
			t.Logf("Position after swimming: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Agent should have moved upward
			yDisplacement := afterPos.Y - beforePos.Y
			t.Logf("Y displacement: %.2f", yDisplacement)

			assert.Greater(t, yDisplacement, 1.0,
				"agent should have swum upward by at least 1 block")
		})
	}
}

// TestSwimming_PathfindingAcrossWater verifies that the pathfinder can find and
// execute a path that crosses a water channel. The channel has shallow edge
// shelves (solid floor for entry/exit) and a deep middle section that forces
// the agent to use Swim movement steps.
//
// Layout (side view, X going right):
//
//	[Ground]  Water  Water  Water  Water  Water  [Ground]
//	[Ground]  Stone  Water  Water  Water  Stone  [Ground]  <- edge shelves
//	[Ground]  Stone  Stone  Stone  Stone  Stone  [Ground]  <- pool floor
//
// Walls on the Z-sides prevent the agent from walking around the channel.
func TestSwimming_PathfindingAcrossWater(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.MCVersion
			serverCfg.Difficulty = DifficultyPeaceful
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()

			agentCfg := DefaultAgentConfig(
				"SwimPathBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version,
				fmt.Sprintf("swimming_pathfind_across_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")),
				agentCfg.Name)

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			defer func() {
				if agent != nil {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = agent.Stop(stopCtx)
					stopCancel()
					if agent.BotClient() != nil {
						_ = agent.BotClient().Close()
					}
				}
			}()

			time.Sleep(5 * time.Second)

			// Get agent position on the flat world (surface at Y=63, standing at Y=64)
			startPos, initialized := agent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position initialized")
			t.Logf("Agent start: %s", startPos)

			// Channel coordinates: 5 blocks wide in X, centered around startZ
			// Flat world surface: Y=63 (grass). Agent feet at Y=64.
			groundY := int(math.Floor(startPos.Y)) - 1 // Y=63
			channelStartX := int(math.Floor(startPos.X)) + 4
			channelEndX := channelStartX + 5
			channelZ1 := int(math.Floor(startPos.X)) - 3 // reuse startX's integer for a nearby Z
			channelZ2 := channelZ1 + 6

			// Use agent Z for the path
			pos, _ := agent.Agent.GetPositionSimple()
			_, _, agentZ := pos.X, pos.Y, pos.Z
			channelZ1 = int(math.Floor(agentZ)) - 3
			channelZ2 = channelZ1 + 6

			// Dig the channel: remove grass and dirt layers
			for digY := groundY; digY >= groundY-2; digY-- {
				digCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
					channelStartX, digY, channelZ1, channelEndX, digY, channelZ2)
				_, err := inst.RCON.Exec(ctx, digCmd)
				require.NoError(t, err, "dig channel at Y=%d", digY)
			}

			// Place stone floor at the bottom of the channel
			floorY := groundY - 3
			floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
				channelStartX, floorY, channelZ1, channelEndX, floorY, channelZ2)
			_, err = inst.RCON.Exec(ctx, floorCmd)
			require.NoError(t, err, "place channel floor")

			// Place edge shelves (stone at floorY+1) for first and last X column
			for _, shelfX := range []int{channelStartX, channelEndX} {
				shelfCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
					shelfX, floorY+1, channelZ1, shelfX, floorY+1, channelZ2)
				_, err = inst.RCON.Exec(ctx, shelfCmd)
				require.NoError(t, err, "place edge shelf at X=%d", shelfX)
			}

			// Fill the channel with water from floorY+1 to groundY (surface)
			for waterY := floorY + 1; waterY <= groundY; waterY++ {
				waterCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					channelStartX, waterY, channelZ1, channelEndX, waterY, channelZ2)
				_, err = inst.RCON.Exec(ctx, waterCmd)
				require.NoError(t, err, "fill water at Y=%d", waterY)
			}

			// Build walls on Z-sides to prevent walking around
			for wallY := floorY; wallY <= groundY+3; wallY++ {
				for _, wallZ := range []int{channelZ1 - 1, channelZ2 + 1} {
					wallCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
						channelStartX-1, wallY, wallZ, channelEndX+1, wallY, wallZ)
					_, _ = inst.RCON.Exec(ctx, wallCmd)
				}
			}

			time.Sleep(3 * time.Second)

			// Teleport agent to start side, facing the channel
			tpX := float64(channelStartX) - 2.5
			tpZ := float64(channelZ1+channelZ2) / 2.0
			tpY := float64(groundY + 1)
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", agentCfg.Name, tpX, tpY, tpZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to start")
			t.Logf("Agent teleported to (%.1f, %.1f, %.1f)", tpX, tpY, tpZ)

			time.Sleep(3 * time.Second)

			// Goal: on the far side of the channel
			goalX := float64(channelEndX) + 2.5
			goalY := float64(groundY + 1)
			goalZ := tpZ
			t.Logf("Goal: (%.1f, %.1f, %.1f)", goalX, goalY, goalZ)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Navigate using pathfinding
			err = agent.Agent.MoveTo(ctx, goalX, goalY, goalZ, true)
			if err != nil {
				t.Logf("MoveTo returned error: %v", err)
			}

			// Wait for agent to reach destination
			timeout := 60 * time.Second
			err = tracker.WaitForPosition(ctx, agentCfg.Name, models.V3{X: goalX, Y: goalY, Z: goalZ}, 2.5, timeout)

			finalPos, posOK := tracker.GetPosition(agentCfg.Name)
			require.True(t, posOK, "should have final position")
			t.Logf("Final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

			goal := models.V3{X: goalX, Y: goalY, Z: goalZ}
			finalDist := finalPos.DistanceTo(goal)
			t.Logf("Distance from goal: %.2f", finalDist)

			assert.LessOrEqual(t, finalDist, 3.0,
				"agent should reach the goal on the far side of the water channel")
		})
	}
}

// TestSwimming_PathfindingDropIntoWater verifies that the pathfinder can route
// an agent from an elevated platform down into a water-filled trench and across
// to a goal on the far side. The agent must descend from the platform into the
// water (testing the Descend move into water) and then navigate through the
// water to reach the goal.
//
// Layout (side view, X going right):
//
//	Y=67:  [Platform]
//	Y=66:  [Stone   ]
//	Y=65:  [Stone   ]  Water  Water  Water
//	Y=64:  [Ground  ]  Water  Water  Water  [Ground = Goal]
//	Y=63:  [Ground  ]  Stone  Water  Stone  [Ground]
//	Y=62:  [Ground  ]  Stone  Stone  Stone  [Ground]
func TestSwimming_PathfindingDropIntoWater(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.MCVersion
			serverCfg.Difficulty = DifficultyPeaceful
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()

			agentCfg := DefaultAgentConfig(
				"SwimDropBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version,
				fmt.Sprintf("swimming_phys_pathfind_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")),
				agentCfg.Name)

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			defer func() {
				if agent != nil {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = agent.Stop(stopCtx)
					stopCancel()
					if agent.BotClient() != nil {
						_ = agent.BotClient().Close()
					}
				}
			}()

			time.Sleep(5 * time.Second)

			// Get agent position on the flat world
			agentPos, posOK := agent.Agent.GetPositionSimple()
			startY := agentPos.Y
			require.True(t, posOK, "agent position initialized")

			// Flat world surface Y=63, standing at Y=64
			groundY := int(math.Floor(startY)) - 1 // Y=63
			pos, _ := agent.Agent.GetPositionSimple()
			_, _, agentZ := pos.X, pos.Y, pos.Z
			platformX := int(math.Floor(startY)) // pick an X near spawn
			platZ := int(math.Floor(agentZ))

			// Use the agent's actual position for the platform

			agentPos, _ = agent.Agent.GetPositionSimple()
			agentX := agentPos.X
			platformX = int(math.Floor(agentX))

			// Build elevated platform: 3 blocks of stone at platformX, Z=platZ±1
			platformHeight := 3
			for platY := groundY + 1; platY <= groundY+platformHeight; platY++ {
				platCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
					platformX-1, platY, platZ-1, platformX+1, platY, platZ+1)
				_, err = inst.RCON.Exec(ctx, platCmd)
				require.NoError(t, err, "build platform at Y=%d", platY)
			}
			platTopY := groundY + platformHeight // Y=66
			agentOnPlatY := platTopY + 1         // Y=67

			// Water trench: starts 2 blocks away from platform in +X direction
			trenchStartX := platformX + 3
			trenchEndX := trenchStartX + 4 // 5 blocks wide
			trenchZ1 := platZ - 2
			trenchZ2 := platZ + 2

			// Dig the trench (remove blocks down to groundY-2)
			for digY := groundY; digY >= groundY-2; digY-- {
				digCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
					trenchStartX, digY, trenchZ1, trenchEndX, digY, trenchZ2)
				_, err = inst.RCON.Exec(ctx, digCmd)
				require.NoError(t, err, "dig trench at Y=%d", digY)
			}

			// Stone floor at the bottom
			floorY := groundY - 3
			floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
				trenchStartX, floorY, trenchZ1, trenchEndX, floorY, trenchZ2)
			_, err = inst.RCON.Exec(ctx, floorCmd)
			require.NoError(t, err, "place trench floor")

			// Edge shelves: stone at floorY+1 on the first and last X columns
			for _, shelfX := range []int{trenchStartX, trenchEndX} {
				shelfCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
					shelfX, floorY+1, trenchZ1, shelfX, floorY+1, trenchZ2)
				_, err = inst.RCON.Exec(ctx, shelfCmd)
				require.NoError(t, err, "place edge shelf at X=%d", shelfX)
			}

			// Fill trench with water
			for waterY := floorY + 1; waterY <= groundY; waterY++ {
				waterCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					trenchStartX, waterY, trenchZ1, trenchEndX, waterY, trenchZ2)
				_, err = inst.RCON.Exec(ctx, waterCmd)
				require.NoError(t, err, "fill water at Y=%d", waterY)
			}

			// Build walls on Z-sides to force path through water
			for wallY := floorY; wallY <= groundY+4; wallY++ {
				for _, wallZ := range []int{trenchZ1 - 1, trenchZ2 + 1} {
					wallCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
						platformX-1, wallY, wallZ, trenchEndX+2, wallY, wallZ)
					_, _ = inst.RCON.Exec(ctx, wallCmd)
				}
			}

			time.Sleep(3 * time.Second)

			// Teleport agent to top of the elevated platform
			tpX := float64(platformX) + 0.5
			tpZ := float64(platZ) + 0.5
			tpCmd := fmt.Sprintf("tp %s %.1f %d %.1f", agentCfg.Name, tpX, agentOnPlatY, tpZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to platform top")
			t.Logf("Agent teleported to platform top (%.1f, %d, %.1f)", tpX, agentOnPlatY, tpZ)

			time.Sleep(3 * time.Second)

			// Goal: on the ground on the far side of the trench
			goalX := float64(trenchEndX) + 2.5
			goalY := float64(groundY + 1) // Y=64 (standing on grass)
			goalZ := tpZ
			t.Logf("Goal: (%.1f, %.1f, %.1f) — drop from Y=%d to water then across",
				goalX, goalY, goalZ, agentOnPlatY)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Navigate using pathfinding
			err = agent.Agent.MoveTo(ctx, goalX, goalY, goalZ, true)
			if err != nil {
				t.Logf("MoveTo returned error: %v", err)
			}

			// Wait for agent to reach destination
			timeout := 90 * time.Second
			err = tracker.WaitForPosition(ctx, agentCfg.Name, models.V3{X: goalX, Y: goalY, Z: goalZ}, 2.5, timeout)

			finalPos, posOK := tracker.GetPosition(agentCfg.Name)
			require.True(t, posOK, "should have final position")
			t.Logf("Final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

			goal := models.V3{X: goalX, Y: goalY, Z: goalZ}
			finalDist := finalPos.DistanceTo(goal)
			t.Logf("Distance from goal: %.2f", finalDist)

			assert.LessOrEqual(t, finalDist, 3.0,
				"agent should reach the goal after dropping from elevation through water")
		})
	}
}

// TestSwimming_PathfindingSwimUp verifies that the agent can pathfind upward through
// a deep vertical water column to reach a goal above. The agent must use SwimUp movements
// to ascend through multiple water blocks without any shelves.
func TestSwimming_PathfindingSwimUp(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.MCVersion
			serverCfg.Difficulty = DifficultyPeaceful
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()

			agentCfg := DefaultAgentConfig(
				"SwimUpBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version,
				fmt.Sprintf("swimming_phys_fluid_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")),
				agentCfg.Name)

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			defer func() {
				if agent != nil {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = agent.Stop(stopCtx)
					stopCancel()
					if agent.BotClient() != nil {
						_ = agent.BotClient().Close()
					}
				}
			}()

			time.Sleep(5 * time.Second)

			// Get agent position on flat world
			agentPos, posOK := agent.Agent.GetPositionSimple()
			require.True(t, posOK, "agent position initialized")

			// Flat world surface: Y=0, standing at Y=1
			groundY := int(math.Floor(agentPos.Y))
			wellX := int(math.Floor(agentPos.X)) + 5
			wellZ := int(math.Floor(agentPos.Z))

			// Create a vertical water column (well): 5 blocks wide, 8 blocks deep
			wellWidth := 5
			wellDepth := 8
			wellBottomY := groundY - wellDepth

			// Dig the well
			for digY := groundY; digY >= wellBottomY; digY-- {
				digCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
					wellX-wellWidth/2, digY, wellZ-wellWidth/2, wellX+wellWidth/2, digY, wellZ+wellWidth/2)
				_, err := inst.RCON.Exec(ctx, digCmd)
				require.NoError(t, err, "dig well at Y=%d", digY)
			}

			// Place stone floor at bottom
			floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
				wellX-wellWidth/2, wellBottomY, wellZ-wellWidth/2, wellX+wellWidth/2, wellBottomY, wellZ+wellWidth/2)
			_, err = inst.RCON.Exec(ctx, floorCmd)
			require.NoError(t, err, "place well floor")

			// Fill well with water from floor up to ground level
			for waterY := wellBottomY + 1; waterY < groundY; waterY++ {
				waterCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					wellX-wellWidth/2, waterY, wellZ-wellWidth/2, wellX+wellWidth/2, waterY, wellZ+wellWidth/2)
				_, err = inst.RCON.Exec(ctx, waterCmd)
				require.NoError(t, err, "fill well at Y=%d", waterY)
			}

			// Teleport agent to the bottom of the well
			tpX := float64(wellX) + 0.5
			tpZ := float64(wellZ) + 0.5
			tpY := float64(wellBottomY + 1)
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", agentCfg.Name, tpX, tpY, tpZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to well bottom")
			t.Logf("Agent teleported to well bottom (%.1f, %.1f, %.1f)", tpX, tpY, tpZ)

			time.Sleep(3 * time.Second)

			// Goal: on the ground away from the well
			goalX := float64(wellX) + 5
			goalY := float64(groundY)     // At ground level
			goalZ := float64(wellZ) + 3.0 // Offset from well center
			t.Logf("Goal: (%.1f, %.1f, %.1f) — swim up from well bottom to ground level",
				goalX, goalY, goalZ)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond).Start(ctx)
			defer tracker.Stop()

			// Navigate using pathfinding
			// Give MoveTo a reasonable timeout to avoid hanging indefinitely
			moveCtx, moveCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			err = agent.Agent.MoveTo(moveCtx, goalX, goalY, goalZ, true)
			moveCancel()
			if err != nil {
				t.Logf("MoveTo returned error: %v", err)
			}
			t.Logf("MoveTo completed")

			t.Logf("Waiting for agent %s to reach target position (%.2f %.2f %.2f)", agent.Name, goalX, goalY, goalZ)

			// Wait for agent to reach destination
			timeout := 90 * time.Second
			t.Logf("Calling WaitForPosition with %v timeout at %v", timeout, time.Now())
			err = tracker.WaitForPosition(ctx, agentCfg.Name, models.V3{X: goalX, Y: goalY, Z: goalZ}, 2.5, timeout)
			t.Logf("WaitForPosition returned at %v with error: %v", time.Now(), err)

			finalPos, posOK := tracker.GetPosition(agentCfg.Name)
			t.Logf("GetPosition called, found: %v", posOK)
			require.True(t, posOK, "should have final position")
			t.Logf("Final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

			goal := models.V3{X: goalX, Y: goalY, Z: goalZ}
			finalDist := finalPos.DistanceTo(goal)
			t.Logf("Distance from goal: %.2f", finalDist)

			assert.LessOrEqual(t, finalDist, 3.0,
				"agent should swim up from well bottom to reach the ground level goal")
		})
	}
}

// TestSwimming_PathfindingSwimDown verifies that the agent can pathfind downward
// through a deep vertical water column to reach a goal at depth. The agent must enter
// water from solid ground and use SwimDown movements to descend through multiple levels.
func TestSwimming_PathfindingSwimDown(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.MCVersion
			serverCfg.Difficulty = DifficultyPeaceful
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()

			agentCfg := DefaultAgentConfig(
				"SwimDownBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version,
				fmt.Sprintf("swimming_jump_climb_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")),
				agentCfg.Name)

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			defer func() {
				if agent != nil {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = agent.Stop(stopCtx)
					stopCancel()
					if agent.BotClient() != nil {
						_ = agent.BotClient().Close()
					}
				}
			}()

			time.Sleep(5 * time.Second)

			// Get agent position on flat world
			agentPos, posOK := agent.Agent.GetPositionSimple()
			require.True(t, posOK, "agent position initialized")

			// Flat world surface: Y=0, standing at Y=1
			groundY := int(math.Floor(agentPos.Y))
			wellX := int(math.Floor(agentPos.X)) + 5
			wellZ := int(math.Floor(agentPos.Z))

			// Create a vertical water well: 5 blocks wide, 6 blocks deep from surface
			wellWidth := 5
			wellDepth := 6
			wellBottomY := groundY - wellDepth

			t.Logf("agentPos: %s", agentPos)
			t.Logf("wellPos : (X=%d, Z=%d), groundY=%d, wellBottomY=%d", wellX, wellZ, groundY, wellBottomY)

			// Dig the well
			for digY := groundY; digY >= wellBottomY; digY-- {
				digCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
					wellX-wellWidth/2, digY, wellZ-wellWidth/2, wellX+wellWidth/2, digY, wellZ+wellWidth/2)
				_, err := inst.RCON.Exec(ctx, digCmd)
				require.NoError(t, err, "dig well at Y=%d", digY)
			}

			// Place stone floor with goal marker at bottom
			floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
				wellX-wellWidth/2, wellBottomY, wellZ-wellWidth/2, wellX+wellWidth/2, wellBottomY, wellZ+wellWidth/2)
			_, err = inst.RCON.Exec(ctx, floorCmd)
			require.NoError(t, err, "place well floor")

			// Fill well with water from floor up to ground level
			for waterY := wellBottomY + 1; waterY < groundY; waterY++ {
				waterCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water",
					wellX-wellWidth/2, waterY, wellZ-wellWidth/2, wellX+wellWidth/2, waterY, wellZ+wellWidth/2)
				_, err = inst.RCON.Exec(ctx, waterCmd)
				require.NoError(t, err, "fill well at Y=%d", waterY)
			}

			time.Sleep(3 * time.Second)

			// Start position: on solid ground next to the well
			startX := float64(wellX-wellWidth/2-2) + 0.5
			startY := float64(groundY + 1)
			startZ := float64(wellZ) + 0.5
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", agentCfg.Name, startX, startY, startZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to start position")
			t.Logf("Agent teleported to start (%.1f, %.1f, %.1f)", startX, startY, startZ)

			time.Sleep(3 * time.Second)

			// Goal: at the bottom of the well
			goalX := float64(wellX) + 0.5
			goalY := float64(wellBottomY + 1)
			goalZ := float64(wellZ) + 0.5
			t.Logf("Goal: (%.1f, %.1f, %.1f) — swim down into well from solid ground",
				goalX, goalY, goalZ)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Navigate using pathfinding
			err = agent.Agent.MoveTo(ctx, goalX, goalY, goalZ, true)
			if err != nil {
				t.Logf("MoveTo returned error: %v", err)
			}

			// Wait for agent to reach destination
			timeout := 90 * time.Second
			err = tracker.WaitForPosition(ctx, agentCfg.Name, models.V3{X: goalX, Y: goalY, Z: goalZ}, 2.5, timeout)

			finalPos, posOK := tracker.GetPosition(agentCfg.Name)
			require.True(t, posOK, "should have final position")
			t.Logf("Final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

			goal := models.V3{X: goalX, Y: goalY, Z: goalZ}
			finalDist := finalPos.DistanceTo(goal)
			t.Logf("Distance from goal: %.2f", finalDist)

			assert.LessOrEqual(t, finalDist, 3.0,
				"agent should swim down from ground level into the well to reach the goal")
		})
	}
}
