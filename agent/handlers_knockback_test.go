package agent

import (
	"bytes"
	"context"
	"math"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// velocityCaptureMoveExec captures SetVelocity calls for knockback testing.
// startVelocity is a settable baseline GetVelocity returns, so tests can
// verify additive knockback (e.g. explosions) sums correctly onto existing
// motion rather than replacing it.
type velocityCaptureMoveExec struct {
	fakeMoveExec
	velocityCalls [][3]float64
	startVelocity [3]float64
}

func (v *velocityCaptureMoveExec) SetVelocity(x, y, z float64) error {
	v.velocityCalls = append(v.velocityCalls, [3]float64{x, y, z})
	return nil
}

func (v *velocityCaptureMoveExec) GetVelocity() (float64, float64, float64) {
	return v.startVelocity[0], v.startVelocity[1], v.startVelocity[2]
}

// buildDamageEventPacket constructs a raw DamageEvent packet.
// sourceCauseID and sourceDirectID are sent as ID+1 in the protocol (0 = absent).
func buildDamageEventPacket(packetID int32, entityID, sourceTypeID, sourceCauseID, sourceDirectID int32, sourcePos *[3]float64) pk.Packet {
	var buf bytes.Buffer
	appendField(&buf, pk.VarInt(entityID))
	appendField(&buf, pk.VarInt(sourceTypeID))
	appendField(&buf, pk.VarInt(sourceCauseID+1))  // protocol: entity ID + 1
	appendField(&buf, pk.VarInt(sourceDirectID+1)) // protocol: entity ID + 1

	if sourcePos != nil {
		appendField(&buf, pk.Boolean(true)) // has position
		appendField(&buf, pk.Double(sourcePos[0]))
		appendField(&buf, pk.Double(sourcePos[1]))
		appendField(&buf, pk.Double(sourcePos[2]))
	} else {
		appendField(&buf, pk.Boolean(false)) // no position
	}

	return pk.Packet{ID: packetID, Data: buf.Bytes()}
}

func TestOnDamageEvent_KnockbackFromTrackedEntity(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	// Set agent position at origin, entity ID = 42
	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)

	// Place attacker at (5, 0, 0) — east of agent
	testAgent.entities = map[int32]*trackedEntity{
		99: {EntityID: 99, X: 5, Y: 0, Z: 0},
	}

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	damagePacket := buildDamageEventPacket(packetID, 42, 1, 99, 99, nil)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	// Should have applied knockback
	require.Len(t, velCapture.velocityCalls, 1, "expected one SetVelocity call")

	vel := velCapture.velocityCalls[0]

	// Agent at (0,0,0), attacker at (5,0,0) → knockback should push agent in -X direction
	assert.InDelta(t, -physics.KnockbackHorizontalStrength, vel[0], 0.01, "knockback X should be negative (away from attacker)")
	assert.InDelta(t, physics.KnockbackVerticalStrength, vel[1], 0.01, "knockback Y should be upward")
	assert.InDelta(t, 0, vel[2], 0.01, "knockback Z should be ~0 (attacker directly east)")
}

func TestOnDamageEvent_KnockbackFromSourcePosition(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)
	testAgent.entities = map[int32]*trackedEntity{} // no tracked entities

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	// Source position at (0, 0, -10) — south of agent
	sourcePos := [3]float64{0, 0, -10}
	damagePacket := buildDamageEventPacket(packetID, 42, 1, -1, -1, &sourcePos)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	require.Len(t, velCapture.velocityCalls, 1, "expected one SetVelocity call")
	vel := velCapture.velocityCalls[0]

	// Agent at (0,0,0), source at (0,0,-10) → knockback should push agent in +Z direction
	assert.InDelta(t, 0, vel[0], 0.01, "knockback X should be ~0")
	assert.InDelta(t, physics.KnockbackVerticalStrength, vel[1], 0.01, "knockback Y should be upward")
	assert.InDelta(t, physics.KnockbackHorizontalStrength, vel[2], 0.01, "knockback Z should be positive (away from source)")
}

