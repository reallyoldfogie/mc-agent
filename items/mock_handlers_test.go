package items

import (
	"bytes"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
)

// MockContainerHandler implements models.ContainerHandler for testing
type MockContainerHandler struct {
}

func (m *MockContainerHandler) SendContainerClick(conn models.PacketWriter, windowID int8, stateID, slot int32, button int8, mode int32, changedSlots map[int16]models.InventorySlot, carriedItem models.InventorySlot) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendContainerClose(conn models.PacketWriter, windowID int8) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendSetCreativeModeSlot(conn models.PacketWriter, slot int16, item models.InventorySlot) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendPickItem(conn models.PacketWriter, slot int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendSetCarriedItem(conn models.PacketWriter, slot int16) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) SendUseItemOn(conn models.PacketWriter, hand models.Hand, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockContainerHandler) ParseOpenScreen(p pk.Packet) (int8, int32, string, error) {
	return 0, 0, "", nil
}

func (m *MockContainerHandler) ParseContainerSetContent(p pk.Packet) (int8, int32, []models.InventorySlot, models.InventorySlot, error) {
	return 0, 0, nil, models.InventorySlot{}, nil
}

func (m *MockContainerHandler) ParseContainerSetSlot(p pk.Packet) (int8, int32, int16, models.InventorySlot, error) {
	return 0, 0, 0, models.InventorySlot{}, nil
}

func (m *MockContainerHandler) ParseHeldItemSlot(p pk.Packet) (int16, error) {
	return 0, nil
}

func (m *MockContainerHandler) SendContainerButtonClick(conn models.PacketWriter, windowID int8, buttonID int8) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

// MockActionHandler implements models.ActionHandler for testing
type MockActionHandler struct {
}

func (m *MockActionHandler) SendUseItem(conn models.PacketWriter, hand models.Hand, sequence int32, yaw, pitch float32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockActionHandler) SendPlayerAction(conn models.PacketWriter, status int32, x, y, z int, face int32, sequence int32) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockActionHandler) SendSwing(conn models.PacketWriter, hand models.Hand) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

// MockEntityHandler implements models.EntityHandler for testing
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

func (m *MockEntityHandler) ParseSetEntityMetadata(p pk.Packet) (int32, []models.MetadataEntry, error) {
	return 0, nil, nil
}

func (m *MockEntityHandler) ParseSyncEntityPosition(p pk.Packet) (int32, float64, float64, float64, float64, float64, float64, int8, int8, bool, error) {
	return 0, 0, 0, 0, 0, 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseEntityVelocityUpdate(p pk.Packet) (int32, float64, float64, float64, error) {
	return 0, 0, 0, 0, nil
}

func (m *MockEntityHandler) ParseEntityEquipment(p pk.Packet) (int32, []models.EquipmentEntry, error) {
	return 0, nil, nil
}

func (m *MockEntityHandler) ParseEntityHeadRotation(p pk.Packet) (int32, int8, error) {
	return 0, 0, nil
}

func (m *MockEntityHandler) ParseEntityLook(p pk.Packet) (int32, int8, int8, bool, error) {
	return 0, 0, 0, false, nil
}

func (m *MockEntityHandler) SendInteract(conn models.PacketWriter, entityID int32, hand models.Hand, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockEntityHandler) SendInteractAt(conn models.PacketWriter, entityID int32, targetX, targetY, targetZ float32, hand models.Hand, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockEntityHandler) SendAttack(conn models.PacketWriter, entityID int32, sneaking bool) error {
	var buf bytes.Buffer
	return conn.WritePacket(pk.Packet{Data: buf.Bytes()})
}

func (m *MockEntityHandler) ParseDamageEvent(p pk.Packet) (int32, int32, int32, int32, float64, float64, float64, bool, error) {
	return 0, 0, 0, 0, 0, 0, 0, false, nil
}

func (m *MockEntityHandler) ParseSetPassengers(p pk.Packet) (int32, []int32, error) {
	return 0, nil, nil
}

func (m *MockEntityHandler) ParseEntityUpdateAttributes(p pk.Packet) (int32, map[string]float64, error) {
	return 0, nil, nil
}
