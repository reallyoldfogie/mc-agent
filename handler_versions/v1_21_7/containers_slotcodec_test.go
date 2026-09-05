package v1_21_7

import (
	"bytes"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.7/basetypes"
)

// TestDecodeSlot_RoundTrip builds a real 1.21.7 wire-format (full, non-hashed)
// Slot carrying one item component ("damage", a varint) and one component
// removal ("unbreakable"), decodes it via containerHandler.DecodeSlot, then
// confirms encoding that same screen.Slot as a HashedSlot (the format
// SendContainerClickV2 actually sends -- see its doc comment) hashes the
// component's real Data to the same value common.ComponentHash produces
// directly, per docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md Phase 3.
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

	hashed, err := hashedSlotFromScreenSlot(&decoded)
	if err != nil {
		t.Fatalf("hashedSlotFromScreenSlot: %v", err)
	}
	if !bool(hashed.Has) || hashed.Val == nil {
		t.Fatalf("hashedSlotFromScreenSlot: Has/Val = %v/%v, want true/non-nil", hashed.Has, hashed.Val)
	}
	if hashed.Val.ItemId != decoded.ID || hashed.Val.ItemCount != decoded.Count {
		t.Fatalf("hashed ItemId/ItemCount = %d/%d, want %d/%d", hashed.Val.ItemId, hashed.Val.ItemCount, decoded.ID, decoded.Count)
	}
	hashedComponents := hashed.Val.Components.Get()
	if len(hashedComponents) != 1 || hashedComponents[0].Type.Value != "damage" {
		t.Fatalf("hashed components = %#v, want one entry named %q", hashedComponents, "damage")
	}
	wantHash, err := common.ComponentHash(&damageValue)
	if err != nil {
		t.Fatalf("common.ComponentHash: %v", err)
	}
	if int32(hashedComponents[0].Hash) != wantHash {
		t.Fatalf("hashed component hash = %d, want %d", hashedComponents[0].Hash, wantHash)
	}
	hashedRemoves := hashed.Val.RemoveComponents.Get()
	if len(hashedRemoves) != 1 || hashedRemoves[0].Type.Value != "unbreakable" {
		t.Fatalf("hashed remove components = %#v, want one entry named %q", hashedRemoves, "unbreakable")
	}
}

// TestDecodeSlot_Empty confirms an empty (ItemCount=0) wire Slot decodes to
// an empty screen.Slot, and that the empty case encodes back to a HashedSlot
// Option with Has=false.
func TestDecodeSlot_Empty(t *testing.T) {
	wireSlot := basetypes.Slot{ItemCount: 0, UnnamedType0001: nil}
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

	hashed, err := hashedSlotFromScreenSlot(&decoded)
	if err != nil {
		t.Fatalf("hashedSlotFromScreenSlot: %v", err)
	}
	if bool(hashed.Has) {
		t.Fatalf("hashedSlotFromScreenSlot(empty) Has = true, want false")
	}
}
