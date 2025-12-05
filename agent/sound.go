package agent

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// onSoundPacket parses the sound packet and logs it using the sound manager when available.
func (a *Agent) onSoundPacket(p pk.Packet) error {
	var (
		SoundID       pk.VarInt
		SoundCategory pk.VarInt
		X, Y, Z       pk.Int
		Volume, Pitch pk.Float
		Seed          pk.Long
	)
	if err := p.Scan(&SoundID, &SoundCategory, &X, &Y, &Z, &Volume, &Pitch, &Seed); err != nil {
		return nil
	}

	id := protocol_models.SoundID(SoundID - 1)
	name := ""
	subtitle := ""
	if a.soundMgr != nil {
		name = a.soundMgr.GetSoundNameByID(protocol_models.SoundID(id))
		subtitle = a.soundMgr.GetSubtitleKeyByID(id)
	}
	if a.packetMgr != nil {
		log.Printf("[%s soundid]: %d => %s => %s", a.packetMgr.Name(), id, name, subtitle)
	} else {
		log.Printf("[soundid]: %d => %s => %s", id, name, subtitle)
	}
	return nil
}
