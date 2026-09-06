package agent

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/reallyoldfogie/mc-agent/models"
)

type Ray struct {
	Origin    models.V3
	Direction models.V3
}

func block2Chunk(blockX, blockZ int64) (chunkX, chunkZ int64) {
	chunkX = (int64)(math.Floor(float64(blockX) / 16))
	chunkZ = (int64)(math.Floor(float64(blockZ) / 16))

	return
}

// isObscuring checks if a block at the given coordinate is solid and blocks visibility
func (a *agent) isObscuring(coord models.V3, world models.World) bool {
	stateID, loaded := world.GetBlockAt(coord.X, coord.Y, coord.Z)
	if stateID == 0 {
		// chunkX, chunkZ := block2Chunk(int64(math.Floor(coord.X)), int64(math.Floor(coord.Z)))
		// a.logf("[isObscuring] Block at (%.2f, %.2f, %.2f) chunk (%d, %d)) is air", coord.X, coord.Y, coord.Z, chunkX, chunkZ)
		return false
	}

	if !loaded {
		// chunkX, chunkZ := block2Chunk(int64(math.Floor(coord.X)), int64(math.Floor(coord.Z)))
		// a.logf("[isObscuring] Block at (%.2f, %.2f, %.2f) chunk (%d, %d)) is not loaded", coord.X, coord.Y, coord.Z, chunkX, chunkZ)
		return false
	}

	// a.logf("[isObscuring] Block at (%.2f, %.2f, %.2f) has stateID %d (%s)", coord.X, coord.Y, coord.Z, stateID, a.shapeMgr.BlockName(stateID))

	// Skip passable blocks (water, plants, etc.)
	if a.shapeMgr != nil && a.shapeMgr.IsPassable(stateID) {
		return false
	}

	// Block is solid and obscures visibility
	return true
}

// traceRay finds the first obscuring block along a ray within radius R
// Returns (blockCoord, found)
func (a *agent) traceRay(ray Ray, R float64, world models.World) (models.V3, bool) {
	// a.logf("[traceRay] Tracing ray from (%.2f, %.2f, %.2f) in direction (%.4f, %.4f, %.4f)", ray.Origin.X, ray.Origin.Y, ray.Origin.Z, ray.Direction.X, ray.Direction.Y, ray.Direction.Z)

	// Current voxel coordinates
	x, y, z := int(math.Floor(ray.Origin.X)), int(math.Floor(ray.Origin.Y)), int(math.Floor(ray.Origin.Z))

	stepX, stepY, stepZ := signInt(ray.Direction.X), signInt(ray.Direction.Y), signInt(ray.Direction.Z)

	// Distance to next boundary
	var tDeltaX, tMaxX float64
	if ray.Direction.X == 0 {
		tDeltaX = math.Inf(1)
		tMaxX = math.Inf(1)
	} else {
		tDeltaX = 1.0 / math.Abs(ray.Direction.X)
		if ray.Direction.X > 0 {
			tMaxX = (float64(x) + 1.0 - ray.Origin.X) / ray.Direction.X
		} else {
			tMaxX = (ray.Origin.X - float64(x)) / (-ray.Direction.X)
		}
	}

	var tDeltaY, tMaxY float64
	if ray.Direction.Y == 0 {
		tDeltaY = math.Inf(1)
		tMaxY = math.Inf(1)
	} else {
		tDeltaY = 1.0 / math.Abs(ray.Direction.Y)
		if ray.Direction.Y > 0 {
			tMaxY = (float64(y) + 1.0 - ray.Origin.Y) / ray.Direction.Y
		} else {
			tMaxY = (ray.Origin.Y - float64(y)) / (-ray.Direction.Y)
		}
	}

	var tDeltaZ, tMaxZ float64
	if ray.Direction.Z == 0 {
		tDeltaZ = math.Inf(1)
		tMaxZ = math.Inf(1)
	} else {
		tDeltaZ = 1.0 / math.Abs(ray.Direction.Z)
		if ray.Direction.Z > 0 {
			tMaxZ = (float64(z) + 1.0 - ray.Origin.Z) / ray.Direction.Z
		} else {
			tMaxZ = (ray.Origin.Z - float64(z)) / (-ray.Direction.Z)
		}
	}

	for {
		// Check distance
		coord := models.V3{X: float64(x), Y: float64(y), Z: float64(z)}
		distToOrigin := coord.DistanceTo(ray.Origin)
		if distToOrigin > R {
			return models.V3{}, false
		}

		// Hit detection
		if a.isObscuring(coord, world) {
			return coord, true
		}

		// a.logf("[traceRay] Stepping from voxel (%d, %d, %d), tMaxX %.02f, tMaxY %.02f, tMaxZ %.02f, distance %.2f", x, y, z, tMaxX, tMaxY, tMaxZ, distToOrigin)
		// Step to next voxel - handle ties by stepping all axes at the minimum tMax
		minT := math.Min(math.Min(tMaxX, tMaxY), tMaxZ)
		const epsilon = 1e-6
		if math.Abs(tMaxX-minT) < epsilon {
			x += stepX
			tMaxX += tDeltaX
		}
		if math.Abs(tMaxY-minT) < epsilon {
			y += stepY
			tMaxY += tDeltaY
		}
		if math.Abs(tMaxZ-minT) < epsilon {
			z += stepZ
			tMaxZ += tDeltaZ
		}
		// a.logf("[traceRay] Stepping to voxel (%d, %d, %d), tMaxX %.02f, tMaxY %.02f, tMaxZ %.02f, distance %.2f", x, y, z, tMaxX, tMaxY, tMaxZ, distToOrigin)
	}
}

