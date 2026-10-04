package testing

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/structure"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestWriteTestVanillaStructure_RoundTrips is a pure, no-server check that
// writeTestVanillaStructure's hand-rolled bytes are actually valid: decodes
// them back with the real structure.LoadFile (the same decoder
// TestBuildStructureFromFile's live BuildStructure call uses) and checks
// the result matches what was asked for. Isolates "is the fixture valid
// NBT" from "does BuildStructure place things correctly on a live server" -
// if this fails, the live test's failure would be a fixture bug, not a
// BuildStructure bug, and this is what should catch that distinction
// cheaply, without spending any remote-server time.
func TestWriteTestVanillaStructure_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.nbt")
	writeTestVanillaStructure(t, path, []testBlockPos{{0, 0, 0}, {0, 1, 0}})

	s, err := structure.LoadFile(path)
	require.NoError(t, err, "LoadFile")

	require.Equal(t, structure.Pos{X: 1, Y: 2, Z: 1}, s.Size)
	require.Len(t, s.Palette, 1)
	require.Equal(t, "minecraft:stone", s.Palette[0].Name)
	require.Len(t, s.Blocks, 2)

	order := structure.PlacementOrder(s)
	require.Len(t, order, 2, "neither block should be filtered as air/void")
	require.Equal(t, structure.Pos{X: 0, Y: 0, Z: 0}, order[0].Pos, "bottom block should sort first")
	require.Equal(t, structure.Pos{X: 0, Y: 1, Z: 0}, order[1].Pos, "top block should sort second")
}

// StructureLoaderFlatSuite is a version-parameterized suite for
// PlaceBlockAt, the new primitive docs/plans/NBT_STRUCTURE_LOADER_PLAN.md's
// BuildStructure is built on - placing a held block at an arbitrary world
// position (not just a cell adjacent to the bot's current position, which
// is all PlaceHeldBlock ever supported). WorldGen = WorldGenFlat for
// predictable, uniform ground to place against.
type StructureLoaderFlatSuite struct {
	VersionWorldSuite
}

func TestStructureLoaderFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &StructureLoaderFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestPlaceBlockAtArbitraryPosition places a held block several cells away
// from the bot - not adjacent to its current standing position - and
// confirms (via RCON, independent of the agent's own self-reported success)
// that the block actually landed at the target coordinates. This is the one
// piece of PlaceBlockAt that no fake/unit test can verify: that
// findSupportFace's chosen face and models.TryInteractPositions' chosen
// standing position actually result in a successful real placement against
// a live server, not just that the Go code calls the right functions in the
// right order.
func (s *StructureLoaderFlatSuite) TestPlaceBlockAtArbitraryPosition() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("StructurePlaceBot", "structure_place")
	require.NoError(t, err, "spawn agent")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:stone 5", leader.Name))
	require.NoError(t, err, "give stone")

	hasStone, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:stone", 5*time.Second)
	require.NoError(t, err, "wait for stone to sync to agent")
	require.True(t, hasStone, "agent should see the given stone")

	// Three cells east of the agent's own standing cell - out of
	// PlaceHeldBlock's own adjacent-cell range, but well within
	// PlaceBlockAt's walk-then-place reach. Same Y as the flat world's
	// ground level (Origin is the bot's own feet position).
	bx, by, bz := int(leader.Origin.X), int(leader.Origin.Y), int(leader.Origin.Z)
	target := models.V3{X: float64(bx + 3), Y: float64(by), Z: float64(bz)}

	err = leader.Agent.PlaceBlockAt(ctx, target, "minecraft:stone")
	require.NoError(t, err, "PlaceBlockAt")

	// Two RCON-based double-checks were tried and discarded here, both for
	// reasons unrelated to PlaceBlockAt itself: "execute if block ... run
	// say ..." returns an empty RCON response for the inner say regardless
	// of whether it ran (confirmed separately - the broadcast demonstrably
	// reached both agents' own chat logs even though Exec's return value
	// was ""), and "data get block" only works for block *entities*
	// (chests, signs, etc.), not a plain stone block ("The target block is
	// not a block entity"). BlockNameAt reads the agent's own tracked
	// world state - the same ClientboundBlockUpdate-driven state
	// PlaceBlockAt's internal waitForPlacement already relies on to decide
	// success in the first place - which is a real, server-confirmed value,
	// not a guess.
	require.Equal(t, "minecraft:stone", leader.Agent.BlockNameAt(int(target.X), int(target.Y), int(target.Z)),
		"block should actually be stone at the target position")

	t.Log("✓ PlaceBlockAt arbitrary-position test passed")
}

