# Adding a New Minecraft Version Handler

This guide explains how to add support for a new Minecraft protocol version to mc-agent.

## Prerequisites

Before adding a new version, ensure:

1. **mc-protocol-go** has generated packet definitions for the target version
2. You know the protocol version number (e.g., 1.21.8 = 772, 1.21.5 = 770)
3. You've reviewed any protocol changes from the previous version

**Version Format:** Both legacy (1.X.X) and modern (YY.X.Z) formats are supported. See `scripts/VERSION_FORMATS.md` for details.

**Note on Shared Protocol Versions:**
Some Minecraft versions share the same protocol version number. For example:
- 1.21.7 and 1.21.8 both use protocol 772
- When versions share a protocol, they typically have identical packet structures

If a new version shares its protocol with an existing version:
1. Copy the existing version's implementation (e.g., copy v1_21_8 to v1_21_7)
2. Update package names and version strings
3. Keep the same protocol version number
4. The implementation should be nearly identical - primarily changing version references

## Directory Structure

Each version handler lives in its own package under `versions/`:

```
versions/
├── common/           # Shared interfaces and utilities
│   ├── interfaces.go # Handler interfaces all versions implement
│   ├── errors.go     # Common error types
│   └── registry.go   # Version handler registry
├── init.go           # Central import point for all versions
├── v1_21_5/          # Version 1.21.5 implementation
│   ├── handler.go
│   ├── init.go
│   ├── login.go
│   ├── configuration.go
│   ├── movement.go
│   ├── entities.go
│   ├── containers.go
│   ├── chat.go
│   └── world.go
└── v1_21_8/          # Version 1.21.8 implementation
    └── ... (same structure)
```

## Quick Start: Automated Script

**Recommended:** Use the automation script to create the initial version handler:

```bash
# Auto-detect source version (copies from latest)
./scripts/add_version.sh 1.21.9 773

# Specify source version explicitly
./scripts/add_version.sh 1.21.9 773 1.21.8

# For versions sharing a protocol (e.g., 1.21.7 shares 772 with 1.21.8)
./scripts/add_version.sh 1.21.7 772 1.21.8

# 2026+ version format (26.1.0 -> v26_1_0)
./scripts/add_version.sh 26.1.0 800 1.21.8
```

The script will:
1. Create the version package directory
2. Copy all files from the source version
3. Update package names, version strings, and protocol numbers
4. Add import to `versions/init.go`
5. Format code with `go fmt`
6. Verify compilation

See `scripts/README.md` for detailed usage and troubleshooting.

**After running the script**, continue with the manual steps below to handle protocol changes.

## Manual Step-by-Step Guide

If you prefer not to use the automation script, or need to make manual adjustments:

### 1. Create the Version Package Directory

```bash
mkdir -p versions/v1_XX_X
```

Use underscores instead of dots in the directory name (e.g., `v1_21_8` for version 1.21.8).

### 2. Create handler.go

This is the main entry point that implements `common.VersionHandler`:

```go
// Package v1_XX_X provides version-specific packet handling for Minecraft 1.XX.X.
package v1_XX_X

import (
    "github.com/reallyoldfogie/mc-agent/versions/common"
    protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

const (
    versionString   = "1.XX.X"
    protocolVersion = XXX  // Get from wiki.vg or protocol documentation
)

// Handler implements common.VersionHandler for 1.XX.X.
type Handler struct {
    packetMgr     protocol_models.PacketMgr
    loginHandler  *loginHandler
    configHandler *configurationHandler
    playHandler   *playHandler
}

// NewHandler creates a new version handler for 1.XX.X.
func NewHandler(packetMgr protocol_models.PacketMgr) common.VersionHandler {
    h := &Handler{
        packetMgr: packetMgr,
    }

    h.loginHandler = &loginHandler{packetMgr: packetMgr}
    h.configHandler = &configurationHandler{packetMgr: packetMgr}
    h.playHandler = &playHandler{
        movementHandler:  &movementHandler{packetMgr: packetMgr},
        entityHandler:    &entityHandler{packetMgr: packetMgr},
        containerHandler: &containerHandler{packetMgr: packetMgr},
        chatHandler:      &chatHandler{packetMgr: packetMgr},
        worldHandler:     &worldHandler{packetMgr: packetMgr},
    }

    return h
}

func (h *Handler) Version() string                              { return versionString }
func (h *Handler) ProtocolVersion() uint                        { return protocolVersion }
func (h *Handler) PacketMgr() protocol_models.PacketMgr         { return h.packetMgr }
func (h *Handler) Login() common.LoginHandler                   { return h.loginHandler }
func (h *Handler) Configuration() common.ConfigurationHandler   { return h.configHandler }
func (h *Handler) Play() common.PlayHandler                     { return h.playHandler }

// playHandler provides access to play phase sub-handlers.
type playHandler struct {
    movementHandler  *movementHandler
    entityHandler    *entityHandler
    containerHandler *containerHandler
    chatHandler      *chatHandler
    worldHandler     *worldHandler
}

func (p *playHandler) Movement() common.MovementHandler     { return p.movementHandler }
func (p *playHandler) Entities() common.EntityHandler       { return p.entityHandler }
func (p *playHandler) Containers() common.ContainerHandler  { return p.containerHandler }
func (p *playHandler) Chat() common.ChatHandler             { return p.chatHandler }
func (p *playHandler) World() common.WorldHandler           { return p.worldHandler }
```

