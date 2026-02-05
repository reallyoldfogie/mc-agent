package agent

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// onSoundPacket parses the sound packet and logs it using the sound manager when available.
func (a *agent) onSoundPacket(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	soundID, _, _, _, _, _, _, _, err := a.versionHandler.Play().ParseSound(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Sound IDs from the packet are 1-indexed; convert to 0-indexed for lookup
	id := protocol_models.SoundID(soundID - 1)
	name := ""
	subtitle := ""
	if a.soundMgr != nil {
		name = a.soundMgr.GetSoundNameByID(id)
		subtitle = a.soundMgr.GetSubtitleKeyByID(id)
	}
	if a.packetMgr != nil {
		log.Printf("[%s soundid]: %d => %s => %s", a.packetMgr.Name(), id, name, subtitle)
	} else {
		log.Printf("[soundid]: %d => %s => %s", id, name, subtitle)
	}
	return nil
}
