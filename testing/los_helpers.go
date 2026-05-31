package testing

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

type losAccessPointFinder interface {
	FindLineOfSightAccessPoint(ctx context.Context, x, y, z float64) (float64, float64, float64, bool, error)
}

// LOSCursorForBlock returns cursor coordinates derived from the agent's LOS hit point.
func LOSCursorForBlock(ctx context.Context, ag models.Agent, pos models.V3) (float32, float32, float32, error) {
	finder, ok := ag.(losAccessPointFinder)
	if !ok {
		return 0, 0, 0, fmt.Errorf("agent does not expose LOS access point finder")
	}
	blockX := math.Floor(pos.X)
	blockY := math.Floor(pos.Y)
	blockZ := math.Floor(pos.Z)
	hitX, hitY, hitZ, visible, err := finder.FindLineOfSightAccessPoint(ctx, blockX, blockY, blockZ)
	if err != nil {
		return 0, 0, 0, err
	}
	if !visible {
		agPos, _ := ag.GetPositionSimple()
		return 0, 0, 0, fmt.Errorf("no line of sight from agent (%.1f, %.1f, %.1f) to block at (%.1f, %.1f, %.1f)", agPos.X, agPos.Y, agPos.Z, pos.X, pos.Y, pos.Z)
	}
	cursorX := float32(clampFloat64(hitX-blockX, 0, 1))
	cursorY := float32(clampFloat64(hitY-blockY, 0, 1))
	cursorZ := float32(clampFloat64(hitZ-blockZ, 0, 1))
	log.Printf("[Test] LOS hit at (%.3f, %.3f, %.3f) cursor=(%.3f, %.3f, %.3f) block=(%.1f, %.1f, %.1f)",
		hitX, hitY, hitZ, cursorX, cursorY, cursorZ, pos.X, pos.Y, pos.Z)
	return cursorX, cursorY, cursorZ, nil
}

// OpenContainerWithLOS opens a container using an LOS-derived cursor point.
func OpenContainerWithLOS(ctx context.Context, ag models.Agent, pos models.V3, face models.BlockFace, timeout time.Duration) (byte, error) {
	cursorX, cursorY, cursorZ, err := LOSCursorForBlock(ctx, ag, pos)
	if err != nil {
		return 0, err
	}
	return ag.OpenContainer(pos, models.BlockFace(face), timeout, cursorX, cursorY, cursorZ)
}

func clampFloat64(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
