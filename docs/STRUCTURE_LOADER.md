# Structure Loader

## Overview

A Minecraft **Structure Block** lets a player save a region of the world — walls, a house,
anything — to a `.nbt` file, and load it back later. This feature lets the agent read one of
those files and build it in the world itself, via real client interaction (right-clicking blocks
into place, the same way `place <itemName>` already does) — never RCON `/setblock`/`/clone`. RCON
in this codebase is a test-verification and test-world-setup tool only; it's never a substitute
for the agent's own actions, and this feature follows that rule.

Only vanilla Structure Block `.nbt` files are supported today — not `.litematic` (Litematica) or
`.schem`/`.schematic` (Sponge Schematic). The package is structured so adding either later doesn't
require touching the format-agnostic parts; see [Format support](#format-support-and-adding-a-new-one)
below.

## The four layers

```
.nbt file on disk
      │
      ▼
┌──────────────────────┐
│ structure/ package    │  Decode the file into a plain, format-agnostic
│ (parsing)              │  Go value. No agent, no world, no network.
└──────────────────────┘
      │
      ├─────────────────────────────┐
      ▼                              ▼
┌──────────────────────┐   ┌──────────────────────┐
│ PlaceBlockAt          │   │ Material lists        │
│ (agent/place_block.go)│   │ (structure/materials. │
│ Place ONE held block  │   │ go) - what the whole  │
│ at an arbitrary world │   │ structure needs,      │
│ position.              │   │ exportable/comparable.│
└──────────────────────┘   └──────────────────────┘
      │
      ▼
┌──────────────────────┐
│ BuildStructure         │  Loop over every block bottom-up, calling
│ (agent/build_          │  PlaceBlockAt for each. Checks materials
│  structure.go)         │  up front; keeps going past individual
│                        │  placement failures.
└──────────────────────┘
```

## 1. Parsing (`structure/` package)

A `.nbt` file is gzip-compressed **NBT** (Named Binary Tag) data — Minecraft's own generic
serialization format, the same thing world saves and player data use. It's a tree of typed tags:
compounds (like a JSON object), lists, strings, integers, etc.

- **`nbt.go`** — a from-scratch NBT decoder (deliberately not using any third-party NBT library;
  the format is simple enough, and this avoids a dependency on an unmaintained external package).
  Decodes the raw bytes into a generic `Tag` tree. Handles gzip, zlib, or raw (uncompressed) input.
- **`structure.go`** — the clean, format-agnostic result type: `Structure{Size, Palette, Blocks}`.
  `Palette` is the list of distinct block types used (name + block-state properties); `Blocks` is
  a list of `{local position, which palette entry}` pairs.
- **`vanilla.go`** — knows the specific shape of a vanilla structure-block file: a root compound
  with `size`/`palette`/`blocks` keys. Accepts **both** `Name`/`Properties` (capitalized) and
  `id`/`properties` (lowercase) for each palette entry's keys — a real file saved by a current
  Minecraft client uses the lowercase form; only hand-built test fixtures used the capitalized one
  exclusively until a real file (`testdata/generated/.../simple_hut.nbt`) caught the mismatch.
- **`format.go`** — a small plugin registry `LoadFile` dispatches through, matching by file
  extension first and then by sniffing the decoded root tag's shape. `vanilla.go` is the only
  registered format today. See [Format support](#format-support-and-adding-a-new-one).
- **`order.go`** — `PlacementOrder` turns the raw block list into a build order: strips `air`/
  `structure_void` entries (a structure file is normally a full rectangular box, mostly empty
  space) and sorts everything bottom-up (ascending Y) so nothing is ever asked to place on nothing.
- **`materials.go`** — see [Material lists](#material-lists) below.

## 2. Placing one block anywhere (`PlaceBlockAt`)

`place <itemName>` (via `PlaceHeldBlock`) only ever placed a block **next to the agent's current
position**. A structure can be larger than that, and the agent needs to place at *arbitrary*
coordinates while it's somewhere else entirely.

`PlaceBlockAt(ctx, pos, itemName)`:

1. **Finds something solid to click against.** You can't place a block floating in mid-air —
   placement works by right-clicking an existing solid block's face. `findSupportFace` checks the
   target cell's six neighbors (below first, then the four sides, then above — the order a person
   naturally builds in) and picks the first one that's already solid.
2. **Walks somewhere it can reach that from, then places.** Reuses `models.TryInteractPositions`
   (originally built for walking up to an existing block, e.g. to open a chest) to find a nearby
   standable position with line of sight to the target, moves there, and right-clicks.
3. **Verifies the placement actually took effect**, polling the agent's own world view, before
   returning success — the same verification `PlaceHeldBlock` already does.

## 3. Building the whole thing (`BuildStructure`)

`BuildStructure(ctx, path, origin)`:

1. Loads the file.
2. Checks the agent has every material it'll need, counting across the **whole** structure, before
   placing a single block (see [Material lists](#material-lists)) — reports exactly what's short
   and stops, rather than building half of something and leaving the caller to work out why.
3. Walks the bottom-up block list, and for each one: `origin + that block's local position` →
   `PlaceBlockAt`.
4. Keeps going if an individual block fails to place — a transient hiccup on one block shouldn't
   abandon an otherwise-successful large build. Failures are collected (position, item, reason) and
   reported at the end, not fatal.

### Chat command

```
>>>ROF_bot<<< buildStructure <path> <x> <y> <z>
```

## Material lists

Before (or instead of) attempting a build, you often want to know what it'll take: a flat shopping
list of "item name → how many," independent of positions or palette indices, usable in-memory or
exported to disk.

```go
s, _ := structure.LoadFile("house.nbt")
ml := structure.ComputeMaterialList(s)       // MaterialList{Items: []MaterialEntry, sorted by name}

ml.Total()                                    // total block count across every item type
ml.Count("minecraft:oak_log")                 // how many of one specific item

missing := ml.Missing(agent.InventoryCount)   // shortfall against any "have" source
```

`Missing` takes a plain `func(item string) int` rather than a concrete inventory type, deliberately:
the exact same comparison works against the agent's current held inventory today, and is meant to
work unchanged against a future long-term-memory system that reports what's stored across chests,
shulker boxes, barrels, etc. — only the function passed to `Missing` changes, never `Missing`
itself or its caller.

```go
ml.WriteJSON("house-materials.json")                        // persist
loaded, _ := structure.LoadMaterialListJSON("house-materials.json") // read back later, elsewhere
```

The JSON form is a simple, self-describing document:

```json
{
  "source": "house.nbt",
  "items": [
    { "item": "minecraft:oak_log", "count": 63 },
    { "item": "minecraft:oak_planks", "count": 2 }
  ]
}
```

### Chat command

```
>>>ROF_bot<<< materialList <structurePath> [exportPath]
```

Reports a chat summary (total blocks, distinct item-type count, up to 10 item types listed
directly); if `exportPath` is given, writes the complete list there as JSON regardless of how much
the chat line itself had to truncate.

## Known limitations (v1)

- **Block-state/orientation is ignored.** A structure file records that a stair faces north,
  half-top, etc. — none of that gets applied. The block type lands correctly; its facing is
  whatever vanilla placement produces by default. (The same limitation `place <itemName>` already
  has — not a new gap this feature introduces.)
- **Block-entity contents are dropped.** A chest's inventory, a sign's text — the block itself is
  placed, its contents are not. (The data is captured during decode on `BlockEntry.BlockEntityData`
  but unused by the placement pipeline.)
- **No rotation/mirroring** of the template before placement.
- **No auto-sourcing of missing materials** — `BuildStructure` and `materialList`'s `Missing` both
  report a shortfall; neither will go mine or craft what's short.
- **No resuming** a build that was interrupted partway through.
- **A structure's palette *block* name is assumed to equal the *item* name** needed to place it —
  true for most blocks, false for a few (e.g. `minecraft:wall_torch`'s item is
  `minecraft:torch`) — the same assumption `place <itemName>` already makes.

## Format support and adding a new one

`structure.Format` is a small interface (`Name`, `Sniff`, `Decode`) that `LoadFile` dispatches
through via a registry (`RegisterFormat`, called from each format's own `init()`). Adding support
for `.litematic` or `.schem` means writing one new file implementing that interface — the NBT
decoder (`nbt.go`), the format-agnostic `Structure` type, `PlacementOrder`, and everything in
`agent/`/`actions/` built on `*Structure` need no changes at all.

Two things worth knowing in advance if that work happens:

- **Litematica's block storage is bit-packed** (one packed `long[]` array per region, palette
  index → block state) — unlike vanilla structure files' flat per-block list. The paletted-
  container bit-unpacking already implemented for live network chunk data (`world/chunk.go`,
  `parsePaletteContainer`/`getPackedIndex`) is closer prior art for this than anything in
  `vanilla.go`. Confirm bit-per-entry and long-boundary-spanning behavior against Litematica's
  actual spec before reusing that math directly — it's a different on-disk format from both the
  network protocol and Anvil chunk NBT.
- **Sponge Schematic (`.schem`) versions 1–3 differ from each other** (v2+ uses a compound-keyed
  palette rather than a list, and varint-encoded block data) — pick one version to support first
  rather than all three at once.

## Testing

- **Pure unit tests** (`structure/*_test.go`) cover the NBT decoder, the vanilla-format parser,
  placement ordering, the format registry, and material-list computation/export — all against
  hand-built in-memory or temp-file fixtures, no server involved.
- **A real-file regression test** (`structure/testdata_test.go`) loads an actual structure-block
  file saved by hand from a Minecraft client (`testdata/generated/reallyoldfogie/structure/
  simple_hut.nbt`, a 7x7x7 wooden hut) and checks the decode against its real, known contents. This
  is what caught the `Name`/`id` key mismatch described above — a decoder only ever proven against
  fixtures it was written to produce isn't actually proven against what Minecraft itself emits.
- **`agent/build_structure_test.go`** covers `BuildStructure`'s orchestration logic (material
  check, bottom-up ordering, continuing past an individual failure, progress reporting) against a
  fake placer — fast and deterministic, no live agent or server.
- **Live integration tests** (`testing/structure_loader_test.go`) exercise `PlaceBlockAt` and
  `BuildStructure` against a real server and a real connected agent. A separate local (no-server)
  test in the same file decodes that live test's hand-rolled fixture back through the real decoder
  first, to isolate "is the fixture valid NBT" from "does placement actually work" as two different
  failure modes. When rerunning a live test manually, pass `-count=1` — Go's test result caching
  doesn't know the live server is actually different each run, and will happily replay a stale
  cached result otherwise.
