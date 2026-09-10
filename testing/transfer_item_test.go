// TestTransferItemAcrossServers exercises mc-item-transfer-mod +
// mc-agent's courier end-to-end against two real, independently-configured
// Fabric servers: trust setup (via RCON, mirroring the /itemtransfer
// commands an operator would type - see
// mc-item-transfer-mod/shared-src/.../command/ItemTransferCommands.java),
// item_transfer:handshake, and a full request->hold->claim->promised->commit
// transfer of a STACK of items (5 diamonds - this specifically answers
// "does this support stacks, not just single items": yes, because
// ItemCodec.encode (mc-item-transfer-mod) uses ItemStack.CODEC, which always
// includes count, not just item id).
//
// # Prerequisites (this test SKIPS, not fails, if unmet)
//
//   - Docker running - the same gate every other test in this package uses
//     (RequireIntegrationEnv).
//   - testing/mods/v1_21_11/ populated with the item-transfer-mod jar for
//     1.21.11 AND a matching fabric-api jar (Fabric API is a modImplementation
//     dependency of that mod, not bundled into its jar - a real Fabric server
//     needs both jars present). See testing/mods/README.md for exactly how to
//     build/find them.
//
// # Why two servers of the SAME version need distinct ServerConfig.CacheDir
//
// Framework.StartServer's DataDir (world save, and - since this test leaves
// ConfigDir unset - the mod's own config/item_transfer/{identity.key,
// config.json, trusted_keys.json}) defaults to one directory shared by every
// server of a given version (see getModsDir/getConfigDir's doc comments in
// framework.go), which is fine for the usual "one server at a time" case
// this suite was built around. Two servers of the *same* version running
// *simultaneously* would collide badly if they shared it: same identity,
// same serverId, same world save opened by two processes at once - exactly
// the opposite of what a cross-server trust test needs. Setting
// ServerConfig.CacheDir explicitly to two distinct paths (an override point
// Framework.StartServer already supports, not something added for this
// test) avoids this entirely; ModsDir stays shared since it's the same
// read-only jar content either way, and per-version ConfigDir stays unset
// (do NOT populate testing/configs/v1_21_11/item_transfer/ for this test -
// see the same collision reasoning).
package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/courier"
	"github.com/stretchr/testify/require"
)

const transferTestVersion = "1.21.11"

var itemTransferPubkeyRe = regexp.MustCompile(`serverId=(\S+)\s+pubkey=(\S+)`)

// itemTransferModsAvailable reports whether testing/mods/v1_21_11 contains
// both jars this test needs. A generic "directory is non-empty" check would
// too easily pass on an unrelated leftover jar from some other test.
func itemTransferModsAvailable(t *testing.T) bool {
	t.Helper()
	dir := getModsDir(transferTestVersion)
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	hasMod, hasFabricAPI := false, false
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jar") {
			continue
		}
		switch {
		case strings.Contains(name, "item-transfer-mod"):
			hasMod = true
		case strings.Contains(name, "fabric-api"):
			hasFabricAPI = true
		}
	}
	return hasMod && hasFabricAPI
}

// fetchPubkey runs "/itemtransfer pubkey" over RCON (server-console
// permission is always above the mod's default adminPermissionLevel, so no
// player needs to be op'd) and parses its "serverId=X pubkey=Y" output.
func fetchPubkey(ctx context.Context, rcon rconExecer) (serverID, pubkey string, err error) {
	resp, err := rcon.Exec(ctx, "itemtransfer pubkey")
	if err != nil {
		return "", "", fmt.Errorf("itemtransfer pubkey: %w", err)
	}
	m := itemTransferPubkeyRe.FindStringSubmatch(resp)
	if m == nil {
		return "", "", fmt.Errorf("unexpected /itemtransfer pubkey output: %q", resp)
	}
	return m[1], m[2], nil
}

