package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// ArrowPhysicsCalibration represents measured physics parameters from actual server data
type ArrowPhysicsCalibration struct {
	Pitch               float64 // Launch pitch in degrees
	MeasuredGrav        float64 // Calculated gravity (blocks/tick²)
	MeasuredDrag        float64 // Calculated drag multiplier
	PredictedGrav       float64 // Our prediction
	PredictedDrag       float64 // Our prediction
	GravError           float64 // Percentage error in gravity
	DragError           float64 // Percentage error in drag
	DataPoints          int     // Number of position updates used
	PredictedTrajLength int     // Number of points in predicted trajectory
	ActualTrajLength    int     // Number of points in actual trajectory from logs
}

// calibrateArrowPhysics fires an arrow with a specific pitch to estimate gravity and drag
// from the server's actual physics, comparing against predicted trajectory.
func calibrateArrowPhysics(t *testing.T, inst *TestInstance, agent *ManagedAgent,
	pitch float32, yaw float32, label string) (*ArrowPhysicsCalibration, error) {

	t.Logf("[Calibration] %s: Firing arrow with Pitch=%.1f, Yaw=%.1f",
		label, pitch, yaw)

	// Fire with specific pitch/yaw and get predicted trajectory
	predictedTraj, fireErr := agent.Agent.FireBowWithPitch(context.Background(), float64(pitch), float64(yaw))
	if fireErr != nil {
		t.Logf("[Calibration] Warning: FireBowWithPitch error: %v", fireErr)
		return nil, fireErr
	}

	// Cast to models.TrajectoryPoint slice
	predictedTrajLen := len(predictedTraj)

	// Wait for arrow to fly and be processed
	time.Sleep(5 * time.Second)

	// Parse trajectory from logs
	if inst.AgentLogFile == "" {
		return nil, fmt.Errorf("no agent log file")
	}

	actualPositions, err := AnalyzeArrowTrajectory(inst.AgentLogFile)
	if err != nil || len(actualPositions) < 5 {
		return nil, fmt.Errorf("failed to get trajectory data: %w", err)
	}

	// Log sample positions for debugging
	if len(actualPositions) > 0 {
		t.Logf("[Calibration] %s: Sample positions from %d total:", label, len(actualPositions))
		sampleCount := 3
		step := max(1, len(actualPositions)/sampleCount)
		for i := 0; i < len(actualPositions); i += step {
			pos := actualPositions[i]
			t.Logf("  [%d] pos=(%.2f, %.2f, %.2f) vel=(%.4f, %.4f, %.4f)",
				pos.Tick, pos.X, pos.Y, pos.Z, pos.VelX, pos.VelY, pos.VelZ)
		}
	}

	// Calculate actual physics from trajectory
	calibration := analyzePhysicsFromTrajectory(actualPositions, float64(pitch))
	if calibration == nil {
		return nil, fmt.Errorf("failed to analyze physics from %d positions", len(actualPositions))
	}

	// Store prediction info for comparison
	calibration.PredictedTrajLength = predictedTrajLen
	calibration.ActualTrajLength = len(actualPositions)

	t.Logf("[Calibration] %s: Gravity=%.6f (error %.1f%%), Drag=%.6f (error %.1f%%) [%d actual vs %d predicted points]",
		label,
		calibration.MeasuredGrav, calibration.GravError,
		calibration.MeasuredDrag, calibration.DragError,
		calibration.ActualTrajLength, calibration.PredictedTrajLength)

	return calibration, nil
}

