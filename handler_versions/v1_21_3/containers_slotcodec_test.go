package v1_21_3

import (
	"bytes"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.3/basetypes"
)

// TestDecodeSlot_RoundTrip builds a real 1.21.3 wire-format Slot carrying one
// item component ("damage", a varint) and one component removal
// ("unbreakable"), decodes it via containerHandler.DecodeSlot, re-encodes the
// resulting screen.Slot via slotFromScreenSlot, and confirms decoding that
// re-encoded form produces the same screen.Slot -- proving components (not
// just plain item id/count) survive the round trip, per
// docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md Phase 3.
func TestDecodeSlot_RoundTrip(t *testing.T) {
	damageValue := pk.VarInt(5)
	components := []basetypes.SlotComponent{
		{Type: basetypes.SlotComponentType{Value: "damage"}, Data: &damageValue},
	}
	removeComponents := []basetypes.SlotUnnamedType0001DefaultRemoveComponentsArrayType{
		{Type: basetypes.SlotComponentType{Value: "unbreakable"}},
	}

	def := &basetypes.SlotUnnamedType0001Default{
		ItemId:                42,
		AddedComponentCount:   pk.VarInt(len(components)),
		RemovedComponentCount: pk.VarInt(len(removeComponents)),
	}
	def.Components.Set(&components)
	def.RemoveComponents.Set(&removeComponents)

	wireSlot := basetypes.Slot{ItemCount: 3, UnnamedType0001: def}

	var buf bytes.Buffer
	if _, err := wireSlot.WriteTo(&buf); err != nil {
		t.Fatalf("wireSlot.WriteTo: %v", err)
	}

	h := &containerHandler{}
	decoded, _, err := h.DecodeSlot(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeSlot: %v", err)
	}

	if decoded.ID != 42 || decoded.Count != 3 {
		t.Fatalf("decoded ID/Count = %d/%d, want 42/3", decoded.ID, decoded.Count)
	}
	if len(decoded.Components) != 1 {
		t.Fatalf("decoded %d components, want 1", len(decoded.Components))
	}
	damageTypeID, ok := componentTypeNameToID["damage"]
	if !ok {
		t.Fatalf("componentTypeNameToID missing %q", "damage")
	}
	if decoded.Components[0].Type != pk.VarInt(damageTypeID) {
		t.Fatalf("decoded component type = %d, want %d", decoded.Components[0].Type, damageTypeID)
	}
	gotDamage, ok := decoded.Components[0].Data.(*pk.VarInt)
	if !ok || *gotDamage != damageValue {
		t.Fatalf("decoded component data = %#v, want *pk.VarInt(%d)", decoded.Components[0].Data, damageValue)
	}
	if len(decoded.RemoveComponents) != 1 {
		t.Fatalf("decoded %d remove components, want 1", len(decoded.RemoveComponents))
	}
	unbreakableTypeID, ok := componentTypeNameToID["unbreakable"]
	if !ok {
		t.Fatalf("componentTypeNameToID missing %q", "unbreakable")
	}
	if decoded.RemoveComponents[0] != pk.VarInt(unbreakableTypeID) {
		t.Fatalf("decoded remove component type = %d, want %d", decoded.RemoveComponents[0], unbreakableTypeID)
	}

	// Re-encode and decode again; the second decode must match the first.
	reEncoded, err := slotFromScreenSlot(&decoded)
	if err != nil {
		t.Fatalf("slotFromScreenSlot: %v", err)
	}
	var buf2 bytes.Buffer
	if _, err := reEncoded.WriteTo(&buf2); err != nil {
		t.Fatalf("reEncoded.WriteTo: %v", err)
	}
	redecoded, _, err := h.DecodeSlot(bytes.NewReader(buf2.Bytes()))
	if err != nil {
		t.Fatalf("second DecodeSlot: %v", err)
	}
	if redecoded.ID != decoded.ID || redecoded.Count != decoded.Count {
		t.Fatalf("round-tripped ID/Count = %d/%d, want %d/%d", redecoded.ID, redecoded.Count, decoded.ID, decoded.Count)
	}
	if len(redecoded.Components) != 1 || redecoded.Components[0].Type != decoded.Components[0].Type {
		t.Fatalf("round-tripped components = %#v, want %#v", redecoded.Components, decoded.Components)
	}
	gotDamage2, ok := redecoded.Components[0].Data.(*pk.VarInt)
	if !ok || *gotDamage2 != damageValue {
		t.Fatalf("round-tripped component data = %#v, want *pk.VarInt(%d)", redecoded.Components[0].Data, damageValue)
	}
	if len(redecoded.RemoveComponents) != 1 || redecoded.RemoveComponents[0] != decoded.RemoveComponents[0] {
		t.Fatalf("round-tripped remove components = %#v, want %#v", redecoded.RemoveComponents, decoded.RemoveComponents)
	}
}

// TestDecodeSlot_Empty confirms an empty (ItemCount=0) wire Slot decodes to
// an empty screen.Slot, matching the pre-1.21.5 "no separate presence flag"
// wire convention (see slotFromScreenSlot's doc comment).
func TestDecodeSlot_Empty(t *testing.T) {
	wireSlot := basetypes.Slot{ItemCount: 0, UnnamedType0001: nil}
	// Empty slots use the Void switch case, which requires ItemCount==0 to
	// be read back correctly by ReadFrom; WriteTo only needs ItemCount==0.
	var buf bytes.Buffer
	if _, err := wireSlot.WriteTo(&buf); err != nil {
		t.Fatalf("wireSlot.WriteTo: %v", err)
	}

	h := &containerHandler{}
	decoded, _, err := h.DecodeSlot(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeSlot: %v", err)
	}
	if decoded.ID != 0 || decoded.Count != 0 || len(decoded.Components) != 0 || len(decoded.RemoveComponents) != 0 {
		t.Fatalf("decoded empty slot = %#v, want zero value", decoded)
	}
}
