package vehicles

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	semver "github.com/aquasecurity/go-version/pkg/version"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// VehicleTestHelper encapsulates common vehicle test utilities
type VehicleTestHelper struct {
	Framework    *testingpkg.Framework
	Instance     *testingpkg.TestInstance
	ManagedAgent *testingpkg.ManagedAgent
	AgentName    string
	t            *testing.T
}

// NewVehicleTestHelper creates a new vehicle test helper with a running server and agent
func NewVehicleTestHelper(t *testing.T, mcVersion, agentName string) (*VehicleTestHelper, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	if len(agentName) > 16 {
		agentName = agentName[:16] // Minecraft usernames have a max length of 16 characters
	}

	framework, err := testingpkg.NewFramework()
	if err != nil {
		cancel()
		t.Fatalf("create framework: %v", err)
	}

	serverCfg := testingpkg.FlatWorldServerConfig()
	serverCfg.Memory = "512M"
	serverCfg.Version = mcVersion
	serverCfg.GameMode = testingpkg.GameModeSurvival
	serverCfg.ExtraEnv = map[string]string{
		"FORCE_GAMEMODE": "true",
	}
	// serverCfg.MountDirs = []string{"loggingConfig"}
	// serverCfg.ExtraEnv = map[string]string{"JVM_OPTS": "-Dfabric.development=true -Dlog4j2.configurationFile=/data/loggingConfig/log4j2.xml"}

	serverCfg.PullImage = false
	testingpkg.RequireIntegrationEnv(t, serverCfg)

	inst, err := framework.StartServer(ctx, serverCfg)
	if err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}

	// Agent logging is setup automatically by framework

	// Construct the helper early so t.Cleanup can reference it even if
	// a t.Fatal call below prevents NewVehicleTestHelper from returning.
	helper := &VehicleTestHelper{
		Framework: framework,
		Instance:  inst,
		AgentName: agentName,
		t:         t,
	}

	// Register cleanup via t.Cleanup so the server container is always
	// stopped, even when t.Fatal is called before this function returns
	// and the caller's defer cleanup() is never registered.
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			cancel()
			helper.Cleanup()
		})
	}
	t.Cleanup(cleanup)

	addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)

	// Note: MCDataGenPath is left empty to use auto-detection.
	// This allows the agent to download data to .cache/mc-data-gen or find it automatically.
	// The FindOrCreateCacheDir utility can be used by other code that manages caches explicitly.

	agentCfg := testingpkg.AgentConfig{
		Name:           agentName,
		ServerAddress:  addr,
		Version:        serverCfg.Version,
		MCDataGenPath:  "", // Auto-detect: downloads to .cache/mc-data-gen by default
		EnableCamAgent: true,
	}

	managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	if err != nil {
		t.Fatalf("spawn agent: %v", err)
	}
	helper.ManagedAgent = managedAgent

	// Wait for agent to appear in player list
	if !testingpkg.WaitForPlayerOnline(ctx, inst.RCON, agentName, 30*time.Second) {
		t.Fatal("agent never appeared in server player list")
	}

	// Wait for essential registries to be populated before tests use them.
	// Registries are loaded from file during Init and from server config packets
	// during Start; this ensures they are available before test code runs.
	if err := helper.WaitForRegistry(ctx, "minecraft:entity_type", 10*time.Second); err != nil {
		t.Fatalf("entity_type registry not ready: %v", err)
	}

	return helper, ctx, cleanup
}

// Cleanup stops the agent, server, and closes logging
func (vh *VehicleTestHelper) Cleanup() {
	// CRITICAL: Stop the agent first to ensure replay files are properly closed
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	if vh.ManagedAgent != nil {
		if err := vh.ManagedAgent.Stop(stopCtx); err != nil {
			vh.t.Logf("WARNING: Agent stop returned error: %v", err)
		}
	}

	// Allow camera agent to process final frames and close replay files cleanly
	// This ensures the entire test sequence is captured in the replay
	time.Sleep(2 * time.Second)

	// Close agent logging
	vh.Framework.CloseAgentLog()

	// Stop the server
	stopCtx2, stopCancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel2()
	_ = vh.Framework.StopServer(stopCtx2, vh.Instance, true)
}