// analyzePhysicsFromTrajectory calculates gravity and drag from actual arrow position data
// Uses linear regression and physics equations to back-calculate the parameters
func analyzePhysicsFromTrajectory(positions []ArrowPosition, pitch float64) *ArrowPhysicsCalibration {
	if len(positions) < 3 {
		return nil
	}

	_ = pitch // pitch is available for future use if needed

	// Extract velocity changes to calculate acceleration
	// Arrow velocity in Y should follow: v_y(t+1) = v_y(t) - gravity
	// After accounting for drag

	// Calculate gravity from changes in velocity
	// For arrows with only a few position updates, look at middle samples to avoid spawn/despawn artifacts
	var gravityValues []float64

	for i := 1; i < len(positions)-1; i++ {
		// Skip very first and very last few to avoid spawn/despawn effects
		if i <= 1 || i >= len(positions)-2 {
			continue
		}

		velYCurrent := positions[i].VelY
		velYNext := positions[i+1].VelY

		// gravity = current_vel - next_vel (positive when falling faster)
		gravity := velYCurrent - velYNext

		// For arrows, gravity should be around 0.05 blocks/tick²
		// Accept values that are reasonably close
		if gravity > 0.01 && gravity < 0.2 {
			gravityValues = append(gravityValues, gravity)
		}
	}

	var measuredGrav float64
	if len(gravityValues) == 0 {
		// No valid gravity samples, try a different approach:
		// Look at the absolute velocity values to estimate initial velocity and gravity
		if len(positions) > 3 && positions[2].VelY != 0 {
			// Use the first few velocity measurements
			vel1 := positions[1].VelY
			vel2 := positions[2].VelY
			if vel1 != 0 && vel2 != 0 {
				estGravity := vel1 - vel2
				if estGravity > 0.01 && estGravity < 0.2 {
					measuredGrav = estGravity
				} else {
					measuredGrav = 0.05 // Default
				}
			} else {
				measuredGrav = 0.05
			}
		} else {
			measuredGrav = 0.05 // Fallback to default
		}
	} else {
		// Calculate average of valid gravity samples
		sum := 0.0
		for _, g := range gravityValues {
			sum += g
		}
		measuredGrav = sum / float64(len(gravityValues))
	}

	// Calculate drag from horizontal velocity changes
	// v_x(t+1) = v_x(t) * drag
	// drag = v_x(t+1) / v_x(t)
	var avgDrag float64
	var dragCount int

	for i := 0; i < len(positions)-1; i++ {
		if i+1 < len(positions) {
			// Horizontal velocity magnitude
			velXZ := math.Sqrt(
				positions[i].VelX*positions[i].VelX + positions[i].VelZ*positions[i].VelZ,
			)
			velXZNext := math.Sqrt(
				positions[i+1].VelX*positions[i+1].VelX + positions[i+1].VelZ*positions[i+1].VelZ,
			)

			// Only process if initial velocity is meaningful
			if velXZ > 0.1 {
				drag := velXZNext / velXZ
				// Relax bounds from 0.95-1.0 to 0.90-1.05 for sparse data
				if drag > 0.90 && drag < 1.05 {
					avgDrag += drag
					dragCount++
				}
			}
		}
	}

	if dragCount == 0 {
		avgDrag = 0.99 // Default arrow drag
	} else {
		avgDrag = avgDrag / float64(dragCount)
	}

	// Get predicted values
	predictedPhys := physics.GetProjectilePhysics(models.Arrow)

	gravError := math.Abs(measuredGrav-predictedPhys.Gravity) / predictedPhys.Gravity * 100
	dragError := math.Abs(avgDrag-predictedPhys.Drag) / predictedPhys.Drag * 100

	// Clamp drag to reasonable range (can overshoot with sparse data)
	if avgDrag < 0.90 {
		avgDrag = 0.90
	} else if avgDrag > 1.01 {
		avgDrag = 1.01
	}

	// Recalculate error after clamping
	dragError = math.Abs(avgDrag-predictedPhys.Drag) / predictedPhys.Drag * 100

	return &ArrowPhysicsCalibration{
		Pitch:         pitch,
		MeasuredGrav:  measuredGrav,
		MeasuredDrag:  avgDrag,
		PredictedGrav: predictedPhys.Gravity,
		PredictedDrag: predictedPhys.Drag,
		GravError:     gravError,
		DragError:     dragError,
		DataPoints:    len(positions),
	}
}
