package agent

import "math"

// voxelRayWalker performs a 3D-DDA (Amanatides-Woo) walk through the block
// grid, one voxel at a time, starting at the voxel containing an origin
// position and advancing along a direction vector that must already be
// unit length - tMax/tDelta below are expressed in units of "distance
// traveled along the ray", which only equals world-space distance when the
// direction is normalized, and every caller already normalizes before
// constructing one.
//
// This is the single ray-marching primitive behind every block-grid
// raycast in this package (HasLineOfSight, collectLineOfSightBlocks,
// hasLineOfSightForAccessToPoint, traceRay). Found necessary, not merely
// tidy: each of those four used to carry its own independent copy of this
// same stepping logic, and had already drifted - traceRay advanced every
// axis simultaneously whenever their tMax values were tied (within an
// epsilon), while the other three advanced only the single smallest,
// breaking ties X, then Y, then Z. That's a real behavioral difference
// between "copies" of the same algorithm, not just a style difference, and
// nobody had decided it on purpose. Consolidated onto the single-axis
// tie-break here since three of the four call sites already used it -
// this only changes traceRay's behavior, and only on an exact tie (two or
// three axes' tMax equal to within its old 1e-6 epsilon), a case that's
// effectively unreachable with the Fibonacci-sphere-sampled directions
// traceRay's caller actually passes.
//
// Each call site keeps its own stop condition - some check accumulated
// world-space distance against a radius, one applies a deliberate epsilon
// to survive a target sitting exactly on a voxel boundary (see
// HasLineOfSight's own comment) - so the walker only owns cell tracking
// and stepping, never when to stop; baking one call site's stopping
// epsilon into every other one without separately verifying it there would
// be its own, different bug.
type voxelRayWalker struct {
	x, y, z             int
	stepX, stepY, stepZ int
	tMax                [3]float64
	tDelta              [3]float64
}

// newVoxelRayWalker starts a walk at the voxel containing (ox, oy, oz),
// stepping along the unit-length direction (dirX, dirY, dirZ).
func newVoxelRayWalker(ox, oy, oz, dirX, dirY, dirZ float64) *voxelRayWalker {
	x := int(math.Floor(ox))
	y := int(math.Floor(oy))
	z := int(math.Floor(oz))
	tMaxX, tDeltaX := initialRayStep(ox, dirX, x)
	tMaxY, tDeltaY := initialRayStep(oy, dirY, y)
	tMaxZ, tDeltaZ := initialRayStep(oz, dirZ, z)
	return &voxelRayWalker{
		x: x, y: y, z: z,
		stepX: int(stepSign(dirX)), stepY: int(stepSign(dirY)), stepZ: int(stepSign(dirZ)),
		tMax:   [3]float64{tMaxX, tMaxY, tMaxZ},
		tDelta: [3]float64{tDeltaX, tDeltaY, tDeltaZ},
	}
}

// cell returns the walker's current voxel coordinates.
func (w *voxelRayWalker) cell() (x, y, z int) {
	return w.x, w.y, w.z
}

// nextT returns the ray-parameter (== world-space distance, since the
// direction is unit length) at which advance would next move the walker -
// callers compare this against their own stopping distance before
// deciding whether to advance at all.
func (w *voxelRayWalker) nextT() float64 {
	return minFloat64(w.tMax[0], w.tMax[1], w.tMax[2])
}

// advance steps the walker to the next voxel along the ray: whichever
// single axis has the smallest tMax, ties broken X, then Y, then Z (the
// canonical Amanatides-Woo rule) - see the type's own doc comment on why
// this, not simultaneously stepping every tied axis, is the one behavior
// every call site now shares.
func (w *voxelRayWalker) advance() {
	switch {
	case w.tMax[0] <= w.tMax[1] && w.tMax[0] <= w.tMax[2]:
		w.x += w.stepX
		w.tMax[0] += w.tDelta[0]
	case w.tMax[1] <= w.tMax[0] && w.tMax[1] <= w.tMax[2]:
		w.y += w.stepY
		w.tMax[1] += w.tDelta[1]
	default:
		w.z += w.stepZ
		w.tMax[2] += w.tDelta[2]
	}
}

// initialRayStep computes one axis' DDA step: tMax (the ray parameter at
// which the walk first crosses into the next cell along this axis) and
// tDelta (how much tMax advances per subsequent crossing of this axis).
func initialRayStep(origin, dir float64, cell int) (tMax, tDelta float64) {
	if dir > 0 {
		next := float64(cell+1) - origin
		tMax = next / dir
		tDelta = 1.0 / dir
		return tMax, tDelta
	}
	if dir < 0 {
		next := float64(cell) - origin
		tMax = next / dir
		tDelta = -1.0 / dir
		return tMax, tDelta
	}
	return math.Inf(1), math.Inf(1)
}

// stepSign returns the unit step direction (-1, 0, or 1) for a DDA axis.
func stepSign(v float64) float64 {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

// minFloat64 returns the smallest of three values.
func minFloat64(a, b, c float64) float64 {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}
