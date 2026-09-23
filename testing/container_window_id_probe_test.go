package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestWindowIDLimitProbe empirically re-measures the "~6-7 container
// window-ID limit per connection" constraint documented in
// container_standalone_test.go. That number was never re-measured after two directly
// relevant container-lifecycle fixes landed in agent/ (7f26b84 "Fix
// TestAnvil panic: prevent double-closing containers" and 4873559 "fix
// entity container race conditions") - either could mean containers now
// close (and free their window ID) more reliably than whatever produced
// the original estimate.
//
// This opens and closes the SAME chest, on a SINGLE agent connection,
// probeIterations times in a row - well past the old ~6-7 estimate - and
// records the first failure (if any). A clean run of all iterations means
// the ~6-7 number is stale and at least some container-bound files are
// safe to convert to the shared-server pattern; a failure around the old
// estimate means it's still a real constraint.
func TestWindowIDLimitProbe(t *testing.T) {
	const probeIterations = 25

	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "chest", tt.MCVersion)
			defer env.Cancel()

			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err, "teleport near chest")
			time.Sleep(500 * time.Millisecond)

			lastGoodWindowID := -1
			for i := 1; i <= probeIterations; i++ {
				windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceEast, 5*time.Second)
				if err != nil {
					t.Logf("open #%d FAILED after %d clean open/close cycles (last good window ID %d): %v", i, i-1, lastGoodWindowID, err)
					require.NoError(t, err, "open chest iteration %d", i)
				}
				t.Logf("open #%d succeeded with window ID %d", i, windowID)
				lastGoodWindowID = int(windowID)

				err = env.Agent.Agent.CloseContainer()
				require.NoError(t, err, "close chest iteration %d", i)
			}

			t.Logf("all %d open/close cycles succeeded on a single connection (last window ID %d)", probeIterations, lastGoodWindowID)
		})
	}
}