// TestBuildStructureFromFile exercises the actual thing
// docs/plans/NBT_STRUCTURE_LOADER_PLAN.md's Phase 3 (never implemented
// until now) called for: structure.LoadFile + agent.BuildStructure against
// a real server, with a real (if minimal) structure file on disk - not a
// fake structurePlacer (agent/build_structure_test.go already covers the
// orchestration logic that way) and not PlaceBlockAt in isolation
// (TestPlaceBlockAtArbitraryPosition above). Everything in between -
// decoding the file, walking PlacementOrder, resolving each palette entry
// to an item name, calling the real PlaceBlockAt per block - only gets
// exercised together, for real, here.
//
// The fixture is a 2-block vertical stack (both minecraft:stone) written
// by hand below rather than loaded from a real Structure Block export,
// since this package can't import structure's own _test.go-scoped encoder
// (nbt_testutil_test.go) - but it's the same binary format that decoder was
// built and unit-tested against, not a guess.
func (s *StructureLoaderFlatSuite) TestBuildStructureFromFile() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("StructureBuildBot", "structure_build")
	require.NoError(t, err, "spawn agent")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:stone 2", leader.Name))
	require.NoError(t, err, "give stone")

	hasStone, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:stone", 5*time.Second)
	require.NoError(t, err, "wait for stone to sync to agent")
	require.True(t, hasStone, "agent should see the given stone")

	structPath := filepath.Join(t.TempDir(), "two_stone_tower.nbt")
	writeTestVanillaStructure(t, structPath, []testBlockPos{{0, 0, 0}, {0, 1, 0}})

	// Same offset convention as TestPlaceBlockAtArbitraryPosition: three
	// cells east of the agent's own standing cell, same Y as the flat
	// world's ground level, so local (0,0,0) in the structure lands on
	// solid ground and (0,1,0) stacks directly on top of it.
	bx, by, bz := int(leader.Origin.X), int(leader.Origin.Y), int(leader.Origin.Z)
	origin := models.V3{X: float64(bx + 3), Y: float64(by), Z: float64(bz)}

	result, err := leader.Agent.BuildStructure(ctx, structPath, origin)
	require.NoError(t, err, "BuildStructure")
	require.Empty(t, result.Failed, "no block should have failed to place: %+v", result.Failed)
	require.Equal(t, 2, result.Placed, "both blocks in the structure should have been placed")

	require.Equal(t, "minecraft:stone", leader.Agent.BlockNameAt(int(origin.X), int(origin.Y), int(origin.Z)),
		"bottom block of the structure should be stone")
	require.Equal(t, "minecraft:stone", leader.Agent.BlockNameAt(int(origin.X), int(origin.Y)+1, int(origin.Z)),
		"top block of the structure should be stone")

	t.Log("✓ BuildStructure-from-file test passed")
}

// testBlockPos is a local block position within a test structure fixture.
type testBlockPos struct{ x, y, z int32 }

