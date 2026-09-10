package pathfinding

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// RCONSummoner is a minimal interface for summoning entities
type RCONSummoner interface {
	SummonEntity(ctx context.Context, x, y, z float64, entityType, nbtData string)
}

// HPADebugVisualizerConfig controls debug visualization styling.
type HPADebugVisualizerConfig struct {
	// PathBlock sets the block to use for path steps (expects <color>_stained_glass).
	PathBlock string
	// PathColor sets the color for path steps (used when PathBlock is empty).
	PathColor string
}

// HPADebugVisualizer handles visualization of HPA* paths and entrances
type HPADebugVisualizer struct {
	rcon      RCONSummoner
	enabled   bool
	pathBlock string
	logger    *slog.Logger
}

// NewHPADebugVisualizer creates a new debug visualizer
func NewHPADebugVisualizer(rcon RCONSummoner, cfg HPADebugVisualizerConfig, logger *slog.Logger) *HPADebugVisualizer {
	logger = utils.SafeLogger(logger)
	enabled := os.Getenv("MC_AGENT_DEBUG_HPA") != ""
	if enabled && rcon == nil {
		logger.Warn("[HPA Debug] MC_AGENT_DEBUG_HPA set but no RCON available, visualization disabled")
		enabled = false
	}

	pathBlock := resolveHPADebugPathBlock(logger, cfg.PathBlock, cfg.PathColor)
	if enabled {
		logger.Info("[HPA Debug] visualization enabled", "pathBlock", pathBlock)
	}

	return &HPADebugVisualizer{
		rcon:      rcon,
		enabled:   enabled,
		pathBlock: pathBlock,
		logger:    logger,
	}
}

// IsEnabled returns whether visualization is enabled
func (v *HPADebugVisualizer) IsEnabled() bool {
	return v != nil && v.enabled
}

// VisualizeEntrances shows entrances as red glass block_display entities
func (v *HPADebugVisualizer) VisualizeEntrances(ctx context.Context, entrances []*Entrance) {
	if !v.IsEnabled() {
		return
	}

	utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug] Visualizing %d entrances with red glass", len(entrances)))

	for i, entrance := range entrances {
		// Show Pos1 as red stained glass
		nbt := `{Tags:["hpa_debug"],block_state:{Name:"minecraft:red_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]}}`
		v.rcon.SummonEntity(ctx, entrance.Pos1.X+0.5, entrance.Pos1.Y+0.25, entrance.Pos1.Z+0.5, "block_display", nbt)
		nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s"}', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
			buildMultilineTextDisplay([]string{
				"ENTRANCE",
				fmt.Sprintf("%.0f %.0f %.0f", entrance.Pos1.X, entrance.Pos1.Y, entrance.Pos1.Z),
				fmt.Sprintf("C(%d,%d,%d)<->C(%d,%d,%d)",
					entrance.Cluster1.X, entrance.Cluster1.Y, entrance.Cluster1.Z,
					entrance.Cluster2.X, entrance.Cluster2.Y, entrance.Cluster2.Z),
			}))
		v.rcon.SummonEntity(ctx, entrance.Pos1.X+0.5, entrance.Pos1.Y+1.25, entrance.Pos1.Z+0.5, "text_display", nbt)

		if i < 5 || (i+1)%10 == 0 {
			utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug]   Entrance %d: (%.0f,%.0f,%.0f) <-> (%.0f,%.0f,%.0f)",
				i, entrance.Pos1.X, entrance.Pos1.Y, entrance.Pos1.Z,
				entrance.Pos2.X, entrance.Pos2.Y, entrance.Pos2.Z))
		}
	}

	if len(entrances) > 5 {
		utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug]   ... and %d more entrances", len(entrances)-5))
	}
}

func buildMultilineTextDisplay(in []string) string {
	return strings.Join(in, "\\n")
}

func resolveHPADebugPathBlock(logger *slog.Logger, block, color string) string {
	if normalized, ok := normalizeHPADebugBlock(block); ok {
		return normalized
	}
	if block != "" {
		logger.Warn("[HPA Debug] invalid path debug block, falling back to color/default", "block", block)
	}
	if normalized, ok := blockFromHPADebugColor(color); ok {
		return normalized
	}
	if color != "" {
		logger.Warn("[HPA Debug] invalid path debug color, falling back to default", "color", color)
	}
	return "minecraft:lime_stained_glass"
}

func normalizeHPADebugBlock(block string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(block))
	if trimmed == "" {
		return "", false
	}

	if after, ok := strings.CutPrefix(trimmed, "minecraft:"); ok {
		trimmed = after
	}

	if !strings.HasSuffix(trimmed, "_stained_glass") {
		return "", false
	}

	color := strings.TrimSuffix(trimmed, "_stained_glass")
	if !isHPADebugColorAllowed(color) {
		return "", false
	}
	return "minecraft:" + color + "_stained_glass", true
}

func blockFromHPADebugColor(color string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(color))
	if normalized == "" {
		return "", false
	}
	if !isHPADebugColorAllowed(normalized) {
		return "", false
	}
	return "minecraft:" + normalized + "_stained_glass", true
}