### 3. Create init.go

This registers the version handler on package import:

```go
package v1_XX_X

import (
    "log"

    "github.com/reallyoldfogie/mc-agent/versions/common"
    "github.com/reallyoldfogie/mc-protocol-go/loader"
)

func init() {
    log.Printf("[versions/v1_XX_X] Registering version handler for %s", versionString)

    common.RegisterVersionHandler(versionString, func() (common.VersionHandler, error) {
        log.Printf("[versions/v1_XX_X] Creating new handler")

        packetMgr, err := loader.LoadPacketMgr(versionString)
        if err != nil {
            return nil, err
        }

        handler := NewHandler(packetMgr)
        log.Printf("[versions/v1_XX_X] Created handler")
        return handler, nil
    })
}
```

### 4. Create Sub-Handler Files

Create each sub-handler file, implementing the corresponding interface from `common/interfaces.go`:

#### login.go
Implements `common.LoginHandler`:
- `SendLoginStart`
- `SendEncryptionResponse`
- `SendLoginAcknowledged`
- `ParseLoginSuccess`
- `ParseEncryptionRequest`

#### configuration.go
Implements `common.ConfigurationHandler`:
- `SendFinishConfiguration`
- `SendKeepAlive`
- `SendPong`
- `SendClientInformation`
- `ParseRegistryData`
- `ParseKeepAlive`
- `ParsePing`

#### movement.go
Implements `common.MovementHandler`:
- `SendPosition`
- `SendPositionAndRotation`
- `SendRotation`
- `SendPlayerCommand`
- `SendTeleportConfirm`
- `SendPlayerAbilities`
- `ParsePlayerPosition`

#### entities.go
Implements `common.EntityHandler`:
- `ParseAddEntity`
- `ParseMoveEntityPos`
- `ParseMoveEntityPosRot`
- `ParseTeleportEntity`
- `ParseRemoveEntities`
- `ParseEntityEvent`

#### containers.go
Implements `common.ContainerHandler`:
- `SendContainerClick`
- `SendContainerClose`
- `SendSetCreativeModeSlot`
- `SendPickItem`
- `SendSetCarriedItem`
- `ParseOpenScreen`
- `ParseContainerSetContent`
- `ParseContainerSetSlot`

#### chat.go
Implements `common.ChatHandler`:
- `SendChat`
- `SendCommand`
- `ParseSystemChat`
- `ParsePlayerChat`
- `ParseDisguisedChat`

#### world.go
Implements `common.WorldHandler`:
- `ParseBlockUpdate`
- `ParseSectionBlocksUpdate`
- `ParseChunkData`
- `ParseUnloadChunk`
- `SendChunkBatchReceived`

### 5. Update versions/init.go

Add the import for your new version package:

```go
package versions

import (
    // Import all version-specific packages to trigger their init() functions
    _ "github.com/reallyoldfogie/mc-agent/versions/v1_21_5"
    _ "github.com/reallyoldfogie/mc-agent/versions/v1_XX_X"  // Add new version
)
```

### 6. Build and Test

```bash
# Build to check for compilation errors
go build ./versions/v1_XX_X/...

# Build entire project
go build ./...

# Run version tests
go test ./versions/... -v
```

