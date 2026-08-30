package testing

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// CourseWaypoint is one point in a NavigationCourse.
type CourseWaypoint struct {
	Pos models.V3
}

// WaypointResult is the server-authoritative outcome for one waypoint.
type WaypointResult struct {
	// TriggeredTime is the vanilla world-age tick (from `time query
	// gametime`) the waypoint was reached, or 0 if it never was.
	TriggeredTime int64
}

func navWaypointTag(index int) string {
	return fmt.Sprintf("nav_wp_%d", index)
}

// NavigationCourse is a server-tracked sequence of waypoints, set up by
// SetupNavigationCourse. See PHASE_9_PLAN.md for the full design and
// rationale: this gives a navigation test independent,
// server-authoritative confirmation that it visited a sequence of points
// in order, rather than relying solely on the same client-side position
// tracking the movement command under test also uses to decide when it's
// "arrived."
//
// Two things learned only by testing live, neither obvious from the
// vanilla command reference alone:
//
//  1. Per-waypoint state (whether/when it was reached) lives on a
//     scoreboard objective, not custom entity NBT: block_display entities
//     were found to silently discard an arbitrary custom top-level NBT
//     compound added via /summon (confirmed by dumping an entity's full
//     NBT immediately after summoning it with one - the tag was simply
//     absent, no error reported). A scoreboard is the mechanism vanilla
//     actually exposes for attaching an arbitrary integer to an entity for
//     command logic like this.
//  2. Each waypoint's command block pair is placed immediately next to
//     that waypoint, not gathered in one place elsewhere in the world
//     (e.g. near the player's start). The identical condition/run chain,
//     targeting a far-away waypoint via "execute as <tag> at @s if entity
//     @a[...] run ...", consistently failed (SuccessCount stuck at 0
//     despite LastExecution incrementing normally - the block was ticking,
//     its command just never ran) whenever the command block itself sat
//     far from that waypoint, and consistently succeeded the moment the
//     same command block was moved next to it instead - confirmed with
//     the command block colocated with the player and separately
//     colocated with a distant waypoint, both cases working, only a
//     distant command-block-to-waypoint separation failing. This appears
//     to be a real constraint on "at"-repositioned execution context
//     resolving entity selectors, not a chunk-loading or
//     simulation-distance effect (both were independently ruled out with
//     the command block's own chunk explicitly force-loaded and
//     simulation-distance raised well past the separation distance).
//  3. Each waypoint uses two *independent* repeating command blocks (color
//     flip and score stamp), not a trigger+chain pair. A chain block only
//     fires when the block behind it reports success, and that report -
//     queryable via "data get block ... SuccessCount" - was found to be
//     unreliable in this environment: a live diagnostic showed the trigger
//     block's "data merge entity" visibly took effect (the waypoint really
//     did turn green) while its own SuccessCount still read 0, so the
//     chained block's "conditionMet" never went true and it never ran.
//     Since both halves of the job gate on the exact same, independently
//     computable condition ("if score ... matches 0 if entity ... distance
//     ..R"), there's no need for one to depend on the other's reported
//     success at all: two unchained repeating blocks checking that
//     condition on their own, every tick, both fire the tick it first
//     becomes true and both stop firing (self-limiting) once the score
//     write flips it to non-zero.
type NavigationCourse struct {
	objective     string
	tags          []string
	commandPairs  [][2][3]int // [i] = {triggerBlockPos, chainBlockPos}, both next to waypoint i
	forceloadedXZ [][2]int
}

