package testing

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"

	"github.com/reallyoldfogie/mc-agent/physics"
)

// ArrowPosition represents a single position update of an arrow
type ArrowPosition struct {
	Tick             int
	X, Y, Z          float64
	VelX, VelY, VelZ float64 // Velocity per tick (delta)
}

// AnalyzeArrowTrajectory reads agent logs and extracts arrow trajectory data
func AnalyzeArrowTrajectory(logFilePath string) ([]ArrowPosition, error) {
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var positions []ArrowPosition
	scanner := bufio.NewScanner(file)

	// Regex patterns for different log messages
	spawnPattern := regexp.MustCompile(`\[onAddEntity\] ARROW SPAWN: entityID=(\d+), pos=\(([\d.]+), ([\d.]+), ([\d.]+)\)`)
	movePattern := regexp.MustCompile(`\[onMoveEntityPosRot\] ARROW: entityID=(\d+), oldPos=\(([\d.-]+), ([\d.-]+), ([\d.-]+)\), delta=\(([\d.-]+), ([\d.-]+), ([\d.-]+)\), newPos=\(([\d.-]+), ([\d.-]+), ([\d.-]+)\)`)

	tick := 0
	for scanner.Scan() {
		line := scanner.Text()

		// Check for spawn
		if matches := spawnPattern.FindStringSubmatch(line); matches != nil {
			x, _ := strconv.ParseFloat(matches[2], 64)
			y, _ := strconv.ParseFloat(matches[3], 64)
			z, _ := strconv.ParseFloat(matches[4], 64)
			positions = append(positions, ArrowPosition{
				Tick: 0,
				X:    x,
				Y:    y,
				Z:    z,
			})
			tick = 1
			continue
		}

		// Check for movement
		if matches := movePattern.FindStringSubmatch(line); matches != nil {
			newX, _ := strconv.ParseFloat(matches[8], 64)
			newY, _ := strconv.ParseFloat(matches[9], 64)
			newZ, _ := strconv.ParseFloat(matches[10], 64)
			deltaX, _ := strconv.ParseFloat(matches[5], 64)
			deltaY, _ := strconv.ParseFloat(matches[6], 64)
			deltaZ, _ := strconv.ParseFloat(matches[7], 64)
			positions = append(positions, ArrowPosition{
				Tick: tick,
				X:    newX,
				Y:    newY,
				Z:    newZ,
				VelX: deltaX,
				VelY: deltaY,
				VelZ: deltaZ,
			})
			tick++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return positions, nil
}

// CompareTrajectories compares actual arrow trajectory with physics predictions
func CompareTrajectories(actualPositions []ArrowPosition, botOrigin physics.V3, targetOrigin physics.V3, predictedTrajectory []physics.TrajectoryPoint) {
	if len(actualPositions) < 2 {
		log.Printf("[Arrow Analysis] Not enough position data (got %d positions)", len(actualPositions))
		return
	}

	// Get arrow spawn position (first actual position)
	arrowSpawn := actualPositions[0]
	arrowSpawnPos := physics.V3{X: arrowSpawn.X, Y: arrowSpawn.Y, Z: arrowSpawn.Z}

	// Calculate horizontal distance from bot to target
	dx := targetOrigin.X - botOrigin.X
	dz := targetOrigin.Z - botOrigin.Z
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	verticalDist := targetOrigin.Y - botOrigin.Y

	// Calculate yaw for rotating trajectory from Z-axis to actual direction
	yaw := math.Atan2(-dx, -dz) * 180 / math.Pi
	yawRad := yaw * math.Pi / 180

	log.Printf("[Arrow Analysis] === Arrow Trajectory Analysis ===")
	log.Printf("[Arrow Analysis] Bot position: (%.2f, %.2f, %.2f)", botOrigin.X, botOrigin.Y, botOrigin.Z)
	log.Printf("[Arrow Analysis] Arrow spawn: (%.2f, %.2f, %.2f)", arrowSpawnPos.X, arrowSpawnPos.Y, arrowSpawnPos.Z)
	log.Printf("[Arrow Analysis] Target origin: (%.2f, %.2f, %.2f)", targetOrigin.X, targetOrigin.Y, targetOrigin.Z)
	log.Printf("[Arrow Analysis] Horizontal distance: %.2f blocks, Vertical: %.2f blocks, Yaw: %.1f°", horizontalDist, verticalDist, yaw)
	log.Printf("[Arrow Analysis] Actual positions received: %d", len(actualPositions))
	log.Printf("[Arrow Analysis] Predicted trajectory points: %d", len(predictedTrajectory))

	// Find landing position (last non-zero velocity position or last position)
	lastActual := actualPositions[len(actualPositions)-1]
	actualHorizontalDist := math.Sqrt((lastActual.X-botOrigin.X)*(lastActual.X-botOrigin.X) +
		(lastActual.Z-botOrigin.Z)*(lastActual.Z-botOrigin.Z))
	actualVerticalDrop := arrowSpawnPos.Y - lastActual.Y

	log.Printf("[Arrow Analysis] ")
	log.Printf("[Arrow Analysis] Actual landing position: (%.2f, %.2f, %.2f)", lastActual.X, lastActual.Y, lastActual.Z)
	log.Printf("[Arrow Analysis] Actual horizontal distance traveled: %.2f blocks", actualHorizontalDist)
	log.Printf("[Arrow Analysis] Actual vertical drop: %.2f blocks", actualVerticalDrop)

	if len(predictedTrajectory) > 0 {
		// Find where predicted trajectory landed (last point with significant Z position)
		lastPredicted := predictedTrajectory[len(predictedTrajectory)-1]
		
		// Trajectory is in relative coordinates: Z=horizontal distance, Y=vertical position
		// Need to apply yaw rotation and offset to world coordinates
		predictedHorizontalDist := lastPredicted.Pos.Z
		predictedVerticalDrop := -lastPredicted.Pos.Y // Negative because Y decreases as arrow falls
		
		// Rotate trajectory to world coordinates (for comparison)
		// Using yaw: X' = Z*sin(yaw), Z' = -Z*cos(yaw)
		predictedWorldX := arrowSpawnPos.X + lastPredicted.Pos.Z*math.Sin(yawRad)
		predictedWorldZ := arrowSpawnPos.Z - lastPredicted.Pos.Z*math.Cos(yawRad)
		predictedWorldY := arrowSpawnPos.Y + lastPredicted.Pos.Y
		
		log.Printf("[Arrow Analysis] ")
		log.Printf("[Arrow Analysis] Predicted landing (relative): Z=%.2f, Y=%.2f", predictedHorizontalDist, lastPredicted.Pos.Y)
		log.Printf("[Arrow Analysis] Predicted landing (world): (%.2f, %.2f, %.2f)", predictedWorldX, predictedWorldY, predictedWorldZ)
		log.Printf("[Arrow Analysis] Predicted horizontal distance: %.2f blocks", predictedHorizontalDist)
		log.Printf("[Arrow Analysis] Predicted vertical drop: %.2f blocks", predictedVerticalDrop)

		// Calculate differences
		horizontalDiff := actualHorizontalDist - predictedHorizontalDist
		verticalDiff := actualVerticalDrop - predictedVerticalDrop

		log.Printf("[Arrow Analysis] ")
		log.Printf("[Arrow Analysis] === Comparison ===")
		log.Printf("[Arrow Analysis] Horizontal difference: %.2f blocks (actual vs predicted)", horizontalDiff)
		log.Printf("[Arrow Analysis] Vertical difference: %.2f blocks (actual vs predicted)", verticalDiff)

		if math.Abs(horizontalDiff) > 0.01 {
			if horizontalDiff > 0 {
				percentDiff := (horizontalDiff / predictedHorizontalDist) * 100
				log.Printf("[Arrow Analysis] Arrow traveled %.1f%% FURTHER than predicted", percentDiff)
			} else {
				percentDiff := (math.Abs(horizontalDiff) / predictedHorizontalDist) * 100
				log.Printf("[Arrow Analysis] Arrow traveled %.1f%% SHORTER than predicted", percentDiff)
			}
		} else {
			log.Printf("[Arrow Analysis] Arrow traveled approximately the predicted distance (within tolerance)")
		}
	}

	// Sample some intermediate positions
	if len(actualPositions) > 2 {
		log.Printf("[Arrow Analysis] ")
		log.Printf("[Arrow Analysis] === Sample Positions ===")
		sampleIndices := []int{0, len(actualPositions) / 4, len(actualPositions) / 2, len(actualPositions) - 1}
		for _, idx := range sampleIndices {
			if idx < len(actualPositions) {
				pos := actualPositions[idx]
				log.Printf("[Arrow Analysis] Tick %d: pos=(%.2f, %.2f, %.2f), vel/tick=(%.4f, %.4f, %.4f)",
					pos.Tick, pos.X, pos.Y, pos.Z, pos.VelX, pos.VelY, pos.VelZ)
			}
		}
	}
}