func TestOnDamageEvent_DiagonalKnockback(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)

	// Attacker at (-3, 0, -3) — southwest of agent
	testAgent.entities = map[int32]*trackedEntity{
		50: {EntityID: 50, X: -3, Y: 0, Z: -3},
	}

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	damagePacket := buildDamageEventPacket(packetID, 42, 1, 50, 50, nil)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	require.Len(t, velCapture.velocityCalls, 1)
	vel := velCapture.velocityCalls[0]

	// Should be pushed in +X,+Z direction (northeast), magnitude = KnockbackHorizontalStrength
	assert.Greater(t, vel[0], 0.0, "knockback X should be positive")
	assert.Greater(t, vel[2], 0.0, "knockback Z should be positive")

	// Horizontal magnitude should match knockback strength
	horizontalMag := math.Sqrt(vel[0]*vel[0] + vel[2]*vel[2])
	assert.InDelta(t, physics.KnockbackHorizontalStrength, horizontalMag, 0.01, "horizontal knockback magnitude")
}

func TestOnDamageEvent_IgnoresOtherEntities(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	// Damage event for entity 99, NOT the agent (42)
	damagePacket := buildDamageEventPacket(packetID, 99, 1, 50, 50, nil)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	assert.Empty(t, velCapture.velocityCalls, "should not apply knockback for other entities")
}

func TestOnDamageEvent_NoPositionAvailable(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)
	testAgent.entities = map[int32]*trackedEntity{} // no tracked entities

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	// No source entities, no source position
	damagePacket := buildDamageEventPacket(packetID, 42, 1, -1, -1, nil)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	assert.Empty(t, velCapture.velocityCalls, "should not apply knockback without attacker position")
}

func TestOnDamageEvent_FallbackToCauseEntity(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.8", Address: "*********:25565"})
	require.NoError(t, err)

	testAgent := agentInt.(*agent)
	err = testAgent.Init(context.Background())
	require.NoError(t, err)

	testAgent.UpdatePosition(models.V3{}, 0, 0)
	testAgent.setEntityID(42)

	// sourceDirectID (77) is not tracked, but sourceCauseID (88) is
	testAgent.entities = map[int32]*trackedEntity{
		88: {EntityID: 88, X: 0, Y: 0, Z: 10}, // cause entity north of agent
	}

	velCapture := &velocityCaptureMoveExec{}
	testAgent.SetMovementExecutor(velCapture)

	packetID := int32(testAgent.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"))
	damagePacket := buildDamageEventPacket(packetID, 42, 1, 88, 77, nil)

	err = testAgent.onDamageEvent(damagePacket)
	require.NoError(t, err)

	require.Len(t, velCapture.velocityCalls, 1, "expected knockback from cause entity")
	vel := velCapture.velocityCalls[0]

	// Agent at (0,0,0), cause at (0,0,10) → knockback in -Z direction
	assert.InDelta(t, 0, vel[0], 0.01)
	assert.InDelta(t, -physics.KnockbackHorizontalStrength, vel[2], 0.01, "knockback Z should be negative (away from cause at +Z)")
}

// TestApplyExplosionKnockback_AddsToExistingVelocity verifies that
// explosion knockback is added onto the executor's current velocity rather than
// replacing it — mirroring vanilla's Entity.addVelocityInternal, unlike
// onDamageEvent's SetVelocity-style knockback above. Tested directly
// against applyExplosionKnockback rather than through a real
// ClientboundExplosion packet: that packet's trailing Particle/
// ItemSoundHolder fields make a hand-built wire packet impractical, and
// per-version ParseExplosion parsing already has its own coverage in each
// handler_versions/*/world_test.go.
func TestApplyExplosionKnockback_AddsToExistingVelocity(t *testing.T) {
	velCapture := &velocityCaptureMoveExec{startVelocity: [3]float64{1.0, 0.2, -0.5}}

	err := applyExplosionKnockback(velCapture, 0.4, 0.6, -1.0)
	require.NoError(t, err)

	require.Len(t, velCapture.velocityCalls, 1)
	got := velCapture.velocityCalls[0]
	assert.InDelta(t, 1.4, got[0], 1e-9, "X should be existing velocity plus knockback delta, not just the delta")
	assert.InDelta(t, 0.8, got[1], 1e-9, "Y should be existing velocity plus knockback delta")
	assert.InDelta(t, -1.5, got[2], 1e-9, "Z should be existing velocity plus knockback delta")
}

func TestApplyExplosionKnockback_NilExecutorIsNoop(t *testing.T) {
	err := applyExplosionKnockback(nil, 1, 1, 1)
	assert.NoError(t, err, "a nil executor should be a harmless no-op, not an error")
}
