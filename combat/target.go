// Package combat contains transport-independent combat decisions and models.
// Packet sending remains in agent; this package only reasons about snapshots
// and produces deterministic decisions for a controller to execute.
package combat

import (
	"sort"
	"strings"
)

// Category is the combat relevance of a tracked entity.
type Category uint8

const (
	Unknown Category = iota
	Neutral
	Hostile
	Player
)

// Target is the minimum snapshot required for target ranking.
type Target struct {
	EntityID   int32
	TypeName   string
	Category   Category
	X          float64
	Y          float64
	Z          float64
	VelocityX  float64
	VelocityY  float64
	VelocityZ  float64
	Distance   float64
	DirectionX float64
	DirectionZ float64
	Health     float32
	MaxHealth  float32
	Visible    bool
	Removed    bool
}

// TargetPolicy controls which non-hostile target categories combat may select.
// Players are opt-in so callers do not accidentally enable friendly fire.
type TargetPolicy struct {
	IncludePlayers bool
	IncludeNeutral bool
}

// Classify maps a registry type name to the default combat category.
func Classify(typeName string) Category {
	name := strings.ToLower(strings.TrimSpace(typeName))
	name = strings.TrimPrefix(name, "minecraft:")
	if name == "player" {
		return Player
	}
	switch name {
	case "blaze", "bogged", "breeze", "cave_spider", "creeper", "drowned",
		"elder_guardian", "ender_dragon", "endermite", "evoker", "ghast",
		"guardian", "hoglin", "husk", "magma_cube", "phantom", "piglin_brute",
		"pillager", "ravager", "shulker", "silverfish", "skeleton",
		"slime", "spider", "stray", "vex", "vindicator", "warden", "witch",
		"wither", "wither_skeleton", "zoglin", "zombie", "zombie_villager",
		"zombified_piglin":
		return Hostile
	default:
		return Neutral
	}
}

// Priority returns a deterministic score. Category is dominant, proximity
// is next, and a lower remaining health ratio is a tie-breaker.
func Priority(target Target) float64 {
	categoryScore := map[Category]float64{Unknown: 0, Neutral: 100, Hostile: 200, Player: 300}[target.Category]
	distance := target.Distance
	if distance < 0 {
		distance = 0
	}
	proximityScore := 100 / (1 + distance)
	healthScore := 0.0
	if target.MaxHealth > 0 {
		ratio := float64(target.Health) / float64(target.MaxHealth)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		healthScore = (1 - ratio) * 10
	}
	return categoryScore + proximityScore + healthScore
}

// Rank returns eligible targets from highest priority to lowest priority.
// Neutral targets are excluded unless includeNeutral is true.
func Rank(targets []Target, includeNeutral bool) []Target {
	return RankWithPolicy(targets, TargetPolicy{
		IncludePlayers: true,
		IncludeNeutral: includeNeutral,
	})
}

// RankWithPolicy returns eligible targets ordered by combat priority while
// applying the caller's explicit player and neutral-target policy.
func RankWithPolicy(targets []Target, policy TargetPolicy) []Target {
	eligible := make([]Target, 0, len(targets))
	for _, target := range targets {
		// A tracked living entity may remain in the table briefly between its
		// final health metadata and the server's removal packet. Once a known
		// maximum health is available, zero health is terminal and must not be
		// reacquired for another attack cycle. MaxHealth == 0 remains eligible
		// because it represents an incomplete/unknown health snapshot.
		if target.Removed || (target.MaxHealth > 0 && target.Health <= 0) ||
			!target.Visible || target.Category == Unknown {
			continue
		}
		if target.Category == Neutral && !policy.IncludeNeutral {
			continue
		}
		if target.Category == Player && !policy.IncludePlayers {
			continue
		}
		eligible = append(eligible, target)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, right := Priority(eligible[i]), Priority(eligible[j])
		if left != right {
			return left > right
		}
		if eligible[i].Distance != eligible[j].Distance {
			return eligible[i].Distance < eligible[j].Distance
		}
		return eligible[i].EntityID < eligible[j].EntityID
	})
	return eligible
}
