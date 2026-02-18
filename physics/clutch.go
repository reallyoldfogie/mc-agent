package physics

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// ClutchType identifies the type of clutch action to attempt.
type ClutchType string

const (
	ClutchWaterBucket ClutchType = "water_bucket"
	ClutchPowderSnow  ClutchType = "powder_snow"
)

// ClutchPlan describes a potential clutch action based on fall prediction.
type ClutchPlan struct {
	Type          ClutchType
	PlacePos      models.V3
	LandingY      float64
	FallDistance  float64
	TicksToImpact int
}

// PlanClutch evaluates whether a clutch action is advisable from the current state.
// Returns a plan when the predicted landing is unsafe and a clutch could prevent damage.
func PlanClutch(state models.PhysicsState, w World, shapeProvider BlockShapeProvider) (ClutchPlan, bool) {
	if state == nil || w == nil || shapeProvider == nil {
		return ClutchPlan{}, false
	}
	if state.OnGround() || state.Velocity().Y >= -0.05 {
		return ClutchPlan{}, false
	}

	fallDistance, landingY := PredictFallDistance(state.Position(), w, shapeProvider)
	if fallDistance <= SafeFallDistance {
		return ClutchPlan{}, false
	}

	landingBlock := GetLandingBlock(state.Position(), w, shapeProvider)
	if IsSafeLanding(fallDistance, landingBlock, shapeProvider) {
		return ClutchPlan{}, false
	}

	placePos := models.V3{
		X: math.Floor(state.Position().X),
		Y: landingY - 1,
		Z: math.Floor(state.Position().Z),
	}

	return ClutchPlan{
		Type:          ClutchWaterBucket,
		PlacePos:      placePos,
		LandingY:      landingY,
		FallDistance:  fallDistance,
		TicksToImpact: estimateFallTicks(fallDistance, state.Velocity().Y),
	}, true
}

func estimateFallTicks(fallDistance, initialVelY float64) int {
	if fallDistance <= 0 {
		return 0
	}
	posY := 0.0
	velY := initialVelY
	for tick := 0; tick < 200; tick++ {
		velY -= Gravity
		velY *= Drag
		posY += velY
		if -posY >= fallDistance {
			return tick + 1
		}
	}
	return 200
}
