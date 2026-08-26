package v1_21_1

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"

	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/clientbound"
)

// TestWorldHandler_ParseExplosion_AlwaysReportsKnockback covers
// PHASE_4_PLAN.md §2.2's generic explosion-knockback handling for 1.21.1,
// which predates the optional PlayerKnockback field introduced in 1.21.2+
// (see v1_21_11/world_test.go for that version's coverage) — it always
// sends a PlayerMotionX/Y/Z triple, so hasKnockback is always true here,
// matching 1.21.1's own vanilla client (which adds the triple
// unconditionally, a harmless no-op when it's (0,0,0)).
func TestWorldHandler_ParseExplosion_AlwaysReportsKnockback(t *testing.T) {
	pkt := cb.NewExplosion()
	pkt.X = pk.Double(10)
	pkt.Y = pk.Double(64)
	pkt.Z = pk.Double(-5)
	pkt.Radius = pk.Float(4.0)
	pkt.AffectedBlockOffsets.Set([]cb.ExplosionAffectedBlockOffsetsArrayType{})
	pkt.PlayerMotionX = pk.Float(0.4)
	pkt.PlayerMotionY = pk.Float(0.6)
	pkt.PlayerMotionZ = pk.Float(-1.0)
	pkt.SmallExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.LargeExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.Sound = basetypes.ItemSoundHolder{IsRegistryID: true, RegistryID: 0}

	handler := &worldHandler{}
	marshaled := pkt.Marshal()

	hasKnockback, x, y, z, err := handler.ParseExplosion(marshaled)
	if err != nil {
		t.Fatalf("ParseExplosion failed: %v", err)
	}
	if !hasKnockback {
		t.Fatal("expected hasKnockback=true always on 1.21.1 (PlayerMotionX/Y/Z has no presence flag)")
	}
	// PlayerMotionX/Y/Z are pk.Float (float32) on the wire, so compare with
	// tolerance rather than exact equality to avoid float32->float64
	// widening noise (e.g. 0.4 becomes 0.4000000059604645).
	const eps = 1e-6
	if absDiff(x, 0.4) > eps || absDiff(y, 0.6) > eps || absDiff(z, -1.0) > eps {
		t.Errorf("expected knockback (0.4, 0.6, -1.0), got (%v, %v, %v)", x, y, z)
	}
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestWorldHandler_ParseExplosion_ZeroMotionStillReportsTrue confirms an
// unaffected player (PlayerMotionX/Y/Z all zero) still reports
// hasKnockback=true on 1.21.1 — adding (0,0,0) is a harmless no-op, matching
// what 1.21.1's own vanilla client does unconditionally.
func TestWorldHandler_ParseExplosion_ZeroMotionStillReportsTrue(t *testing.T) {
	pkt := cb.NewExplosion()
	pkt.Radius = pk.Float(4.0)
	pkt.AffectedBlockOffsets.Set([]cb.ExplosionAffectedBlockOffsetsArrayType{})
	pkt.SmallExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.LargeExplosionParticle.Type = basetypes.ParticleType{Value: "angry_villager"}
	pkt.Sound = basetypes.ItemSoundHolder{IsRegistryID: true, RegistryID: 0}
	// X/Y/Z/PlayerMotionX/Y/Z left at zero value.

	handler := &worldHandler{}
	marshaled := pkt.Marshal()

	hasKnockback, x, y, z, err := handler.ParseExplosion(marshaled)
	if err != nil {
		t.Fatalf("ParseExplosion failed: %v", err)
	}
	if !hasKnockback {
		t.Fatal("expected hasKnockback=true even with zero motion on 1.21.1")
	}
	if x != 0 || y != 0 || z != 0 {
		t.Errorf("expected zero knockback, got (%v, %v, %v)", x, y, z)
	}
}

var _ models.WorldHandler = (*worldHandler)(nil)
