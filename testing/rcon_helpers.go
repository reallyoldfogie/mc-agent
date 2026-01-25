package testing

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// Orientation represents cardinal directions for structure building
type Orientation int

const (
	North Orientation = iota // -Z direction
	South                    // +Z direction
	East                     // +X direction
	West                     // -X direction
)

// ChestItem represents an item in a chest (from RCON)
type ChestItem struct {
	Slot  int
	ID    string
	Count int
}

// InventoryItem represents an item in player inventory (from RCON)
type InventoryItem struct {
	Slot  int
	ID    string
	Count int
}

// PlayerPosition represents a player's position
type PlayerPosition struct {
	X float64
	Y float64
	Z float64
}

func blockCoords(pos models.V3) (int, int, int) {
	return int(math.Floor(pos.X)), int(math.Floor(pos.Y)), int(math.Floor(pos.Z))
}

func normalizeBlockName(name string) string {
	if name == "" {
		return ""
	}
	if idx := strings.Index(name, "["); idx != -1 {
		name = name[:idx]
	}
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	return name
}

func normalizeBlockSpecForCommand(name string) string {
	if name == "" {
		return ""
	}
	idx := strings.IndexAny(name, "[{")
	base := name
	if idx != -1 {
		base = name[:idx]
	}
	if strings.Contains(base, ":") {
		return name
	}
	return "minecraft:" + name
}

