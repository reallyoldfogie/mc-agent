package items

import (
	"bytes"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
)

// MockContainerHandler implements common.ContainerHandler for testing
type MockContainerHandler struct {
}

func (m *MockContainerHandler) SendContainerClick(conn common.PacketWriter, windowID int8, stateID, slot int32, button int8, mode int32, changedSlots map[int16]common.Slot, carriedItem common.Slot) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendContainerClose(conn common.PacketWriter, windowID int8) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendSetCreativeModeSlot(conn common.PacketWriter, slot int16, item common.Slot) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendPickItem(conn common.PacketWriter, slot int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendSetCarriedItem(conn common.PacketWriter, slot int16) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendUseItemOn(conn common.PacketWriter, hand models.Hand, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) ParseOpenScreen(p pk.Packet) (int8, int32, string, error) {
	return 0, 0, "", nil
}

func (m *MockContainerHandler) ParseContainerSetContent(p pk.Packet) (int8, int32, []common.Slot, common.Slot, error) {
	return 0, 0, nil, common.Slot{}, nil
}

func (m *MockContainerHandler) ParseContainerSetSlot(p pk.Packet) (int8, int32, int16, common.Slot, error) {
	return 0, 0, 0, common.Slot{}, nil
}

func (m *MockContainerHandler) ParseHeldItemSlot(p pk.Packet) (int16, error) {
	return 0, nil
}

func (m *MockContainerHandler) SendContainerButtonClick(conn common.PacketWriter, windowID int8, buttonID int8) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

// MockActionHandler implements common.ActionHandler for testing
type MockActionHandler struct {
}

func (m *MockActionHandler) SendUseItem(conn common.PacketWriter, hand models.Hand, sequence int32, yaw, pitch float32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockActionHandler) SendPlayerAction(conn common.PacketWriter, status int32, x, y, z int, face int32, sequence int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockActionHandler) SendSwing(conn common.PacketWriter, hand models.Hand) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

// MockEntityHandler implements common.EntityHandler for testing
type MockEntityHandler struct {
}

func (m *MockEntityHandler) ParseAddEntity(p pk.Packet) (int32, int32, int32, [16]byte, float64, float64, float64, int8, int8, float64, float64, float64, error) {
	return 0, 0, 0, [16]byte{}, 0, 0, 0, 0, 0, 0, 0, 0, nil
}

func (m *MockEntityHandler) ParseMoveEntityPos(p pk.Packet) (int32, int16, int16, int16, bool, error) {
	return 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseMoveEntityPosRot(p pk.Packet) (int32, int16, int16, int16, int8, int8, bool, error) {
	return 0, 0, 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseTeleportEntity(p pk.Packet) (int32, float64, float64, float64, int8, int8, bool, error) {
	return 0, 0, 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseRemoveEntities(p pk.Packet) ([]int32, error) {
	return nil, nil
}

func (m *MockEntityHandler) ParseEntityEvent(p pk.Packet) (int32, int8, error) {
	return 0, 0, nil
}

func (m *MockEntityHandler) ParseSetEntityMetadata(p pk.Packet) (int32, []common.MetadataEntry, error) {
	return 0, nil, nil
}

func (m *MockEntityHandler) ParseSyncEntityPosition(p pk.Packet) (int32, float64, float64, float64, float64, float64, float64, int8, int8, bool, error) {
	return 0, 0, 0, 0, 0, 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseEntityVelocityUpdate(p pk.Packet) (int32, float64, float64, float64, error) {
	return 0, 0, 0, 0, nil
}

func (m *MockEntityHandler) ParseEntityEquipment(p pk.Packet) (int32, []common.EquipmentEntry, error) {
	return 0, nil, nil
}

func (m *MockEntityHandler) ParseEntityHeadRotation(p pk.Packet) (int32, int8, error) {
	return 0, 0, nil
}

func (m *MockEntityHandler) ParseEntityLook(p pk.Packet) (int32, int8, int8, bool, error) {
	return 0, 0, 0, false, nil
}

func (m *MockEntityHandler) SendInteract(conn common.PacketWriter, entityID int32, hand models.Hand, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockEntityHandler) SendInteractAt(conn common.PacketWriter, entityID int32, targetX, targetY, targetZ float32, hand models.Hand, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockEntityHandler) SendAttack(conn common.PacketWriter, entityID int32, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}
