package testing

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// TerrainBuilder provides helpers to make deterministic test arenas using RCON.
// It avoids relying on world generator randomness by programmatically shaping terrain.
type TerrainBuilder struct {
	h testenv.RCONHelper
}

func NewTerrainBuilder(h testenv.RCONHelper) *TerrainBuilder { return &TerrainBuilder{h: h} }

// EnsureFlatSquare clears air from [y+1..y+6] and lays a solid base beneath [y-3..y-1],
// then creates a single flat ground layer at y using the provided block ID.
// Example block IDs: "minecraft:grass_block", "minecraft:stone".
func (tb *TerrainBuilder) EnsureFlatSquare(ctx context.Context, centerX, centerZ, halfSize, groundY int, groundBlock string) error {
	x1, z1 := centerX-halfSize, centerZ-halfSize
	x2, z2 := centerX+halfSize, centerZ+halfSize
	// Fill below ground with stone to avoid caves and ensure reachability
	if _, err := tb.h.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone replace", x1, groundY-3, z1, x2, groundY-1, z2)); err != nil {
		return fmt.Errorf("fill base: %w", err)
	}
	// Lay ground layer
	if _, err := tb.h.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d %s replace", x1, groundY, z1, x2, groundY, z2, groundBlock)); err != nil {
		return fmt.Errorf("fill ground: %w", err)
	}
	// Clear air above ground so movement isn't obstructed
	if _, err := tb.h.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air replace", x1, groundY+1, z1, x2, groundY+6, z2)); err != nil {
		return fmt.Errorf("clear air: %w", err)
	}
	return nil
}

// Wall builds a solid wall rectangle from (x1,y1,z1) to (x2,y2,z2).
func (tb *TerrainBuilder) Wall(ctx context.Context, x1, y1, z1, x2, y2, z2 int, block string) error {
	_, err := tb.h.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d %s replace", x1, y1, z1, x2, y2, z2, block))
	return err
}

// Gap clears a rectangular gap (air) from (x1,y1,z1) to (x2,y2,z2).
func (tb *TerrainBuilder) Gap(ctx context.Context, x1, y1, z1, x2, y2, z2 int) error {
	_, err := tb.h.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air replace", x1, y1, z1, x2, y2, z2))
	return err
}
