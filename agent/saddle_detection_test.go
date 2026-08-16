package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Saddle detection mirrors the vanilla client, which reads two different fields
// depending on version but shares one property: the default is "not saddled",
// and the server never transmits a negative.
//
//   - 1.21.5+: the saddle equipment slot (MobEntity.hasSaddleEquipped).
//     EntityTrackerEntry.sendPackets builds the equipment list from non-empty
//     slots only and skips the packet entirely when nothing is equipped.
//   - pre-1.21.5: the SADDLED bit of the horse flags metadata byte
//     (AbstractHorseEntity.isSaddled). DataTracker.getChangedEntries filters out
//     entries still at their default, so an all-zero flags byte is never sent.
//
// So absence of a positive signal is itself the answer, and neither path needs
// to wait for anything.

// fakeVersionOnlyHandler implements models.VersionHandler but only answers
// Version(). Saddle detection reads nothing else, so the remaining methods are
// deliberately inert rather than propped up with fixtures.
type fakeVersionOnlyHandler struct {
	version string
}

func (f *fakeVersionOnlyHandler) Version() string                            { return f.version }
func (f *fakeVersionOnlyHandler) ProtocolVersion() uint                      { return 0 }
func (f *fakeVersionOnlyHandler) PacketMgr() protocol_models.PacketMgr       { return nil }
func (f *fakeVersionOnlyHandler) Login() models.LoginHandler                 { return nil }
func (f *fakeVersionOnlyHandler) Configuration() models.ConfigurationHandler { return nil }
func (f *fakeVersionOnlyHandler) Play() models.PlayHandler                   { return nil }

// TestSaddleSlotSupported pins which field each supported version reads.
func TestSaddleSlotSupported(t *testing.T) {
	tests := []struct {
		version           string
		usesEquipmentSlot bool
	}{
		{version: "1.21.1", usesEquipmentSlot: false},
		{version: "1.21.2", usesEquipmentSlot: false},
		{version: "1.21.3", usesEquipmentSlot: false},
		{version: "1.21.4", usesEquipmentSlot: false},
		{version: "1.21.5", usesEquipmentSlot: true},
		{version: "1.21.6", usesEquipmentSlot: true},
		{version: "1.21.7", usesEquipmentSlot: true},
		{version: "1.21.8", usesEquipmentSlot: true},
		{version: "1.21.9", usesEquipmentSlot: true},
		{version: "1.21.10", usesEquipmentSlot: true},
		{version: "1.21.11", usesEquipmentSlot: true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			usesEquipmentSlot, versionKnown := saddleSlotSupported(tt.version)
			require.True(t, versionKnown, "every supported version must resolve to a path")
			assert.Equal(t, tt.usesEquipmentSlot, usesEquipmentSlot)
		})
	}
}

func TestSaddleSlotSupportedRejectsUnusableVersions(t *testing.T) {
	// An unparseable version means neither field can be trusted, so callers
	// must be told the state is unknown rather than a path being guessed.
	for _, version := range []string{"", "not-a-version"} {
		_, versionKnown := saddleSlotSupported(version)
		assert.False(t, versionKnown, "version %q should not resolve", version)
	}
}

func TestEquipmentHasSaddle(t *testing.T) {
	t.Run("no equipment at all is not saddled", func(t *testing.T) {
		// The case this whole change exists for: the server sends no equipment
		// packet for an unsaddled mount, so an empty map must read as unsaddled
		// rather than as "don't know yet".
		assert.False(t, equipmentHasSaddle(nil))
		assert.False(t, equipmentHasSaddle(map[models.EquipmentSlotType]models.InventorySlot{}))
	})

	t.Run("occupied saddle slot is saddled", func(t *testing.T) {
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotSaddle: {Present: true, Count: 1},
		}
		assert.True(t, equipmentHasSaddle(equipment))
	})

	t.Run("empty saddle slot is not saddled", func(t *testing.T) {
		// A slot is cleared by sending it with count 0 rather than omitting it.
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotSaddle: {Present: false, Count: 0},
		}
		assert.False(t, equipmentHasSaddle(equipment))
	})

	t.Run("body armour alone is not a saddle", func(t *testing.T) {
		// Body (6) and saddle (7) are adjacent slots; an armoured but
		// unsaddled horse must not read as saddled.
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotBody: {Present: true, Count: 1},
		}
		assert.False(t, equipmentHasSaddle(equipment))
	})

	t.Run("other equipment does not mask the saddle", func(t *testing.T) {
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotMainHand: {Present: true, Count: 1},
			models.EquipmentSlotBody:     {Present: true, Count: 1},
			models.EquipmentSlotSaddle:   {Present: true, Count: 1},
		}
		assert.True(t, equipmentHasSaddle(equipment))
	})
}

func TestHorseFlagConstants(t *testing.T) {
	// Values taken verbatim from AbstractHorseEntity's private constants.
	assert.EqualValues(t, 2, models.HorseFlagTamed)
	assert.EqualValues(t, 4, models.HorseFlagSaddled)
	assert.EqualValues(t, 8, models.HorseFlagBred)
	assert.EqualValues(t, 16, models.HorseFlagEatingGrass)
	assert.EqualValues(t, 32, models.HorseFlagAngry)
	assert.EqualValues(t, 64, models.HorseFlagEating)

	// Index derived by counting DataTracker registrations up the hierarchy; a
	// wrong value here would silently read an unrelated field (on a player,
	// key 17 is the score VarInt).
	assert.EqualValues(t, 17, models.EntityMetadataKeyHorseFlags)
}

