package models

// PluginMessageCallback is called for every inbound CustomPayload (plugin
// message) packet, regardless of channel - mirrors EntityPositionCallback's
// registry pattern. channel is the raw channel identifier (e.g.
// "item_transfer:hold"); data is the packet's raw payload bytes, unparsed.
// Callers filter by channel themselves.
type PluginMessageCallback func(channel string, data []byte)

// PluginMessaging is the plugin-messaging (CustomPayload) send/receive
// surface every agent exposes. It is deliberately channel-agnostic and
// payload-agnostic - it carries raw bytes, not a specific mod's protocol -
// so mod-specific packages (e.g. courier, for item_transfer:*) can be built
// on top of it without any change to this interface.
type PluginMessaging interface {
	// RegisterPluginMessageCallback registers fn to be called for every
	// inbound CustomPayload packet.
	RegisterPluginMessageCallback(PluginMessageCallback)

	// SendPluginMessage sends a serverbound CustomPayload packet with the
	// given channel and raw payload bytes.
	SendPluginMessage(channel string, data []byte) error
}
