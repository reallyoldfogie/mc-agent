package agent

import (
	"io"
	"log/slog"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// versionHandlerAdapter adapts mc-agent's models.VersionHandler to mc-bot-go's bot.VersionHandler interface
type versionHandlerAdapter struct {
	handler models.VersionHandler
	logger  *slog.Logger
}

// NewVersionHandlerAdapter creates a new adapter that wraps an mc-agent version handler
// for use with mc-bot-go
func NewVersionHandlerAdapter(handler models.VersionHandler, logger *slog.Logger) bot.VersionHandler {
	return &versionHandlerAdapter{handler: handler, logger: safeLogger(logger)}
}

func (a *versionHandlerAdapter) Version() string {
	return a.handler.Version()
}

func (a *versionHandlerAdapter) Login() bot.LoginHandler {
	a.logger.Info("[Adapter] Login() called - creating loginHandlerAdapter")
	return &loginHandlerAdapter{handler: a.handler.Login(), logger: a.logger}
}

func (a *versionHandlerAdapter) Configuration() bot.ConfigurationHandler {
	return &configurationHandlerAdapter{handler: a.handler.Configuration()}
}

// DecodeSlot and SendContainerClick below give versionHandlerAdapter the
// exact method set bot/screen.SlotCodec needs (no explicit "implements"
// declaration is possible in Go; screen.NewManager's own
// c.VersionHandler().(screen.SlotCodec) type assertion picks this up once
// both methods exist with these exact signatures). They delegate to the
// per-version models.ContainerHandler methods added alongside them.
func (a *versionHandlerAdapter) DecodeSlot(r io.Reader) (screen.Slot, int64, error) {
	return a.handler.Play().Containers().DecodeSlot(r)
}

func (a *versionHandlerAdapter) SendContainerClick(conn bot.PacketWriter, windowID int, stateID int32, slot int16, button byte, mode int32, changedSlots screen.ChangedSlots, cursor *screen.Slot) error {
	return a.handler.Play().Containers().SendContainerClickV2(conn, windowID, stateID, slot, button, mode, changedSlots, cursor)
}

// loginHandlerAdapter adapts mc-agent's LoginHandler to mc-bot-go's bot.LoginHandler interface
type loginHandlerAdapter struct {
	handler models.LoginHandler
	logger  *slog.Logger
}

func (a *loginHandlerAdapter) SendLoginStart(conn bot.PacketWriter, username string, uuid [16]byte) error {
	return a.handler.SendLoginStart(conn, username, uuid)
}

func (a *loginHandlerAdapter) SendEncryptionResponse(conn bot.PacketWriter, sharedSecret, verifyToken []byte) error {
	return a.handler.SendEncryptionResponse(conn, sharedSecret, verifyToken)
}

func (a *loginHandlerAdapter) SendLoginAcknowledged(conn bot.PacketWriter) error {
	a.logger.Info("[Adapter] SendLoginAcknowledged called")
	return a.handler.SendLoginAcknowledged(conn)
}

func (a *loginHandlerAdapter) ParseLoginSuccess(p pk.Packet) (username string, uuid [16]byte, err error) {
	a.logger.Info("[Adapter] ParseLoginSuccess called")
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