## Handling Protocol Changes

When implementing a new version, be aware of protocol changes. Common areas where changes occur:

### Packet Structure Changes

Import paths change between versions:
```go
// Old version
import cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"

// New version
import cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.8/play/clientbound"
```

### Type Changes

Protocol types may change between versions. For example, in 1.21.8:

**EntityAction ActionId changed from numeric to string-based:**
```go
// 1.21.5
pkt.ActionId = pk.VarInt(actionID)

// 1.21.8
pkt.ActionId = sb.EntityActionActionId{Value: "start_sprinting"}
```

**New packets may be introduced:**
```go
// 1.21.8 added PlayerInput packet for sneaking
pkt := sb.NewPlayerInput()
pkt.Inputs.SetShift(true)  // Sneaking
```

### Action ID Mappings

Action IDs may be renumbered or moved to different packets:

| Action | 1.21.5 EntityAction | 1.21.8 |
|--------|---------------------|--------|
| Start Sneaking | 0 | PlayerInput (Shift flag) |
| Stop Sneaking | 1 | PlayerInput (Shift flag) |
| Leave Bed | 2 | EntityAction "leave_bed" |
| Start Sprinting | 3 | EntityAction "start_sprinting" |
| Stop Sprinting | 4 | EntityAction "stop_sprinting" |

### Chunk Data Format Changes

Chunk parsing may differ between versions. Check `world/chunk.go` for version-aware parsing:
- Pre-1.21.5: Data array length is sent as VarInt
- 1.21.5+: Data array length is calculated

## Best Practices

1. **Copy from the nearest version** - Start by copying files from the closest existing version and modify as needed.

2. **Check mc-protocol-go** - Review the generated packet structures in mc-protocol-go for your target version before implementing.

3. **Add logging** - Include log statements for debugging:
   ```go
   log.Printf("[v1.XX.X Movement] SendPosition: (%.2f, %.2f, %.2f)", x, y, z)
   ```

4. **Handle backward compatibility** - If action IDs or packet semantics change, translate legacy values in your implementation to maintain interface compatibility.

5. **Document protocol changes** - Add comments explaining any significant protocol changes from the previous version.

6. **Test incrementally** - Build and test after implementing each sub-handler rather than waiting until the end.

7. **Use PacketMgr for dynamic packet IDs** - In tests, use PacketMgr to look up packet IDs dynamically instead of hardcoding them:
   ```go
   // Good: Version-agnostic test
   packetMgr := v1_XX_X.NewPackets()
   expectedID := packetMgr.GetClientboundPacketID("ClientboundSpawnEntity")
   if pkt.PacketID() != int32(expectedID) {
       t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
   }
   
   // Bad: Hardcoded packet ID
   if pkt.PacketID() != 1 {
       t.Errorf("Expected packet ID 1, got %d", pkt.PacketID())
   }
   ```
   This makes tests work across versions without modification when packet IDs change.

## Finding Protocol Information

- **wiki.vg** - Protocol documentation and version numbers: https://wiki.vg/Protocol_version_numbers
- **mc-protocol-go** - Check generated packet definitions in `data/{version}/` and verify the version exists
  - Protocol version numbers are embedded in the generated code
  - You can also check if a version is supported by looking at the `data/` directory
- **Minecraft source** - Decompiled client can reveal packet handling details
- **Online references** - Various protocol libraries maintain version mappings (e.g., PrismarineJS, ProtocolLib)

## Quick Reference: Version Differences

Key protocol changes to watch for when adding new versions:

| Feature | v1.21.4 | v1.21.5 | v1.21.6 | v1.21.7/v1.21.8 |
|---------|---------|---------|---------|------------------|
| EntityAction ActionId | Numeric (`pk.VarInt`) | Numeric (`pk.VarInt`) | String enum (`EntityActionActionId{Value: "..."}`) | String enum |
| Sneaking | EntityAction packet | EntityAction packet | PlayerInput packet (Shift flag) | PlayerInput packet |
| ChatMessage Checksum | ❌ Not present | ✅ Present | ✅ Present | ✅ Present |
| Bitflag methods | May have Get prefix | May have Get prefix | No Get prefix | No Get prefix |
| Slot structure | UnnamedType0003 switch | HashedSlot with Has/Val | Option[HashedSlot] | Option[HashedSlot] |
| Protocol Version | 769 | 770 | 771 | **772 (shared)** |

