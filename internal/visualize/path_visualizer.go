package visualize

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Waypoint represents a single point in an expected path with optional label
type Waypoint struct {
	Pos   models.V3
	Label string
}

// PathVisualizerConfig controls visualization styling
type PathVisualizerConfig struct {
	BlockColor string // e.g., "lime", "cyan", "orange"
}

// DisplayEntitySummoner provides minimal interface for spawning display entities
type DisplayEntitySummoner interface {
	SummonEntity(ctx context.Context, x, y, z float64, entityType, nbtData string)
}

// RCONCommandExecutor provides an interface for executing RCON commands
type RCONCommandExecutor interface {
	Exec(ctx context.Context, cmd string) (string, error)
}

// VisualizerAdapter bridges RCON to the DisplayEntitySummoner interface
type VisualizerAdapter struct {
	rcon RCONCommandExecutor
}

func NewVisualizerAdapter(rcon RCONCommandExecutor) *VisualizerAdapter {
	return &VisualizerAdapter{rcon: rcon}
}

func (a *VisualizerAdapter) SummonEntity(ctx context.Context, x, y, z float64, entityType, nbtData string) {
	cmd := fmt.Sprintf("summon minecraft:%s %f %f %f %s", entityType, x, y, z, nbtData)
	_, err := a.rcon.Exec(ctx, cmd)
	if err != nil {
		log.Printf("[DisplayEntity] Failed to summon %s at (%.1f, %.1f, %.1f): %v", entityType, x, y, z, err)
	}
}

// VisualizeExpectedPath renders waypoints as display entities
func VisualizeExpectedPath(ctx context.Context, summoner DisplayEntitySummoner, waypoints []Waypoint, color string) {
	if summoner == nil || len(waypoints) == 0 {
		return
	}

	blockType := normalizeBlockColor(color)
	if blockType == "" {
		blockType = "minecraft:lime_stained_glass"
	} else {
		blockType = fmt.Sprintf("minecraft:%s_stained_glass", blockType)
	}

	log.Printf("[PathViz] Visualizing %d waypoints with %s", len(waypoints), blockType)

	for i, wp := range waypoints {
		// Block display for waypoint position
		nbt := fmt.Sprintf(`{Tags:["path_viz"],block_state:{Name:"%s"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`, blockType)
		summoner.SummonEntity(ctx, wp.Pos.X, wp.Pos.Y+0.2, wp.Pos.Z, "block_display", nbt)

		// Text display for label (if provided)
		if wp.Label != "" {
			text := buildMultilineText([]string{wp.Label, fmt.Sprintf("%.2f %.2f %.2f", wp.Pos.X, wp.Pos.Y, wp.Pos.Z)})
			nbt = fmt.Sprintf(`{Tags:["path_viz"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`, text)
			summoner.SummonEntity(ctx, wp.Pos.X, wp.Pos.Y+1.5, wp.Pos.Z, "text_display", nbt)
		}

		if i < 5 || (i+1)%5 == 0 {
			log.Printf("[PathViz]   Waypoint %d: (%.2f, %.2f, %.2f) %s",
				i, wp.Pos.X, wp.Pos.Y, wp.Pos.Z, wp.Label)
		}
	}

	if len(waypoints) > 5 {
		log.Printf("[PathViz]   ... and %d more waypoints", len(waypoints)-5)
	}
}

// VisualizeExpectedPathSimple is a convenience for visualizing a simple path without labels
func VisualizeExpectedPathSimple(ctx context.Context, summoner DisplayEntitySummoner, positions []models.V3, color string) {
	waypoints := make([]Waypoint, len(positions))
	for i, pos := range positions {
		waypoints[i] = Waypoint{
			Pos:   pos,
			Label: fmt.Sprintf("STEP %d", i),
		}
	}
	VisualizeExpectedPath(ctx, summoner, waypoints, color)
}

// ClearPathVisualizations removes all path visualization entities
// Note: This removes all entities with the "path_viz" tag
func ClearPathVisualizations(ctx context.Context, rcon RCONCommandExecutor) {
	cmd := "/kill @e[tag=path_viz]"
	_, err := rcon.Exec(ctx, cmd)
	if err != nil {
		log.Printf("[PathViz] Failed to clear visualizations: %v", err)
	} else {
		log.Printf("[PathViz] Cleared all path visualizations")
	}
}

// Helper functions

func buildMultilineText(lines []string) string {
	return strings.Join(lines, "\\n")
}

func normalizeBlockColor(color string) string {
	trimmed := strings.ToLower(strings.TrimSpace(color))
	if trimmed == "" {
		return ""
	}
	if isColorAllowed(trimmed) {
		return trimmed
	}
	log.Printf("[PathViz] Invalid color %q; falling back to lime", color)
	return "lime"
}

func isColorAllowed(color string) bool {
	switch color {
	case "white", "orange", "magenta", "light_blue", "yellow", "lime",
		"pink", "gray", "light_gray", "cyan", "purple", "blue", "brown",
		"green", "red", "black":
		return true
	default:
		return false
	}
}
