package physics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests exercise the elytra-gliding formulas directly, without a live
// Minecraft server or a physics.State instance — see elytra.go's doc
// comment for the decompiled-source citations.

func TestRotationVector(t *testing.T) {
	tests := []struct {
		name         string
		yaw, pitch   float64
		wantX, wantY float64
		wantZ        float64
	}{
		{name: "yaw 0 (south), level", yaw: 0, pitch: 0, wantX: 0, wantY: 0, wantZ: 1},
		{name: "yaw 90 (west), level", yaw: 90, pitch: 0, wantX: -1, wantY: 0, wantZ: 0},
		{name: "yaw 180 (north), level", yaw: 180, pitch: 0, wantX: 0, wantY: 0, wantZ: -1},
		{name: "yaw 270 (east), level", yaw: 270, pitch: 0, wantX: 1, wantY: 0, wantZ: 0},
		{name: "pitch -90 (straight up)", yaw: 0, pitch: -90, wantX: 0, wantY: 1, wantZ: 0},
		{name: "pitch 90 (straight down)", yaw: 0, pitch: 90, wantX: 0, wantY: -1, wantZ: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, z := RotationVector(tt.yaw, tt.pitch)
			assert.InDelta(t, tt.wantX, x, 1e-9)
			assert.InDelta(t, tt.wantY, y, 1e-9)
			assert.InDelta(t, tt.wantZ, z, 1e-9)
		})
	}
}

func TestGlidingVelocity(t *testing.T) {
	t.Run("from rest, level, looking south: gravity blend plus horizontal-ease drift toward look direction", func(t *testing.T) {
		// Hand-computed against calcGlidingVelocity step by step:
		// h=1, vy after gravity = 0.08*(-1+0.75) = -0.02
		// diving block (vy<0, d>0): i = -0.02*-0.1*1 = 0.002; vz += 1*0.002/1 = 0.002; vy += 0.002 = -0.018
		// pitch not <0: skip climb block
		// ease block: vz += (1*0 - 0.002)*0.1 = -0.0002 -> vz = 0.0018
		// final: vx=0*0.99=0, vy=-0.018*0.98=-0.01764, vz=0.0018*0.99=0.001782
		nx, ny, nz := GlidingVelocity(0, 0, 0, 0, 0, Gravity)
		assert.InDelta(t, 0.0, nx, 1e-9)
		assert.InDelta(t, -0.01764, ny, 1e-9)
		assert.InDelta(t, 0.001782, nz, 1e-9)
	})

	t.Run("looking straight down: full gravity, no horizontal terms (d=0 guard)", func(t *testing.T) {
		nx, ny, nz := GlidingVelocity(0, 0, 0, 0, 90, Gravity)
		assert.InDelta(t, 0.0, nx, 1e-9)
		assert.InDelta(t, -Gravity*GlideVerticalDrag, ny, 1e-9)
		assert.InDelta(t, 0.0, nz, 1e-9)
	})

	t.Run("looking straight up: full gravity, no horizontal terms (d=0 guard)", func(t *testing.T) {
		nx, ny, nz := GlidingVelocity(0, 0, 0, 0, -90, Gravity)
		assert.InDelta(t, 0.0, nx, 1e-9)
		assert.InDelta(t, -Gravity*GlideVerticalDrag, ny, 1e-9)
		assert.InDelta(t, 0.0, nz, 1e-9)
	})

	t.Run("diving while already falling converts fall speed into forward speed", func(t *testing.T) {
		// Falling at 0.5 blocks/tick, looking level toward south: the dive
		// term should bleed some of that fall speed into forward (+Z)
		// velocity, so vy ends up less negative than plain gravity
		// subtraction alone would give (-0.5 - Gravity = -0.58), and vz
		// (starting at 0) should have picked up a positive component.
		_, ny, nz := GlidingVelocity(0, -0.5, 0, 0, 0, Gravity)
		assert.Greater(t, ny, -0.58, "some fall speed should convert to forward speed rather than compounding purely as gravity")
		assert.Greater(t, nz, 0.0, "forward (+Z, looking south) velocity should pick up from the dive")
	})

	t.Run("pitching upward trades horizontal speed for a vertical boost", func(t *testing.T) {
		// Moving south at 0.5 blocks/tick, pitched up 30 degrees.
		_, nyUp, _ := GlidingVelocity(0, 0, 0.5, 0, -30, Gravity)
		_, nyLevel, _ := GlidingVelocity(0, 0, 0.5, 0, 0, Gravity)
		assert.Greater(t, nyUp, nyLevel, "pitching upward should add vertical velocity relative to level flight at the same speed")
	})

	t.Run("horizontal velocity eases toward the look direction over time", func(t *testing.T) {
		// Moving east (vx=0.5) while looking south (0,0,1) - horizontal
		// velocity should start bending toward the look direction.
		nx, _, nz := GlidingVelocity(0.5, 0, 0, 0, 0, Gravity)
		assert.Less(t, nx, 0.5, "off-look-direction horizontal speed should decay")
		assert.Greater(t, nz, 0.0, "velocity should start bending toward the look direction (south, +Z)")
	})
}

