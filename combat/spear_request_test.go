package combat

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestSpearAttackRequestValidatesRangeAndItem(t *testing.T) {
	req := SpearAttackRequest{
		TargetID:       4,
		ItemName:       "minecraft:iron_spear",
		Mode:           SpearJab,
		Distance:       4,
		MinReach:       2,
		MaxReach:       6,
		TargetPosition: models.V3{X: 4, Y: 1, Z: 0},
	}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}

	req.Distance = 1
	if err := req.Validate(); err == nil {
		t.Fatal("expected minimum reach validation error")
	}
	req.Distance = 4
	req.ItemName = "minecraft:diamond_sword"
	if err := req.Validate(); err == nil {
		t.Fatal("expected spear item validation error")
	}
}

func TestSpearChargeEligibilityUsesMotionThresholds(t *testing.T) {
	req := SpearAttackRequest{
		TargetID: 4, ItemName: "minecraft:iron_spear", Mode: SpearCharge,
		Distance: 4, MinReach: 2, MaxReach: 6,
		RelativeSpeed: 0.8, MinSpeed: 0.6, ViewAlignment: 0.9, MinAlignment: 0.75,
		HoldDuration: time.Second,
		Profile:      SpearChargeProfile{EngagedDuration: 500 * time.Millisecond, TiredDuration: time.Second},
	}
	if err := req.Validate(); err != nil || !req.ChargeEligible() {
		t.Fatalf("eligible charge rejected: err=%v eligible=%v", err, req.ChargeEligible())
	}
	req.RelativeSpeed = 0.2
	if req.ChargeEligible() {
		t.Fatal("charge should be rejected below relative-speed threshold")
	}
}
