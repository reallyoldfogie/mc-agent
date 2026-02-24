package agent

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// Verify that agent implements physics.TrajectoryValidator
var _ = (*agent)(nil)

// RankedAimSolution represents an aiming solution with ranking information.
type RankedAimSolution struct {
	Pitch      float64                  // Pitch in degrees
	Power      float64                  // Power factor (0.0-1.0)
	Error      float64                  // Vertical error at target
	Trajectory []models.TrajectoryPoint // Full trajectory points
	IsBlocked  bool                     // Whether trajectory is blocked by obstacle
	BlockedAt  *models.V3               // Position where blocked (nil if clear)
	Rank       int                      // Ranking (0 = best, 1 = second best, etc)
}

// FindValidTrajectory finds the best unobstructed trajectory to hit a target.
// Tries trajectories in order of preference (low angle first) until finding one
// that is not blocked by obstacles.
//
// Returns the best valid trajectory, or error if no clear path exists.
// Sets IsBlocked=false for valid trajectories, IsBlocked=true for blocked ones.
func (a *agent) FindValidTrajectory(
	projectileType models.ProjectileType,
	origin, target models.V3) (*RankedAimSolution, error) {

	agentName := "unknown"
	if a.client != nil {
		agentName = a.client.Name()
	}

	if a.worldMgr == nil || a.shapeMgr == nil {
		// Can't validate trajectories without world state
		// Fall back to simple unvalidated trajectory
		log.Printf("[Agent %s][FindValidTrajectory] No world manager, using unvalidated trajectory", agentName)
		pitch, power, errorY, trajectory := physics.FindOptimalAiming(projectileType, origin, target)
		if len(trajectory) == 0 {
			return nil, fmt.Errorf("target unreachable")
		}
		return &RankedAimSolution{
			Pitch:      pitch,
			Power:      power,
			Error:      errorY,
			Trajectory: trajectory,
			IsBlocked:  false,
			BlockedAt:  nil,
			Rank:       0,
		}, nil
	}

	// Find all valid trajectories (low angle and high angle if available)
	rankedSolutions := a.findAllTrajectories(projectileType, origin, target)
	log.Printf("[Agent %s][FindValidTrajectory] findAllTrajectories returned %d solutions for %s", agentName, len(rankedSolutions), projectileType.String())

	if len(rankedSolutions) == 0 {
		log.Printf("[Agent %s][FindValidTrajectory] ERROR: No trajectory solutions found! Origin=(%.2f,%.2f,%.2f), Target=(%.2f,%.2f,%.2f)",
			agentName, origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z)
		return nil, fmt.Errorf("no trajectories found to target")
	}

	log.Printf("[Agent %s][FindValidTrajectory] Found %d trajectory solutions to target", agentName, len(rankedSolutions))

	// Try each trajectory in order until finding one that's clear
	for i, solution := range rankedSolutions {
		log.Printf("[Agent %s][FindValidTrajectory] Testing solution %d: pitch=%.2f°, power=%.3f, trajectory points=%d",
			agentName, i, solution.Pitch, solution.Power, len(solution.Trajectory))
		clear, hitPos, hitBlockName := physics.ValidateTrajectory(solution.Trajectory, a, target)

		if clear {
			solution.IsBlocked = false
			solution.BlockedAt = nil
			solution.Rank = i
			log.Printf("[Agent %s][FindValidTrajectory] Found clear trajectory at rank %d: pitch=%.2f°, power=%.3f",
				agentName, i, solution.Pitch, solution.Power)
			return solution, nil
		}

		// Mark as blocked for logging/debugging
		solution.IsBlocked = true
		solution.BlockedAt = hitPos
		solution.Rank = i
		if hitPos != nil {
			log.Printf("[Agent %s][FindValidTrajectory] Trajectory %d blocked at (%.2f, %.2f, %.2f)[%s]: pitch=%.2f°, power=%.3f",
				agentName, i, hitPos.X, hitPos.Y, hitPos.Z, hitBlockName, solution.Pitch, solution.Power)
		}
	}

	// All trajectories blocked - write detailed diagnostics to stderr for test output
	if len(rankedSolutions) > 0 {
		var diagBuf strings.Builder
		fmt.Fprintf(&diagBuf, "\n========== ALL TRAJECTORIES BLOCKED [Agent %s] ==========\n", agentName)
		fmt.Fprintf(&diagBuf, "Origin: (%.2f, %.2f, %.2f)\n", origin.X, origin.Y, origin.Z)
		fmt.Fprintf(&diagBuf, "Target: (%.2f, %.2f, %.2f)\n", target.X, target.Y, target.Z)
		fmt.Fprintf(&diagBuf, "Found %d solution(s), all blocked:\n\n", len(rankedSolutions))

		for i, solution := range rankedSolutions {
			fmt.Fprintf(&diagBuf, "------- SOLUTION %d (Rank=%d) -------\n", i, solution.Rank)
			fmt.Fprintf(&diagBuf, "Pitch: %.2f°, Power: %.3f, Error: %.4f blocks\n", solution.Pitch, solution.Power, solution.Error)
			if solution.BlockedAt != nil {
				fmt.Fprintf(&diagBuf, "Blocked at: (%.2f, %.2f, %.2f)\n", solution.BlockedAt.X, solution.BlockedAt.Y, solution.BlockedAt.Z)
			}

			// Show trajectory points with block info
			if len(solution.Trajectory) > 0 {
				fmt.Fprintf(&diagBuf, "Trajectory table (showing all points):\n")
				fmt.Fprintf(&diagBuf, "| Tick | X (blocks) | Y (blocks) | Z (blocks) | VelX | VelY | VelZ | Block Type |\n")
				fmt.Fprintf(&diagBuf, "|------|------------|------------|------------|------|------|------|------------|\n")

				for _, pt := range solution.Trajectory {
					blockType := "air"
					stateID, loaded := a.GetBlockAt(pt.Pos.X, pt.Pos.Y, pt.Pos.Z)
					if loaded && stateID != 0 {
						blockType = a.FullBlockName(stateID)
					}
					fmt.Fprintf(&diagBuf, "| %4d | %10.2f | %10.2f | %10.2f | %5.2f | %5.2f | %5.2f | %10s |\n",
						pt.Tick, pt.Pos.X, pt.Pos.Y, pt.Pos.Z, pt.Vel.X, pt.Vel.Y, pt.Vel.Z, blockType)
				}
			}
			fmt.Fprintf(&diagBuf, "\n")
		}

		fmt.Fprintf(&diagBuf, "==========================================\n")
		// Write to stderr so it appears in test output
		fmt.Fprint(os.Stderr, diagBuf.String())
		// Also log it
		log.Printf("[Agent %s][FindValidTrajectory] %s", agentName, diagBuf.String())

		return nil, fmt.Errorf("all trajectories to target are blocked by obstacles")
	}
	return nil, fmt.Errorf("no valid trajectories found")
}

