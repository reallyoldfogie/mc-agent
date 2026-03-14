package v1_21_10

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/login/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/login/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// loginHandler implements models.LoginHandler for 1.21.10.
type loginHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendLoginStart sends a login start packet with username and UUID.
// Uses the generated LoginStart packet struct from mc-protocol-go.
func (l *loginHandler) SendLoginStart(conn models.PacketWriter, username string, uuid [16]byte) error {
	pkt := sb.NewLoginStart()
	pkt.Username = pk.String(username)
	pkt.PlayerUUID = pk.UUID(uuid)

	log.Printf("[v1.21.10 Login] SendLoginStart: username=%s uuid=%x", username, uuid)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "LoginStart", Cause: err}
	}
	return nil
}

// SendEncryptionResponse sends an encryption response packet with shared secret and verify token.
// Uses the generated EncryptionBegin packet struct from mc-protocol-go.
func (l *loginHandler) SendEncryptionResponse(conn models.PacketWriter, sharedSecret, verifyToken []byte) error {
	pkt := sb.NewEncryptionBegin()
	pkt.SharedSecret = pk.ByteArray(sharedSecret)
	pkt.VerifyToken = pk.ByteArray(verifyToken)

	log.Printf("[v1.21.10 Login] SendEncryptionResponse: sharedSecretLen=%d verifyTokenLen=%d",
		len(sharedSecret), len(verifyToken))

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "EncryptionBegin", Cause: err}
	}
	return nil
}

// SendLoginAcknowledged sends a login acknowledged packet to transition to configuration state.
// Uses the generated LoginAcknowledged packet struct from mc-protocol-go.
func (l *loginHandler) SendLoginAcknowledged(conn models.PacketWriter) error {
	pkt := sb.NewLoginAcknowledged()

	log.Printf("[v1.21.10 Login] SendLoginAcknowledged")

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "LoginAcknowledged", Cause: err}
	}
	return nil
}

// ParseLoginSuccess parses a login success packet and extracts username and UUID.
// Uses the generated Success packet struct from mc-protocol-go.
func (l *loginHandler) ParseLoginSuccess(p pk.Packet) (username string, uuid [16]byte, err error) {
	pkt := cb.NewSuccess()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "Success", Cause: scanErr}
		return
	}

	username = string(pkt.Username)
	uuid = [16]byte(pkt.Uuid)

	log.Printf("[v1.21.10 Login] ParseLoginSuccess: username=%s uuid=%x", username, uuid)

	return
}

// ParseEncryptionRequest parses an encryption request packet and extracts server ID, public key, and verify token.
// Uses the generated EncryptionBegin packet struct from mc-protocol-go.
func (l *loginHandler) ParseEncryptionRequest(p pk.Packet) (serverID string, publicKey, verifyToken []byte, err error) {
	pkt := cb.NewEncryptionBegin()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "EncryptionBegin", Cause: scanErr}
		return
	}

	serverID = string(pkt.ServerId)
	publicKey = []byte(pkt.PublicKey)
	verifyToken = []byte(pkt.VerifyToken)

	log.Printf("[v1.21.10 Login] ParseEncryptionRequest: serverID=%s publicKeyLen=%d verifyTokenLen=%d",
		serverID, len(publicKey), len(verifyToken))

	return
}