func isHPADebugColorAllowed(color string) bool {
	switch color {
	case "white", "orange", "magenta", "light_blue", "yellow", "lime",
		"pink", "gray", "light_gray", "cyan", "purple", "blue", "brown",
		"green", "red", "black":
		return true
	default:
		return false
	}
}

// VisualizePath shows path steps as stained glass block_display entities
func (v *HPADebugVisualizer) VisualizePath(ctx context.Context, path *Path) {
	if !v.IsEnabled() || path == nil {
		return
	}

	utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug] Visualizing path with %d steps using %s", len(path.Steps), v.pathBlock))

	for i, step := range path.Steps {
		// Jump movements already have centered coordinates (X+0.5, Z+0.5)
		// Other movements use block corners, so we center them for visualization
		visX, visZ := step.Position.X, step.Position.Z
		switch step.Movement {
		case AscendJump, DiagonalAscend, JumpToClimb, Jump2ToClimb, Jump2:
			// Already centered - use as-is
		default:
			// Block corner - add 0.5 to center for visualization
			visX += 0.5
			visZ += 0.5
		}

		// Show each step as stained glass (smaller so they don't overlap)
		nbt := fmt.Sprintf(`{Tags:["hpa_debug"],block_state:{Name:"%s"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`, v.pathBlock)
		v.rcon.SummonEntity(ctx, visX, step.Position.Y+0.2, visZ, "block_display", nbt)

		nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
			buildMultilineTextDisplay([]string{fmt.Sprintf("PATH %d", i), fmt.Sprintf("%.2f %.2f %.2f", step.Position.X, step.Position.Y, step.Position.Z), step.Movement.String()}))
		v.rcon.SummonEntity(ctx, visX, step.Position.Y+1.5, visZ, "text_display", nbt)

		if i < 10 || (i+1)%5 == 0 {
			utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug]   Step %d: (%.0f,%.0f,%.0f) %s",
				i, step.Position.X, step.Position.Y, step.Position.Z, step.Movement.String()))
		}
	}

	if len(path.Steps) > 10 {
		utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug]   ... and %d more steps", len(path.Steps)-10))
	}
}

// VisualizeAbstractPath shows the high-level abstract path (before refinement)
func (v *HPADebugVisualizer) VisualizeAbstractPath(ctx context.Context, edges []*AbstractEdge, start, goal models.V3) {
	if !v.IsEnabled() || len(edges) == 0 {
		return
	}

	utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug] Visualizing abstract path with %d edges", len(edges)))

	// Show start as yellow glass
	nbt := `{Tags:["hpa_debug"],block_state:{Name:"minecraft:yellow_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.6f,0.6f,0.6f], right_rotation:[0f,0f,0f,1f]}}`
	v.rcon.SummonEntity(ctx, start.X+0.5, start.Y+0.5, start.Z+0.5, "block_display", nbt)

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"START", fmt.Sprintf("%.2f %.2f %.2f", start.X, start.Y, start.Z)}))
	v.rcon.SummonEntity(ctx, start.X+0.5, start.Y+1.5, start.Z+0.5, "text_display", nbt)

	// Show each edge endpoint as orange glass
	for i, edge := range edges {
		pos := edge.To.GetPosition()
		nbt := `{Tags:["hpa_debug"],block_state:{Name:"minecraft:orange_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]}}`
		v.rcon.SummonEntity(ctx, pos.X+0.5, pos.Y+0.5, pos.Z+0.5, "block_display", nbt)

		nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
			buildMultilineTextDisplay([]string{"EDGE", fmt.Sprintf("%.2f %.2f %.2f", pos.X, pos.Y, pos.Z)}))
		v.rcon.SummonEntity(ctx, pos.X+0.5, pos.Y+2.5, pos.Z+0.5, "text_display", nbt)

		utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug]   Edge %d -> (%.0f,%.0f,%.0f) cost=%.2f",
			i, pos.X, pos.Y, pos.Z, edge.Cost))
	}

	// Show goal as cyan glass
	nbt = `{Tags:["hpa_debug"],block_state:{Name:"minecraft:cyan_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.6f,0.6f,0.6f], right_rotation:[0f,0f,0f,1f]}}`
	v.rcon.SummonEntity(ctx, goal.X+0.5, goal.Y+0.5, goal.Z+0.5, "block_display", nbt)

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.5f,0.5f,0.5f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"GOAL", fmt.Sprintf("%.2f %.2f %.2f", goal.X, goal.Y, goal.Z)}))
	v.rcon.SummonEntity(ctx, goal.X+0.5, goal.Y+2.5, goal.Z+0.5, "text_display", nbt)
}

// ClearVisualizations removes all display entities (requires command)
func (v *HPADebugVisualizer) ClearVisualizations(ctx context.Context) {
	if !v.IsEnabled() {
		return
	}

	utils.SafeLogger(v.logger).Debug(fmt.Sprintf("[HPA Debug] To clear visualizations, run: /kill @e[type=block_display,tag=hpa_debug]"))
}