// DiagnosticTrajectoryReport generates a detailed report of trajectory solutions for debugging
type DiagnosticTrajectoryReport struct {
	ProjectileType string
	Origin         models.V3
	Target         models.V3
	HorizontalDist float64
	VerticalDist   float64
	Solutions      []DiagnosticSolution
}

type DiagnosticSolution struct {
	Rank             int
	Pitch            float64 // degrees
	Power            float64
	Error            float64
	IsBlocked        bool
	BlockedAt        *models.V3
	BlockName        string
	TrajectoryPoints []DiagnosticTrajectoryPoint
}

type DiagnosticTrajectoryPoint struct {
	Tick      int
	X         float64
	Y         float64
	Z         float64
	VelX      float64
	VelY      float64
	VelZ      float64
	BlockType string
	Blocked   bool
}

// findAllTrajectories finds multiple trajectory solutions ranked by preference.
// Tries multiple power levels to find unobstructed paths. Returns solutions sorted by:
// 1. Higher power first (prefer full power)
// 2. Low-angle arc first (more reliable than high-angle)
func (a *agent) findAllTrajectories(
	projectileType models.ProjectileType,
	origin, target models.V3) []*RankedAimSolution {

	solutions := make([]*RankedAimSolution, 0)

	// Get base physics properties
	baseProps := physics.GetProjectileProps(projectileType)

	// Try multiple power levels in order of preference (higher power first)
	// This allows fallback to lower power if higher power trajectories are blocked
	powerLevels := []float64{1.0, 0.75, 0.5, 0.25}

	// var powerIdx int

	for powerIdx, power := range powerLevels {
		// for power100 := .25; power100 <= 1.0; power100 += .01 {
		// power := power100 / 100
		// powerIdx++
		// Create props with adjusted speed for this power level
		props := baseProps
		props.Speed = baseProps.Speed * power

		// Low arc solution (preferHighArc = false)
		lowSolution, errLow := physics.SolveAim(origin, target, props, false)
		if errLow == nil {
			adjustedV0 := lowSolution.V0
			trajectory := physics.SimulateProjectileTrajectory(projectileType, origin, adjustedV0, props.MaxTicks)
			if len(trajectory) > 0 {
				// Trim trajectory
				if lowSolution.Tick >= 0 && lowSolution.Tick < len(trajectory) {
					endIndex := min(lowSolution.Tick+5, len(trajectory))
					trimmed := make([]models.TrajectoryPoint, endIndex)
					copy(trimmed, trajectory[:endIndex])
					trajectory = trimmed
				}
				pitch := lowSolution.PitchRad * 180.0 / math.Pi // Convert radians to degrees
				solutions = append(solutions, &RankedAimSolution{
					Pitch:      pitch,
					Power:      power,
					Error:      lowSolution.ErrorY,
					Trajectory: trajectory,
					IsBlocked:  false, // Will be set by caller
					BlockedAt:  nil,
					Rank:       powerIdx * 2, // Rank by power level first, then arc
				})
			}
		}

		// High arc solution (preferHighArc = true)
		highSolution, errHigh := physics.SolveAim(origin, target, props, true)
		if errHigh == nil {
			adjustedV0 := highSolution.V0
			trajectory := physics.SimulateProjectileTrajectory(projectileType, origin, adjustedV0, props.MaxTicks)
			if len(trajectory) > 0 {
				// Trim trajectory
				if highSolution.Tick >= 0 && highSolution.Tick < len(trajectory) {
					endIndex := min(highSolution.Tick+5, len(trajectory))
					trimmed := make([]models.TrajectoryPoint, endIndex)
					copy(trimmed, trajectory[:endIndex])
					trajectory = trimmed
				}
				pitch := highSolution.PitchRad * 180.0 / math.Pi // Convert radians to degrees
				solutions = append(solutions, &RankedAimSolution{
					Pitch:      pitch,
					Power:      power,
					Error:      highSolution.ErrorY,
					Trajectory: trajectory,
					IsBlocked:  false, // Will be set by caller
					BlockedAt:  nil,
					Rank:       powerIdx*2 + 1, // High arc is secondary preference
				})
			}
		}
	}

	return solutions
}

