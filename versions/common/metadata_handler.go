package common

import (
	"fmt"
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
)

// MetadataProcessResult contains structured results from metadata processing
type MetadataProcessResult struct {
	Health      float32     // Health value if present in metadata
	MaxHealth   float32     // Max health if present
	IsInGround  bool        // For projectiles: is stuck in ground
	Velocity    *[3]float64 // X, Y, Z velocity if extracted from metadata
	Position    *[3]float64 // X, Y, Z position if extracted from metadata
	Shake       int8        // Shake animation counter for projectiles (0-7)
	CriticalHit bool        // Critical hit flag for projectiles
	PierceLevel int8        // Piercing level for projectiles
	PotionColor int32       // Potion color for arrows (-1 = no potion)
	HasHealth   bool        // Whether health was present in metadata
	HasVelocity bool        // Whether velocity was extracted
	HasPosition bool        // Whether position was extracted
	HasShake    bool        // Whether shake was extracted
	HasCritical bool        // Whether critical hit flag was extracted
	HasPierce   bool        // Whether pierce level was extracted
	HasColor    bool        // Whether potion color was extracted
}

// MetadataHandler processes entity metadata updates
type MetadataHandler interface {
	// HandleMetadata processes a metadata entry for the given entity
	// Returns structured result with extracted values and error only for critical failures
	// Unknown keys are logged but don't error
	HandleMetadata(entityID int32, entry MetadataEntry) (MetadataProcessResult, error)
}

// BasicMetadataProcessor implements MetadataHandler with basic logging and value extraction
type BasicMetadataProcessor struct {
	// EntityRegistry provides entity type information for context-aware metadata interpretation
	entityRegistry *EntityRegistry
}

// NewBasicMetadataProcessor creates a new metadata processor with an entity registry
func NewBasicMetadataProcessor(registry *EntityRegistry) *BasicMetadataProcessor {
	return &BasicMetadataProcessor{
		entityRegistry: registry,
	}
}

// HandleMetadata processes a metadata entry and returns structured results
func (p *BasicMetadataProcessor) HandleMetadata(entityID int32, entry MetadataEntry) (MetadataProcessResult, error) {
	result := MetadataProcessResult{}

	entityType := "unknown"
	if p.entityRegistry != nil {
		if et := p.entityRegistry.GetEntityType(entityID); et != EntityTypeUnknown {
			entityType = string(et)
		}
	}

	// Log the metadata entry
	log.Printf("[Metadata] EntityID=%d Type=%s Key=%d HandlerID=%s Value=%v",
		entityID, entityType, entry.Key, entry.HandlerID.String(), entry.Value)

	// Extract common values and populate result
	switch entry.HandlerID {
	case HandlerByte:
		if val, ok := entry.Value.(*pk.Byte); ok {
			p.handleByteMetadata(entityID, entry.Key, uint8(*val))
		}

	case HandlerInteger:
		if val, ok := entry.Value.(*pk.VarInt); ok {
			p.handleIntegerMetadata(entityID, entry.Key, int32(*val))
		}

	case HandlerFloat:
		if val, ok := entry.Value.(*pk.Float); ok {
			p.handleFloatMetadata(entityID, entry.Key, float32(*val))
			// Extract health specifically
			if entry.Key == 9 {
				result.Health = float32(*val)
				result.HasHealth = true
				result.MaxHealth = 20.0 // Default max health
			}
		}

	case HandlerBoolean:
		if val, ok := entry.Value.(*pk.Boolean); ok {
			p.handleBooleanMetadata(entityID, entry.Key, bool(*val))
			// Extract isInGround for arrows
			if entry.Key == 10 {
				result.IsInGround = bool(*val)
			}
		}

	case HandlerVector3F:
		// Extract velocity if present (VECTOR_3F = handler type 33)
		if val, ok := entry.Value.(*[3]float32); ok {
			result.Velocity = &[3]float64{
				float64(val[0]),
				float64(val[1]),
				float64(val[2]),
			}
			result.HasVelocity = true
			log.Printf("[Metadata] EntityID=%d Vector3F velocity: (%.4f, %.4f, %.4f)",
				entityID, val[0], val[1], val[2])
		}

	case HandlerEntityPose:
		if val, ok := entry.Value.(*pk.VarInt); ok {
			p.handlePoseMetadata(entityID, int32(*val))
		}

	case HandlerLazyEntityReference:
		// Optional entity reference - may be nil or contain entity ID
		if val, ok := entry.Value.(*pk.VarInt); ok && val != nil {
			log.Printf("[Metadata] EntityID=%d references entity %d", entityID, *val)
		}

	case HandlerString:
		if val, ok := entry.Value.(*pk.String); ok {
			log.Printf("[Metadata] EntityID=%d string value: %s", entityID, *val)
		}

	default:
		// Log other types generically
		log.Printf("[Metadata] EntityID=%d Key=%d Handler=%s (type not specially handled)",
			entityID, entry.Key, entry.HandlerID.String())
	}

	return result, nil
}