func signInt(f float64) int {
	if f > 0 {
		return 1
	}
	if f < 0 {
		return -1
	}
	return 0
}

// getSurfaceTargets generates uniformly distributed points on a SPHERE of radius R,
// centered at the given center position.
//
// SPHERE-BASED SAMPLING: Instead of using a cube shell (which has gaps and can miss blocks),
// we use the Fibonacci sphere algorithm to generate uniformly distributed points on an actual sphere.
// This ensures better coverage in all directions and avoids missing visible blocks that fall
// between the cube shell's faces.
//
// The Fibonacci sphere algorithm is elegant and efficient:
// - Uses the golden angle (based on the golden ratio) to spiral around the sphere
// - Provides O(1) generation time per point
// - Naturally produces uniform distribution without clustering
//
// CRITICAL: The sphere is centered at the given agent position, not at world origin (0,0,0).
// This ensures rays are cast in all directions around the agent, not biased toward a fixed point.
func getSurfaceTargets(R int, centerX, centerY, centerZ float64) []models.V3 {
	// Estimate number of samples: proportional to surface area of sphere (4*pi*R^2)
	// For R=32, this gives ~25,000 samples, matching the old cube shell density
	numSamples := int(float64(R*R) * 25)
	targets := make([]models.V3, 0, numSamples)

	goldenAngle := math.Pi * (1.0 + math.Sqrt(5.0)) // golden angle in radians

	for i := range numSamples {
		// Latitude angle: distribute from top (-pi/2) to bottom (pi/2)
		// Using 1 - 2*i/N to go from -1 to +1, then acos to get angle
		theta := math.Acos(1.0 - 2.0*float64(i)/float64(numSamples))

		// Longitude angle: use golden angle for spiral distribution
		phi := goldenAngle * float64(i)

		// Convert spherical coordinates to Cartesian on unit sphere
		// Then scale by radius R and translate to center position
		sinTheta := math.Sin(theta)
		x := centerX + sinTheta*math.Cos(phi)*float64(R)
		y := centerY + math.Cos(theta)*float64(R)
		z := centerZ + sinTheta*math.Sin(phi)*float64(R)

		targets = append(targets, models.V3{X: x, Y: y, Z: z})
	}

	return targets
}

// calculateDirection returns a normalized direction vector from origin to target.
func calculateDirection(origin models.V3, target models.V3) models.V3 {
	// Find the raw difference (delta)
	// origin is the eye position (already includes block center offsets)
	// target is a sphere point that has already been shifted by +0.5 for block center targeting
	// (see call site: target.Add(models.V3{X: 0.5, Y: 0.5, Z: 0.5}))
	dx := target.X - origin.X
	dy := target.Y - origin.Y
	dz := target.Z - origin.Z

	// Calculate the magnitude (distance)
	magnitude := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Handle the zero-vector case (origin == target)
	if magnitude == 0 {
		return models.V3{X: 0, Y: 0, Z: 0}
	}

	// Normalize the vector
	return models.V3{
		X: dx / magnitude,
		Y: dy / magnitude,
		Z: dz / magnitude,
	}
}