// trustPeer runs "/itemtransfer trust add <peerServerID> <peerPubkey>" over
// RCON - the ready-to-paste command /itemtransfer trust command now prints
// for a human operator, issued here directly instead.
func trustPeer(ctx context.Context, rcon rconExecer, peerServerID, peerPubkey string) error {
	cmd := fmt.Sprintf("itemtransfer trust add %s %s", peerServerID, peerPubkey)
	resp, err := rcon.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("%s: %w", cmd, err)
	}
	if !strings.Contains(resp, "Trusted") {
		return fmt.Errorf("unexpected /itemtransfer trust add response: %q", resp)
	}
	return nil
}

// rconExecer is the narrow slice of testenv.RCONHelper this file's own
// helpers need - just Exec - so they aren't coupled to the full RCONHelper
// interface's much larger surface (mirrors courier.AgentMessenger's and
// courier.ChatResponder's own narrow-interface reasoning).
type rconExecer interface {
	Exec(ctx context.Context, cmd string) (string, error)
}

// waitForHandshake polls Courier.HandshakeComplete until it reports true or
// timeout elapses.
func waitForHandshake(t *testing.T, c *courier.Courier, label string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.HandshakeComplete(label) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("handshake with %s never completed within %s", label, timeout)
}

func TestTransferItemAcrossServers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cwd, err := os.Getwd()
	require.NoError(t, err, "get current working directory")

	baseCfg := FlatWorldServerConfig()
	baseCfg.Version = transferTestVersion
	RequireIntegrationEnv(t, baseCfg)

	if !itemTransferModsAvailable(t) {
		t.Skipf("testing/mods/v%s missing item-transfer-mod and/or fabric-api jars - see testing/mods/README.md",
			strings.ReplaceAll(transferTestVersion, ".", "_"))
	}

	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// --- Start two independent servers of the same version (see the file
	// doc comment on why CacheDir must differ) ---
	startServer := func(label string) *TestInstance {
		cfg := baseCfg
		cfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestTransferItemAcrossServers", label)
		inst, err := framework.StartServer(ctx, cfg)
		require.NoError(t, err, "start server %s", label)
		t.Logf("%s: server started at %s:%d", label, inst.Server.Host, inst.Server.HostServerPort)
		return inst
	}

	instA := startServer("serverA")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = framework.StopServer(stopCtx, instA, true)
	}()

	instB := startServer("serverB")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = framework.StopServer(stopCtx, instB, true)
	}()

	// --- Trust setup, exactly what an operator would type per
	// cmd/item-courier/README.md, just issued over RCON instead of by hand ---
	serverIDA, pubkeyA, err := fetchPubkey(ctx, instA.RCON)
	require.NoError(t, err, "fetch serverA pubkey")
	serverIDB, pubkeyB, err := fetchPubkey(ctx, instB.RCON)
	require.NoError(t, err, "fetch serverB pubkey")
	require.NotEqual(t, serverIDA, serverIDB, "the two servers must not have generated the same serverId")

	require.NoError(t, trustPeer(ctx, instA.RCON, serverIDB, pubkeyB), "trust serverB from serverA")
	require.NoError(t, trustPeer(ctx, instB.RCON, serverIDA, pubkeyA), "trust serverA from serverB")

	// --- One shared player identity, one agent per server (per
	// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md's architecture) ---
	const botName = "ItemCourierTest"
	addrA := fmt.Sprintf("%s:%d", instA.Server.Host, instA.Server.HostServerPort)
	addrB := fmt.Sprintf("%s:%d", instB.Server.Host, instB.Server.HostServerPort)

	agentA, err := framework.SpawnAgent(ctx, instA, AgentConfig{
		Name:           botName,
		ServerAddress:  addrA,
		Version:        transferTestVersion,
		EnableCamAgent: false,
	})
	require.NoError(t, err, "spawn agent on serverA")
	defer func() { _ = agentA.Stop(context.Background()) }()
	require.True(t, WaitForPlayerOnline(ctx, instA.RCON, botName, 30*time.Second), "agent never appeared on serverA")

	agentB, err := framework.SpawnAgent(ctx, instB, AgentConfig{
		Name:           botName,
		ServerAddress:  addrB,
		Version:        transferTestVersion,
		EnableCamAgent: false,
	})
	require.NoError(t, err, "spawn agent on serverB")
	defer func() { _ = agentB.Stop(context.Background()) }()
	require.True(t, WaitForPlayerOnline(ctx, instB.RCON, botName, 30*time.Second), "agent never appeared on serverB")

	// --- Courier wiring + handshake ---
	c := courier.New(map[string]courier.AgentMessenger{
		serverIDA: agentA.Agent,
		serverIDB: agentB.Agent,
	}, nil)
	c.Attach()

	require.NoError(t, c.Handshake(serverIDA), "send handshake to serverA")
	require.NoError(t, c.Handshake(serverIDB), "send handshake to serverB")
	waitForHandshake(t, c, serverIDA, 15*time.Second)
	waitForHandshake(t, c, serverIDB, 15*time.Second)
	t.Log("handshake complete with both servers")

	// --- Seed a STACK of items (5 diamonds) into a specific slot on
	// serverA, using the same "Slot" NBT numbering (0-35) TransferManager
	// itself indexes with and GetInventoryItems already parses back ---
	const sourceSlot = "0"
	const itemID = "minecraft:diamond"
	const itemCount = 5
	// `/data merge|modify entity` is rejected by vanilla for a live,
	// connected player ("Unable to modify player data" - confirmed via RCON,
	// not assumed), so seeding has to go through `/item replace entity`
	// instead. That command uses its own slot vocabulary (hotbar.0-8 for the
	// hotbar, inventory.0-26 for the rest of the main inventory) rather than
	// the raw Inventory NBT Slot index TransferManager indexes with (see the
	// file doc comment above), so the raw index is translated here.
	rawSlot, err := strconv.Atoi(sourceSlot)
	require.NoError(t, err, "parse sourceSlot")
	var slotArg string
	if rawSlot < 9 {
		slotArg = fmt.Sprintf("hotbar.%d", rawSlot)
	} else {
		slotArg = fmt.Sprintf("inventory.%d", rawSlot-9)
	}

	seedResp, err := instA.RCON.Exec(ctx, fmt.Sprintf(
		"item replace entity %s %s with %s %d", botName, slotArg, itemID, itemCount))
	require.NoError(t, err, "seed source inventory")
	require.NotContains(t, seedResp, "Unable to modify", "seed command rejected: %s", seedResp)
	time.Sleep(500 * time.Millisecond) // give the server a moment to process

	before, err := GetInventoryItems(ctx, instA.RCON, botName)
	require.NoError(t, err, "get serverA inventory before transfer")
	require.Len(t, before, 1, "expected exactly the seeded stack before transfer")
	require.Equal(t, itemID, before[0].ID)
	require.Equal(t, itemCount, before[0].Count)

	// --- The transfer itself, via the direct API trigger surface (Phase
	// 3) - the same Courier.StartTransfer a chat command or RL/LLM action
	// would call ---
	outcome, err := c.StartTransfer(ctx, serverIDA, serverIDB, sourceSlot)
	require.NoError(t, err, "StartTransfer")
	require.NotEmpty(t, outcome.JTI, "transfer outcome should carry the passport's jti")
	t.Logf("transfer complete: jti=%s", outcome.JTI)

	// --- Verify: gone from serverA, present (as a full stack) on serverB.
	// insertOrDrop (TransferManager.java) uses Inventory.add, which doesn't
	// guarantee landing back in the same slot number, so search by
	// id+count rather than a specific slot ---
	afterA, err := GetInventoryItems(ctx, instA.RCON, botName)
	require.NoError(t, err, "get serverA inventory after transfer")
	for _, item := range afterA {
		require.NotEqual(t, itemID, item.ID, "item should have been removed from serverA, found in slot %d", item.Slot)
	}

	afterB, err := GetInventoryItems(ctx, instB.RCON, botName)
	require.NoError(t, err, "get serverB inventory after transfer")
	found := false
	for _, item := range afterB {
		if item.ID == itemID && item.Count == itemCount {
			found = true
			break
		}
	}
	require.True(t, found, "expected a %d-count stack of %s on serverB, got %+v", itemCount, itemID, afterB)
}