// SummonBoat summons a boat at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonBoat(ctx context.Context, x, y, z float64, variant string) (int32, error) {
	var cmd string
	// Summon the boat
	if vh.Instance.Server.Version == "1.21.1" {
		cmd = fmt.Sprintf(`summon minecraft:boat %f %f %f {Type:"%s"}`, x, y, z, variant)
	} else {
		if variant == "bamboo" {
			cmd = fmt.Sprintf("summon minecraft:bamboo_raft %f %f %f", x, y, z)
		} else {
			cmd = fmt.Sprintf("summon minecraft:%s_boat %f %f %f", variant, x, y, z)
		}
	}
	response, err := vh.Instance.RCON.Exec(ctx, cmd)
	if err != nil {
		return 0, fmt.Errorf("summon boat: %w", err)
	}

	fmt.Printf("[SummonBoat] %s => %s\n", cmd, response)

	// Wait for the entity to spawn and be tracked
	time.Sleep(500 * time.Millisecond)

	// Get boat entity type ID from the agent's registry
	// Special case: bamboo becomes bamboo_raft
	entityTypeName := fmt.Sprintf("minecraft:%s_boat", variant)
	if variant == "bamboo" {
		entityTypeName = "minecraft:bamboo_raft"
	}

	if vh.Instance.Server.Version == "1.21.1" {
		entityTypeName = "minecraft:boat"
	}

	boatTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID(entityTypeName)
	if !ok {
		vh.ManagedAgent.Agent.DumpRegistry("minecraft:entity_type")
		return 0, fmt.Errorf("boat entity type not found in registry for %s", entityTypeName)
	}

	// Find the boat by searching for nearest boat entity
	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(boatTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("boat entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// BuildHorseEnclosure creates a fence ring around the summon location to prevent wandering.
// Builds a 5x5 fence perimeter at ground level (y), leaving the center 3x3 area open for the horse to stand.
// This prevents the horse from wandering off while allowing normal mounting interaction.
func (vh *VehicleTestHelper) BuildHorseEnclosure(ctx context.Context, x, y, z float64) error {
	fenceY := int(y)

	// Build a 5x5 perimeter fence (only outer edge, not filled)
	// Center is at (x, z), so perimeter extends from (x-2) to (x+2) and (z-2) to (z+2)
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			// Only place fence on the outer perimeter (edges)
			if dx == -2 || dx == 2 || dz == -2 || dz == 2 {
				fenceX := int(x) + dx
				fenceZ := int(z) + dz
				cmd := fmt.Sprintf("setblock %d %d %d oak_fence", fenceX, fenceY, fenceZ)
				resp, err := vh.Instance.RCON.Exec(ctx, cmd)
				fmt.Printf("%s => %s\n", cmd, resp)
				if err != nil {
					return fmt.Errorf("build fence at (%d,%d,%d): %w", fenceX, fenceY, fenceZ, err)
				}
			}
		}
	}
	return nil
}

// RemoveHorseEnclosure removes the fence ring around the summon location.
func (vh *VehicleTestHelper) RemoveHorseEnclosure(ctx context.Context, x, y, z float64) error {
	fenceY := int(y)

	// Remove the 5x5 perimeter fence
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			// Only remove fence from the outer perimeter (edges)
			if dx == -2 || dx == 2 || dz == -2 || dz == 2 {
				fenceX := int(x) + dx
				fenceZ := int(z) + dz
				cmd := fmt.Sprintf("setblock %d %d %d air", fenceX, fenceY, fenceZ)
				resp, err := vh.Instance.RCON.Exec(ctx, cmd)
				fmt.Printf("%s => %s\n", cmd, resp)
				if err != nil {
					return fmt.Errorf("remove fence at (%d,%d,%d): %w", fenceX, fenceY, fenceZ, err)
				}
			}
		}
	}
	return nil
}

