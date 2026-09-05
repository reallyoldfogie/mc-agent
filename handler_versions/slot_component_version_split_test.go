package versions

import (
	"bytes"
	"testing"

	basetypes1211 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/basetypes"
	basetypes1215 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/basetypes"
)

// TestSlotComponentTypeMapping_VersionSplit proves mc-agent's SlotCodec
// implementations use real per-version component-type numbering, not
// mc-bot-go's built-in 2-bucket pre/post-1.21.5 approximation (data/1.21.1
// and data/1.21.5 as representative packages for every version in each
// bucket -- see docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md's Background
// and Phase 3).
//
// Component type ID 10 means "can_place_on" in 1.21.1's own protocol
// numbering but "enchantments" in 1.21.5's (component IDs were renumbered
// when the HashedSlot rework landed -- mirrors mc-bot-go's own
// TestComponentTypeMapping_VersionSplit, which demonstrates the same fact
// for its 2-bucket fallback). Decoding the identical raw wire bytes (a
// single-byte VarInt encoding of 10) via each version's own generated
// basetypes.SlotComponentType.ReadFrom -- exactly what each version's
// DecodeSlot uses internally to resolve a component's name -- must resolve
// to a different name.
func TestSlotComponentTypeMapping_VersionSplit(t *testing.T) {
	raw := []byte{10} // VarInt(10), single byte

	var name1211 basetypes1211.SlotComponentType
	if _, err := name1211.ReadFrom(bytes.NewReader(raw)); err != nil {
		t.Fatalf("1.21.1 SlotComponentType.ReadFrom: %v", err)
	}
	if name1211.Value != "can_place_on" {
		t.Fatalf("1.21.1 component type 10 = %q, want %q", name1211.Value, "can_place_on")
	}

	var name1215 basetypes1215.SlotComponentType
	if _, err := name1215.ReadFrom(bytes.NewReader(raw)); err != nil {
		t.Fatalf("1.21.5 SlotComponentType.ReadFrom: %v", err)
	}
	if name1215.Value != "enchantments" {
		t.Fatalf("1.21.5 component type 10 = %q, want %q", name1215.Value, "enchantments")
	}

	if name1211.Value == name1215.Value {
		t.Fatalf("expected divergent names for the same numeric ID 10 across versions, got %q for both", name1211.Value)
	}
}