func TestFireworkBoostVelocity(t *testing.T) {
	t.Run("from rest, looking south, level", func(t *testing.T) {
		// rotationVector(0,0) = (0,0,1); nz = 0 + 1*0.1 + (1*1.5-0)*0.5 = 0.85
		nx, ny, nz := FireworkBoostVelocity(0, 0, 0, 0, 0)
		assert.InDelta(t, 0.0, nx, 1e-9)
		assert.InDelta(t, 0.0, ny, 1e-9)
		assert.InDelta(t, 0.85, nz, 1e-9)
	})

	t.Run("pitching upward gains real altitude, not just speed", func(t *testing.T) {
		_, nyLevel, _ := FireworkBoostVelocity(0, 0, 0, 0, 0)
		_, nyUp, _ := FireworkBoostVelocity(0, 0, 0, 0, -45)
		assert.Greater(t, nyUp, nyLevel, "pitching up while boosting should add vertical velocity")
	})

	t.Run("repeated boosting converges to a steady-state speed in the look direction", func(t *testing.T) {
		// Steady state solves v = v + blend + (target-v)*ease for v:
		// v = v + 0.1 + (1.5-v)*0.5 => 0.5v = 0.85 => v = 1.7 - the flat
		// FireworkBoostBlend term keeps nudging even after the ease term
		// alone would have settled at FireworkBoostTarget, so the true
		// steady state is higher than 1.5.
		const steadyState = (FireworkBoostBlend + FireworkBoostTarget*FireworkBoostEase) / FireworkBoostEase
		vx, vy, vz := 0.0, 0.0, 0.0
		for range 50 {
			vx, vy, vz = FireworkBoostVelocity(vx, vy, vz, 0, 0)
		}
		assert.InDelta(t, 0.0, vx, 1e-6)
		assert.InDelta(t, 0.0, vy, 1e-6)
		assert.InDelta(t, steadyState, vz, 1e-3, "should converge toward the algebraic steady-state speed in the look direction")
	})

	t.Run("boosting decelerates existing overshoot back toward the steady state", func(t *testing.T) {
		// Already moving faster than the steady-state speed in the look direction.
		_, _, nz := FireworkBoostVelocity(0, 0, 3.0, 0, 0)
		assert.Less(t, nz, 3.0, "overshooting the steady-state speed should ease back down, not keep accelerating")
	})
}

func TestCanGlide(t *testing.T) {
	tests := []struct {
		name                                                      string
		onGround, hasVehicle, hasLevitation, elytraEquipped, want bool
	}{
		{name: "airborne with elytra: can glide", onGround: false, hasVehicle: false, hasLevitation: false, elytraEquipped: true, want: true},
		{name: "on ground: cannot glide even with elytra", onGround: true, hasVehicle: false, hasLevitation: false, elytraEquipped: true, want: false},
		{name: "mounted: cannot glide even with elytra", onGround: false, hasVehicle: true, hasLevitation: false, elytraEquipped: true, want: false},
		{name: "levitating: cannot glide even with elytra", onGround: false, hasVehicle: false, hasLevitation: true, elytraEquipped: true, want: false},
		{name: "airborne without elytra: cannot glide", onGround: false, hasVehicle: false, hasLevitation: false, elytraEquipped: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanGlide(tt.onGround, tt.hasVehicle, tt.hasLevitation, tt.elytraEquipped)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCanStartGliding(t *testing.T) {
	tests := []struct {
		name                                            string
		alreadyGliding, isTouchingWater, canGlide, want bool
	}{
		{name: "eligible and not already gliding: starts", alreadyGliding: false, isTouchingWater: false, canGlide: true, want: true},
		{name: "already gliding: no-op", alreadyGliding: true, isTouchingWater: false, canGlide: true, want: false},
		{name: "touching water: blocked even if otherwise eligible", alreadyGliding: false, isTouchingWater: true, canGlide: true, want: false},
		{name: "canGlide false: blocked", alreadyGliding: false, isTouchingWater: false, canGlide: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanStartGliding(tt.alreadyGliding, tt.isTouchingWater, tt.canGlide)
			assert.Equal(t, tt.want, got)
		})
	}
}
