package agent

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// buildObsidianFrame seeds a rectangular nether portal frame into a's
// world: an obsidian border around an all-air interior of width x height
// (vanilla "opening" terms), with its bottom-left column's lowest block at
// origin, oriented along axis ("x" or "z"). If skipCorners is true, the
// four corner cells are left air instead of obsidian, exercising vanilla's
// "corners not required" rule.
func buildObsidianFrame(t *testing.T, a *agent, origin models.V3, axis string, width, height int, skipCorners bool) {
	t.Helper()
	obsidianStateID := findStateIDByName(t, a, "minecraft:obsidian")
	dx, dz := 1, 0
	if axis == "z" {
		dx, dz = 0, 1
	}
	x0, y0, z0 := int(origin.X), int(origin.Y), int(origin.Z)

	setBlock := func(x, y, z int, obsidian bool) {
		stateID := int32(0)
		if obsidian {
			stateID = int32(obsidianStateID)
		}
		a.mcAgentWorld.HandleBlockUpdate(int64(x), int64(y), int64(z), stateID)
	}
	isCorner := func(i, dy int) bool {
		return (i == 0 || i == width+1) && (dy == -1 || dy == height)
	}

	for i := 0; i <= width+1; i++ {
		ix, iz := x0+dx*i, z0+dz*i
		for dy := -1; dy <= height; dy++ {
			y := y0 + dy
			var obsidian bool
			switch {
			case isCorner(i, dy):
				// Optional: vanilla-generated portals always include
				// them, but detection must not require them either way.
				obsidian = !skipCorners
			case i == 0 || i == width+1:
				// Side columns span the interior height (dy 0..height-1).
				obsidian = dy >= 0 && dy < height
			case dy == -1 || dy == height:
				// Bottom/top bridge rows, excluding corners.
				obsidian = true
			default:
				// Interior: air.
				obsidian = false
			}
			setBlock(ix, y, iz, obsidian)
		}
	}
}

func TestFindPortalFrame_DetectsMinimumSize(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Minimum vanilla interior: 2 wide x 3 tall.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 2, 3, false)

	frame, found, err := a.FindPortalFrame(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "x", frame.Axis)
	require.Equal(t, models.V3{X: 1, Y: 64, Z: 0}, frame.InteriorMin)
	require.Equal(t, models.V3{X: 2, Y: 66, Z: 0}, frame.InteriorMax)
}

func TestFindPortalFrame_RejectsTooNarrow(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Width 1 is narrower than vanilla's minimum (2) - not a valid frame.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 1, 3, false)

	_, found, err := a.FindPortalFrame(context.Background(), 10)
	require.NoError(t, err)
	require.False(t, found, "width-1 interior should not be detected as a valid frame")
}

func TestFindPortalFrame_RejectsTooShort(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Height 2 is shorter than vanilla's minimum (3) - not a valid frame.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 2, 2, false)

	_, found, err := a.FindPortalFrame(context.Background(), 10)
	require.NoError(t, err)
	require.False(t, found, "height-2 interior should not be detected as a valid frame")
}

func TestFindPortalFrame_DetectsNonStandardSize(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Well beyond the canonical 2x3 - this is the "non-standard size"
	// case the user specifically asked for.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 6, 9, false)

	frame, found, err := a.FindPortalFrame(context.Background(), 20)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.V3{X: 1, Y: 64, Z: 0}, frame.InteriorMin)
	require.Equal(t, models.V3{X: 6, Y: 72, Z: 0}, frame.InteriorMax)
}

func TestFindPortalFrame_DetectsAtVanillaMaximumSize(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Vanilla's actual maximum: 21x21 interior (23x23 outer frame).
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 21, 21, false)

	frame, found, err := a.FindPortalFrame(context.Background(), 25)
	require.NoError(t, err)
	require.True(t, found, "a 21x21 interior is within vanilla's own maximum and must be detected")
	require.Equal(t, models.V3{X: 1, Y: 64, Z: 0}, frame.InteriorMin)
	require.Equal(t, models.V3{X: 21, Y: 84, Z: 0}, frame.InteriorMax)
}

func TestFindPortalFrame_RejectsBeyondVanillaMaximumSize(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// One wider than vanilla's actual maximum (21) - confirms there IS an
	// enforced upper bound, not just an assumed one.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 22, 21, false)

	_, found, err := a.FindPortalFrame(context.Background(), 26)
	require.NoError(t, err)
	require.False(t, found, "a 22-wide interior exceeds vanilla's own maximum (21) and must not be detected")
}

func TestFindPortalFrame_IgnoresMissingCorners(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// Corners are optional in vanilla - a frame missing all four must
	// still be detected.
	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "x", 3, 4, true)

	_, found, err := a.FindPortalFrame(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, found, "a frame missing its (optional) corners must still be detected")
}

func TestFindPortalFrame_DetectsZAxisOrientation(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	buildObsidianFrame(t, a, models.V3{X: 0, Y: 64, Z: 0}, "z", 3, 5, false)

	frame, found, err := a.FindPortalFrame(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "z", frame.Axis)
}

func TestLightPortal_NoFrameFound(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))
	a.UpdatePosition(models.V3{}, 0, 0)

	err = a.LightPortal(context.Background(), 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no unlit portal frame found")
}