func TestHorseFlagIsSet(t *testing.T) {
	t.Run("zero flags means nothing is set", func(t *testing.T) {
		assert.False(t, models.HorseFlagSaddled.IsSet(0))
		assert.False(t, models.HorseFlagTamed.IsSet(0))
	})

	t.Run("saddled bit alone", func(t *testing.T) {
		assert.True(t, models.HorseFlagSaddled.IsSet(0x04))
		assert.False(t, models.HorseFlagTamed.IsSet(0x04))
	})

	t.Run("tamed without saddled", func(t *testing.T) {
		// The unsaddled test horse's state: tamed so it can be mounted, but
		// carrying no saddle.
		assert.True(t, models.HorseFlagTamed.IsSet(0x02))
		assert.False(t, models.HorseFlagSaddled.IsSet(0x02))
	})

	t.Run("tamed and saddled together", func(t *testing.T) {
		const tamedAndSaddled = 0x02 | 0x04
		assert.True(t, models.HorseFlagTamed.IsSet(tamedAndSaddled))
		assert.True(t, models.HorseFlagSaddled.IsSet(tamedAndSaddled))
	})

	t.Run("unrelated bits do not imply saddled", func(t *testing.T) {
		// Bred, eating grass, angry, eating: none may read as a saddle.
		assert.False(t, models.HorseFlagSaddled.IsSet(0x08|0x10|0x20|0x40))
	})
}

// newSaddleTestAgent builds an agent whose version handler reports the given
// version, tracking a single entity to interrogate.
func newSaddleTestAgent(version string, entity *trackedEntity) *agent {
	return &agent{
		versionHandler: &fakeVersionOnlyHandler{version: version},
		entities:       map[int32]*trackedEntity{entity.EntityID: entity},
	}
}

func TestIsMountedEntitySaddledEquipmentPath(t *testing.T) {
	const entityID int32 = 42

	t.Run("occupied saddle slot reads saddled", func(t *testing.T) {
		testAgent := newSaddleTestAgent("1.21.11", &trackedEntity{
			EntityID: entityID,
			Equipment: map[models.EquipmentSlotType]models.InventorySlot{
				models.EquipmentSlotSaddle: {Present: true, Count: 1},
			},
		})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.True(t, saddled)
	})

	t.Run("no equipment reads unsaddled, not unknown", func(t *testing.T) {
		// Regression guard. Treating an empty map as "unknown" made the
		// unsaddled case unreachable, so the mount was driven anyway.
		testAgent := newSaddleTestAgent("1.21.11", &trackedEntity{EntityID: entityID})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known, "absence of equipment is a definite answer, per vanilla")
		assert.False(t, saddled)
	})

	t.Run("horse flags are ignored on the equipment path", func(t *testing.T) {
		// A 1.21.5+ server should not set these, but stale flags must not
		// override the equipment reading.
		testAgent := newSaddleTestAgent("1.21.11", &trackedEntity{
			EntityID:      entityID,
			HorseFlags:    uint8(models.HorseFlagSaddled),
			HasHorseFlags: true,
		})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.False(t, saddled, "1.21.5+ reads equipment, not the flags byte")
	})
}

func TestIsMountedEntitySaddledMetadataPath(t *testing.T) {
	const entityID int32 = 42

	t.Run("saddled bit reads saddled", func(t *testing.T) {
		testAgent := newSaddleTestAgent("1.21.1", &trackedEntity{
			EntityID:      entityID,
			HorseFlags:    uint8(models.HorseFlagTamed | models.HorseFlagSaddled),
			HasHorseFlags: true,
		})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.True(t, saddled)
	})

	t.Run("tamed but unsaddled reads unsaddled", func(t *testing.T) {
		testAgent := newSaddleTestAgent("1.21.1", &trackedEntity{
			EntityID:      entityID,
			HorseFlags:    uint8(models.HorseFlagTamed),
			HasHorseFlags: true,
		})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.False(t, saddled)
	})

	t.Run("no flags seen reads unsaddled, not unknown", func(t *testing.T) {
		// Tracked data at its default is never transmitted, so "no flags
		// update" and "no flags set" are the same thing.
		testAgent := newSaddleTestAgent("1.21.1", &trackedEntity{EntityID: entityID})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.False(t, saddled)
	})

	t.Run("equipment is ignored on the metadata path", func(t *testing.T) {
		// Pre-1.21.5 has no saddle equipment slot at all, so a stray entry
		// must not be mistaken for a saddle.
		testAgent := newSaddleTestAgent("1.21.1", &trackedEntity{
			EntityID: entityID,
			Equipment: map[models.EquipmentSlotType]models.InventorySlot{
				models.EquipmentSlotSaddle: {Present: true, Count: 1},
			},
		})

		saddled, known := testAgent.IsMountedEntitySaddled(entityID)
		require.True(t, known)
		assert.False(t, saddled, "pre-1.21.5 reads the flags byte, not equipment")
	})
}

func TestIsMountedEntitySaddledUnknownCases(t *testing.T) {
	const entityID int32 = 42

	t.Run("no version handler", func(t *testing.T) {
		// Hit in tests and during early startup. The two paths read different
		// fields, so without a version there is no defensible answer.
		testAgent := &agent{
			entities: map[int32]*trackedEntity{
				entityID: {
					EntityID: entityID,
					Equipment: map[models.EquipmentSlotType]models.InventorySlot{
						models.EquipmentSlotSaddle: {Present: true, Count: 1},
					},
				},
			},
		}

		_, known := testAgent.IsMountedEntitySaddled(entityID)
		assert.False(t, known)
	})

	t.Run("untracked entity", func(t *testing.T) {
		testAgent := newSaddleTestAgent("1.21.11", &trackedEntity{EntityID: entityID})

		_, known := testAgent.IsMountedEntitySaddled(999)
		assert.False(t, known, "an entity we cannot see is no basis to demote the rider")
	})
}
