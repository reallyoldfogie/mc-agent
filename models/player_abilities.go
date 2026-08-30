package models

// PlayerAbilities is the local player's own ability state as reported by the
// server's clientbound Abilities packet (sent at login and whenever it
// changes, e.g. a game mode switch). See decompiled
// PlayerAbilitiesS2CPacket.java/PlayerAbilities.java: flags bit0=invulnerable,
// bit1=flying, bit2=allowFlying, bit3=creativeMode, plus two float speeds.
type PlayerAbilities struct {
	Invulnerable bool
	Flying       bool
	AllowFlying  bool
	CreativeMode bool
	// FlySpeed is blocks/tick of *vertical* ascend/descend impulse scale
	// (vanilla default 0.05) - horizontal flying speed uses the normal
	// movement-speed attribute, not this value. See
	// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4.
	FlySpeed float32
	// WalkSpeed mirrors vanilla's field (default 0.1) but is not currently
	// consumed anywhere in this codebase's movement code.
	WalkSpeed float32
}

// AbilitiesFlags bit masks, matching decompiled PlayerAbilitiesS2CPacket.java.
const (
	AbilitiesFlagInvulnerable byte = 1 << 0
	AbilitiesFlagFlying       byte = 1 << 1
	AbilitiesFlagAllowFlying  byte = 1 << 2
	AbilitiesFlagCreativeMode byte = 1 << 3
)

// DecodePlayerAbilitiesFlags unpacks the clientbound Abilities packet's flags
// byte into individual booleans.
func DecodePlayerAbilitiesFlags(flags byte) (invulnerable, flying, allowFlying, creativeMode bool) {
	return flags&AbilitiesFlagInvulnerable != 0,
		flags&AbilitiesFlagFlying != 0,
		flags&AbilitiesFlagAllowFlying != 0,
		flags&AbilitiesFlagCreativeMode != 0
}

// GameMode is the player's current game mode, matching vanilla's wire values
// (SpawnInfo.Gamemode in the Login packet, and the "value" field of a
// ClientboundGameEvent with reason CHANGE_GAME_MODE).
type GameMode int32

const (
	GameModeSurvival  GameMode = 0
	GameModeCreative  GameMode = 1
	GameModeAdventure GameMode = 2
	GameModeSpectator GameMode = 3
)

// ParseGameModeString maps the string vanilla's wire mapper decodes
// (SpawnInfo.Gamemode in the Login packet: "survival"/"creative"/
// "adventure"/"spectator") to a GameMode. Unrecognized values (including
// the mapper's own "unknown_<n>" fallback for undocumented byte values)
// return GameModeSurvival.
func ParseGameModeString(s string) GameMode {
	switch s {
	case "creative":
		return GameModeCreative
	case "adventure":
		return GameModeAdventure
	case "spectator":
		return GameModeSpectator
	default:
		return GameModeSurvival
	}
}

func (m GameMode) String() string {
	switch m {
	case GameModeSurvival:
		return "survival"
	case GameModeCreative:
		return "creative"
	case GameModeAdventure:
		return "adventure"
	case GameModeSpectator:
		return "spectator"
	default:
		return "unknown"
	}
}
