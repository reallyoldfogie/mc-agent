package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ContainerRepeatCreativeSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for TestRepeatedContainerOpen below: one server per version instead
// of the previous per-test-function StartServer/StopServer pattern. GameMode = creative, matching
// the pre-conversion function's own inline DefaultServerConfig() override exactly.
//
// This diagnostic (repeatedly opening/closing the same chest to see where the server stops
// accepting opens) is now largely superseded by TestWindowIDLimitProbe
// (container_window_id_probe_test.go, added for
// docs/plans/integration-test-shared-server/31-window-id-limit-remeasurement.md), which does the
// same thing but with real assertions (require.NoError per cycle) and more iterations (25 vs 15);
// this file's own loop only logs a failure and breaks, never actually failing the test regardless
// of how many attempts succeeded. Converted anyway per this plan's own file-by-file checklist
// rather than unilaterally deleted - that's a judgment call for whoever owns this test suite, not
// something to fold into a mechanical port.
type ContainerRepeatCreativeSuite struct {
	VersionWorldSuite
}

func TestContainerRepeatCreativeSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerRepeatCreativeSuite{}
		s.WorldGen = WorldGenFlat
		s.GameMode = GameModeCreative
		return s
	})
}

// TestRepeatedContainerOpen tests opening the same container multiple times.
// This isolates whether the issue is with:
// - Opening different container types
// - Moving between positions
// - Server-side window ID exhaustion
//
// Equivalent to the pre-Phase-1 TestRepeatedContainerOpen.
func (s *ContainerRepeatCreativeSuite) TestRepeatedContainerOpen() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("RepeatBot", "repeated_container_open")
	require.NoError(t, err, "spawn agent")

	screenMgr := leader.ScreenManager()

	chestX := int(math.Floor(leader.Origin.X)) + 5
	chestY := int(math.Floor(leader.Origin.Y))
	chestZ := int(math.Floor(leader.Origin.Z))

	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, models.V3{X: float64(chestX), Y: float64(chestY), Z: float64(chestZ)}, "minecraft:chest", "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest")
	t.Logf("placed chest at (%d, %d, %d)", chestX, chestY, chestZ)

	t.Log("waiting for chunks to load...")
	time.Sleep(3 * time.Second)

	chestPos := models.V3{
		X: float64(chestX) + 0.5,
		Y: float64(chestY),
		Z: float64(chestZ) + 0.5,
	}

	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, chestPos.X-2, chestPos.Y, chestPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport to chest")
	t.Logf("teleported to (%.1f, %.1f, %.1f)", chestPos.X-2, chestPos.Y, chestPos.Z)
	time.Sleep(1 * time.Second)

	// Attempt to open and close the SAME chest 15 times.
	const numAttempts = 15
	successCount := 0

	for i := 1; i <= numAttempts; i++ {
		t.Logf("\n=== Attempt %d/%d ===", i, numAttempts)

		screensBefore := len(screenMgr.Screens())
		t.Logf("Screens before open: %d %v", screensBefore, getScreenIDs(screenMgr.Screens()))

		windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, chestPos, models.FaceEast, 5*time.Second)
		if err != nil {
			t.Logf("❌ Attempt %d FAILED to open: %v", i, err)
			t.Logf("   Server stopped responding after %d successful opens", successCount)
			break
		}

		successCount++
		t.Logf("✅ Attempt %d: Opened chest with window ID %d", i, windowID)

		screen, ok := screenMgr.Screens()[int(windowID)]
		require.True(t, ok, "chest window should exist")

		chest, ok := screen.(*mcscreen.Chest)
		require.True(t, ok, "screen should be a chest")
		require.Equal(t, 3, chest.Rows, "should be single chest (3 rows)")

		err = leader.Agent.CloseContainer()
		require.NoError(t, err, "close chest")
		t.Logf("   Closed window ID %d", windowID)

		screensAfter := len(screenMgr.Screens())
		t.Logf("   Screens after close: %d %v", screensAfter, getScreenIDs(screenMgr.Screens()))

		pauseDuration := 500 * time.Millisecond
		if i < numAttempts {
			t.Logf("   Pausing %v before next attempt...", pauseDuration)
			time.Sleep(pauseDuration)
		}
	}

	t.Logf("\n=== SUMMARY ===")
	t.Logf("Attempted: %d", numAttempts)
	t.Logf("Succeeded: %d", successCount)
	t.Logf("Failed:    %d", numAttempts-successCount)

	if successCount < numAttempts {
		t.Logf("\n⚠️  Server stopped accepting container opens after %d attempts", successCount)
	} else {
		t.Logf("\n✅ All %d attempts succeeded!", numAttempts)
	}
}
