package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBowPowerComparison compares actual arrow behavior between FireBowAt and FireBowAtFullPower
// to determine if the discrepancy in long-range shots is due to bow power not reaching max
func TestBowPowerComparison(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.mcVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot",
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

			// Let agent join
			time.Sleep(2 * time.Second)

			// Give equipment
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			// Teleport to test location
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`teleport %s 0 100 0`, ag.Name))
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			t.Logf("[%s] Starting bow power comparison tests", tt.mcVersion)

			// Test at various distances
			testCases := []struct {
				distance float64
				label    string
			}{
				{10, "10m"},
				{20, "20m"},
				{30, "30m"},
				{40, "40m"},
			}

			for _, tc := range testCases {
				tc := tc
				t.Run(tc.label, func(t *testing.T) {
					botX, botY, botZ, ok := ag.Agent.GetPositionSimple()
					require.True(t, ok, "bot position initialized")

					targetX := botX + tc.distance
					targetY := botY
					targetZ := botZ

					t.Logf("[%s] Testing at distance %.1f: target=(%.1f, %.1f, %.1f)",
						tc.label, tc.distance, targetX, targetY, targetZ)

					// Fire using standard FireBowAt
					t.Logf("  Firing with FireBowAt...")
					err = ag.Agent.FireBowAt(targetX, targetY, targetZ)
					require.NoError(t, err)

					time.Sleep(3 * time.Second)

					// Analyze trajectory
					actualStandard, err := AnalyzeArrowTrajectory(inst.AgentLogFile)
					if err != nil {
						t.Logf("    Warning: Could not analyze standard trajectory: %v", err)
					} else {
						t.Logf("    Standard: actual=%d points",
							len(actualStandard))
						if len(actualStandard) > 0 {
							lastPos := actualStandard[len(actualStandard)-1]
							distTraveled := math.Sqrt(
								(lastPos.X-botX)*(lastPos.X-botX) +
									(lastPos.Z-botZ)*(lastPos.Z-botZ))
							t.Logf("    Standard: arrow traveled %.1f blocks (predicted %.1f)",
								distTraveled, tc.distance)
						}
					}

					time.Sleep(2 * time.Second)

					// Fire using FireBowAtFullPower
					t.Logf("  Firing with FireBowAtFullPower...")
					_, err = ag.Agent.FireBowAtFullPower(targetX, targetY, targetZ)
					require.NoError(t, err)

					time.Sleep(3 * time.Second)

					// Analyze trajectory
					actualFullPower, err := AnalyzeArrowTrajectory(inst.AgentLogFile)
					if err != nil {
						t.Logf("    Warning: Could not analyze full power trajectory: %v", err)
					} else {
						t.Logf("    FullPower: actual=%d points",
							len(actualFullPower))
						if len(actualFullPower) > 0 {
							lastPos := actualFullPower[len(actualFullPower)-1]
							distTraveled := math.Sqrt(
								(lastPos.X-botX)*(lastPos.X-botX) +
									(lastPos.Z-botZ)*(lastPos.Z-botZ))
							t.Logf("    FullPower: arrow traveled %.1f blocks (predicted %.1f)",
								distTraveled, tc.distance)
						}
					}

					time.Sleep(2 * time.Second)
				})
			}
		})
	}
}
