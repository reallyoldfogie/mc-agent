package v26_1

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"

	"github.com/reallyoldfogie/mc-protocol-go/data/26.1/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/26.1/play/clientbound"
)

// TestWorldHandler_ParseExplosion_WithPlayerKnockback covers PHASE_4_PLAN.md
// §2.2's generic explosion-knockback handling: 1.21.2+ delivers player
// knockback via an optional PlayerKnockback field (see 1.21.1's own
// separate test for the older always-present PlayerMotionX/Y/Z format).
func TestWorldHandler_ParseExplosion_WithPlayerKnockback(t *testing.T) {
	pkt := cb.NewExplosion()
	pkt.Center = basetypes.Vec3f64{X: pk.Double(10), Y: pk.Double(64), Z: pk.Double(-5)}
	pkt.Radius = pk.Float(4.0)
	pkt.PlayerKnockback.Has = pk.Boolean(true)
	pkt.PlayerKnockback.Val = &basetypes.Vec3f64{X: pk.Double(0.4), Y: pk.Double(0.6), Z: pk.Double(-1.0)}
	pkt.ExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.Sound = basetypes.ItemSoundHolder{IsRegistryID: true, RegistryID: 0}
	pkt.BlockParticles.Set([]cb.ExplosionParticleEntry{})

	handler := &worldHandler{}
	marshaled := pkt.Marshal()

	hasKnockback, x, y, z, err := handler.ParseExplosion(marshaled)
	if err != nil {
		t.Fatalf("ParseExplosion failed: %v", err)
	}
	if !hasKnockback {
		t.Fatal("expected hasKnockback=true when PlayerKnockback.Has is set")
	}
	if x != 0.4 || y != 0.6 || z != -1.0 {
		t.Errorf("expected knockback (0.4, 0.6, -1.0), got (%v, %v, %v)", x, y, z)
	}
}

// TestWorldHandler_ParseExplosion_NoPlayerKnockback verifies that an
// explosion the receiving player wasn't pushed by (PlayerKnockback.Has ==
// false) reports hasKnockback=false rather than a spurious (0,0,0) add.
func TestWorldHandler_ParseExplosion_NoPlayerKnockback(t *testing.T) {
	pkt := cb.NewExplosion()
	pkt.Center = basetypes.Vec3f64{X: pk.Double(0), Y: pk.Double(0), Z: pk.Double(0)}
	pkt.Radius = pk.Float(4.0)
	pkt.ExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.Sound = basetypes.ItemSoundHolder{IsRegistryID: true, RegistryID: 0}
	pkt.BlockParticles.Set([]cb.ExplosionParticleEntry{})
	// PlayerKnockback left at its zero value: Has=false.

	handler := &worldHandler{}
	marshaled := pkt.Marshal()

	hasKnockback, _, _, _, err := handler.ParseExplosion(marshaled)
	if err != nil {
		t.Fatalf("ParseExplosion failed: %v", err)
	}
	if hasKnockback {
		t.Fatal("expected hasKnockback=false when PlayerKnockback is absent")
	}
}

var _ models.WorldHandler = (*worldHandler)(nil)
