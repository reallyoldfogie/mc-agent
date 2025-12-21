# Skin Fallback Simplification

**Date:** December 5, 2025
**Issue:** Generated default skins using hardcoded texture URLs were creating empty props arrays in cached files
**Solution:** Simplified fallback chain to rely only on extracted skins

## Problem

The old skin system had a 5-step fallback chain:

1. Cached player skin
2. Network fetch from Mojang
3. Extracted skins from client jar
4. Cached defaults (from `skins/defaults/`)
5. Generated defaults (using hardcoded texture URLs)

**Issue:** Steps 4 and 5 were problematic:
- The hardcoded URLs in `generateDefaultSkin()` don't work without proper authentication
- This created empty properties in cached JSON files
- The cached defaults were unusable

## Solution

Simplified to a 3-step fallback chain:

1. **Cached player skin** - Player-specific cached skin
2. **Network fetch** - Mojang session servers (if enabled)
3. **Extracted skins** - High-quality skins from client jar (final fallback)

If no skins are available (client jar not downloaded), return `nil`.

## Changes Made

### Code Changes

**`agent/skins.go`:**
- Removed `loadAnyCachedDefault()` method
- Removed `generateDefaultSkin()` method
- Removed `encodeTexturesValue()` helper function
- Removed commented-out `persistGeneratedDefault()` method
- Updated `Get()` to return `nil` when no skins available
- Kept `rng` field (still used by `GetRandomExtractedSkin()`)

### Documentation Updates

**`docs/SKINS.md`:**
- Updated "Skin Resolution Priority" section to show 3-step chain
- Removed `defaults/` directory from directory structure
- Added note about calling `Initialize()` first

**`docs/SKIN_IMPLEMENTATION_SUMMARY.md`:**
- Updated "Skin Resolution Order" section
- Removed `defaults/` from file structure diagram

## Benefits

1. **Simpler code** - Removed ~60 lines of problematic code
2. **No broken caches** - No more empty JSON files in `skins/defaults/`
3. **Better defaults** - Extracted skins are higher quality than generated URLs
4. **Clearer expectations** - `nil` return makes it obvious when initialization is needed
5. **More reliable** - No dependency on external texture URLs that may not work

## Migration

For existing code:

**Before:**
```go
fetcher := agent.NewSkinFetcher(...)
props := fetcher.Get(uuid, name)
// Always got something (even if broken)
```

**After:**
```go
skinMgr := agent.NewSkinManager(...)
skinMgr.Initialize("1.21.5") // Download skins first!
props := skinMgr.GetSkinForPlayer(uuid, name)
// Returns nil if skins not available
if len(props) == 0 {
    log.Println("No skins available - call Initialize() first")
}
```

## Backward Compatibility

This is a **breaking change** for code that relied on generated defaults:

- Old code using `SkinFetcher` directly may now get `nil` instead of broken skins
- Solution: Use `SkinManager` and call `Initialize()` first
- Or ensure extracted skins are downloaded before using `SkinFetcher`

## Testing

All existing tests still pass:
- ✅ `TestSkinManager_Initialize`
- ✅ `TestSkinManager_ListAvailableSkins`
- ✅ `TestSkinManager_GetRandomSkin`
- ✅ `TestSkinManager_GetSkinByName`
- ✅ `TestSkinManager_CachedDownload`

## Cleanup

No manual cleanup needed:
- Old `skins/defaults/` files (if any) can be safely deleted
- They are no longer referenced by the code
- The directory is not created by new code

## Recommendation

**Best Practice:** Always use `SkinManager` instead of `SkinFetcher` directly:

```go
// Good ✅
skinMgr := agent.NewSkinManager(agent.SkinManagerConfig{
    MinecraftVersion: "1.21.5",
})
skinMgr.Initialize("1.21.5")
props := skinMgr.GetSkinForPlayer(uuid, name)

// Avoid ⚠️
fetcher := agent.NewSkinFetcher(...)
props := fetcher.Get(uuid, name) // May return nil if skins not extracted
```

The `SkinManager` handles initialization automatically and ensures skins are available.
