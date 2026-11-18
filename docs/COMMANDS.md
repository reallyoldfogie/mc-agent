# Chat Command Reference

Commands are defined in `actions/commands.go` as `models.Action[models.CommandAgent]`
implementations (`Name`/`Usage`/`Execute`, `Execute` returning `(models.Completion, error)` —
see `models/completion.go`), registered in `actions/registry.go`'s `RegisterDefaults`:

- `help` - Show available commands
- `pos` - Display current position
- `say <text>` - Send chat message
- `testMove` - Test movement (+1 block on X)
- `moveTo <x> <y> <z>` - Pathfind to coordinates
- `lineTo <x> <y> <z>` - Straight-line movement (flat worlds only)
- `moveForward <distance>` - Move forward by distance
- `moveUp <distance>` - Move up by distance
- `moveUpAndSneak <distance>` - Move up and enable sneaking
- `moveToAndSneak <x> <y> <z>` - Pathfind and sneak at destination
- `lineToAndSneak <x> <y> <z>` - Straight-line move and sneak at destination
- `stopSneak` - Stop sneaking
- `findPath <x> <y> <z>` - Calculate path without moving
- `testPath` - Test pathfinding system
- `follow [<player>]` - Follow a player
- `stopFollow` - Stop following
- `followStatus` - Show follow status
- `startTracking` - Continuously look at nearest player
- `stopTracking` - Stop tracking
- `fireBow` - Fire equipped bow (basic)
- `fireBowAt <x> <y> <z> | nearest | <player>` - Fire bow at a target with ballistic calculation
- `mount <entityID | entityType>` - Mount a nearby vehicle/mob by entity ID or type
- `dismount` - Dismount the current vehicle
- `vehicleJump [power]` - Jump while mounted (e.g. horse)
- `mine <x> <y> <z> | <blockName>` - Mine (break) a block at exact coordinates, or the nearest visible block matching a name
- `lookAround [radius]` - List every visible entity/item within radius blocks (default 16) and their positions
- `pickUpNearbyItem [maxDistance]` - Find the nearest visible dropped item within maxDistance blocks (default 8) and walk to it (vanilla auto-collects on arrival)
- `craft <itemName>` - Craft an item, using the player's 2x2 inventory grid when the recipe fits it, or a nearby crafting table's 3x3 grid otherwise (`crafting_special_*` recipes like armor dye/repair/book cloning are out of scope - no declarative ingredient list to match)
- `place <itemName>` - Place a block from inventory on a standable cell beside the bot, confirming it appeared (was missing from this list; added alongside `buildStructure` below)
- `buildStructure <path> <x> <y> <z>` - Load a vanilla Structure Block `.nbt` template and build it in-world via real block placement, origin at `<x> <y> <z>` (see `docs/STRUCTURE_LOADER.md`)
- `materialList <structurePath> [exportPath]` - Report (and, with `exportPath`, persist to JSON) the full list of materials a structure file needs to build - the "what needs to be gathered" shopping list, comparable today against the bot's inventory and, later, against long-term storage (chests/shulker boxes/barrels)
- `equip <item>` - Find an item in inventory and wear/wield it
- `useItem [offhand]` - Use/right-click the currently held item
- `flyTo <x> <y> <z>` - Elytra flight to coordinates (takeoff, cruise, land)
- `fly` / `land` - Toggle creative/spectator-style flying (distinct from `flyTo`'s elytra glide)
- `followCam <playerName> [maxDistance]` - Spectator-mode camera auto-follow via RCON
- `stopFollowCam` - Stop camera auto-follow
- `planStatus` - Show plan execution status
- `planStop` - Stop current plan

See **[ADDING_NEW_ACTION.md](ADDING_NEW_ACTION.md)** for the checklist to add a new one,
including what's additionally needed for the RL policy (`rlenv`,
`github.com/reallyoldfogie/cRL-go`) to be able to use it.

Commands are triggered via chat with the `>>>ROF_bot<<<` prefix:
```
>>>ROF_bot<<< moveTo 100 64 100
>>>ROF_bot<<< follow PlayerName
```
