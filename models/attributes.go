package models

// AttributeOperation identifies how an attribute modifier combines with the
// base value, mirroring Java EntityAttributeModifier.Operation's wire
// ordinal. Unlike registry IDs elsewhere in this codebase, these three
// values are a fixed part of the packet format itself (not a dynamic,
// per-version registry), so hardcoding them here is correct rather than a
// shortcut that will break on a future version.
type AttributeOperation int8

const (
	// AttributeOperationAddValue adds the modifier's amount to the base
	// value (stage 1).
	AttributeOperationAddValue AttributeOperation = 0
	// AttributeOperationAddMultipliedBase multiplies the stage-1 result by
	// (1 + sum of every ADD_MULTIPLIED_BASE modifier's amount) (stage 2).
	AttributeOperationAddMultipliedBase AttributeOperation = 1
	// AttributeOperationAddMultipliedTotal applies each modifier as its own
	// separate multiply against the running total, not summed with others
	// of the same operation first (stage 3).
	AttributeOperationAddMultipliedTotal AttributeOperation = 2
)

// AttributeModifier is one live modifier on an entity attribute, as sent by
// ClientboundEntityUpdateAttributes.
type AttributeModifier struct {
	Amount    float64
	Operation AttributeOperation
}

// AttributeValue is an entity attribute's base value plus every currently
// active modifier, as last reported by ClientboundEntityUpdateAttributes.
// The packet always carries the complete current set of modifiers, not an
// incremental add/remove, so this is a last-known snapshot to be recomputed
// fresh on receipt — like ActiveEffect, not something built up piecemeal
// across multiple packets.
type AttributeValue struct {
	Base      float64
	Modifiers []AttributeModifier
}

// Compute applies every modifier to the base value in the three ordered
// stages vanilla uses (EntityAttributeInstance.computeValue,
// EntityAttributeModifier.Operation's doc comments): every ADD_VALUE
// modifier is summed and added to the base; every ADD_MULTIPLIED_BASE
// modifier is summed and applied as (1 + sum) against the stage-1 result;
// every ADD_MULTIPLIED_TOTAL modifier is then applied as its own separate
// multiply, unlike the other two stages which sum same-operation modifiers
// together first.
func (v AttributeValue) Compute() float64 {
	d := v.Base
	for _, m := range v.Modifiers {
		if m.Operation == AttributeOperationAddValue {
			d += m.Amount
		}
	}

	e := d
	for _, m := range v.Modifiers {
		if m.Operation == AttributeOperationAddMultipliedBase {
			e += d * m.Amount
		}
	}

	for _, m := range v.Modifiers {
		if m.Operation == AttributeOperationAddMultipliedTotal {
			e *= 1.0 + m.Amount
		}
	}

	return e
}
