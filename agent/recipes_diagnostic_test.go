package agent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
)

// TestDiagnoseStonecutterStructure manually decodes the first few stonecutter entries
// to determine the actual packet structure
func TestDiagnoseStonecutterStructure(t *testing.T) {
	// Raw packet data from bot.log line 688
	rawHex := "07176d696e6563726166743a736d697468696e675f626173651eef068d0789079707ee06c206880796078b078c0794078607840783079307900782078a079507f20685078f079207f006f1068e078707910780078107186d696e6563726166743a63616d70666972655f696e707574099907aa099a08e30788099d0981029808e407176d696e6563726166743a6675726e6163655f696e7075748c01de02a401e906db02ec06ed06cb038a019107d103990146bd018c01a701b301e506ff02d403940a8d019d0986018907c407b9014f3ea30981028809ca03429207cd03990749d20640aa09c001e60688018b079f01a001c802c2019601be01d60350a601b101d803ab01d4068607cf03a301ea063b9a084a4cd20347439a01d503b60145b8018b01ac0190078807c903a409ba01d60689018b03bf01518701940141e307b001f002a101cc03af01eb069701238a074bf20201ea02d70352c5039c04bb019c0148e8069808094dd303d003bc01b701c601ad01e706e406ce039501ae014e8e019b018507ca028407b201a2019307449801bd09a501d4018707e4071b6d696e6563726166743a736d697468696e675f74656d706c61746513c00ab80abe0ac20abc0ac60ac10abf0ab90aba0abd0ab50ab60ac50ac70abb0ac30ac40ab70a1d6d696e6563726166743a626c6173745f6675726e6163655f696e7075742e8a07ea068807474c44e40684078b0785075186079007d6064dec0641e8064093074543e706eb06920789074fe606a30942d2064b494850d4064e87074aed06e90646e506a409529107166d696e6563726166743a736d6f6b65725f696e707574099907aa099a08e30788099d0981029808e4071b6d696e6563726166743a736d697468696e675f6164646974696f6e0bce06af05cf06d106cd06d706d506d806d0069a09d306fe01"

	packetData, err := hex.DecodeString(rawHex)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	r := bytes.NewReader(packetData)

	// Skip property sets
	var numPropertySets pk.VarInt
	if _, err := numPropertySets.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read property set count: %v", err)
	}
	t.Logf("Property sets: %d", numPropertySets)

	for i := 0; i < int(numPropertySets); i++ {
		var id pk.Identifier
		if _, err := id.ReadFrom(r); err != nil {
			t.Fatalf("Failed to read property set ID: %v", err)
		}
		var count pk.VarInt
		if _, err := count.ReadFrom(r); err != nil {
			t.Fatalf("Failed to read item count: %v", err)
		}
		for j := 0; j < int(count); j++ {
			var itemID pk.VarInt
			if _, err := itemID.ReadFrom(r); err != nil {
				t.Fatalf("Failed to read item ID: %v", err)
			}
		}
	}

	// Now at stonecutter recipes
	var numStonecutter pk.VarInt
	if _, err := numStonecutter.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read stonecutter count: %v", err)
	}
	t.Logf("Stonecutter entries: %d", numStonecutter)
	t.Logf("Bytes remaining: %d", r.Len())

	// Try to parse first entry both ways
	t.Log("\n=== Attempting OLD structure (IDSet + SlotDisplay) ===")
	savePos, _ := r.Seek(0, 1)
	if err := tryOldStructure(r, t); err != nil {
		t.Logf("OLD structure failed: %v", err)
	}
	r.Seek(savePos, 0)

	t.Log("\n=== Attempting NEW structure (SlotDisplay + count + SlotDisplays) ===")
	if err := tryNewStructure(r, t); err != nil {
		t.Logf("NEW structure failed: %v", err)
	}
}

func tryOldStructure(r *bytes.Reader, t *testing.T) error {
	// OLD: IDSet, then SlotDisplay
	var idSetType pk.VarInt
	if _, err := idSetType.ReadFrom(r); err != nil {
		return fmt.Errorf("reading idset type: %w", err)
	}
	t.Logf("  IDSet type: %d", idSetType)

	switch int(idSetType) {
	case 0: // empty
		t.Log("  IDSet: empty")
	case 1: // single
		var id pk.VarInt
		if _, err := id.ReadFrom(r); err != nil {
			return fmt.Errorf("reading single id: %w", err)
		}
		t.Logf("  IDSet: single item %d", id)
	case 2: // tag
		var tagID pk.Identifier
		if _, err := tagID.ReadFrom(r); err != nil {
			return fmt.Errorf("reading tag id: %w", err)
		}
		t.Logf("  IDSet: tag %s", tagID)
	case 3: // multiple
		var count pk.VarInt
		if _, err := count.ReadFrom(r); err != nil {
			return fmt.Errorf("reading multiple count: %w", err)
		}
		t.Logf("  IDSet: multiple count=%d", count)
		for i := 0; i < int(count); i++ {
			var id pk.VarInt
			if _, err := id.ReadFrom(r); err != nil {
				return fmt.Errorf("reading multiple id %d: %w", i, err)
			}
			t.Logf("    Item %d: %d", i, id)
		}
	}

	// Now SlotDisplay
	var slotType pk.VarInt
	if _, err := slotType.ReadFrom(r); err != nil {
		return fmt.Errorf("reading slot display type: %w", err)
	}
	t.Logf("  SlotDisplay type: %d", slotType)

	return nil
}

func tryNewStructure(r *bytes.Reader, t *testing.T) error {
	// NEW: Input SlotDisplay, count, result SlotDisplays
	var inputType pk.VarInt
	if _, err := inputType.ReadFrom(r); err != nil {
		return fmt.Errorf("reading input slot type: %w", err)
	}
	t.Logf("  Input SlotDisplay type: %d", inputType)

	// Handle input data based on type
	switch int(inputType) {
	case 0, 1: // empty, any_fuel - no data
	case 2: // item
		var itemID pk.VarInt
		if _, err := itemID.ReadFrom(r); err != nil {
			return fmt.Errorf("reading item id: %w", err)
		}
		t.Logf("  Input item: %d", itemID)
	default:
		return fmt.Errorf("unsupported input slot type: %d", inputType)
	}

	// Results count
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return fmt.Errorf("reading results count: %w", err)
	}
	t.Logf("  Results count: %d", count)

	// Parse results
	for i := 0; i < int(count); i++ {
		var resType pk.VarInt
		if _, err := resType.ReadFrom(r); err != nil {
			return fmt.Errorf("reading result %d type: %w", i, err)
		}
		t.Logf("    Result %d type: %d", i, resType)

		// Handle result data based on type
		switch int(resType) {
		case 0, 1: // empty, any_fuel - no data
		case 2: // item
			var itemID pk.VarInt
			if _, err := itemID.ReadFrom(r); err != nil {
				return fmt.Errorf("reading result %d item id: %w", i, err)
			}
			t.Logf("      Result %d item: %d", i, itemID)
		default:
			return fmt.Errorf("unsupported result slot type: %d", resType)
		}
	}

	return nil
}
