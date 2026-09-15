package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// This file replaces the old TestAlgorithmPerformanceComparison (a single-shot
// t.Run timing pass over 100x100 flat ground) with real testing.B benchmarks
// over a deterministic, feature-rich course. Flat ground only ever exercises
// Traverse/DiagonalTraverse; the course below also forces AscendStairs,
// DescendStairs, Descend, Jump2, WadeWater/Swim, and Climb/EnterClimb/
// ExitClimb, so the comparison actually reflects each algorithm's search
// behavior instead of terrain-independent overhead. See
// docs/PATHFINDING_BENCHMARKS.md for how to run and interpret it.

const (
	courseBaseY = 64.0 // ground level (block Y); feet stand at courseBaseY+1
	// courseZMin/courseZMax span the *entire* corridor width. Every "full-width"
	// feature below must cover this whole range with no untouched column -
	// otherwise DiagonalTraverse (cost 0.9, cheaper than Traverse's 1.0) makes
	// a zigzag along any permanently flat, unobstructed column strictly
	// cheaper than engaging with the terrain, and the benchmark degenerates
	// back into the flat-ground case it's meant to replace.
	courseZMin    = 0.0
	courseZMax    = 7.0
	moduleSpacing = 32.0 // budget per module; every module resolves well within it
	moduleLeadIn  = 4.0  // flat run-up before the first module
	moduleCount   = 6
)

// moduleStart returns the X coordinate where feature module `index` begins.
func moduleStart(index int) float64 {
	return moduleLeadIn + float64(index)*moduleSpacing
}

// benchmarkCourse bundles the constructed world with start/goal pairs at
// increasing distances, so each tier crosses proportionally more feature
// modules rather than just a longer straight line.
type benchmarkCourse struct {
	world  *mctesting.MockWorld
	start  models.V3
	short  models.V3 // crosses pillars + stairs
	medium models.V3 // + gap + ledge drop
	long   models.V3 // + water crossing + ladder shaft
}

// buildBenchmarkCourse lays out a long corridor from six chained, deterministic
// feature modules. It is deterministic (no math/rand) so repeated benchmark
// runs measure the same search problem.
func buildBenchmarkCourse() *benchmarkCourse {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("grass_block", nil)
	stoneID := registry.GetStateID("stone", nil)
	stairsID := registry.GetStateID("oak_stairs", nil)
	waterID := registry.GetStateID("water", nil)
	ladderID := registry.GetStateID("ladder", nil)

	corridorEndX := moduleStart(moduleCount) + 8

	// Z range matches courseZMin/courseZMax exactly (no extra margin column) so
	// no lateral strip is ever left permanently untouched by every module.
	wb := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-2, courseZMin, corridorEndX, courseZMax, courseBaseY, grassID)

	buildPillarField(wb, moduleStart(0), stoneID)
	buildStepUpDown(wb, moduleStart(1), stairsID, grassID)
	buildGapField(wb, moduleStart(2))
	buildLedgeDrop(wb, moduleStart(3), grassID, stairsID)
	buildWaterCrossing(wb, moduleStart(4), waterID)
	buildLadderShaft(wb, moduleStart(5), grassID, ladderID)

	return &benchmarkCourse{
		world:  wb.Build(),
		start:  models.V3{X: 2, Y: courseBaseY + 1, Z: 4},
		short:  models.V3{X: moduleStart(2) - 4, Y: courseBaseY + 1, Z: 4},
		medium: models.V3{X: moduleStart(4) - 4, Y: courseBaseY + 1, Z: 4},
		long:   models.V3{X: moduleStart(6) - 4, Y: courseBaseY + 1, Z: 4},
	}
}

// buildPillarField scatters isolated two-block-tall stone pillars across the
// corridor width, forcing every algorithm to route around real local
// obstacles instead of following a single straight or diagonal line.
func buildPillarField(wb *mctesting.WorldBuilder, xStart float64, stoneID uint32) {
	pillars := []struct{ dx, z float64 }{
		{4, 2}, {8, 5}, {12, 3}, {16, 6}, {20, 2}, {24, 5},
	}
	for _, p := range pillars {
		x := xStart + p.dx
		wb.WallDirect(x, p.z, x, p.z, courseBaseY+1, courseBaseY+2, stoneID)
	}
}

