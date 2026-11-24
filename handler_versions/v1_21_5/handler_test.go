package v1_21_5

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// TestPlayHandler_ParseDisconnect_Translate guards against the live bug
// found 2026-11-24: the reason field on the play-phase KickDisconnect
// packet is an NBT text component, not a plain pk.String, as of the
// protocol's 1.20.3+ migration. A handler that scans it as a plain
// string reads only the type/length-prefix bytes of the NBT structure's
// leading tag, producing garbage like "\b\x00\ttransla" (the start of a
// translate-key compound) instead of the actual reason - which is
// exactly what every bot's disconnect log showed in production before
// this fix.
func TestPlayHandler_ParseDisconnect_Translate(t *testing.T) {
	protocol_models.SetCurrentNBTVersion("1.21.5")

	pkt := cb.NewKickDisconnect()
	pkt.Reason = protocol_models.AnonymousNBT{
		TagType: protocol_models.TypeCompound,
		Value: &protocol_models.NBTCompound{
			Tags: []protocol_models.NBTTag{
				{Name: "translate", Value: &protocol_models.NBTString{Value: "disconnect.timeout"}},
			},
		},
	}

	handler := &playHandler{}
	reason, err := handler.ParseDisconnect(pkt.Marshal())
	require.NoError(t, err)
	assert.Equal(t, "disconnect.timeout", reason)
}

// TestPlayHandler_ParseDisconnect_Text covers the other text-component
// shape extractNBTString resolves - a literal "text" field, as opposed
// to a "translate" key with no literal text.
func TestPlayHandler_ParseDisconnect_Text(t *testing.T) {
	protocol_models.SetCurrentNBTVersion("1.21.5")

	pkt := cb.NewKickDisconnect()
	pkt.Reason = protocol_models.AnonymousNBT{
		TagType: protocol_models.TypeCompound,
		Value: &protocol_models.NBTCompound{
			Tags: []protocol_models.NBTTag{
				{Name: "text", Value: &protocol_models.NBTString{Value: "Kicked for flying"}},
			},
		},
	}

	handler := &playHandler{}
	reason, err := handler.ParseDisconnect(pkt.Marshal())
	require.NoError(t, err)
	assert.Equal(t, "Kicked for flying", reason)
}
