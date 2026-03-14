package agent

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot"
)

// versionHandlerAdapter adapts mc-agent's models.VersionHandler to mc-bot-go's bot.VersionHandler interface
type versionHandlerAdapter struct {
	handler models.VersionHandler
}

// NewVersionHandlerAdapter creates a new adapter that wraps an mc-agent version handler
// for use with mc-bot-go
func NewVersionHandlerAdapter(handler models.VersionHandler) bot.VersionHandler {
	return &versionHandlerAdapter{handler: handler}
}

func (a *versionHandlerAdapter) Version() string {
	return a.handler.Version()
}

func (a *versionHandlerAdapter) Login() bot.LoginHandler {
	log.Println("[Adapter] Login() called - creating loginHandlerAdapter")
	return &loginHandlerAdapter{handler: a.handler.Login()}
}

func (a *versionHandlerAdapter) Configuration() bot.ConfigurationHandler {
	return &configurationHandlerAdapter{handler: a.handler.Configuration()}
}

// loginHandlerAdapter adapts mc-agent's LoginHandler to mc-bot-go's bot.LoginHandler interface
type loginHandlerAdapter struct {
	handler models.LoginHandler
}

func (a *loginHandlerAdapter) SendLoginStart(conn bot.PacketWriter, username string, uuid [16]byte) error {
	return a.handler.SendLoginStart(conn, username, uuid)
}

func (a *loginHandlerAdapter) SendEncryptionResponse(conn bot.PacketWriter, sharedSecret, verifyToken []byte) error {
	return a.handler.SendEncryptionResponse(conn, sharedSecret, verifyToken)
}

func (a *loginHandlerAdapter) SendLoginAcknowledged(conn bot.PacketWriter) error {
	log.Println("[Adapter] SendLoginAcknowledged called")
	return a.handler.SendLoginAcknowledged(conn)
}

func (a *loginHandlerAdapter) ParseLoginSuccess(p pk.Packet) (username string, uuid [16]byte, err error) {
	log.Println("[Adapter] ParseLoginSuccess called")
	return a.handler.ParseLoginSuccess(p)
}

func (a *loginHandlerAdapter) ParseEncryptionRequest(p pk.Packet) (serverID string, publicKey, verifyToken []byte, err error) {
	return a.handler.ParseEncryptionRequest(p)
}

// configurationHandlerAdapter adapts mc-agent's ConfigurationHandler to mc-bot-go's bot.ConfigurationHandler interface
type configurationHandlerAdapter struct {
	handler models.ConfigurationHandler
}

func (a *configurationHandlerAdapter) SendFinishConfiguration(conn bot.PacketWriter) error {
	return a.handler.SendFinishConfiguration(conn)
}

func (a *configurationHandlerAdapter) SendKeepAlive(conn bot.PacketWriter, id int64) error {
	return a.handler.SendKeepAlive(conn, id)
}

func (a *configurationHandlerAdapter) SendPong(conn bot.PacketWriter, pingID int32) error {
	return a.handler.SendPong(conn, pingID)
}

func (a *configurationHandlerAdapter) ParseKeepAlive(p pk.Packet) (id int64, err error) {
	return a.handler.ParseKeepAlive(p)
}

func (a *configurationHandlerAdapter) ParsePing(p pk.Packet) (pingID int32, err error) {
	return a.handler.ParsePing(p)
}