// buildStairRampFullWidth lays a parallel staircase across every z line in
// [zFrom, zTo], since WorldBuilder.StairsDirect only builds a single-file
// line. Walked in the increasing-X direction the ramp ascends; walked in the
// decreasing-X direction (by placing it with a "west"-style direction) it
// descends, per StairsDirect's own indexing.
func buildStairRampFullWidth(wb *mctesting.WorldBuilder, startX, startY, zFrom, zTo float64, length int, direction string, stateID uint32) {
	for z := zFrom; z <= zTo; z++ {
		wb.StairsDirect(startX, startY, z, length, direction, stateID)
	}
}

// buildStepUpDown climbs three stair steps to an elevated platform and back
// down, forcing AscendStairs and DescendStairs.
func buildStepUpDown(wb *mctesting.WorldBuilder, xStart float64, stairsID, grassID uint32) {
	ascendX := xStart + 2
	buildStairRampFullWidth(wb, ascendX, courseBaseY+1, courseZMin, courseZMax, 3, "east", stairsID)

	platformY := courseBaseY + 3
	wb.FlatGroundDirect(xStart+5, courseZMin, xStart+11, courseZMax, platformY, grassID)

	descendX := xStart + 14
	buildStairRampFullWidth(wb, descendX, courseBaseY+1, courseZMin, courseZMax, 3, "west", stairsID)
}

// buildGapField carves a one-block-wide trench across most of the corridor
// width, leaving a single bridge column, so the algorithms must weigh a cheap
// Jump2 hop against a longer detour across the bridge. The trench is exactly
// one block wide because CanJump2 only offers an exact two-block hop.
func buildGapField(wb *mctesting.WorldBuilder, xStart float64) {
	trenchX := xStart + 12
	const bridgeZ = 3.0
	for z := courseZMin; z <= courseZMax; z++ {
		if z == bridgeZ {
			continue
		}
		wb.Gap(trenchX, z, courseBaseY, 1)
	}
}

// buildLedgeDrop drops two blocks onto a lower shelf (forcing Descend) and
// climbs back to the baseline via a short stair ramp (forcing AscendStairs),
// so the module is self-contained and the next module can assume baseline
// elevation.
func buildLedgeDrop(wb *mctesting.WorldBuilder, xStart float64, grassID, stairsID uint32) {
	shelfXFrom := xStart + 8
	shelfXTo := xStart + 13
	shelfY := courseBaseY - 2

	climbX := shelfXTo + 1
	const climbLength = 2
	climbXEnd := climbX + climbLength - 1

	// Remove the baseline floor over the shelf AND the climb ramp's footprint
	// (not just the shelf) - otherwise the untouched baseline Y=64 layer sits
	// exactly where the ramp needs airspace for its higher steps, and
	// CanAscendStairs's passability check fails silently, stranding the agent
	// on the shelf. Then lay the shelf's own lower floor.
	wb.WallDirect(shelfXFrom, courseZMin, climbXEnd, courseZMax, courseBaseY, courseBaseY, 0)
	wb.FlatGroundDirect(shelfXFrom, courseZMin, shelfXTo, courseZMax, shelfY, grassID)

	buildStairRampFullWidth(wb, climbX, shelfY+1, courseZMin, courseZMax, climbLength, "east", stairsID)
}

// buildWaterCrossing floods a strip spanning the entire corridor width, so
// there's no dry column to sidestep it - crossing requires WadeWater/Swim.
// (A narrow, nearby dry detour - as in
// TestNarrowRiverCrossing_PrefersDirectCrossingOverDetour - is cheaper than
// wading via DiagonalTraverse's 0.9 cost, so any detour close enough to be
// worth offering ends up always winning and WadeWater/Swim never fire.)
func buildWaterCrossing(wb *mctesting.WorldBuilder, xStart float64, waterID uint32) {
	waterXFrom := xStart + 8
	waterXTo := xStart + 11
	wb.WaterDirect(waterXFrom, courseBaseY+1, courseZMin, waterXTo, courseBaseY+1, courseZMax, waterID)
}

