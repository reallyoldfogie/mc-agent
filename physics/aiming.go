package physics

import (
	"errors"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

func Len2(x, z float64) float64 { return math.Hypot(x, z) }

type UpdateOrder int

const (
	// OrderMoveDragGravity: position → drag → gravity (CURRENT - NOT CORRECT)
	// This was the original incorrect order in engine.go
	// DO NOT USE for new code - kept only for backwards compatibility
	OrderMoveDragGravity UpdateOrder = iota

	// OrderGravityDragMove: gravity → drag → position (CORRECT for ThrownEntity)
	// Used by: Snowballs, Eggs, Ender Pearls, and other ThrownEntity variants
	// This matches the actual Minecraft ThrownEntity.tick() order
	OrderGravityDragMove

	// OrderDragGravityPosition: drag → gravity → position (CORRECT for PersistentProjectileEntity)
	// Used by: Arrows, Tridents, and other PersistentProjectileEntity variants
	// This matches the actual Minecraft PersistentProjectileEntity physics order
	OrderDragGravityPosition
)

type ProjectileProps struct {
	Type     models.ProjectileType // Projectile type (for model lookup)
	Speed    float64               // blocks/tick
	Drag     float64               // multiplicative per tick, e.g. 0.99
	Gravity  float64               // blocks/tick^2, e.g. 0.05
	MaxTicks int                   // e.g. 200..600 depending on projectile lifetime
	Order    UpdateOrder           // choose per projectile (deprecated - use model instead)
	// Optional hooks if you model special cases:
	// DragInWater float64
	// DragInLava  float64
}

type AimSolution struct {
	YawRad   float64
	PitchRad float64
	// Initial velocity you should send/apply in your client simulation
	V0 models.V3
	// Vertical error at solution (should be near 0)
	ErrorY float64
	// Tick when we crossed horizontal range
	Tick int
}

var ErrUnreachable = errors.New("target unreachable with given projectile properties")

// SolveAim finds a pitch (optionally low/high arc via `preferHighArc`) that hits `target`
// in a discrete tick sim using `props`. Assumes +Y is up.
func SolveAim(start, target models.V3, props ProjectileProps, preferHighArc bool) (AimSolution, error) {
	d := target.Sub(start)
	R := Len2(d.X, d.Z)

	// If target is basically on top of you horizontally, yaw is undefined; handle separately.
	if R < 1e-9 {
		// Straight up/down: just pitch toward target; yaw = 0
		pitch := math.Pi / 2
		if d.Y < 0 {
			pitch = -math.Pi / 2
		}
		v0 := models.V3{X: 0, Y: -props.Speed * math.Sin(pitch), Z: 0}
		return AimSolution{YawRad: 0, PitchRad: pitch, V0: v0}, nil
	}

	// Compute yaw so that velocity points toward target in XZ.
	// Convention: yaw = atan2(dZ, dX). Adjust if your engine uses different axes.
	yaw := math.Atan2(d.Z, d.X)

	// We will binary search pitch in a safe range.
	lo := deg2rad(-89.9)
	hi := deg2rad(89.9)

	// For “two solutions” behavior, you can:
	// - either scan a set of pitches to find sign changes and then root-find in that bracket
	// - or just choose a bracket biased toward low or high arc.
	// Below: quick scan to find up to two brackets, then pick based on preferHighArc.
	brackets := findBrackets(start, target, yaw, props, lo, hi, 180) // 180 samples
	if len(brackets) == 0 {
		return AimSolution{}, ErrUnreachable
	}

	// NOTE: brackets are ordered by pitch (low to high).
	// Lower pitch values produce higher arcs (nearly vertical).
	// Higher pitch values produce lower arcs (shallow angles).
	// So we need to reverse the selection logic.
	br := brackets[len(brackets)-1] // FIXED: last bracket is lowest-arc
	if preferHighArc && len(brackets) > 1 {
		br = brackets[0] // FIXED: first bracket is highest-arc
	}

	pitch, sol, ok := bisectPitch(start, target, yaw, props, br[0], br[1], 40)
	if !ok {
		return AimSolution{}, ErrUnreachable
	}
	sol.YawRad = yaw
	sol.PitchRad = pitch
	return sol, nil
}

// findBrackets scans the pitch range and returns up to two brackets where the vertical error changes sign (crosses zero).
func findBrackets(start, target models.V3, yaw float64, props ProjectileProps, lo, hi float64, samples int) [][2]float64 {
	out := make([][2]float64, 0, 2)
	prevP := lo
	prevE, prevOK := rangeError(start, target, yaw, prevP, props)
	step := (hi - lo) / float64(samples)
	for i := 1; i <= samples; i++ {
		p := lo + float64(i)*step
		e, ok := rangeError(start, target, yaw, p, props)
		if prevOK && ok {
			// sign change => bracket
			if (prevE <= 0 && e >= 0) || (prevE >= 0 && e <= 0) {
				out = append(out, [2]float64{prevP, p})
				if len(out) == 2 {
					return out
				}
			}
		}
		prevP, prevE, prevOK = p, e, ok
	}
	return out
}

// bisectPitch performs a binary search to find a pitch that hits the target, given a bracket [a,b] where the error changes sign.
func bisectPitch(start, target models.V3, yaw float64, props ProjectileProps, a, b float64, iters int) (pitch float64, sol AimSolution, ok bool) {
	ea, oka := rangeError(start, target, yaw, a, props)
	eb, okb := rangeError(start, target, yaw, b, props)
	if !oka || !okb {
		return 0, AimSolution{}, false
	}
	// Must bracket a root.
	if ea == 0 {
		s := evalAtPitch(start, target, yaw, a, props)
		return a, s, true
	}
	if eb == 0 {
		s := evalAtPitch(start, target, yaw, b, props)
		return b, s, true
	}
	if (ea > 0 && eb > 0) || (ea < 0 && eb < 0) {
		return 0, AimSolution{}, false
	}

	lo, hi := a, b
	for range iters {
		mid := 0.5 * (lo + hi)
		em, okm := rangeError(start, target, yaw, mid, props)
		if !okm {
			return 0, AimSolution{}, false
		}
		// Close enough in vertical error (tune epsilon if needed)
		if math.Abs(em) < 1e-4 {
			s := evalAtPitch(start, target, yaw, mid, props)
			s.ErrorY = em
			return mid, s, true
		}
		// Keep the bracket with sign change
		el, _ := rangeError(start, target, yaw, lo, props)
		if (el <= 0 && em >= 0) || (el >= 0 && em <= 0) {
			hi = mid
		} else {
			lo = mid
		}
	}

	// Return best effort at mid
	mid := 0.5 * (lo + hi)
	s := evalAtPitch(start, target, yaw, mid, props)
	s.ErrorY, _ = rangeError(start, target, yaw, mid, props)
	return mid, s, true
}

// rangeError returns y_at_R - targetY when the projectile crosses the target horizontal range R.
// ok=false if it never reaches range within MaxTicks (or numerical issues).
func rangeError(start, target models.V3, yaw, pitch float64, props ProjectileProps) (err float64, ok bool) {
	s := evalAtPitch(start, target, yaw, pitch, props)
	return s.ErrorY, s.Tick != 0 // Tick==0 used as "didn't cross" below; tweak if needed
}

// evalAtPitch simulates the projectile trajectory for a given pitch and returns the vertical error at the point where it crosses the target's horizontal range R.
func evalAtPitch(start, target models.V3, yaw, pitch float64, props ProjectileProps) AimSolution {
	d := target.Sub(start)
	Rtarget := Len2(d.X, d.Z)

	// Initial velocity components
	// Standard convention: positive Y = up, positive pitch = looking down
	// Negate sin(pitch) so that pitch=-90 (looking up) gives positive velY (upward)
	// Matches: projectile.go:265, projectile.go:113, projectile_commands.go:110
	cp := math.Cos(pitch)
	sp := math.Sin(pitch)
	cy := math.Cos(yaw)
	sy := math.Sin(yaw)

	v := models.V3{
		X: props.Speed * cp * cy,
		Y: -props.Speed * sp,
		Z: props.Speed * cp * sy,
	}
	pos := start

	prevPos := pos
	prevR := Len2((prevPos.X - start.X), (prevPos.Z - start.Z))

	// Get the physics model for this projectile type
	// The model encapsulates the correct physics order (gravity→drag→position, etc.)
	model := GetProjectilePhysicsModel(props.Type)

	// Use zero-based indexing to match SimulateProjectileTrajectory
	for t := 0; t < props.MaxTicks; t++ {
		// Apply physics using the model (which handles the correct order for this projectile type)
		pos, v = TickProjectile(pos, v, model)

		// Check if we've crossed the horizontal range from the start position
		curR := Len2(pos.X-start.X, pos.Z-start.Z)
		if prevR <= Rtarget && curR >= Rtarget {
			// Interpolate between prevPos and pos to estimate y_at_R
			// Alpha based on horizontal distance along the segment (in XZ plane).
			segDX := pos.X - prevPos.X
			segDZ := pos.Z - prevPos.Z
			segLen := Len2(segDX, segDZ)
			alpha := 0.0
			if segLen > 1e-9 {
				// how much of this step's horizontal length we need to reach Rtarget
				need := Rtarget - prevR
				alpha = clamp01(need / segLen)
			}

			yAtR := prevPos.Y + alpha*(pos.Y-prevPos.Y)
			errY := yAtR - target.Y

			return AimSolution{
				V0:     models.V3{X: props.Speed * cp * cy, Y: -props.Speed * sp, Z: props.Speed * cp * sy},
				ErrorY: errY,
				Tick:   t,
			}
		}

		prevPos = pos
		prevR = curR
	}

	// Didn't reach the target range within MaxTicks -> mark unreachable for this pitch
	return AimSolution{ErrorY: math.NaN(), Tick: 0}
}

// clamp01 clamps x to the range [0,1]. Useful for interpolation alpha.
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// deg converts degrees to radians.
func deg2rad(d float64) float64 { return d * math.Pi / 180.0 }

// rad converts radians to degrees.
func rad2deg(r float64) float64 { return r * 180.0 / math.Pi }