// GenerateDiagnosticReport creates a detailed report of all trajectory solutions for debugging
// Shows each trajectory point, block types, and collision information
func (a *agent) GenerateDiagnosticReport(
	projectileType models.ProjectileType,
	origin, target models.V3) *DiagnosticTrajectoryReport {

	report := &DiagnosticTrajectoryReport{
		ProjectileType: projectileType.String(),
		Origin:         origin,
		Target:         target,
		HorizontalDist: math.Sqrt((target.X-origin.X)*(target.X-origin.X) + (target.Z-origin.Z)*(target.Z-origin.Z)),
		VerticalDist:   target.Y - origin.Y,
		Solutions:      make([]DiagnosticSolution, 0),
	}

	// Find all solutions
	rankedSolutions := a.findAllTrajectories(projectileType, origin, target)

	// Process each solution
	for _, solution := range rankedSolutions {
		diagSol := DiagnosticSolution{
			Rank:             solution.Rank,
			Pitch:            solution.Pitch,
			Power:            solution.Power,
			Error:            solution.Error,
			TrajectoryPoints: make([]DiagnosticTrajectoryPoint, 0),
		}

		// Validate trajectory
		clear, hitPos, hitBlockName := physics.ValidateTrajectory(solution.Trajectory, a, target)
		diagSol.IsBlocked = !clear
		diagSol.BlockedAt = hitPos
		diagSol.BlockName = hitBlockName

		// Add each trajectory point with block information
		for _, point := range solution.Trajectory {
			blockX := int(math.Floor(point.Pos.X))
			blockY := int(math.Floor(point.Pos.Y))
			blockZ := int(math.Floor(point.Pos.Z))

			blockType := "air"
			isBlocked := false

			stateID, loaded := a.GetBlockAt(float64(blockX), float64(blockY), float64(blockZ))
			if loaded && stateID != 0 {
				blockType = a.FullBlockName(stateID)
				collisionBoxes := a.GetCollisionBoxes(stateID, blockX, blockY, blockZ)
				if len(collisionBoxes) > 0 {
					isBlocked = true
				}
			}

			diagPoint := DiagnosticTrajectoryPoint{
				Tick:      point.Tick,
				X:         point.Pos.X,
				Y:         point.Pos.Y,
				Z:         point.Pos.Z,
				VelX:      point.Vel.X,
				VelY:      point.Vel.Y,
				VelZ:      point.Vel.Z,
				BlockType: blockType,
				Blocked:   isBlocked,
			}
			diagSol.TrajectoryPoints = append(diagSol.TrajectoryPoints, diagPoint)
		}

		report.Solutions = append(report.Solutions, diagSol)
	}

	return report
}