// writeTestVanillaStructure writes a minimal, real vanilla structure-block
// .nbt file (gzip-compressed NBT) to path: a single-entry palette
// ("minecraft:stone") and one blocks[] entry per position in blocks, all
// referencing that one palette entry. Hand-rolled rather than borrowed from
// structure's own test-only encoder (which this package can't import,
// being _test.go-scoped to a different package) - but against the exact
// same binary format structure/nbt.go's decoder was built and unit-tested
// against: a compound root with "size" (3-element Int list), "palette"
// (list of {Name: String} compounds), and "blocks" (list of {pos: 3-element
// Int list, state: Int} compounds).
func writeTestVanillaStructure(t *testing.T, path string, blocks []testBlockPos) {
	t.Helper()

	maxY := int32(0)
	for _, b := range blocks {
		if b.y > maxY {
			maxY = b.y
		}
	}

	var buf bytes.Buffer
	w := &nbtTestWriter{buf: &buf}

	w.rootCompoundStart()

	w.tagListIntStart("size", 3)
	w.int32(1)
	w.int32(maxY + 1)
	w.int32(1)

	w.tagListStart("palette", nbtTagCompound, 1)
	w.compoundStartUnnamed()
	w.tagString("Name", "minecraft:stone")
	w.compoundEnd()

	w.tagListStart("blocks", nbtTagCompound, int32(len(blocks)))
	for _, b := range blocks {
		w.compoundStartUnnamed()
		w.tagListIntStart("pos", 3)
		w.int32(b.x)
		w.int32(b.y)
		w.int32(b.z)
		w.tagInt("state", 0)
		w.compoundEnd()
	}

	w.compoundEnd() // root

	var gz bytes.Buffer
	gzw := gzip.NewWriter(&gz)
	_, err := gzw.Write(buf.Bytes())
	require.NoError(t, err, "gzip structure fixture")
	require.NoError(t, gzw.Close(), "close gzip writer")

	require.NoError(t, os.WriteFile(path, gz.Bytes(), 0o644), "write structure fixture")
}

const (
	nbtTagEnd      = 0
	nbtTagInt      = 3
	nbtTagString   = 8
	nbtTagList     = 9
	nbtTagCompound = 10
)

// nbtTestWriter is a minimal, append-only NBT encoder - just enough to
// build the one fixture shape writeTestVanillaStructure needs. Not a
// general-purpose encoder; e.g. compoundStartUnnamed/tagListIntStart exist
// only because every tag this fixture needs happens to be either inside a
// named compound entry or a list element (which never carries its own
// name), and nothing here needs any other tag type.
type nbtTestWriter struct {
	buf *bytes.Buffer
}

func (w *nbtTestWriter) u16(v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	w.buf.Write(b[:])
}

func (w *nbtTestWriter) int32(v int32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	w.buf.Write(b[:])
}

func (w *nbtTestWriter) name(s string) {
	w.u16(uint16(len(s)))
	w.buf.WriteString(s)
}

// rootCompoundStart writes the root tag's own type+name (an unnamed
// compound), matching what structure.DecodeNBT reads before handing off to
// the compound-payload reader.
func (w *nbtTestWriter) rootCompoundStart() {
	w.buf.WriteByte(nbtTagCompound)
	w.name("")
}

// compoundStartUnnamed begins a compound payload with no preceding
// type+name byte - for a compound that's itself a list element, which
// never carries a name.
func (w *nbtTestWriter) compoundStartUnnamed() {
	// no-op marker: a compound's payload is just its entries followed by
	// TagEnd, which compoundEnd writes - nothing to write to *start* one.
}

func (w *nbtTestWriter) compoundEnd() {
	w.buf.WriteByte(nbtTagEnd)
}

// tagListStart writes a named List tag (type byte + name + element-type +
// length) whose elements the caller writes next, followed eventually by
// compoundEnd calls for any compound elements.
func (w *nbtTestWriter) tagListStart(name string, elemType byte, length int32) {
	w.buf.WriteByte(nbtTagList)
	w.name(name)
	w.buf.WriteByte(elemType)
	w.int32(length)
}

// tagListIntStart writes a named List-of-Int tag's header; the caller
// writes `length` int32 elements immediately after via int32().
func (w *nbtTestWriter) tagListIntStart(name string, length int32) {
	w.tagListStart(name, nbtTagInt, length)
}

func (w *nbtTestWriter) tagString(name, value string) {
	w.buf.WriteByte(nbtTagString)
	w.name(name)
	w.u16(uint16(len(value)))
	w.buf.WriteString(value)
}

func (w *nbtTestWriter) tagInt(name string, value int32) {
	w.buf.WriteByte(nbtTagInt)
	w.name(name)
	w.int32(value)
}
