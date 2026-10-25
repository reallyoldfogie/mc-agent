package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

var (
	inventorySnapshotSlotRe  = regexp.MustCompile(`(?i)Slot:\s*(-?\d+)b`)
	inventorySnapshotIDRe    = regexp.MustCompile(`(?i)id:\s*"([^"]+)"`)
	inventorySnapshotCountRe = regexp.MustCompile(`(?i)count:\s*(\d+)b?`)
)

// InventorySnapshotSlot captures one occupied inventory slot's full state -
// id, count, and raw per-slot "components" NBT (not just id/count, which
// loses enchantments, custom names, damage, etc.) - so RestoreInventorySnapshot
// can faithfully replay it. Slot uses the vanilla Inventory NBT numbering:
// hotbar 0-8, main inventory 9-35, armor 100-103, offhand -106 - the same
// numbering inventorySlotArg below translates into /item replace entity's
// own slot vocabulary (hotbar.N, inventory.N, armor.*, weapon.offhand).
// docs/plans/COMBAT_COMPETITION_PLAN.md Phase 0 item 5.
type InventorySnapshotSlot struct {
	Slot       int
	ID         string
	Count      int
	Components string // raw "{...}" NBT text of this slot's "components" compound, or "" if absent
}

// InventorySnapshot is every occupied slot SaveInventorySnapshot captured.
type InventorySnapshot []InventorySnapshotSlot

// SaveInventorySnapshot captures playerName's full inventory - every
// occupied hotbar/main-inventory slot's id, count, and raw NBT component
// data via /data get entity ... Inventory, PLUS worn armor and the offhand
// item via a separate /data get entity ... equipment query. These live in
// two different NBT locations depending on server version: confirmed live
// (not assumed) that at least one supported version no longer reports worn
// armor/offhand inside the "Inventory" list at all - they only appear under
// a top-level "equipment" compound (keys head/chest/legs/feet/offhand) -
// while hotbar/main inventory stayed in "Inventory" as before. Both are
// read and merged into one InventorySnapshot so callers don't need to know
// which location a given version actually used.
//
// Requires VerifyPlayerNearPosition first, the same safety reasoning
// SeedMatchReset's own doc comment gives: this framework may run on a
// shared server, so naming a player here doesn't by itself mean they're
// standing in the arena right now.
func (a *agent) SaveInventorySnapshot(ctx context.Context, playerName string, center models.V3, radius float64) (InventorySnapshot, error) {
	near, err := a.VerifyPlayerNearPosition(ctx, playerName, center, radius)
	if err != nil {
		return nil, fmt.Errorf("save inventory snapshot: %w", err)
	}
	if !near {
		return nil, fmt.Errorf("save inventory snapshot: %s is not within %.1f blocks of the match arena center - refusing to read their inventory", playerName, radius)
	}
	if a.cfg.RCON == nil {
		return nil, fmt.Errorf("save inventory snapshot: RCON not configured for this agent")
	}
	resp, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("data get entity %s Inventory", playerName))
	if err != nil {
		return nil, fmt.Errorf("save inventory snapshot: %w", err)
	}
	snapshot := parseInventorySnapshot(resp)

	// Best-effort: an older server version with no "equipment" compound at
	// all (armor/offhand still inline in "Inventory") answers with a
	// command-syntax error here, which parseEquipmentSnapshot simply
	// parses as empty rather than erroring.
	equipResp, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("data get entity %s equipment", playerName))
	if err == nil {
		snapshot = append(snapshot, parseEquipmentSnapshot(equipResp)...)
	}
	return snapshot, nil
}