// SetupNavigationCourse spawns one glowing red marker entity per waypoint,
// plus a command block pair next to each one, that flips that waypoint's
// marker green and stamps the tick it happened the moment botName comes
// within radius blocks of it.
//
// radius applies to the whole course, not per waypoint: vanilla selectors
// can't compare distance against a value that varies per entity, so this
// bakes the literal radius into each command block's own command text,
// appropriate to whichever single transport mode (walking, elytra, a
// vehicle) the course is testing.
func SetupNavigationCourse(ctx context.Context, rcon testenv.RCONHelper, botName string, radius float64, waypoints []CourseWaypoint) (*NavigationCourse, error) {
	if len(waypoints) == 0 {
		return nil, fmt.Errorf("navigation course needs at least one waypoint")
	}

	objective := "nav_" + botName
	if _, err := rcon.Exec(ctx, fmt.Sprintf("scoreboard objectives add %s dummy", objective)); err != nil {
		return nil, fmt.Errorf("create scoreboard objective: %w", err)
	}

	course := &NavigationCourse{
		objective:    objective,
		tags:         make([]string, len(waypoints)),
		commandPairs: make([][2][3]int, len(waypoints)),
	}

	for i, wp := range waypoints {
		tag := navWaypointTag(i)
		course.tags[i] = tag

		summonCmd := fmt.Sprintf(
			`summon minecraft:block_display %f %f %f {Tags:["%s"],block_state:{Name:"minecraft:red_stained_glass"},glowing:1b,glow_color_override:16711680}`,
			wp.Pos.X, wp.Pos.Y, wp.Pos.Z, tag,
		)
		if _, err := rcon.Exec(ctx, summonCmd); err != nil {
			return nil, fmt.Errorf("summon waypoint %d: %w", i, err)
		}

		// Scores start unset, and "if score ... matches <n>" never matches
		// an unset score (not even 0) - this initialization is what makes
		// the trigger command block's "matches 0" gate below meaningful.
		if _, err := rcon.Exec(ctx, fmt.Sprintf("scoreboard players set @e[tag=%s,limit=1] %s 0", tag, objective)); err != nil {
			return nil, fmt.Errorf("initialize score for waypoint %d: %w", i, err)
		}

		// /forceload add takes block coordinates and resolves the
		// containing chunk itself - no chunk-coordinate conversion needed.
		// This also covers the command block pair below: both sit in the
		// same chunk as their waypoint (see the type doc comment for why
		// that placement, not just the loading, is what matters).
		blockX, blockZ := int(wp.Pos.X), int(wp.Pos.Z)
		if _, err := rcon.Exec(ctx, fmt.Sprintf("forceload add %d %d", blockX, blockZ)); err != nil {
			return nil, fmt.Errorf("forceload waypoint %d: %w", i, err)
		}
		course.forceloadedXZ = append(course.forceloadedXZ, [2]int{blockX, blockZ})

		tx, ty, tz := int(wp.Pos.X), int(wp.Pos.Y)+3, int(wp.Pos.Z)
		course.commandPairs[i] = [2][3]int{{tx, ty, tz}, {tx + 1, ty, tz}}

		// Both blocks below are independent repeating blocks (not a
		// trigger+chain pair - see the type doc comment for why) that
		// each check the exact same gate: while this waypoint isn't yet
		// triggered (score still 0), is botName specifically (not just
		// any player - a test's stationary camera agent shouldn't be
		// able to trigger this) within radius. The score write is what
		// makes this self-limiting: once either block's tick sees the
		// condition true and writes a non-zero score, both stop
		// matching on every subsequent tick.
		colorCmd := fmt.Sprintf(
			`setblock %d %d %d minecraft:repeating_command_block[facing=up]{Command:"execute as @e[tag=%s,limit=1] at @s if score @s %s matches 0 if entity @a[name=%s,distance=..%g] run data merge entity @s {glow_color_override:65280,block_state:{Name:\"minecraft:green_stained_glass\"}}",auto:1b} replace`,
			tx, ty, tz, tag, objective, botName, radius,
		)
		if _, err := rcon.Exec(ctx, colorCmd); err != nil {
			return nil, fmt.Errorf("place color command block for waypoint %d: %w", i, err)
		}

		scoreCmd := fmt.Sprintf(
			`setblock %d %d %d minecraft:repeating_command_block[facing=up]{Command:"execute as @e[tag=%s,limit=1] at @s if score @s %s matches 0 if entity @a[name=%s,distance=..%g] run execute store result score @s %s run time query gametime",auto:1b} replace`,
			tx+1, ty, tz, tag, objective, botName, radius, objective,
		)
		if _, err := rcon.Exec(ctx, scoreCmd); err != nil {
			return nil, fmt.Errorf("place score command block for waypoint %d: %w", i, err)
		}
	}

	return course, nil
}

// scoreboardGetRe matches the response to `scoreboard players get <target>
// <objective>`, e.g. "ChestAccess has 1234 [nav_ChestAccess]".
var scoreboardGetRe = regexp.MustCompile(`has (-?\d+) \[`)

// Results queries the real, server-authoritative outcome of each waypoint,
// in course order.
func (c *NavigationCourse) Results(ctx context.Context, rcon testenv.RCONHelper) ([]WaypointResult, error) {
	results := make([]WaypointResult, len(c.tags))
	for i, tag := range c.tags {
		resp, err := rcon.Exec(ctx, fmt.Sprintf("scoreboard players get @e[tag=%s,limit=1] %s", tag, c.objective))
		if err != nil {
			return nil, fmt.Errorf("query waypoint %d: %w", i, err)
		}
		match := scoreboardGetRe.FindStringSubmatch(resp)
		if match == nil {
			return nil, fmt.Errorf("waypoint %d: could not parse score from response %q", i, resp)
		}
		triggeredTime, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("waypoint %d: parse score %q: %w", i, match[1], err)
		}
		results[i] = WaypointResult{TriggeredTime: triggeredTime}
	}
	return results, nil
}

// Cleanup removes the course's marker entities, command blocks,
// force-loaded chunks, and scoreboard objective.
func (c *NavigationCourse) Cleanup(ctx context.Context, rcon testenv.RCONHelper) error {
	for _, tag := range c.tags {
		if _, err := rcon.Exec(ctx, fmt.Sprintf("kill @e[tag=%s]", tag)); err != nil {
			return fmt.Errorf("kill waypoint entity %s: %w", tag, err)
		}
	}
	for i, pair := range c.commandPairs {
		tx, ty, tz := pair[0][0], pair[0][1], pair[0][2]
		if _, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d air replace", tx, ty, tz)); err != nil {
			return fmt.Errorf("remove trigger command block for waypoint %d: %w", i, err)
		}
		cx, cy, cz := pair[1][0], pair[1][1], pair[1][2]
		if _, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d air replace", cx, cy, cz)); err != nil {
			return fmt.Errorf("remove chain command block for waypoint %d: %w", i, err)
		}
	}
	for _, xz := range c.forceloadedXZ {
		if _, err := rcon.Exec(ctx, fmt.Sprintf("forceload remove %d %d", xz[0], xz[1])); err != nil {
			return fmt.Errorf("forceload remove (%d,%d): %w", xz[0], xz[1], err)
		}
	}
	if _, err := rcon.Exec(ctx, fmt.Sprintf("scoreboard objectives remove %s", c.objective)); err != nil {
		return fmt.Errorf("remove scoreboard objective: %w", err)
	}
	return nil
}
