package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// raceWorld is a minimal models.World fake: every coordinate is air (0)
// except raceCoord, which reports solidID on the FIRST read and air (0) on
// every read after that - simulating a concurrent mine landing between a
// ray-trace's own read and a later, separate re-read of the same
// coordinate. Every call is counted.
type raceWorld struct {
	solidID   uint32
	raceCoord models.V3
	calls     map[models.V3]int
}

func newRaceWorld(solidID uint32, raceCoord models.V3) *raceWorld {
	return &raceWorld{solidID: solidID, raceCoord: raceCoord, calls: map[models.V3]int{}}
}

func (w *raceWorld) GetBlockAt(x, y, z float64) (uint32, bool) {
	coord := models.V3{X: x, Y: y, Z: z}
	w.calls[coord]++
	if coord == w.raceCoord && w.calls[coord] < 2 {
		return w.solidID, true
	}
	return 0, true // everywhere else, and raceCoord from its second read on
}

func (w *raceWorld) GetBlockStatus(x, y, z int) (uint32, bool) {
	return w.GetBlockAt(float64(x), float64(y), float64(z))
}
func (w *raceWorld) GetEntitiesInRange(models.AABB) []models.EntityBounds   { return nil }
func (w *raceWorld) GetWorldAge() (int64, bool)                             { return 0, false }
func (w *raceWorld) GetTimeOfDay() (int64, bool)                            { return 0, false }
func (w *raceWorld) SetWorldTime(worldAge, timeOfDay int64)                 {}
func (w *raceWorld) GetLightLevel(x, y, z int) (uint8, uint8, bool)         { return 0, 0, false }
func (w *raceWorld) GetBiomeAt(x, y, z int) (uint32, bool)                  { return 0, false }
func (w *raceWorld) GetWorldBorder() (models.WorldBorder, bool)             { return models.WorldBorder{}, false }
func (w *raceWorld) SetWorldBorder(models.WorldBorder)                      {}
func (w *raceWorld) SetWorldBorderCenter(x, z float64)                      {}
func (w *raceWorld) SetWorldBorderSize(diameter float64)                    {}
func (w *raceWorld) SetWorldBorderLerpSize(oldD, newD float64, speed int64) {}
func (w *raceWorld) SetWorldBorderWarningDelay(warningTimeTicks int32)      {}
func (w *raceWorld) SetWorldBorderWarningDistance(warningBlocks int32)      {}
func (w *raceWorld) GetDifficulty() (uint8, bool, bool)                     { return 0, false, false }
func (w *raceWorld) SetDifficulty(difficulty uint8, locked bool)            {}

// TestIsObscuringReturnsTheStateIDItRead covers the field this function
// gained when the TOCTOU race in findAllVisibleSurfaceBlocks was fixed
// (see that function's own doc comment): callers now use isObscuring's
// own stateID instead of re-reading the world separately afterward, so
// isObscuring reporting the wrong one would silently reintroduce the race.
func TestIsObscuringReturnsTheStateIDItRead(t *testing.T) {
	a := &agent{}
	coord := models.V3{X: 1, Y: 2, Z: 3}

	t.Run("solid block reports its own stateID, obscuring", func(t *testing.T) {
		world := newRaceWorld(99, coord) // coord is raceWorld's one solid cell
		stateID, obscuring := a.isObscuring(coord, world)
		assert.True(t, obscuring)
		assert.Equal(t, uint32(99), stateID)
	})

	t.Run("air reports stateID 0, not obscuring", func(t *testing.T) {
		world := newRaceWorld(99, models.V3{X: 999, Y: 999, Z: 999}) // coord itself stays air
		stateID, obscuring := a.isObscuring(coord, world)
		assert.False(t, obscuring)
		assert.Equal(t, uint32(0), stateID)
	})
}

// TestTraceRayCarriesTheHitsStateIDThrough confirms traceRay returns the
// SAME stateID isObscuring read at the hit, not a value read again
// afterward - the exact value this test's raceWorld would silently swap
// to air on a second read, if anything in the call chain still triggered one.
func TestTraceRayCarriesTheHitsStateIDThrough(t *testing.T) {
	a := &agent{}
	hitCoord := models.V3{X: 5, Y: 0, Z: 0}
	world := newRaceWorld(42, hitCoord)

	ray := Ray{Origin: models.V3{X: 0, Y: 0, Z: 0}, Direction: models.V3{X: 1, Y: 0, Z: 0}}
	coord, stateID, found := a.traceRay(ray, 10, world)

	require.True(t, found)
	assert.Equal(t, hitCoord, coord)
	assert.Equal(t, uint32(42), stateID, "must be the ray-hit-time stateID, not a later (racy) re-read")
	assert.Equal(t, 1, world.calls[hitCoord], "traceRay must read the hit coordinate exactly once")
}