// RestoreInventorySnapshot replays snapshot onto playerName: /clear followed
// by one /item replace entity ... with <item>[<components>] <count> per
// slot - the only vanilla-sanctioned way to bulk-write a live, connected
// player's inventory (/data merge|modify entity is rejected for one, "Unable
// to modify player data" - confirmed via RCON, not assumed). Requires
// VerifyPlayerNearPosition first, like every other match-setup helper in
// this package.
func (a *agent) RestoreInventorySnapshot(ctx context.Context, playerName string, snapshot InventorySnapshot, center models.V3, radius float64) error {
	near, err := a.VerifyPlayerNearPosition(ctx, playerName, center, radius)
	if err != nil {
		return fmt.Errorf("restore inventory snapshot: %w", err)
	}
	if !near {
		return fmt.Errorf("restore inventory snapshot: %s is not within %.1f blocks of the match arena center - refusing to modify their inventory", playerName, radius)
	}
	if a.cfg.RCON == nil {
		return fmt.Errorf("restore inventory snapshot: RCON not configured for this agent")
	}
	if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("clear %s", playerName)); err != nil {
		return fmt.Errorf("restore inventory snapshot: clear via RCON: %w", err)
	}
	for _, item := range snapshot {
		slotArg, err := inventorySlotArg(item.Slot)
		if err != nil {
			return fmt.Errorf("restore inventory snapshot: %w", err)
		}
		itemArg := normalizeItemName(item.ID)
		if item.Components != "" {
			bracket, err := convertComponentsNBTToBracketSyntax(item.Components)
			if err != nil {
				return fmt.Errorf("restore inventory snapshot: slot %d: %w", item.Slot, err)
			}
			itemArg += bracket
		}
		cmd := fmt.Sprintf("item replace entity %s %s with %s %d", playerName, slotArg, itemArg, item.Count)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			return fmt.Errorf("restore inventory snapshot: restore slot %d via RCON: %w", item.Slot, err)
		}
	}
	return nil
}

// inventorySlotArg translates a raw Inventory NBT slot index into
// /item replace entity's own slot vocabulary. See InventorySnapshotSlot's
// own doc comment for the numbering this accepts.
func inventorySlotArg(slot int) (string, error) {
	switch {
	case slot >= 0 && slot <= 8:
		return fmt.Sprintf("hotbar.%d", slot), nil
	case slot >= 9 && slot <= 35:
		return fmt.Sprintf("inventory.%d", slot-9), nil
	case slot == 100:
		return "armor.feet", nil
	case slot == 101:
		return "armor.legs", nil
	case slot == 102:
		return "armor.chest", nil
	case slot == 103:
		return "armor.head", nil
	case slot == -106:
		return "weapon.offhand", nil
	default:
		return "", fmt.Errorf("unsupported inventory slot %d", slot)
	}
}

// parseInventorySnapshot parses a /data get entity ... Inventory response
// (e.g. `... has the following entity data: [{Slot: 0b, id: "minecraft:...",
// count: 1, components: {...}}, ...]`) into one InventorySnapshotSlot per
// occupied slot. An empty inventory ("[]" or "has no item") yields an empty,
// non-nil snapshot.
func parseInventorySnapshot(resp string) InventorySnapshot {
	snapshot := InventorySnapshot{}
	if strings.Contains(resp, "has no item") {
		return snapshot
	}

	for _, segment := range splitTopLevelBraceEntriesAgent(resp) {
		slotMatch := inventorySnapshotSlotRe.FindStringSubmatch(segment)
		idMatch := inventorySnapshotIDRe.FindStringSubmatch(segment)
		countMatch := inventorySnapshotCountRe.FindStringSubmatch(segment)
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
		snapshot = append(snapshot, InventorySnapshotSlot{
			Slot:       slot,
			ID:         idMatch[1],
			Count:      count,
			Components: extractComponentsNBT(segment),
		})
	}
	return snapshot
}

// equipmentSlotNumbers maps the "equipment" compound's own key names to the
// same synthetic Inventory slot numbers inventorySlotArg already translates
// (armor 100-103, offhand -106) - see InventorySnapshotSlot's own doc
// comment and SaveInventorySnapshot's doc comment for why a second NBT
// location exists for these.
var equipmentSlotNumbers = map[string]int{
	"feet": 100, "legs": 101, "chest": 102, "head": 103, "offhand": -106,
}

// parseEquipmentSnapshot parses a /data get entity ... equipment response
// (e.g. `... has the following entity data: {head: {id: "minecraft:...",
// count: 1, components: {...}}, ...}`) into one InventorySnapshotSlot per
// occupied equipment key. A response with no recognizable "{...}" compound
// (e.g. an older server with no "equipment" path at all) yields an empty,
// non-nil snapshot rather than an error - SaveInventorySnapshot treats this
// path as best-effort.
func parseEquipmentSnapshot(resp string) InventorySnapshot {
	snapshot := InventorySnapshot{}
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start == -1 || end == -1 || end <= start {
		return snapshot
	}
	inner := resp[start+1 : end]
	for _, entry := range splitTopLevelCommaEntries(inner) {
		key, value, err := splitTopLevelKeyValue(entry)
		if err != nil {
			continue
		}
		slot, ok := equipmentSlotNumbers[strings.TrimSpace(key)]
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		idMatch := inventorySnapshotIDRe.FindStringSubmatch(value)
		if idMatch == nil {
			continue
		}
		count := 1
		if countMatch := inventorySnapshotCountRe.FindStringSubmatch(value); countMatch != nil {
			if c, err := strconv.Atoi(countMatch[1]); err == nil {
				count = c
			}
		}
		snapshot = append(snapshot, InventorySnapshotSlot{
			Slot:       slot,
			ID:         idMatch[1],
			Count:      count,
			Components: extractComponentsNBT(value),
		})
	}
	return snapshot
}