// buildLadderShaft gates a raised platform behind a ladder with no parallel
// stair route, and removes the baseline floor under the whole footprint -
// otherwise the platform is purely decorative, since the agent could just
// keep walking underneath it at baseline level. Crossing this module
// therefore requires Climb/EnterClimb/ExitClimb; the far edge drops back to
// baseline via a plain Descend.
func buildLadderShaft(wb *mctesting.WorldBuilder, xStart float64, grassID, ladderID uint32) {
	ladderX := xStart + 6
	platformXFrom := ladderX + 1
	platformXTo := platformXFrom + 9
	platformY := courseBaseY + 3

	wb.WallDirect(ladderX, courseZMin, platformXTo, courseZMax, courseBaseY, courseBaseY, 0)
	wb.FlatGroundDirect(platformXFrom, courseZMin, platformXTo, courseZMax, platformY, grassID)

	const ladderZ = 3.0
	wb.LadderDirect(ladderX, ladderZ, courseBaseY+1, platformY, ladderID)
}

// BenchmarkPathfinding compares A*, EPEA*, and Bidirectional A* over the
// deterministic course above at three distance tiers. World construction and
// PathFinder creation happen outside the timed loop; all three PathFinder
// implementations allocate fresh open/closed sets per FindPath call, so
// reusing one instance across b.Loop iterations doesn't bias the comparison.
func BenchmarkPathfinding(b *testing.B) {
	course := buildBenchmarkCourse()
	shapeMgr := mctesting.NewMockShapeManager()

	tiers := []struct {
		name     string
		goal     models.V3
		maxSteps int
	}{
		{"Short", course.short, 20000},
		{"Medium", course.medium, 60000},
		{"Long", course.long, 150000},
	}

	for _, tier := range tiers {
		for algoName, factory := range algorithmTestSuite {
			b.Run(tier.name+"/"+algoName, func(b *testing.B) {
				pathFinder := factory(course.world, shapeMgr)

				var steps int
				var cost float64
				for b.Loop() {
					path, err := pathFinder.FindPath(context.Background(), course.start, tier.goal, tier.maxSteps)
					if err != nil {
						b.Fatalf("%s failed to find path: %v", algoName, err)
					}
					steps = len(path.Steps)
					cost = path.TotalCost
				}

				b.ReportMetric(float64(steps), "steps/op")
				b.ReportMetric(cost, "cost/op")
			})
		}
	}
}

// TestComplexCourseCorrectness guards the benchmark course: if a future edit
// makes it unsolvable for any algorithm, this fails loudly instead of the
// benchmark silently reporting "path not found" errors.
func TestComplexCourseCorrectness(t *testing.T) {
	course := buildBenchmarkCourse()
	shapeMgr := mctesting.NewMockShapeManager()

	goals := map[string]models.V3{
		"Short":  course.short,
		"Medium": course.medium,
		"Long":   course.long,
	}

	for goalName, goal := range goals {
		for algoName, factory := range algorithmTestSuite {
			t.Run(goalName+"/"+algoName, func(t *testing.T) {
				pathFinder := factory(course.world, shapeMgr)

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				path, err := pathFinder.FindPath(ctx, course.start, goal, 200000)
				if err != nil {
					t.Fatalf("%s failed to find path to %s: %v", algoName, goalName, err)
				}
				if path == nil || !path.Found || len(path.Steps) == 0 {
					t.Fatalf("%s did not find a usable path to %s", algoName, goalName)
				}

				moveCounts := map[string]int{}
				for _, step := range path.Steps {
					moveCounts[step.Movement.String()]++
				}
				t.Logf("%s -> %s: %d steps, cost=%.2f, moves=%v", algoName, goalName, len(path.Steps), path.TotalCost, moveCounts)
			})
		}
	}
}
