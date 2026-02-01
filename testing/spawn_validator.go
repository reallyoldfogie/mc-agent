package testing

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

// SpawnValidator checks if multi-agent spawn locations are mutually reachable.
// Agents spawning in caves, on different elevation levels, or separated by terrain
// are marked as unreachable and cause test validation to fail.
type SpawnValidator struct {
	positions map[string]models.V3
	reachable map[string]map[string]bool // name -> name -> reachable
}

// ValidationReport summarizes spawn location viability.
type ValidationReport struct {
	AllReachable       bool
	TotalAgents        int
	UnreachablePairs   []UnreachablePair
	Positions          map[string]models.V3
	ReachabilityMatrix map[string]map[string]bool
}

type UnreachablePair struct {
	AgentA string
	AgentB string
	Reason string
}

// NewSpawnValidator creates a new spawn validator.
func NewSpawnValidator() *SpawnValidator {
	return &SpawnValidator{
		positions: make(map[string]models.V3),
		reachable: make(map[string]map[string]bool),
	}
}

// RecordPosition stores an agent's spawn position.
func (sv *SpawnValidator) RecordPosition(name string, pos models.V3) {
	sv.positions[name] = pos
	if sv.reachable[name] == nil {
		sv.reachable[name] = make(map[string]bool)
	}
}

// Validate checks reachability between all agent pairs.
// Returns validation report with details about unreachable spawns.
func (sv *SpawnValidator) Validate(ctx context.Context) *ValidationReport {
	report := &ValidationReport{
		AllReachable:       true,
		TotalAgents:        len(sv.positions),
		UnreachablePairs:   []UnreachablePair{},
		Positions:          sv.positions,
		ReachabilityMatrix: sv.reachable,
	}

	agents := make([]string, 0, len(sv.positions))
	for name := range sv.positions {
		agents = append(agents, name)
	}

	// Check all pairs for reachability
	for i := 0; i < len(agents); i++ {
		for j := i + 1; j < len(agents); j++ {
			posA := sv.positions[agents[i]]
			posB := sv.positions[agents[j]]

			reachable, reason := sv.checkReachable(posA, posB)

			sv.reachable[agents[i]][agents[j]] = reachable
			sv.reachable[agents[j]][agents[i]] = reachable

			if !reachable {
				report.AllReachable = false
				report.UnreachablePairs = append(report.UnreachablePairs,
					UnreachablePair{
						AgentA: agents[i],
						AgentB: agents[j],
						Reason: reason,
					})
			}
		}
	}

	report.ReachabilityMatrix = sv.reachable
	return report
}

// checkReachable determines if two positions are reachable from each other.
// Returns (reachable, reason).
func (sv *SpawnValidator) checkReachable(posA, posB models.V3) (bool, string) {
	// Check horizontal distance
	hDist := math.Sqrt((posA.X-posB.X)*(posA.X-posB.X) + (posA.Z-posB.Z)*(posA.Z-posB.Z))
	if hDist > 256 { // Max practical walking distance for a test
		return false, fmt.Sprintf("too far apart: %.1f blocks horizontal", hDist)
	}

	// Check vertical separation
	yDiff := math.Abs(posA.Y - posB.Y)
	if yDiff > 8 {
		// More than 8 blocks apart vertically suggests different elevation levels
		// (e.g., one on surface, one in cave system)
		return false, fmt.Sprintf("different elevation: %.1f blocks apart (A=%.1f, B=%.1f)", yDiff, posA.Y, posB.Y)
	}

	// Check if one is suspiciously low (underground/cave)
	if posA.Y < 60 && posB.Y > 64 {
		return false, fmt.Sprintf("spawn location disparity: A underground (Y=%.1f) vs B surface (Y=%.1f)", posA.Y, posB.Y)
	}
	if posB.Y < 60 && posA.Y > 64 {
		return false, fmt.Sprintf("spawn location disparity: B underground (Y=%.1f) vs A surface (Y=%.1f)", posB.Y, posA.Y)
	}

	// All checks passed - likely reachable
	return true, ""
}

// String returns a human-readable validation report.
func (report *ValidationReport) String() string {
	var sb strings.Builder
	sb.WriteString("=== Spawn Location Validation Report ===\n")
	sb.WriteString(fmt.Sprintf("Total Agents: %d\n", report.TotalAgents))
	sb.WriteString(fmt.Sprintf("All Reachable: %v\n", report.AllReachable))
	sb.WriteString("\nAgent Positions:\n")

	for name, pos := range report.Positions {
		sb.WriteString(fmt.Sprintf("  %s: (%.2f, %.2f, %.2f)\n", name, pos.X, pos.Y, pos.Z))
	}

	if len(report.UnreachablePairs) > 0 {
		sb.WriteString("\nUnreachable Pairs:\n")
		for _, pair := range report.UnreachablePairs {
			sb.WriteString(fmt.Sprintf("  %s <-> %s: %s\n", pair.AgentA, pair.AgentB, pair.Reason))
		}
	} else if report.TotalAgents > 1 {
		sb.WriteString("\nAll agent pairs are reachable.\n")
	}

	return sb.String()
}