// handleByteMetadata processes byte metadata (flags, counts, etc.)
func (p *BasicMetadataProcessor) handleByteMetadata(entityID int32, key int32, value uint8) {
	switch key {
	case 0:
		// Entity flags
		log.Printf("[Metadata] EntityID=%d Flags: onFire=%v sneaking=%v sprinting=%v swimming=%v invisible=%v glowing=%v elytra=%v",
			entityID,
			(value&0x01) != 0,
			(value&0x02) != 0,
			(value&0x04) != 0,
			(value&0x08) != 0,
			(value&0x10) != 0,
			(value&0x20) != 0,
			(value&0x40) != 0,
		)

	case 18:
		// Player skin parts visibility
		log.Printf("[Metadata] EntityID=%d Skin parts: cape=%v jacket=%v leftSleeve=%v rightSleeve=%v leftPant=%v rightPant=%v hat=%v",
			entityID,
			(value&0x01) != 0,
			(value&0x02) != 0,
			(value&0x04) != 0,
			(value&0x08) != 0,
			(value&0x10) != 0,
			(value&0x20) != 0,
			(value&0x40) != 0,
		)

	default:
		log.Printf("[Metadata] EntityID=%d Key=%d Byte value: %d (0x%02x)", entityID, key, value, value)
	}
}

// handleIntegerMetadata processes integer metadata
func (p *BasicMetadataProcessor) handleIntegerMetadata(entityID int32, key int32, value int32) {
	switch key {
	case 6:
		log.Printf("[Metadata] EntityID=%d Freezing ticks: %d", entityID, value)
	case 8:
		log.Printf("[Metadata] EntityID=%d Air supply: %d ticks", entityID, value)
	case 11:
		log.Printf("[Metadata] EntityID=%d Arrow/Potion count: %d", entityID, value)
	case 12:
		log.Printf("[Metadata] EntityID=%d Bee stinger count: %d", entityID, value)
	case 17:
		log.Printf("[Metadata] EntityID=%d Player score: %d", entityID, value)
	default:
		log.Printf("[Metadata] EntityID=%d Key=%d Integer value: %d", entityID, key, value)
	}
}

// handleFloatMetadata processes floating point metadata
func (p *BasicMetadataProcessor) handleFloatMetadata(entityID int32, key int32, value float32) {
	switch key {
	case 9:
		log.Printf("[Metadata] EntityID=%d Health: %.1f / 20.0 half-hearts", entityID, value)
	case 16:
		log.Printf("[Metadata] EntityID=%d Absorption hearts: %.1f", entityID, value)
	default:
		log.Printf("[Metadata] EntityID=%d Key=%d Float value: %.6f", entityID, key, value)
	}
}

// handleBooleanMetadata processes boolean metadata
func (p *BasicMetadataProcessor) handleBooleanMetadata(entityID int32, key int32, value bool) {
	switch key {
	case 2:
		log.Printf("[Metadata] EntityID=%d Custom name visible: %v", entityID, value)
	case 3:
		log.Printf("[Metadata] EntityID=%d Silent: %v", entityID, value)
	case 4:
		log.Printf("[Metadata] EntityID=%d No gravity: %v", entityID, value)
	case 10:
		log.Printf("[Metadata] EntityID=%d Potion effect ambient: %v", entityID, value)
	default:
		log.Printf("[Metadata] EntityID=%d Key=%d Boolean value: %v", entityID, key, value)
	}
}

// handlePoseMetadata processes pose metadata
func (p *BasicMetadataProcessor) handlePoseMetadata(entityID int32, pose int32) {
	poses := []string{"STANDING", "FALL_FLYING", "SLEEPING", "SWIMMING", "SPIN_ATTACK", "SNEAKING", "DYING"}
	poseName := "UNKNOWN"
	if pose >= 0 && pose < int32(len(poses)) {
		poseName = poses[pose]
	}
	log.Printf("[Metadata] EntityID=%d Pose: %s (%d)", entityID, poseName, pose)
}

// ConvertMetadataEntry takes a raw protocol metadata entry and converts it to our MetadataEntry type
// This bridges between the protocol-specific types and our generic metadata system
func ConvertMetadataEntry(key int32, handlerID int32, value any) (MetadataEntry, error) {
	entry := MetadataEntry{
		Key:       key,
		HandlerID: MetadataHandlerType(handlerID),
		Value:     value,
	}

	// Validate that we have a supported handler type
	if handlerID < 0 || handlerID > 36 {
		return entry, fmt.Errorf("unsupported handler ID %d for metadata key %d", handlerID, key)
	}

	return entry, nil
}