// WaitForBlockState waits until the client world contains the expected block.
func WaitForBlockState(ctx context.Context, agent *ManagedAgent, pos models.V3, expectedBlock string, timeout time.Duration) (uint32, error) {
	if agent == nil || agent.Agent == nil {
		return 0, fmt.Errorf("agent not available")
	}
	if agent.Config.BlockMgr == nil {
		return 0, fmt.Errorf("block manager not available")
	}

	expected := normalizeBlockName(expectedBlock)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		worldMgr := agent.Agent.GetWorld()
		if worldMgr != nil {
			stateID, _ := worldMgr.GetBlockAt(pos.X, pos.Y, pos.Z)
			if stateID != 0 {
				if blockID, ok := agent.Config.BlockMgr.BlockIDByStateID(stateID); ok {
					if block, ok := agent.Config.BlockMgr.GetByID(blockID); ok {
						name := normalizeBlockName(block.Name)
						if expected == "" || name == expected {
							return stateID, nil
						}
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return 0, fmt.Errorf("timeout waiting for block %q at %v", expected, pos)
}

// PlaceBlockAndWait sets a block via RCON and waits for the client to see it.
func PlaceBlockAndWait(ctx context.Context, rcon testenv.RCONHelper, agent *ManagedAgent, pos models.V3, blockSpec, expectedBlock string, timeout time.Duration) (uint32, error) {
	if blockSpec == "" {
		return 0, fmt.Errorf("block spec is empty")
	}
	if expectedBlock == "" {
		expectedBlock = blockSpec
	}
	cmdBlock := normalizeBlockSpecForCommand(blockSpec)
	x, y, z := blockCoords(pos)
	if _, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d %s", x, y, z, cmdBlock)); err != nil {
		return 0, err
	}
	return WaitForBlockState(ctx, agent, pos, expectedBlock, timeout)
}

// GetChestContents retrieves the items in a chest at the given position
func GetChestContents(ctx context.Context, rcon testenv.RCONHelper, pos models.V3) ([]ChestItem, error) {
	x, y, z := blockCoords(pos)
	cmd := fmt.Sprintf("data get block %d %d %d Items", x, y, z)
	resp, err := rcon.Exec(ctx, cmd)
	if err != nil {
		return nil, err
	}

	// Parse response like: "[{Slot: 0b, id: "minecraft:stone", Count: 1b}]"
	items := []ChestItem{}

	// If empty, response is "[]" or similar
	if strings.Contains(resp, "has no item") || resp == "[]" {
		return items, nil
	}

	entryRe := regexp.MustCompile(`\{([^}]*)\}`)
	slotRe := regexp.MustCompile(`(?i)Slot:\s*(-?\d+)b`)
	idRe := regexp.MustCompile(`(?i)id:\s*"([^"]+)"`)
	countRe := regexp.MustCompile(`(?i)count:\s*(\d+)b?`)

	entries := entryRe.FindAllStringSubmatch(resp, -1)
	for _, entry := range entries {
		if len(entry) != 2 {
			continue
		}
		segment := entry[1]

		slotMatch := slotRe.FindStringSubmatch(segment)
		idMatch := idRe.FindStringSubmatch(segment)
		countMatch := countRe.FindStringSubmatch(segment)

		if slotMatch == nil || idMatch == nil || countMatch == nil {
			continue
		}

		slot, err := strconv.Atoi(slotMatch[1])
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(countMatch[1])
		if err != nil {
			continue
		}

		items = append(items, ChestItem{
			Slot:  slot,
			ID:    idMatch[1],
			Count: count,
		})
	}

	return items, nil
}

// GetPlayerPosition gets the player's current position
func GetPlayerPosition(ctx context.Context, rcon testenv.RCONHelper, playerName string) (PlayerPosition, error) {
	cmd := fmt.Sprintf("data get entity %s Pos", playerName)
	resp, err := rcon.Exec(ctx, cmd)
	if err != nil {
		return PlayerPosition{}, err
	}

	// Parse response like: "[8.5d, 70.0d, -155.5d]"
	re := regexp.MustCompile(`\[([^,]+),\s*([^,]+),\s*([^\]]+)\]`)
	match := re.FindStringSubmatch(resp)
	if match == nil || len(match) != 4 {
		return PlayerPosition{}, fmt.Errorf("failed to parse position: %s", resp)
	}

	x, err := strconv.ParseFloat(strings.TrimSuffix(match[1], "d"), 64)
	if err != nil {
		return PlayerPosition{}, err
	}
	y, err := strconv.ParseFloat(strings.TrimSuffix(match[2], "d"), 64)
	if err != nil {
		return PlayerPosition{}, err
	}
	z, err := strconv.ParseFloat(strings.TrimSuffix(match[3], "d"), 64)
	if err != nil {
		return PlayerPosition{}, err
	}

	return PlayerPosition{X: x, Y: y, Z: z}, nil
}

// GetBlockAt gets the block type at a position
func GetBlockAt(ctx context.Context, rcon testenv.RCONHelper, pos models.V3) (string, error) {
	// First try data get for block entities (chests, furnaces, etc.)
	x, y, z := blockCoords(pos)
	cmd := fmt.Sprintf("data get block %d %d %d", x, y, z)
	resp, err := rcon.Exec(ctx, cmd)
	if err == nil && !strings.Contains(resp, "not a block entity") {
		// Response format: "The block at 9, 70, -155 has the following block data: {id: "minecraft:stone", ...}"
		re := regexp.MustCompile(`id:\s*"([^"]+)"`)
		match := re.FindStringSubmatch(resp)
		if len(match) == 2 {
			return match[1], nil
		}
	}

	// For regular blocks, use setblock to check type (replace with same block)
	// This works because setblock reports the previous block
	cmd = fmt.Sprintf("setblock %d %d %d minecraft:barrier keep", x, y, z)
	resp, err = rcon.Exec(ctx, cmd)
	if err == nil && strings.Contains(resp, "already") {
		// "Could not set the block" means there's already a barrier there
		return "minecraft:barrier", nil
	}

	// Fallback: just use execute store to get block data
	cmd = fmt.Sprintf("execute store result score @s test run data get block %d %d %d", x, y, z)
	resp, err = rcon.Exec(ctx, cmd)

	return "minecraft:unknown", fmt.Errorf("could not determine block type: %s", resp)
}

// GetInventoryItems gets all items in a player's inventory
func GetInventoryItems(ctx context.Context, rcon testenv.RCONHelper, playerName string) ([]InventoryItem, error) {
	cmd := fmt.Sprintf("data get entity %s Inventory", playerName)
	resp, err := rcon.Exec(ctx, cmd)
	if err != nil {
		return nil, err
	}

	items := []InventoryItem{}

	// If empty, response is "[]" or similar
	if strings.Contains(resp, "has no item") || resp == "[]" {
		return items, nil
	}

	entryRe := regexp.MustCompile(`\{([^}]*)\}`)
	slotRe := regexp.MustCompile(`(?i)Slot:\s*(-?\d+)b`)
	idRe := regexp.MustCompile(`(?i)id:\s*"([^"]+)"`)
	countRe := regexp.MustCompile(`(?i)count:\s*(\d+)b?`)

	entries := entryRe.FindAllStringSubmatch(resp, -1)
	for _, entry := range entries {
		if len(entry) != 2 {
			continue
		}
		segment := entry[1]

		slotMatch := slotRe.FindStringSubmatch(segment)
		idMatch := idRe.FindStringSubmatch(segment)
		countMatch := countRe.FindStringSubmatch(segment)

		if slotMatch == nil || idMatch == nil || countMatch == nil {
			continue
		}

		slot, err := strconv.Atoi(slotMatch[1])
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(countMatch[1])
		if err != nil {
			continue
		}

		items = append(items, InventoryItem{
			Slot:  slot,
			ID:    idMatch[1],
			Count: count,
		})
	}

	return items, nil
}

// WaitForPlayerOnline waits for a player to appear in the server player list.
// Returns true if the player appears before the timeout, false otherwise.
func WaitForPlayerOnline(ctx context.Context, rcon testenv.RCONHelper, player string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := rcon.Exec(ctx, "list")
		if err == nil && strings.Contains(resp, player) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// String returns the Minecraft-compatible direction name for the orientation
func (o Orientation) String() string {
	switch o {
	case North:
		return "north"
	case South:
		return "south"
	case East:
		return "east"
	case West:
		return "west"
	default:
		return "north"
	}
}

// orientationVector returns the X and Z delta for an orientation
func orientationVector(o Orientation) (dx, dz int) {
	switch o {
	case North:
		return 0, -1 // -Z
	case South:
		return 0, 1 // +Z
	case East:
		return 1, 0 // +X
	case West:
		return -1, 0 // -X
	default:
		return 0, -1
	}
}

// oppositeOrientation returns the opposite orientation
func oppositeOrientation(o Orientation) Orientation {
	switch o {
	case North:
		return South
	case South:
		return North
	case East:
		return West
	case West:
		return East
	default:
		return South
	}
}

// ClearArea clears a rectangular area to air using /fill command
func ClearArea(ctx context.Context, rcon testenv.RCONHelper, x1, y1, z1, x2, y2, z2 int) error {
	cmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air", x1, y1, z1, x2, y2, z2)
	_, err := rcon.Exec(ctx, cmd)
	return err
}

// BuildPlatform builds a solid platform using /fill command
func BuildPlatform(ctx context.Context, rcon testenv.RCONHelper, x, y, z, width, depth int, blockType string) error {
	blockSpec := normalizeBlockSpecForCommand(blockType)
	cmd := fmt.Sprintf("fill %d %d %d %d %d %d %s", x, y, z, x+width-1, y, z+depth-1, blockSpec)
	_, err := rcon.Exec(ctx, cmd)
	return err
}

// BuildPlatformWithNotch builds a platform and clears a 1x2 vertical notch to avoid blocking paths.
func BuildPlatformWithNotch(ctx context.Context, rcon testenv.RCONHelper, x, y, z, width, depth int, blockType string, notchX, notchZ int) error {
	if err := BuildPlatform(ctx, rcon, x, y, z, width, depth, blockType); err != nil {
		return err
	}
	// Clear the notch and headroom so stairs/ladders can connect cleanly.
	if _, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:air", notchX, y, notchZ)); err != nil {
		return err
	}
	if _, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:air", notchX, y+1, notchZ)); err != nil {
		return err
	}
	return nil
}

// BuildLadder builds a vertical ladder tower with backing wall
func BuildLadder(ctx context.Context, rcon testenv.RCONHelper, x, z, yBottom, yTop int, facing Orientation) error {
	// Determine direction vectors
	dx, dz := orientationVector(facing)

	// Build backing wall (one block in the direction the ladder faces)
	wallX, wallZ := x+dx, z+dz
	for y := yBottom; y <= yTop; y++ {
		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone", wallX, y, wallZ)
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	// Place ladders on the wall face (facing away from wall toward origin)
	ladderFacing := oppositeOrientation(facing)
	for y := yBottom + 1; y <= yTop; y++ {
		cmd := fmt.Sprintf("setblock %d %d %d minecraft:ladder[facing=%s]", x, y, z, ladderFacing.String())
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}

// BuildStairsDescending builds a staircase that descends in the facing direction.
func BuildStairsDescending(ctx context.Context, rcon testenv.RCONHelper, x, yTop, z, steps int, facing Orientation) error {
	dx, dz := orientationVector(facing)
	stairFacing := oppositeOrientation(facing)

	for step := 0; step < steps; step++ {
		currentX := x + dx*step
		currentZ := z + dz*step
		currentY := yTop - 1 - step

		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone_stairs[facing=%s,half=bottom]",
			currentX, currentY, currentZ, stairFacing.String())
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}

// BuildStairs builds a staircase with incrementing Y
func BuildStairs(ctx context.Context, rcon testenv.RCONHelper, x, y, z, steps int, facing Orientation) error {
	dx, dz := orientationVector(facing)

	for step := 0; step < steps; step++ {
		currentX := x + dx*step
		currentZ := z + dz*step
		currentY := y + step

		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone_stairs[facing=%s,half=bottom]",
			currentX, currentY, currentZ, facing.String())
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}

// BuildVine builds a pillar with vines attached, orientation-aware.
// The vines are placed at (x, z) and the pillar is placed behind them (opposite of facing direction).
// The agent climbs the vines while moving in the facing direction.
func BuildVine(ctx context.Context, rcon testenv.RCONHelper, x, z, yBottom, yTop int, facing Orientation) error {
	dx, dz := orientationVector(facing)

	// Pillar is behind the vines (opposite of movement direction)
	pillarX := x - dx
	pillarZ := z - dz

	// Build pillar to hang vines from
	for y := yBottom; y <= yTop; y++ {
		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone", pillarX, y, pillarZ)
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	// Vine faces the pillar (opposite of movement direction)
	vineFacing := oppositeOrientation(facing)

	// Place vines at the climb position
	for y := yBottom; y <= yTop; y++ {
		cmd := fmt.Sprintf("setblock %d %d %d minecraft:vine[%s=true]", x, y, z, vineFacing.String())
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}

// BuildBlockSteps builds a staircase using full blocks (each 1 block higher)
func BuildBlockSteps(ctx context.Context, rcon testenv.RCONHelper, x, y, z, steps int, facing Orientation) error {
	dx, dz := orientationVector(facing)

	for step := 0; step < steps; step++ {
		currentX := x + dx*(step+1)
		currentZ := z + dz*(step+1)
		currentY := y + step + 1

		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone", currentX, currentY, currentZ)
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}

// BuildBlockStepsDescending builds a descending staircase using full blocks.
func BuildBlockStepsDescending(ctx context.Context, rcon testenv.RCONHelper, x, yTop, z, steps int, facing Orientation) error {
	dx, dz := orientationVector(facing)

	for step := 0; step < steps; step++ {
		currentX := x + dx*(step+1)
		currentZ := z + dz*(step+1)
		currentY := yTop - 1 - step

		cmd := fmt.Sprintf("setblock %d %d %d minecraft:stone", currentX, currentY, currentZ)
		if _, err := rcon.Exec(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}
