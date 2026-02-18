package agent

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// onSoundPacket parses the sound packet and logs it using the sound manager when available.
// Also identifies arrow-specific sounds for projectile tracking.
func (a *agent) onSoundPacket(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	soundID, category, x, y, z, volume, pitch, seed, err := a.versionHandler.Play().ParseSound(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Convert fixed-point positions to block coordinates (x8 scaling)
	fx := float64(x) / 8.0
	fy := float64(y) / 8.0
	fz := float64(z) / 8.0

	// Sound IDs from the packet are 1-indexed; convert to 0-indexed for lookup
	id := protocol_models.SoundID(soundID - 1)
	name := ""
	subtitle := ""
	if a.soundMgr != nil {
		name = a.soundMgr.GetSoundNameByID(id)
		subtitle = a.soundMgr.GetSubtitleKeyByID(id)
	}
	if a.packetMgr != nil {
		log.Printf("[%s soundid]: %d => %s => %s at (%.2f, %.2f, %.2f) vol=%.1f pitch=%.2f cat=%d seed=%d", a.packetMgr.Name(), id, name, subtitle, fx, fy, fz, volume, pitch, category, seed)
	} else {
		log.Printf("[soundid]: %d => %s => %s at (%.2f, %.2f, %.2f) vol=%.1f pitch=%.2f cat=%d seed=%d", id, name, subtitle, fx, fy, fz, volume, pitch, category, seed)
	}

	// Identify arrow-specific sounds
	if name != "" {
		switch {
		case name == "entity.arrow.shoot":
			log.Printf("[onSoundPacket] ARROW_SHOOT sound at (%.2f, %.2f, %.2f) pitch=%.2f", fx, fy, fz, pitch)
		case name == "entity.arrow.hit":
			log.Printf("[onSoundPacket] ARROW_HIT sound at (%.2f, %.2f, %.2f) pitch=%.2f", fx, fy, fz, pitch)
		case name == "entity.item.pickup":
			log.Printf("[onSoundPacket] ITEM_PICKUP sound (possible arrow) at (%.2f, %.2f, %.2f) pitch=%.2f", fx, fy, fz, pitch)
		}
	}

	return nil
}