// SummonHorse summons a tamed and saddled horse at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonHorse(ctx context.Context, x, y, z, yaw float64) (int32, error) {
	// Summon command varies by version due to saddle NBT location change in 1.21.5
	// See docs/horse-nbt-data.md for version-specific NBT requirements
	// 1.21.1-1.21.4: SaddleItem is a top-level tag
	// 1.21.5+: Saddle is nested under equipment.saddle
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5")) // (">= 1.21.1, < 1.21.5") // 1.21.1 - 1.21.4 uses the old summon syntax

	if c.Check(v) {
		// Versions 1.21.1-1.21.4: Use SaddleItem tag
		cmd = fmt.Sprintf(
			`summon minecraft:horse %f %f %f {Rotation:[%ff,0f],Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1},Variant:0}`,
			x, y, z, yaw,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:horse %f %f %f {Rotation:[%ff,0f],Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1},},Variant:0}`,
			x, y, z, yaw,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon horse: %w", err)
	}

	// Equip the saddle via item replace, which is more reliable than NBT in the
	// summon command. The summon NBT sets initial state, but item replace is the
	// authoritative way to put an item in a specific equipment slot.
	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:horse,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip saddle: %w", err)
	}

	// Wait for entity to spawn and be tracked
	time.Sleep(500 * time.Millisecond)

	// Get horse entity type ID from the agent's registry
	horseTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:horse")
	if !ok {
		return 0, fmt.Errorf("horse entity type not found in registry")
	}

	// Find the horse by searching for nearest horse entity
	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(horseTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("horse entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonOption customizes the NBT applied when summoning an entity.
type SummonOption func(*summonOptions)

type summonOptions struct {
	noAI bool
}

// WithNoAI summons the entity with NoAI:1b so the server never runs its brain.
// Use this in tests that force a pose (e.g. sitting via LastPoseTick) and need
// the entity to hold it. NoAI entities do not respond to rider steering, so
// tests that exercise ridden movement must either leave AI enabled or restore
// it after mounting via EnableEntityAI (which also stops the mount wandering
// out of interact range during test setup).
func WithNoAI() SummonOption {
	return func(options *summonOptions) {
		options.noAI = true
	}
}

// SummonCamel summons a tamed and saddled camel at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonCamel(ctx context.Context, x, y, z, yaw float64, opts ...SummonOption) (int32, error) {
	// Summon command varies by version due to saddle NBT location change in 1.21.5
	// Camels must be tamed (Tame:1b) to be rideable
	var cmd string
	version := vh.Instance.Server.Version

	var options summonOptions
	for _, opt := range opts {
		opt(&options)
	}
	extraNBT := ""
	if options.noAI {
		extraNBT = ",NoAI:1b"
	}

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		// Versions 1.21.1-1.21.4: Use SaddleItem tag
		cmd = fmt.Sprintf(
			`summon minecraft:camel %f %f %f {Rotation:[%ff,0f],Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}%s}`,
			x, y, z, yaw, extraNBT,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:camel %f %f %f {Rotation:[%ff,0f],Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}%s}`,
			x, y, z, yaw, extraNBT,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon camel: %w", err)
	}

	// Equip the saddle via item replace for reliability
	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:camel,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] camel saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip camel saddle: %w", err)
	}

	// Wait for entity to spawn and be tracked
	time.Sleep(500 * time.Millisecond)

	// Get camel entity type ID from the agent's registry
	camelTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:camel")
	if !ok {
		return 0, fmt.Errorf("camel entity type not found in registry")
	}

	// Find the camel by searching for nearest camel entity
	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(camelTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("camel entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonCamelHusk summons a tamed camel husk at the specified location and returns its entity ID
// Camel husk was added in Minecraft 1.21.11.
func (vh *VehicleTestHelper) SummonCamelHusk(ctx context.Context, x, y, z, yaw float64) (int32, error) {
	// Camel husks must be tamed (Tame:1b) to be rideable
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	minVersion, _ := semver.NewConstraints("< 1.21.11")
	if minVersion.Check(v) {
		return 0, fmt.Errorf("camel husk requires Minecraft 1.21.11 or later, got %s", version)
	}

	// 1.21.11+ always uses the equipment.saddle structure
	cmd := fmt.Sprintf(
		`summon minecraft:camel_husk %f %f %f {Rotation:[%ff,0f],Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
		x, y, z, yaw,
	)

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon camel husk: %w", err)
	}

	// Equip the saddle via item replace for reliability
	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:camel_husk,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] camel husk saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip camel husk saddle: %w", err)
	}

	// Wait for entity to spawn and be tracked
	time.Sleep(500 * time.Millisecond)

	// Get camel husk entity type ID from the agent's registry
	camelHuskTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:camel_husk")
	if !ok {
		return 0, fmt.Errorf("camel husk entity type not found in registry")
	}

	// Find the camel husk by searching for nearest camel husk entity
	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(camelHuskTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("camel husk entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonEntity summons a generic entity by type name
// Note: This returns entity ID 0 and requires caller to find the entity manually via FindNearestEntityByType
// or other means, since entity ID parsing from RCON responses is unreliable across versions
func (vh *VehicleTestHelper) SummonEntity(ctx context.Context, entityType string, x, y, z float64) (int32, error) {
	cmd := fmt.Sprintf("summon minecraft:%s %f %f %f", entityType, x, y, z)
	_, err := vh.Instance.RCON.Exec(ctx, cmd)
	if err != nil {
		return 0, fmt.Errorf("summon entity: %w", err)
	}

	// Return 0 - caller should use FindNearestEntityByType to locate the spawned entity
	return 0, nil
}

// GetTrackedEntity retrieves a tracked entity from the agent
func (vh *VehicleTestHelper) GetTrackedEntity(entityID int32) *models.TrackedEntityInfo {
	entities := vh.ManagedAgent.GetTrackedEntities()
	if ent, ok := entities[entityID]; ok {
		return &ent
	}
	return nil
}

// WaitForRegistry polls until the named registry is loaded and ready on the
// main agent, returning an error if the timeout expires first.
func (vh *VehicleTestHelper) WaitForRegistry(ctx context.Context, registryID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			reg := vh.ManagedAgent.Agent.GetRegistry(registryID)
			if reg != nil && reg.IsReady() {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("registry %s not ready after %v", registryID, timeout)
			}
		}
	}
}

// GetDistance calculates distance between two positions
func GetDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	dz := z2 - z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// WaitForEntityTracking waits for an entity to be tracked by the agent
func (vh *VehicleTestHelper) WaitForEntityTracking(ctx context.Context, entityID int32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if vh.GetTrackedEntity(entityID) != nil {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("entity %d not tracked after %v", entityID, timeout)
			}
		}
	}
}

// WaitForAttribute waits for an entity attribute to be set
// Note: This is a placeholder for future attribute tracking when it's exposed via the public interface
func (vh *VehicleTestHelper) WaitForAttribute(ctx context.Context, entityID int32, attrName string, timeout time.Duration) error {
	// For now, attributes are tracked internally but not exposed via the public interface
	// This method can be enhanced when attribute access is exposed
	return fmt.Errorf("attribute tracking not yet exposed via public interface")
}

// WaitForMounted waits for the agent to mount an entity
func (vh *VehicleTestHelper) WaitForMounted(ctx context.Context, timeout time.Duration) error {
	return vh.WaitForMountedWithDiagnostics(ctx, timeout, 0)
}

// WaitForMountedWithDiagnostics waits for agent to mount and logs diagnostic info on timeout
func (vh *VehicleTestHelper) WaitForMountedWithDiagnostics(ctx context.Context, timeout time.Duration, vehicleEntityID int32) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check if agent is mounted by checking agent impl with type assertion
			agentImpl, ok := vh.ManagedAgent.Agent.(interface{ IsMounted() bool })
			if ok && agentImpl.IsMounted() {
				return nil
			}
			if time.Now().After(deadline) {
				// Log diagnostic information on timeout
				vh.logMountingDiagnostics(vehicleEntityID)
				return fmt.Errorf("agent did not mount after %v", timeout)
			}
		}
	}
}

// logMountingDiagnostics logs agent position, vehicle position, and distance
func (vh *VehicleTestHelper) logMountingDiagnostics(vehicleEntityID int32) {
	agentPos, _ := vh.ManagedAgent.Agent.GetPositionSimple()
	entities := vh.ManagedAgent.Agent.GetTrackedEntities()

	vh.t.Logf("[Mount Diagnostics] Agent position: (%.2f, %.2f, %.2f)", agentPos.X, agentPos.Y, agentPos.Z)

	if vehicleEntityID != 0 {
		if entityInfo, exists := entities[vehicleEntityID]; exists {
			vh.t.Logf("[Mount Diagnostics] Vehicle position: (%.2f, %.2f, %.2f)", entityInfo.X, entityInfo.Y, entityInfo.Z)

			// Calculate 3D distance
			dx := entityInfo.X - agentPos.X
			dy := entityInfo.Y - agentPos.Y
			dz := entityInfo.Z - agentPos.Z
			distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
			horizontalDistance := math.Sqrt(dx*dx + dz*dz)

			vh.t.Logf("[Mount Diagnostics] Distance to vehicle: %.2f blocks (horizontal: %.2f)", distance, horizontalDistance)
			vh.t.Logf("[Mount Diagnostics] Minecraft interaction range is ~4.5 blocks")

			if distance > 5.0 {
				vh.t.Logf("[Mount Diagnostics] ⚠️  LIKELY CAUSE: Vehicle is too far away (> 5 blocks)")
			}
		} else {
			vh.t.Logf("[Mount Diagnostics] Vehicle entityID %d not found in tracked entities", vehicleEntityID)
		}
	} else {
		vh.t.Logf("[Mount Diagnostics] Tracked entities: %d total", len(entities))
		for id, info := range entities {
			vh.t.Logf("  - Entity %d: (%.2f, %.2f, %.2f)", id, info.X, info.Y, info.Z)
		}
	}
}

// WaitForDismounted waits for the agent to dismount
func (vh *VehicleTestHelper) WaitForDismounted(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check if agent is not mounted using type assertion
			agentImpl, ok := vh.ManagedAgent.Agent.(interface{ IsMounted() bool })
			if !ok || !agentImpl.IsMounted() {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("agent did not dismount after %v", timeout)
			}
		}
	}
}

// MountEntity mounts the agent on a vehicle
func (vh *VehicleTestHelper) MountEntity(ctx context.Context, entityID int32) error {
	return vh.ManagedAgent.Agent.MountEntity(ctx, entityID)
}

// EnableEntityAI clears the NoAI flag on the single entity of the given type.
// Pair with WithNoAI: summon the mount brainless so it cannot wander out of
// interact range before mounting, then restore AI once mounted so the server
// runs its travel logic (rider steering, gravity) again.
func (vh *VehicleTestHelper) EnableEntityAI(ctx context.Context, entityType string) error {
	cmd := fmt.Sprintf("data merge entity @e[type=%s,limit=1] {NoAI:0b}", entityType)
	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return fmt.Errorf("enable AI on %s: %w", entityType, err)
	}
	return rconResponseError(cmd, resp)
}

// DismountEntity dismounts the agent from a vehicle
func (vh *VehicleTestHelper) DismountEntity() error {
	return vh.ManagedAgent.Agent.DismountEntity()
}

// JumpVehicle makes the agent jump while mounted (horse only)
func (vh *VehicleTestHelper) JumpVehicle(ctx context.Context, power int32) error {
	return vh.ManagedAgent.Agent.JumpVehicle(ctx, power)
}

// SetManualThrottle sets the manual throttle for movement
func (vh *VehicleTestHelper) SetManualThrottle(throttleX, throttleZ float64) {
	if err := vh.ManagedAgent.Agent.SetManualThrottle(throttleX, throttleZ); err != nil {
		vh.t.Fatalf("SetManualThrottle failed: %v", err)
	}
}

// SetManualJump sets whether the jump button is pressed.
// For camels, holding charges the dash; releasing fires the impulse.
func (vh *VehicleTestHelper) SetManualJump(enabled bool) {
	if err := vh.ManagedAgent.Agent.SetManualJump(enabled); err != nil {
		vh.t.Fatalf("SetManualJump failed: %v", err)
	}
}

// EnterManualMode switches to manual movement mode
func (vh *VehicleTestHelper) EnterManualMode() error {
	return vh.ManagedAgent.Agent.EnterManualMode()
}

// ExitManualMode switches out of manual movement mode
func (vh *VehicleTestHelper) ExitManualMode() error {
	return vh.ManagedAgent.Agent.ExitManualMode()
}

// GetRidingPhysicsInspector returns the RidingPhysicsInspector if the agent
// supports it, allowing tests to read actual executor physics state.
func (vh *VehicleTestHelper) GetRidingPhysicsInspector() (models.RidingPhysicsInspector, bool) {
	inspector, ok := vh.ManagedAgent.Agent.(models.RidingPhysicsInspector)
	return inspector, ok
}

// WaitForRidingVelocityZero polls until the executor's riding velocity decays
// to zero, replacing sleep-based coast-to-stop. Falls back to a fixed sleep
// if the agent does not support RidingPhysicsInspector.
func (vh *VehicleTestHelper) WaitForRidingVelocityZero(ctx context.Context, timeout time.Duration) error {
	inspector, ok := vh.GetRidingPhysicsInspector()
	if !ok {
		time.Sleep(timeout)
		return nil
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			velX, velZ := inspector.GetRidingVelocity()
			if velX == 0 && velZ == 0 {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("riding velocity did not reach zero after %v (velX=%.6f, velZ=%.6f)", timeout, velX, velZ)
			}
		}
	}
}

// SummonMinecart summons a minecart at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonMinecart(ctx context.Context, x, y, z float64) (int32, error) {
	cmd := fmt.Sprintf("summon minecraft:minecart %f %f %f", x, y, z)
	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon minecart: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Find the minecart entity
	minecartTypeID, found := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:minecart")
	if !found {
		return 0, fmt.Errorf("minecart entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.Agent.FindNearestEntityByType(minecartTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("minecart entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// BuildRailTrack builds a straight horizontal rail track in the specified direction
// direction: "north" | "south" | "east" | "west"
// length: number of rail blocks to place
func (vh *VehicleTestHelper) BuildRailTrack(ctx context.Context, startX, startY, startZ float64, direction string, length int, powered bool) error {
	railY := int(startY)
	var dx, dz int

	switch direction {
	case "north":
		dx, dz = 0, -1
	case "south":
		dx, dz = 0, 1
	case "east":
		dx, dz = 1, 0
	case "west":
		dx, dz = -1, 0
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	startRailX := int(startX)
	startRailZ := int(startZ)

	railName := "minecraft:rail"
	if powered {
		railName = "minecraft:powered_rail"
	}

	for i := range length {
		railX := startRailX + (dx * i)
		railZ := startRailZ + (dz * i)
		cmd := fmt.Sprintf("setblock %d %d %d %s", railX, railY, railZ, railName)
		resp, err := vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place rail at (%d,%d,%d): %w", railX, railY, railZ, err)
		}
	}
	return nil
}

// BuildAscendingRailTrack builds an ascending rail track at 45-degree angle
// direction: "north" | "south" | "east" | "west" (the upward direction)
// length: number of rail blocks to place
func (vh *VehicleTestHelper) BuildAscendingRailTrack(ctx context.Context, startX, startY, startZ float64, direction string, length int, powered bool) error {
	railStartY := int(startY)
	var dx, dz int

	switch direction {
	case "north":
		dx, dz = 0, -1
	case "south":
		dx, dz = 0, 1
	case "east":
		dx, dz = 1, 0
	case "west":
		dx, dz = -1, 0
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	startRailX := int(startX)
	startRailZ := int(startZ)

	railName := "minecraft:rail"
	blockName := "minecraft:stone"
	if powered {
		blockName = "minecraft:redstone_block"
		railName = "minecraft:powered_rail"
	}

	// Map direction to ascending rail type
	var railType string
	switch direction {
	case "north":
		railType = railName + "[shape=ascending_north]"
	case "south":
		railType = railName + "[shape=ascending_south]"
	case "east":
		railType = railName + "[shape=ascending_east]"
	case "west":
		railType = railName + "[shape=ascending_west]"
	}

	for i := range length {
		railX := startRailX + (dx * i)
		railY := railStartY + i
		railZ := startRailZ + (dz * i)
		cmd := fmt.Sprintf("setblock %d %d %d %s", railX, railY-1, railZ, blockName) // Place solid block under the rail for support
		resp, err := vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place ascending rail at (%d,%d,%d): %w", railX, railY, railZ, err)
		}

		cmd = fmt.Sprintf("setblock %d %d %d %s", railX, railY, railZ, railType)
		resp, err = vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place ascending rail at (%d,%d,%d): %w", railX, railY, railZ, err)
		}
	}
	return nil
}

// BuildWaterloggedRailTrack builds a rail track with waterlogged property in the specified direction
// This tests waterlogged rail detection (rails with waterlogged=true property)
// direction: "north" | "south" | "east" | "west"
// length: number of rail blocks to place
func (vh *VehicleTestHelper) BuildWaterloggedRailTrack(ctx context.Context, startX, startY, startZ float64, direction string, length int, powered bool) error {
	railY := int(startY)
	var dx, dz int
	var shape string

	switch direction {
	case "north":
		dx, dz = 0, -1
		shape = "north_south"
	case "south":
		dx, dz = 0, 1
		shape = "north_south"
	case "east":
		dx, dz = 1, 0
		shape = "east_west"
	case "west":
		dx, dz = -1, 0
		shape = "east_west"
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	startRailX := int(startX)
	startRailZ := int(startZ)

	railName := "minecraft:rail"
	if powered {
		railName = "minecraft:powered_rail"
	}

	// Build waterlogged rails with the waterlogged property and proper shape set
	for i := range length {
		railX := startRailX + (dx * i)
		railZ := startRailZ + (dz * i)

		// Place waterlogged rail: setblock with shape and waterlogged=true properties
		cmd := fmt.Sprintf("setblock %d %d %d %s[shape=%s,waterlogged=true]", railX, railY, railZ, railName, shape)
		resp, err := vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place waterlogged rail at (%d,%d,%d): %w", railX, railY, railZ, err)
		}
	}
	return nil
}

// BuildAscendingWaterloggedRailTrack builds an ascending waterlogged rail track at 45-degree angle
// This tests waterlogged rail detection with ascending shapes
// direction: "north" | "south" | "east" | "west" (the upward direction)
// length: number of rail blocks to place
func (vh *VehicleTestHelper) BuildAscendingWaterloggedRailTrack(ctx context.Context, startX, startY, startZ float64, direction string, length int, powered bool) error {
	railStartY := int(startY)
	var dx, dz int

	switch direction {
	case "north":
		dx, dz = 0, -1
	case "south":
		dx, dz = 0, 1
	case "east":
		dx, dz = 1, 0
	case "west":
		dx, dz = -1, 0
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	startRailX := int(startX)
	startRailZ := int(startZ)

	railName := "minecraft:rail"
	blockName := "minecraft:stone"
	if powered {
		blockName = "minecraft:redstone_block"
		railName = "minecraft:powered_rail"
	}

	// Map direction to ascending rail type with waterlogged property
	var railType string
	switch direction {
	case "north":
		railType = railName + "[shape=ascending_north,waterlogged=true]"
	case "south":
		railType = railName + "[shape=ascending_south,waterlogged=true]"
	case "east":
		railType = railName + "[shape=ascending_east,waterlogged=true]"
	case "west":
		railType = railName + "[shape=ascending_west,waterlogged=true]"
	}

	for i := range length {
		railX := startRailX + (dx * i)
		railY := railStartY + i
		railZ := startRailZ + (dz * i)

		cmd := fmt.Sprintf("setblock %d %d %d %s", railX, railY-1, railZ, blockName)
		resp, err := vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place ascending waterlogged rail support at (%d,%d,%d): %w", railX, railY, railZ, err)
		}

		cmd = fmt.Sprintf("setblock %d %d %d %s", railX, railY, railZ, railType)
		resp, err = vh.Instance.RCON.Exec(ctx, cmd)
		log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
		if err != nil {
			return fmt.Errorf("place ascending waterlogged rail at (%d,%d,%d): %w", railX, railY, railZ, err)
		}
	}
	return nil
}

// SummonPig summons a saddled pig at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonPig(ctx context.Context, x, y, z float64) (int32, error) {
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		// Versions 1.21.1-1.21.4: Use SaddleItem tag
		cmd = fmt.Sprintf(
			`summon minecraft:pig %f %f %f {SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:pig %f %f %f {equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon pig: %w", err)
	}

	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:pig,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] pig saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip pig saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	pigTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:pig")
	if !ok {
		return 0, fmt.Errorf("pig entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(pigTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("pig entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonStrider summons a saddled strider at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonStrider(ctx context.Context, x, y, z float64) (int32, error) {
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		cmd = fmt.Sprintf(
			`summon minecraft:strider %f %f %f {SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z,
		)
	} else {
		cmd = fmt.Sprintf(
			`summon minecraft:strider %f %f %f {equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon strider: %w", err)
	}

	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:strider,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] strider saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip strider saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	striderTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:strider")
	if !ok {
		return 0, fmt.Errorf("strider entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(striderTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("strider entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// rconResponseError returns a non-nil error when an RCON response indicates the
// command did not take effect even though the RCON transport itself succeeded
// (err == nil). Minecraft reports these conditions as ordinary response text —
// e.g. filling an unloaded chunk returns "That position is not loaded", and
// targeting a missing entity returns "No entity was found" — so setup helpers
// must inspect the response to avoid silently continuing with an unchanged world.
func rconResponseError(cmd, resp string) error {
	failurePhrases := []string{"not loaded", "No entity was found"}
	for _, phrase := range failurePhrases {
		if strings.Contains(resp, phrase) {
			return fmt.Errorf("RCON command %q did not take effect: %s", cmd, resp)
		}
	}
	return nil
}

// BuildLavaPool fills a square region with lava at the specified center and radius.
// The pool is one block deep. Useful for strider movement tests.
func (vh *VehicleTestHelper) BuildLavaPool(ctx context.Context, centerX, centerY, centerZ, radius int) error {
	minX := centerX - radius
	maxX := centerX + radius
	minZ := centerZ - radius
	maxZ := centerZ + radius

	// Use fill command for efficiency (single RCON call for the entire region)
	cmd := fmt.Sprintf("fill %d %d %d %d %d %d lava", minX, centerY, minZ, maxX, centerY, maxZ)
	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return fmt.Errorf("build lava pool: %w", err)
	}
	if respErr := rconResponseError(cmd, resp); respErr != nil {
		return fmt.Errorf("build lava pool: %w", respErr)
	}

	return nil
}

// GiveAndEquipWarpedFungus gives the agent a warped_fungus_on_a_stick and equips it.
// This is required for strider control — in vanilla, the player must hold this item
// for getControllingPassenger() to return the player.
func (vh *VehicleTestHelper) GiveAndEquipWarpedFungus(ctx context.Context) error {
	_, err := vh.Instance.RCON.Exec(ctx, fmt.Sprintf("give %s warped_fungus_on_a_stick", vh.AgentName))
	if err != nil {
		return fmt.Errorf("give warped_fungus_on_a_stick: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	found, err := vh.ManagedAgent.Agent.SwitchToItem(ctx, "minecraft:warped_fungus_on_a_stick")
	if err != nil {
		return fmt.Errorf("switch to warped_fungus_on_a_stick: %w", err)
	}
	if !found {
		return fmt.Errorf("warped_fungus_on_a_stick not found in inventory after giving")
	}

	time.Sleep(500 * time.Millisecond) // wait for item switch to register on server
	return nil
}

// SummonSkeletonHorse summons a tamed and saddled skeleton horse at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonSkeletonHorse(ctx context.Context, x, y, z float64) (int32, error) {
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		cmd = fmt.Sprintf(
			`summon minecraft:skeleton_horse %f %f %f {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z,
		)
	} else {
		cmd = fmt.Sprintf(
			`summon minecraft:skeleton_horse %f %f %f {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon skeleton horse: %w", err)
	}

	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:skeleton_horse,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] skeleton horse saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip skeleton horse saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	typeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:skeleton_horse")
	if !ok {
		return 0, fmt.Errorf("skeleton horse entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(typeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("skeleton horse entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonZombieHorse summons a tamed and saddled zombie horse at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonZombieHorse(ctx context.Context, x, y, z float64) (int32, error) {
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		cmd = fmt.Sprintf(
			`summon minecraft:zombie_horse %f %f %f {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z,
		)
	} else {
		cmd = fmt.Sprintf(
			`summon minecraft:zombie_horse %f %f %f {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon zombie horse: %w", err)
	}

	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:zombie_horse,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] zombie horse saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip zombie horse saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	typeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:zombie_horse")
	if !ok {
		return 0, fmt.Errorf("zombie horse entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(typeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("zombie horse entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonNautilus summons a tamed and saddled nautilus at the specified location (in water) and returns its entity ID
func (vh *VehicleTestHelper) SummonNautilus(ctx context.Context, x, y, z float64) (int32, error) {
	// The nautilus is a TameableEntity (like a wolf), so it is tamed via a valid
	// Owner reference, NOT the horse-style Tame:1b byte (which it ignores). Owning
	// it to the agent makes isTamed() true so interactMob() will let the agent mount.
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		// Versions 1.21.1-1.21.4: Use SaddleItem tag
		cmd = fmt.Sprintf(
			`summon minecraft:nautilus %f %f %f {Owner:"%s",SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z, vh.AgentName,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:nautilus %f %f %f {Owner:"%s",equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z, vh.AgentName,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon nautilus: %w", err)
	}

	// Equip the saddle via item replace for reliability
	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:nautilus,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] nautilus saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip nautilus saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	nautilusTypeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:nautilus")
	if !ok {
		return 0, fmt.Errorf("nautilus entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(nautilusTypeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("nautilus entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// SummonZombieNautilus summons a zombie nautilus at the specified location (in water) and returns its entity ID
func (vh *VehicleTestHelper) SummonZombieNautilus(ctx context.Context, x, y, z float64) (int32, error) {
	// Like the nautilus, the zombie nautilus is a TameableEntity tamed via a valid
	// Owner reference (not the horse-style Tame:1b byte). Owning it to the agent
	// makes isTamed() true so interactMob() will let the agent mount.
	var cmd string
	version := vh.Instance.Server.Version

	v, _ := semver.Parse(version)
	c, _ := semver.NewConstraints(("< 1.21.5"))

	if c.Check(v) {
		// Versions 1.21.1-1.21.4: Use SaddleItem tag
		cmd = fmt.Sprintf(
			`summon minecraft:zombie_nautilus %f %f %f {Owner:"%s",SaddleItem:{id:"minecraft:saddle",count:1}}`,
			x, y, z, vh.AgentName,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:zombie_nautilus %f %f %f {Owner:"%s",equipment:{saddle:{id:"minecraft:saddle",count:1}}}`,
			x, y, z, vh.AgentName,
		)
	}

	resp, err := vh.Instance.RCON.Exec(ctx, cmd)
	log.Printf("[VehicleTestHelper] %s => %s", cmd, resp)
	if err != nil {
		return 0, fmt.Errorf("summon zombie nautilus: %w", err)
	}

	// Equip the saddle via item replace for reliability
	saddleResp, err := vh.Instance.RCON.Exec(ctx, "item replace entity @e[type=minecraft:zombie_nautilus,limit=1] saddle with minecraft:saddle")
	log.Printf("[VehicleTestHelper] zombie nautilus saddle equip => %s", saddleResp)
	if err != nil {
		return 0, fmt.Errorf("equip zombie nautilus saddle: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	typeID, ok := vh.ManagedAgent.Agent.GetEntityTypeID("minecraft:zombie_nautilus")
	if !ok {
		return 0, fmt.Errorf("zombie nautilus entity type not found in registry")
	}

	entityID, _, found := vh.ManagedAgent.FindNearestEntityByType(typeID, x, y, z)
	if !found {
		return 0, fmt.Errorf("zombie nautilus entity not found after summoning at (%.1f, %.1f, %.1f)", x, y, z)
	}

	return entityID, nil
}

// TrackEntityPosition creates an EntityPositionTracker for monitoring entity movement.
// The tracker automatically registers itself with the agent and records peak/min coordinates.
func (vh *VehicleTestHelper) TrackEntityPosition(entityID int32) *utils.EntityPositionTracker {
	// Get initial position
	initialPos, _ := vh.ManagedAgent.Agent.GetPositionSimple()

	// Create tracker
	tracker := utils.NewEntityPositionTracker(entityID, initialPos)

	// Register with agent to receive position updates
	tracker.RegisterCallback(vh.ManagedAgent.Agent)

	return tracker
}

// isVersionGreaterOrEqual checks if targetVersion >= minVersion using semantic versioning.
// Returns false if the version cannot be parsed (safe default for older versions).
func isVersionGreaterOrEqual(targetVersion, minVersion string) bool {
	v, err := semver.Parse(targetVersion)
	if err != nil {
		// If we can't parse, assume older version (safer default)
		return false
	}

	c, err := semver.NewConstraints(fmt.Sprintf(">= %s", minVersion))
	if err != nil {
		return false
	}

	return c.Check(v)
}
