package vehicles

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"testing"
	"time"

	semver "github.com/aquasecurity/go-version/pkg/version"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
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
	// This allows the agent to download data to data/mc-data-gen-cache or find it automatically.
	// The FindOrCreateCacheDir utility can be used by other code that manages caches explicitly.

	agentCfg := testingpkg.AgentConfig{
		Name:           agentName,
		ServerAddress:  addr,
		Version:        serverCfg.Version,
		MCDataGenPath:  "", // Auto-detect: downloads to data/mc-data-gen-cache by default
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
				fmt.Printf("%s => %s\n",cmd, resp)
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
				fmt.Printf("%s => %s\n",cmd, resp)
				if err != nil {
					return fmt.Errorf("remove fence at (%d,%d,%d): %w", fenceX, fenceY, fenceZ, err)
				}
			}
		}
	}
	return nil
}

// SummonHorse summons a tamed and saddled horse at the specified location and returns its entity ID
func (vh *VehicleTestHelper) SummonHorse(ctx context.Context, x, y, z float64) (int32, error) {
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
			`summon minecraft:horse %f %f %f {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1},Variant:0}`,
			x, y, z,
		)
	} else {
		// Versions 1.21.5+: Use equipment.saddle structure
		cmd = fmt.Sprintf(
			`summon minecraft:horse %f %f %f {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}},Variant:0}`,
			x, y, z,
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
				return fmt.Errorf("agent did not mount after %v", timeout)
			}
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

// EnterManualMode switches to manual movement mode
func (vh *VehicleTestHelper) EnterManualMode() error {
	return vh.ManagedAgent.Agent.EnterManualMode()
}

// ExitManualMode switches out of manual movement mode
func (vh *VehicleTestHelper) ExitManualMode() error {
	return vh.ManagedAgent.Agent.ExitManualMode()
}