// findAllVisibleSurfaceBlocksWorkerCount returns how many goroutines
// findAllVisibleSurfaceBlocks should split its raycasting across, given
// numTargets rays to trace. Each ray is an independent, read-only query
// against world/block-registry state (no shared mutable state written
// mid-trace), so this is embarrassingly parallel - capped at NumCPU since
// more workers than cores can't speed up CPU-bound raycasting, and at
// numTargets so a small sphere doesn't spin up idle goroutines.
func findAllVisibleSurfaceBlocksWorkerCount(numTargets int) int {
	workers := max(runtime.NumCPU(), 1)
	return min(workers, numTargets)
}

// findAllVisibleSurfaceBlocks orchestrates raycasting to points on a SPHERE and collects visible blocks.
// Uses sphere-based sampling (Fibonacci sphere algorithm) to uniformly sample directions at radius R,
// ensuring better coverage than cube-shell sampling and avoiding geometric gaps.
//
// The ray traces themselves run across a small worker pool (see
// findAllVisibleSurfaceBlocksWorkerCount): each worker owns a contiguous
// slice of targets and accumulates its own hits into a local slice, so
// there's no shared map/lock contention between workers - results are only
// merged, sequentially, after every worker finishes.
func (a *agent) findAllVisibleSurfaceBlocks(ctx context.Context, originBlockCoord models.V3, R int) ([]models.VisibleBlockInfo, error) {
	world := a.GetWorld()
	if world == nil {
		return nil, nil
	}

	// Convert block coords to eye position (add 0.5 for center, add eye height for Y)
	originEye := models.V3{
		X: originBlockCoord.X + 0.5,
		Y: originBlockCoord.Y + 0.5 + float64(a.getEyeHeight()),
		Z: originBlockCoord.Z + 0.5,
	}

	// Generate sphere targets centered at the agent's block position
	targets := getSurfaceTargets(R, originBlockCoord.X, originBlockCoord.Y, originBlockCoord.Z)
	if len(targets) == 0 {
		return nil, nil
	}

	numWorkers := findAllVisibleSurfaceBlocksWorkerCount(len(targets))
	chunkSize := (len(targets) + numWorkers - 1) / numWorkers

	type workerResult struct {
		hits []models.V3
		err  error
	}
	workerResults := make([]workerResult, numWorkers)
	var castCount atomic.Int64
	var wg sync.WaitGroup

	for w := range numWorkers {
		start := w * chunkSize
		if start >= len(targets) {
			break
		}
		end := min(start+chunkSize, len(targets))

		wg.Add(1)
		go func(w, start, end int) {
			defer wg.Done()
			var hits []models.V3
			for _, target := range targets[start:end] {
				if ctx.Err() != nil {
					workerResults[w] = workerResult{err: ctx.Err()}
					return
				}
				if n := castCount.Add(1); n%1000 == 0 {
					a.logf("[FindAllVisibleBlocksInSphere] Cast %d/%d rays", n, len(targets))
				}

				// Calculate normalized direction vector
				dir := calculateDirection(originEye, target.Add(models.V3{X: 0.5, Y: 0.5, Z: 0.5}))

				// Trace ray to find first obscuring block
				if hit, found := a.traceRay(Ray{originEye, dir}, float64(R), world); found {
					hits = append(hits, hit)
				}
			}
			workerResults[w] = workerResult{hits: hits}
		}(w, start, end)
	}
	wg.Wait()

	visibleMap := make(map[models.V3]bool)
	for _, res := range workerResults {
		if res.err != nil {
			return nil, fmt.Errorf("context cancelled in findAllVisibleSurfaceBlocks: %v", res.err)
		}
		for _, hit := range res.hits {
			visibleMap[hit] = true
		}
	}

	// Convert map to sorted slice
	results := make([]models.VisibleBlockInfo, 0, len(visibleMap))
	for coord := range visibleMap {
		ix := int(math.Floor(coord.X))
		iy := int(math.Floor(coord.Y))
		iz := int(math.Floor(coord.Z))

		stateID, _ := world.GetBlockAt(coord.X, coord.Y, coord.Z)

		// Resolve block name
		blockName := ""
		if a.blockMgr != nil {
			if blockID, ok := a.blockMgr.BlockIDByStateID(stateID); ok {
				if block, ok := a.blockMgr.GetByID(blockID); ok {
					blockName = block.Name
				}
			}
		}

		dist := originEye.DistanceTo(coord)
		results = append(results, models.VisibleBlockInfo{
			X:         ix,
			Y:         iy,
			Z:         iz,
			StateID:   stateID,
			BlockName: blockName,
			Distance:  dist,
		})
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled in findAllVisibleSurfaceBlocks: %v", ctx.Err())
		default:
			continue
		}

	}

	// Sort by distance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}
