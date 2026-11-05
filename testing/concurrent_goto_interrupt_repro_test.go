package testing

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ConcurrentGotoInterruptReproSuite is a load/regression check for the
// SetMoveInterruptCheck/withMoveInterrupt mechanism (agent/commands.go,
// rlenv.MoveInterruptSetter), not a realistic training simulation: each
// bot redispatches ActionGoToTarget on literally every Step, which
// cancelInflight already tears down and restarts regardless of this
// feature - a real trained policy doesn't hammer one action every step
// this way, so this test's own absolute timings/completion rate aren't
// meaningful on their own (confirmed live, 2026-11-04: even v0.0.63, with
// no interrupt mechanism at all, is slow under this exact pattern - see
// this test's own git history for that baseline run's numbers). What IS
// meaningful, and what this is actually for: whether several bots'
// concurrent polling goroutines cause the whole thing to hang or
// drastically regress relative to that baseline - exactly what v0.0.64's
// original 250ms-interval, full-MineSearchRadius check did (reproduced
// here: 8 bots never finished 15 dispatches each within 4-5 minutes),
// and what the edge-triggered, moveInterruptCheckRadius-scoped fix
// resolved (same 8 bots, same 15 dispatches: completes in ~3.5 minutes,
// comparable to the no-interrupt baseline). Re-run this after any future
// change to moveInterruptPollInterval or moveInterruptCheckRadius.
type ConcurrentGotoInterruptReproSuite struct {
	VersionWorldSuite
}

func TestConcurrentGotoInterruptReproSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &ConcurrentGotoInterruptReproSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

type botGotoResult struct {
	steps    int
	maxDur   time.Duration
	timeouts int
	done     bool
	err      error
}

func (s *ConcurrentGotoInterruptReproSuite) TestManyConcurrentBuriedGotoDispatches() {
	t := s.T()
	const numBots = 8
	const dispatchesPerBot = 15
	const stepTimeout = 10 * time.Second

	results := make([]botGotoResult, numBots)
	var wg sync.WaitGroup

	for i := 0; i < numBots; i++ {
		leader, err := s.SpawnWorkingAreaAgentNoCam(fmt.Sprintf("ConcBot%d", i), "concurrent_goto_repro")
		require.NoError(t, err, "spawn bot %d", i)

		startX := int(math.Floor(leader.Origin.X))
		startY := int(math.Floor(leader.Origin.Y))
		startZ := int(math.Floor(leader.Origin.Z))
		oreX, oreY, oreZ := startX+6, startY-3, startZ
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:coal_ore", oreX, oreY, oreZ))
		require.NoError(t, err, "place buried ore for bot %d", i)

		liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
		require.True(t, ok, "bot %d must satisfy rlenv.LiveAgent", i)

		cfg := rlenv.Config{
			ArrivalThreshold:       1.5,
			StepTimeout:            stepTimeout,
			MineTargetBlock:        "minecraft:coal_ore",
			MineSearchRadius:       12,
			MineRequiredTool:       "minecraft:wooden_pickaxe",
			MineTargetBuried:       true,
			MineBuriedExploreDepth: 4,
			GoToTargetDisabled:     true,
			CollectDrops:           true,
			Seeder:                 rlenv.DefaultEpisodeSeeder,
		}
		env, err := rlenv.New(liveAgent, actions.NewRegistry(), cfg)
		require.NoError(t, err, "construct env for bot %d", i)
		_, err = env.Reset(s.Ctx)
		require.NoError(t, err, "Reset bot %d", i)

		wg.Add(1)
		go func(idx int, env *rlenv.Environment) {
			defer wg.Done()
			res := botGotoResult{}
			for j := 0; j < dispatchesPerBot; j++ {
				start := time.Now()
				r, stepErr := env.Step(s.Ctx, rlenv.ActionGoToTarget)
				dur := time.Since(start)
				res.steps++
				if dur > res.maxDur {
					res.maxDur = dur
				}
				if stepErr != nil {
					res.err = stepErr
					break
				}
				if dur >= stepTimeout {
					res.timeouts++
				}
				if r.Done {
					res.done = true
					break
				}
			}
			results[idx] = res
		}(i, env)
	}

	wg.Wait()

	for i, r := range results {
		t.Logf("bot %d: steps=%d maxDur=%v timeouts=%d done=%v err=%v", i, r.steps, r.maxDur, r.timeouts, r.done, r.err)
	}
}
