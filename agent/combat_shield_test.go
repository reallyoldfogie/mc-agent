package agent

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
)

func TestSendCombatShieldActionDispatchesRaiseAndRelease(t *testing.T) {
	for _, versionTest := range models.StandardVersionTests {
		t.Run(versionTest.Name, func(t *testing.T) {
			handler, err := common.GetVersionHandler(versionTest.MCVersion, nil)
			if err != nil {
				t.Fatal(err)
			}
			var packets []packetCapture
			conn := &shieldPacketWriter{packets: &packets}

			if err := sendCombatShieldAction(handler.Play().Actions(), conn, true, 12, -4, 7); err != nil {
				t.Fatalf("raise shield: %v", err)
			}
			if err := sendCombatShieldAction(handler.Play().Actions(), conn, false, 0, 0, 8); err != nil {
				t.Fatalf("release shield: %v", err)
			}
			if len(packets) != 2 {
				t.Fatalf("packet count = %d, want 2", len(packets))
			}
		})
	}
}

func TestHasCombatShieldRequiresOffHandShield(t *testing.T) {
	makeAgent := func(name string, count int) *agent {
		return &agent{
			slots:   shieldSlotResolver{itemID: 9, count: count},
			itemMgr: fakeMultiItemMgr{nameByID: map[int32]string{9: name}},
		}
	}
	if !makeAgent("minecraft:shield", 1).hasCombatShield() {
		t.Fatal("off-hand shield should be detected")
	}
	if makeAgent("minecraft:iron_sword", 1).hasCombatShield() {
		t.Fatal("non-shield off-hand item should not be detected")
	}
	if makeAgent("minecraft:shield", 0).hasCombatShield() {
		t.Fatal("empty off-hand slot should not be detected")
	}

	mainHand := &agent{
		slots:   shieldMapResolver{items: map[int16]int32{38: 9}},
		itemMgr: fakeMultiItemMgr{nameByID: map[int32]string{9: "minecraft:shield"}},
	}
	hand, slot, ok := mainHand.combatShieldLocation()
	if !ok || hand != models.MainHand || slot != 2 {
		t.Fatalf("main-hand shield location = hand=%v slot=%d ok=%v, want main hand slot 2", hand, slot, ok)
	}
}

type shieldSlotResolver struct {
	itemID int32
	count  int
}

type shieldMapResolver struct {
	items map[int16]int32
}

func (r shieldMapResolver) ResolveSlot(_ int, index int16) (int32, int, bool) {
	itemID, ok := r.items[index]
	return itemID, 1, ok
}

func (r shieldSlotResolver) ResolveSlot(_ int, index int16) (int32, int, bool) {
	if index != 45 {
		return 0, 0, false
	}
	return r.itemID, r.count, true
}

// packetCapture intentionally does not inspect version-specific packet
// layouts. Successful marshaling and dispatch through every supported handler
// is the contract of this adapter-level test.
type packetCapture struct{}

type shieldPacketWriter struct {
	packets *[]packetCapture
}

func (w *shieldPacketWriter) WritePacket(_ pk.Packet) error {
	*w.packets = append(*w.packets, packetCapture{})
	return nil
}