// FormatDiagnosticReport formats the diagnostic report as a readable string
func (a *agent) FormatDiagnosticReport(report *DiagnosticTrajectoryReport) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "========================================\n")
	fmt.Fprintf(&buf, "TRAJECTORY DIAGNOSTIC REPORT\n")
	fmt.Fprintf(&buf, "========================================\n\n")

	fmt.Fprintf(&buf, "Projectile Type: %s\n", report.ProjectileType)
	fmt.Fprintf(&buf, "Origin:          (%.2f, %.2f, %.2f)\n", report.Origin.X, report.Origin.Y, report.Origin.Z)
	fmt.Fprintf(&buf, "Target:          (%.2f, %.2f, %.2f)\n", report.Target.X, report.Target.Y, report.Target.Z)
	fmt.Fprintf(&buf, "Horizontal Dist: %.2f blocks\n", report.HorizontalDist)
	fmt.Fprintf(&buf, "Vertical Dist:   %.2f blocks\n\n", report.VerticalDist)

	if len(report.Solutions) == 0 {
		fmt.Fprintf(&buf, "ERROR: No trajectory solutions found\n")
		return buf.String()
	}

	fmt.Fprintf(&buf, "Found %d solution(s):\n\n", len(report.Solutions))

	for solIdx, sol := range report.Solutions {
		fmt.Fprintf(&buf, "------- SOLUTION %d (Rank=%d) -------\n", solIdx, sol.Rank)
		fmt.Fprintf(&buf, "Pitch:         %.2f°\n", sol.Pitch)
		fmt.Fprintf(&buf, "Power:         %.3f\n", sol.Power)
		fmt.Fprintf(&buf, "Error:         %.4f blocks\n", sol.Error)
		fmt.Fprintf(&buf, "Blocked:       %v\n", sol.IsBlocked)
		if sol.BlockedAt != nil {
			fmt.Fprintf(&buf, "Blocked At:    (%.2f, %.2f, %.2f) [%s]\n\n",
				sol.BlockedAt.X, sol.BlockedAt.Y, sol.BlockedAt.Z, sol.BlockName)
		} else if sol.IsBlocked {
			fmt.Fprintf(&buf, "Blocked At:    (unknown location)\n\n")
		} else {
			fmt.Fprintf(&buf, "Path Status:   CLEAR\n\n")
		}

		// Trajectory table
		fmt.Fprintf(&buf, "| Tick | X (blocks) | Y (blocks) | Z (blocks) | VelX | VelY | VelZ | Block Type | Hit?\n")
		fmt.Fprintf(&buf, "|------|------------|------------|------------|------|------|------|------------|-----\n")
		for _, point := range sol.TrajectoryPoints {
			hitMarker := " "
			if point.Blocked {
				hitMarker = "✗"
			}
			fmt.Fprintf(&buf, "| %4d | %10.2f | %10.2f | %10.2f | %5.2f | %5.2f | %5.2f | %10s | %s\n",
				point.Tick, point.X, point.Y, point.Z, point.VelX, point.VelY, point.VelZ, point.BlockType, hitMarker)
		}
		fmt.Fprintf(&buf, "\n")
	}

	fmt.Fprintf(&buf, "========================================\n")

	return buf.String()
}