// splitTopLevelBraceEntriesAgent splits a response string into its
// top-level, brace-delimited entries (e.g. each item compound inside an
// Inventory list), ignoring braces inside quoted strings. This mirrors
// testing/rcon_helpers.go's own splitTopLevelBraceEntries exactly; it's
// duplicated here rather than imported because the testing package depends
// on agent, so agent cannot import testing without a cycle.
func splitTopLevelBraceEntriesAgent(s string) []string {
	var entries []string
	depth := 0
	inQuotes := false
	escaped := false
	start := -1
	for i, r := range s {
		if inQuotes {
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuotes = false
			}
			continue
		}
		switch r {
		case '"':
			inQuotes = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				entries = append(entries, s[start:i+1])
				start = -1
			}
		}
	}
	return entries
}

// extractComponentsNBT returns the raw "{...}" text of a slot compound's
// "components" field, or "" if the slot has none.
func extractComponentsNBT(segment string) string {
	key := "components:"
	idx := strings.Index(segment, key)
	if idx == -1 {
		return ""
	}
	braceStart := strings.IndexByte(segment[idx+len(key):], '{')
	if braceStart == -1 {
		return ""
	}
	braceStart += idx + len(key)

	depth := 0
	inQuotes := false
	escaped := false
	for i := braceStart; i < len(segment); i++ {
		c := segment[i]
		if inQuotes {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inQuotes = false
			}
			continue
		}
		switch c {
		case '"':
			inQuotes = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return segment[braceStart : i+1]
			}
		}
	}
	return ""
}

// convertComponentsNBTToBracketSyntax converts a "components" compound's
// NBT text (as /data get entity ... Inventory reports it, e.g.
// `{"minecraft:enchantments": {levels: {"minecraft:sharpness": 1}}}`) into
// the square-bracket component-override syntax the item_stack argument
// (used by /give and /item replace entity ... with) accepts, e.g.
// `[minecraft:enchantments={levels:{"minecraft:sharpness":1}}]` - only the
// top-level ":" separators between component-id keys and their values
// become "=", and the outer braces become brackets; nested NBT keeps its
// own ":" syntax unchanged since it's still valid inside a component value.
func convertComponentsNBTToBracketSyntax(nbt string) (string, error) {
	nbt = strings.TrimSpace(nbt)
	if len(nbt) < 2 || nbt[0] != '{' || nbt[len(nbt)-1] != '}' {
		return "", fmt.Errorf("convert components NBT: not a brace-delimited compound: %q", nbt)
	}
	inner := nbt[1 : len(nbt)-1]
	entries := splitTopLevelCommaEntries(inner)
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		key, value, err := splitTopLevelKeyValue(entry)
		if err != nil {
			return "", err
		}
		key = strings.Trim(strings.TrimSpace(key), `"`)
		parts = append(parts, key+"="+strings.TrimSpace(value))
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

// splitTopLevelCommaEntries splits s on "," that occur outside any
// nested {}/[] grouping and outside quoted strings.
func splitTopLevelCommaEntries(s string) []string {
	var entries []string
	depth := 0
	inQuotes := false
	escaped := false
	start := 0
	for i, r := range s {
		if inQuotes {
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuotes = false
			}
			continue
		}
		switch r {
		case '"':
			inQuotes = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				entries = append(entries, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		entries = append(entries, s[start:])
	}
	return entries
}

// splitTopLevelKeyValue splits entry on its first ":" that occurs outside
// any nested {}/[] grouping and outside quoted strings.
func splitTopLevelKeyValue(entry string) (key, value string, err error) {
	depth := 0
	inQuotes := false
	escaped := false
	for i, r := range entry {
		if inQuotes {
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuotes = false
			}
			continue
		}
		switch r {
		case '"':
			inQuotes = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ':':
			if depth == 0 {
				return entry[:i], entry[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("no top-level ':' found in %q", entry)
}