**Note**: v1.21.7 and v1.21.8 share protocol 772 - they are protocol-compatible and have identical packet structures.

**Action ID Mappings (v1.21.5 → v1.21.6+)**:
- `0` Start Sneaking → `PlayerInput` with `Shift=true`
- `1` Stop Sneaking → `PlayerInput` with `Shift=false`  
- `2` Leave Bed → `"leave_bed"`
- `3` Start Sprinting → `"start_sprinting"`
- `4` Stop Sprinting → `"stop_sprinting"`
- `5` Start Horse Jump → `"start_horse_jump"`
- `6` Stop Horse Jump → `"stop_horse_jump"`
- `7` Open Horse Inventory → `"open_vehicle_inventory"`
- `8` Start Elytra Flying → `"start_elytra_flying"`

## Testing New Versions

When creating tests for a new version, follow these guidelines:

### Copy Tests from Similar Versions

1. **For minor protocol changes**: Copy from the previous version (e.g., v1_21_4 → v1_21_5)
2. **For major protocol changes**: Copy from a version with similar changes (e.g., v1_21_8 → v1_21_6 for string-based EntityAction)

### Use Dynamic Packet ID Lookups

Always use PacketMgr instead of hardcoding packet IDs:

```go
// Initialize PacketMgr for the version being tested
packetMgr := v1_XX_X.NewPackets()

// For clientbound packets (server → client)
expectedID := packetMgr.GetClientboundPacketID("ClientboundSpawnEntity")

// For serverbound packets (client → server)  
expectedID := packetMgr.GetServerboundPacketID("ServerboundMovePlayerPos")

// Compare with actual packet ID
if pkt.PacketID() != int32(expectedID) {
    t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
}
```

**Important**: Packet names use the "Clientbound" or "Serverbound" prefix. Common packet name patterns:
- `ClientboundSpawnEntity` (not just "SpawnEntity")
- `ClientboundRelEntityMove` 
- `ClientboundEntityTeleport` (alternate: "TeleportEntity")
- `ServerboundMovePlayerPos`
- `ServerboundPlayerCommand`

### Handle Protocol-Specific Changes

#### Bitflags API Changes

Bitflag accessor methods may or may not have "Get" prefixes depending on the version:

```go
// Check mc-protocol-go generated code for the specific version
// v1.21.5 and earlier: May use GetOnGround()
if pkt.Flags.GetOnGround() { ... }

// v1.21.6+: Uses OnGround() without Get prefix
if pkt.Flags.OnGround() { ... }
```

#### Enum vs Numeric Action IDs

Entity actions changed from numeric to string-based enums:

```go
// v1.21.5 and earlier: Numeric ActionId
pkt.ActionId = pk.VarInt(3) // Start sprinting

// v1.21.6+: String-based ActionId
pkt.ActionId = sb.EntityActionActionId{Value: "start_sprinting"}
```

#### Sneaking Moved to PlayerInput

In v1.21.6+, sneaking uses a separate packet:

```go
// v1.21.5: Sneaking via EntityAction
pkt := sb.NewEntityAction()
pkt.ActionId = pk.VarInt(0) // Start sneaking

// v1.21.6+: Sneaking via PlayerInput
pkt := sb.NewPlayerInput() 
pkt.Inputs.SetShift(true) // Start sneaking
```

Tests for v1.21.6+ should verify both EntityAction (for sprinting) and PlayerInput (for sneaking).

#### Chat Packet Structure

The ChatMessage packet gained a Checksum field:

```go
// v1.21.4: No Checksum field
pkt.Acknowledged = models.FixedBuffer3{}
// That's it - no checksum

// v1.21.5+: Has Checksum field  
pkt.Acknowledged = models.FixedBuffer3{}
pkt.Checksum = pk.Byte(0)
```

#### Slot/Inventory Structures

Slot data structures changed significantly:

```go
// v1.21.4: Uses UnnamedType0003 switch field
entry.Item.ItemCount = pk.VarInt(count)
// UnnamedType0003 requires proper initialization

// v1.21.5: Uses HashedSlot
hashedSlot := &basetypes.HashedSlot{
    ItemId:    pk.VarInt(itemID),
    ItemCount: pk.VarInt(count),
}
entry.Item.Has = pk.Boolean(true)
entry.Item.Val = hashedSlot

// v1.21.6+: Back to ItemCount-based but with Option type
entry.Item = models.Option[basetypes.HashedSlot]{Has: true, Val: hashedSlot}
```

### Common Test Patterns

#### Testing Serverbound Packets

Use a mock PacketWriter:

```go
type mockPacketWriter struct {
    lastPacket pk.Packet
}

func (m *mockPacketWriter) WritePacket(p pk.Packet) error {
    m.lastPacket = p
    return nil
}

// In test:
writer := &mockPacketWriter{}
handler.SendPosition(writer, 100.0, 64.0, -200.0, true)

// Verify packet ID
packetMgr := v1_XX_X.NewPackets()
expectedID := packetMgr.GetServerboundPacketID("ServerboundMovePlayerPos")
assert.Equal(t, int32(expectedID), writer.lastPacket.ID)

// Parse and verify contents
pkt := sb.NewPosition()
pkt.Scan(writer.lastPacket)
assert.Equal(t, 100.0, float64(pkt.X))
```

#### Testing Clientbound Packets

Create packet directly and marshal:

```go
pkt := cb.NewSpawnEntity()
pkt.EntityId = pk.VarInt(12345)
pkt.X = pk.Double(100.5)
// ... set other fields

// Verify packet ID
packetMgr := v1_XX_X.NewPackets()
expectedID := packetMgr.GetClientboundPacketID("ClientboundSpawnEntity")
assert.Equal(t, int32(expectedID), pkt.PacketID())

// Test handler parsing
handler := &entityHandler{}
marshaled := pkt.Marshal()
entityID, _, _, x, y, z, _, _, err := handler.ParseAddEntity(marshaled)
assert.NoError(t, err)
assert.Equal(t, int32(12345), entityID)
```

### When to Skip Tests

Some tests may not be applicable for certain versions:

1. **Protocol changes make tests incompatible**: Remove or rewrite tests for changed features
2. **Packet IDs changed significantly**: Use PacketMgr instead of removing tests
3. **Feature removed from protocol**: Document why test is skipped

Example:
```go
func TestFeature(t *testing.T) {
    t.Skip("Feature removed in v1.21.6, moved to PlayerInput packet")
}
```

## Troubleshooting

### "packet ID mismatch"
The packet you're trying to parse has a different ID than expected. Check that you're using the correct packet type from mc-protocol-go for this version.

**Solution**: Use PacketMgr to look up the correct packet ID dynamically.

### "field type mismatch"
A packet field type changed between versions. Check the generated packet structure in mc-protocol-go.

**Common causes**:
- Numeric enum → String-based enum (e.g., EntityActionActionId)
- Added/removed fields (e.g., Checksum in ChatMessage)
- Bitflag API changes (Get prefix added/removed)

**Solution**: Inspect the generated packet file in `mc-protocol-go/data/{version}/` and update accordingly.

### "unknown packet name" in GetClientboundPacketID
The packet name doesn't match what's registered in the PacketMgr.

**Common causes**:
- Missing "Clientbound" or "Serverbound" prefix
- Using short name instead of full name (e.g., "SpawnEntity" vs "ClientboundSpawnEntity")
- Packet renamed between versions

**Solution**: Check `mc-protocol-go/data/{version}/packetid.go` for the exact packet name. Look in the switch statement for valid names.

### "has no field or method" errors
API changed between versions (e.g., GetOnGround() → OnGround()).

**Solution**: Check the generated code in mc-protocol-go to see the actual method names.

### Handler not found
Ensure:
1. The `init()` function calls `common.RegisterVersionHandler`
2. The version string matches exactly (e.g., "1.21.8" not "1.21")
3. The package is imported in `versions/init.go`

### Tests pass but functionality doesn't work

Tests verify packet structure, not protocol semantics. Common issues:

1. **Action IDs mapped incorrectly**: Verify the action ID mappings match the protocol version
2. **Missing packet state tracking**: Some versions require tracking state (e.g., isSneaking in v1.21.8)
3. **Packet order dependencies**: Some packets must be sent in specific order

**Solution**: Test against a real server to verify behavior matches expectations.
