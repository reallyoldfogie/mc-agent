# NBT Data for Summoning a Tamed & Saddled Horse — Cross-Version Analysis

## NBT Tags Required

The horse inherits NBT from the class hierarchy: `Entity → LivingEntity → MobEntity → AnimalEntity → AbstractHorseEntity → HorseEntity`. For a **tamed and saddled horse**, the relevant tags are:

### From AbstractHorseEntity (all versions)

- `Tame: 1b` — marks horse as tamed
- `Owner: <UUID>` — owner's UUID (optional but needed for a properly tamed horse)
- `Temper: 100` — max temper (0–100; 100 = fully tamed)
- `Bred: 0b` — whether it was bred
- `EatingHaystack: 0b` — whether it's eating grass

### From HorseEntity (all versions)

- `Variant: <int>` — encodes color (low byte) and marking (high byte) as `color | (marking << 8)`

### Saddle Handling — Where Versions Diverge

This is the only tag that changed location across versions.

## Version Groups

### Group 1: 1.21.1 (oldest format)

The saddle is stored in AbstractHorseEntity as a `SaddleItem` compound tag within the horse's own inventory:

```
SaddleItem: {id: "minecraft:saddle", count: 1}
```

- Serialized via `ItemStack.encode()`
- Armor stored in MobEntity via `ArmorItems` / `HandItems` NbtLists
- Body armor stored as `BodyArmorItem` compound

### Group 2: 1.21.2 — 1.21.4 (minor API change)

Same `SaddleItem` compound tag, but serialization method renamed:

- `ItemStack.encode()` → `ItemStack.toNbt()`
- Otherwise **identical NBT structure** to 1.21.1

### Group 3: 1.21.5 (major restructuring)

**Breaking change:** `SaddleItem` tag is **removed** from AbstractHorseEntity entirely.

- Saddle moves to `EquipmentSlot.SADDLE` (new slot)
- `hasSaddleEquipped()` checks `MobEntity.hasStackEquipped(EquipmentSlot.SADDLE)`
- Equipment now serialized in LivingEntity as a unified `equipment` compound (via `EntityEquipment.CODEC`)
- `isSaddled()` flag replaced with `hasSaddleEquipped()`
- Owner handling: `UUID` → `LazyEntityReference` (still uses `Owner` key)
- NBT getter API adds default params: `nbt.getBoolean("Tame")` → `nbt.getBoolean("Tame", false)`
- Methods still named `writeCustomDataToNbt` / `readCustomDataFromNbt`

### Group 4: 1.21.6 — 1.21.11 (API rename)

Same structure as 1.21.5 but:

- Methods renamed: `writeCustomDataToNbt(NbtCompound)` → `writeCustomData(WriteView)`
- Methods renamed: `readCustomDataFromNbt(NbtCompound)` → `readCustomData(ReadView)`
- Owner: `LazyEntityReference.writeNbt()` → `LazyEntityReference.writeData()`
- NBT access: `NbtCompound` → `WriteView`/`ReadView` abstraction
- **Underlying NBT tag names are identical** to 1.21.5

## Summon Commands

### 1.21.1 — 1.21.4

```
/summon minecraft:horse ~ ~ ~ {Tame:1b,SaddleItem:{id:"minecraft:saddle",count:1},Variant:0}
```

### 1.21.5 — 1.21.11

The saddle is now part of the `equipment` compound:

```
/summon minecraft:horse ~ ~ ~ {Tame:1b,equipment:{saddle:{id:"minecraft:saddle",count:1}},Variant:0}
```

## Summary of Differences

- **NBT tag names** (`Tame`, `Bred`, `Temper`, `Owner`, `EatingHaystack`, `Variant`) are **identical across all 11 versions**
- The **saddle** is the only tag that changed location: `SaddleItem` (compound at top level) → `equipment.saddle` (nested in unified equipment structure) starting in **1.21.5**
- The internal Java API changed significantly (method names, parameter types, `NbtCompound` → `WriteView`/`ReadView`), but the serialized NBT format only changed at the saddle level

## Source Files Referenced

- `net/minecraft/entity/passive/AbstractHorseEntity.java` — tame, owner, temper, saddle (≤1.21.4)
- `net/minecraft/entity/passive/HorseEntity.java` — variant (color + marking)
- `net/minecraft/entity/mob/MobEntity.java` — equipment drop chances, saddle slot (≥1.21.5)
- `net/minecraft/entity/LivingEntity.java` — unified equipment serialization (≥1.21.5)
