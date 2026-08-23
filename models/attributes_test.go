package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests exercise AttributeValue.Compute() directly against
// EntityAttributeInstance.computeValue's exact three-stage formula
// (PHASE_4_PLAN.md §2.1) — the prerequisite fix for any effect implemented
// as an attribute modifier (Speed, Slowness).

func TestAttributeValue_Compute_NoModifiers(t *testing.T) {
	v := AttributeValue{Base: 0.1}
	assert.InDelta(t, 0.1, v.Compute(), 1e-9, "no modifiers should leave the base value unchanged")
}

func TestAttributeValue_Compute_AddValue(t *testing.T) {
	v := AttributeValue{
		Base: 0.1,
		Modifiers: []AttributeModifier{
			{Amount: 0.05, Operation: AttributeOperationAddValue},
			{Amount: 0.02, Operation: AttributeOperationAddValue},
		},
	}
	assert.InDelta(t, 0.17, v.Compute(), 1e-9, "ADD_VALUE modifiers should sum and add to the base")
}

func TestAttributeValue_Compute_AddMultipliedBase(t *testing.T) {
	v := AttributeValue{
		Base: 0.2,
		Modifiers: []AttributeModifier{
			{Amount: 0.5, Operation: AttributeOperationAddMultipliedBase},
			{Amount: 0.25, Operation: AttributeOperationAddMultipliedBase},
		},
	}
	// e = d + d*0.5 + d*0.25 = d * (1 + 0.75) = 0.2 * 1.75
	assert.InDelta(t, 0.35, v.Compute(), 1e-9, "ADD_MULTIPLIED_BASE modifiers should sum and apply against the stage-1 (base+ADD_VALUE) result")
}

func TestAttributeValue_Compute_AddMultipliedTotalAppliesIndividually(t *testing.T) {
	// Unlike the other two stages, ADD_MULTIPLIED_TOTAL modifiers are each
	// applied as their own separate multiply, not summed first — two 50%
	// modifiers compound to *2.25 (1.5*1.5), not *2.0 (1+0.5+0.5).
	v := AttributeValue{
		Base: 0.1,
		Modifiers: []AttributeModifier{
			{Amount: 0.5, Operation: AttributeOperationAddMultipliedTotal},
			{Amount: 0.5, Operation: AttributeOperationAddMultipliedTotal},
		},
	}
	assert.InDelta(t, 0.1*1.5*1.5, v.Compute(), 1e-9, "ADD_MULTIPLIED_TOTAL modifiers should compound individually, not sum first")
}

func TestAttributeValue_Compute_AllThreeStagesInOrder(t *testing.T) {
	v := AttributeValue{
		Base: 1.0,
		Modifiers: []AttributeModifier{
			{Amount: 1.0, Operation: AttributeOperationAddValue},           // stage 1: 1.0 + 1.0 = 2.0
			{Amount: 0.5, Operation: AttributeOperationAddMultipliedBase},  // stage 2: 2.0 + 2.0*0.5 = 3.0
			{Amount: 1.0, Operation: AttributeOperationAddMultipliedTotal}, // stage 3: 3.0 * (1+1.0) = 6.0
		},
	}
	assert.InDelta(t, 6.0, v.Compute(), 1e-9, "all three stages should apply in order: ADD_VALUE, then ADD_MULTIPLIED_BASE, then ADD_MULTIPLIED_TOTAL")
}

func TestAttributeValue_Compute_SpeedFormula(t *testing.T) {
	// Speed is registered as ADD_MULTIPLIED_TOTAL on generic.movement_speed
	// with amount 0.2*(amplifier+1) (StatusEffects.java). Speed II
	// (amplifier=1) on the vanilla base walk speed of 0.1: 0.1 * (1+0.4).
	baseWalkSpeed := 0.1
	speedIIAmount := 0.2 * float64(1+1)
	v := AttributeValue{
		Base:      baseWalkSpeed,
		Modifiers: []AttributeModifier{{Amount: speedIIAmount, Operation: AttributeOperationAddMultipliedTotal}},
	}
	assert.InDelta(t, baseWalkSpeed*(1+speedIIAmount), v.Compute(), 1e-9)
}

func TestAttributeValue_Compute_SlownessFormula(t *testing.T) {
	// Slowness is registered as ADD_MULTIPLIED_TOTAL with amount
	// -0.15*(amplifier+1) (StatusEffects.java) — negative, so it reduces
	// the total rather than increasing it.
	baseWalkSpeed := 0.1
	slownessIAmount := -0.15 * float64(0+1)
	v := AttributeValue{
		Base:      baseWalkSpeed,
		Modifiers: []AttributeModifier{{Amount: slownessIAmount, Operation: AttributeOperationAddMultipliedTotal}},
	}
	got := v.Compute()
	assert.InDelta(t, baseWalkSpeed*(1+slownessIAmount), got, 1e-9)
	assert.Less(t, got, baseWalkSpeed, "slowness should reduce speed below the base value")
}
