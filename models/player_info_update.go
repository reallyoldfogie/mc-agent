package models

// PlayerInfoEntry represents a single player entry in a PlayerInfo packet
type PlayerInfoEntry struct {
	UUID       [16]byte
	Name       string
	Properties []ProfileProperty
	// Action flags - populated based on what actions apply to this entry
	AddPlayer   bool
	GameMode    *int32
	Listed      *bool
	Latency     *int32
	DisplayName *string
}

// PlayerInfoUpdate represents a complete PlayerInfo packet update
type PlayerInfoUpdate struct {
	Action  uint8 // Bitflags indicating what actions are present (0x01=add, 0x80=remove, etc)
	Entries []PlayerInfoEntry
}

// HasAddPlayer returns true if the add_player action is present
func (u *PlayerInfoUpdate) HasAddPlayer() bool {
	return u.Action&0x01 != 0
}

// HasInitializeChat returns true if the initialize_chat action is present
func (u *PlayerInfoUpdate) HasInitializeChat() bool {
	return u.Action&0x02 != 0
}

// HasUpdateGameMode returns true if the update_game_mode action is present
func (u *PlayerInfoUpdate) HasUpdateGameMode() bool {
	return u.Action&0x04 != 0
}

// HasUpdateListed returns true if the update_listed action is present
func (u *PlayerInfoUpdate) HasUpdateListed() bool {
	return u.Action&0x08 != 0
}

// HasUpdateLatency returns true if the update_latency action is present
func (u *PlayerInfoUpdate) HasUpdateLatency() bool {
	return u.Action&0x10 != 0
}

// HasUpdateDisplayName returns true if the update_display_name action is present
func (u *PlayerInfoUpdate) HasUpdateDisplayName() bool {
	return u.Action&0x20 != 0
}

// HasUpdateListOrder returns true if the update_list_order action is present
func (u *PlayerInfoUpdate) HasUpdateListOrder() bool {
	return u.Action&0x40 != 0
}

// HasRemovePlayer returns true if the remove_player action is present
func (u *PlayerInfoUpdate) HasRemovePlayer() bool {
	return u.Action&0x80 != 0
}
