# Pathfinding Algorithm Benchmarks

`pathfinding/algorithm_benchmark_test.go` compares A*, EPEA*, and Bidirectional
A* using real `testing.B` benchmarks over a single, deterministic obstacle
course, replacing the old `TestAlgorithmPerformanceComparison` (a single-shot
`t.Run` timing pass over 100x100 flat ground).

## Why not flat ground

Flat ground only ever exercises `Traverse`/`DiagonalTraverse`. It can't show
where EPEA*'s partial expansion or bidirectional's meet-in-the-middle search
actually pay off, and a single untimed `t.Run` pass is dominated by OS
scheduling/GC noise rather than algorithmic cost.

## The course

`buildBenchmarkCourse` chains six feature modules into one corridor, each
forcing a different, otherwise-rare move type:

- **Pillar field** - scattered obstacles forcing detours.
- **Step up/down** - a staircase bump (`AscendStairs`/`DescendStairs`).
- **Gap field** - a one-block trench with a single bridge column, forcing a
  real `Jump2`-vs-detour cost trade-off.
- **Ledge drop** - a two-block drop and climb-back (`Descend` +
  `AscendStairs`).
- **Water crossing** - a strip spanning the full corridor width, forcing
  `WadeWater`/`Swim` (no dry bypass, or the detour always wins - see the
  comment on `buildWaterCrossing`).
- **Ladder shaft** - a platform reachable *only* via a ladder, with the
  baseline floor removed underneath it, forcing
  `Climb`/`EnterClimb`/`ExitClimb` (see `buildLadderShaft`).

Each feature spans the *entire* corridor width. A narrower feature leaves a
permanently flat, untouched column that a search can always follow, and since
`DiagonalTraverse` (cost 0.9) is cheaper than `Traverse` (cost 1.0), that
column becomes a free zigzag bypass that defeats the terrain entirely - this
is what happened during development and is called out in the constant block's
comment.

`Short`/`Medium`/`Long` tiers reuse the same world and pick goals at
increasing distances, so each tier crosses proportionally more modules.

## Running it

```
go test ./pathfinding/... -bench=BenchmarkPathfinding -benchmem
```

Add `-benchtime=Nx` for a fixed iteration count (useful for quick checks), or
`-count=N` with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
across runs when comparing algorithm changes - a single run's `ns/op` is not
a reliable A-vs-B signal by itself.

Reported metrics per sub-benchmark (e.g. `BenchmarkPathfinding/Long/EPEA*`):

- `ns/op`, `B/op`, `allocs/op` - standard Go benchmark output.
- `cost/op` - the found path's total movement cost (lower is better, and
  should be similar across algorithms for the same tier - large divergence
  suggests one algorithm found a worse path, not just a slower search).
- `steps/op` - the number of steps in the found path (a rough proxy for path
  quality, not search effort).

## Correctness guard

`TestComplexCourseCorrectness` runs all three algorithms against the same
three tiers and asserts each finds a usable path. Run it whenever the course
geometry changes:

```
go test ./pathfinding/... -run TestComplexCourseCorrectness -v
```

It logs each path's step count, cost, and a movement-type histogram, so you
can confirm a course edit still exercises the intended move types.
