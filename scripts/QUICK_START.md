# Quick Start: Adding a New Minecraft Version

This is a condensed guide for quickly adding a new Minecraft version using the automation script.

## Prerequisites

1. Ensure `mc-protocol-go` has packet definitions for the target version
2. Find the protocol version number: https://wiki.vg/Protocol_version_numbers
3. Have a source version to copy from (default: latest)

**Note:** Version format can be any numeric format (e.g., `1.21.9` for legacy, `26.1.0` for 2026+ versions)

## Basic Workflow

### 1. Preview (Dry Run)

```bash
# Legacy version format
./scripts/add_version.sh --dry-run 1.21.9 773

# 2026+ version format
./scripts/add_version.sh --dry-run 26.1.0 800
```

This shows what would be created without making changes.

### 2. Run the Script

```bash
# Auto-detect source version (copies from latest)
./scripts/add_version.sh 1.21.9 773

# Or specify source version explicitly
./scripts/add_version.sh 1.21.9 773 1.21.8
```

The script will:
- ✅ Create `versions/v1_21_9/`
- ✅ Copy all `.go` files
- ✅ Update package names and version strings
- ✅ Update protocol version number
- ✅ Add import to `versions/init.go`
- ✅ Run `go fmt`
- ✅ Verify compilation

### 3. Handle Protocol Changes

**Check for protocol differences:**
```bash
# Search for common protocol changes in mc-protocol-go
cd ../mc-protocol-go
git log --oneline --grep="1.21.9" | head -5
```

**Common changes to check:**
- Packet structure changes (field additions/removals)
- Data type changes (int → string, HashedSlot → Option[HashedSlot])
- Bitflag API changes (GetOnGround() → OnGround())
- Enum changes (numeric → string IDs)
- New packet types

**Files to review/update:**
```
versions/v1_21_9/ (or v26_1_0/ for 2026+ versions)
├── movement.go       # Check for PlayerCommand, Position changes
├── entities.go       # Check for EntityAction, spawn packets
├── configuration.go  # Usually stable
├── login.go          # Usually stable
├── containers.go     # Check for Slot/inventory changes
├── chat.go           # Check for ChatMessage structure
└── world.go          # Usually stable
```

### 4. Update Tests

```bash
# Run unit tests (use your actual version package name)
go test ./versions/v1_21_9/...   # Or v26_1_0 for 2026+ versions

# Fix test failures - common issues:
# - Packet ID lookups (use GetClientboundPacketID/GetServerboundPacketID)
# - Bitflag methods (check if "Get" prefix removed)
# - Enum types (check if switched from int to string)
# - Slot structure (check for Option wrapping)
```

### 5. Add to Integration Tests

Edit `testing/navigation_pathfinding_test.go`:

```go
versionTests := []struct {
    name              string
    mcVersion         string
    useVersionHandler bool
}{
    // ... existing tests ...
    {
        name:              "1.21.9",
        mcVersion:         "1.21.9",
        useVersionHandler: true,
    },
}
```

### 6. Update Documentation

Edit `docs/ADDING_NEW_VERSION.md`, add to Quick Reference table:

```markdown
| Version | Protocol | Key Differences |
|---------|----------|----------------|
| 1.21.9  | 773      | <describe changes> |
```

### 7. Test End-to-End

```bash
# Run integration tests (if you have a test server)
go test -v ./testing -run TestPathfindingVerticalMovement/1.21.9
```

## Special Cases

### Shared Protocol Versions

If the new version shares a protocol with an existing version (e.g., 1.21.7 and 1.21.8 both use 772):

```bash
# Copy from version with same protocol
./scripts/add_version.sh 1.21.7 772 1.21.8
```

In this case:
- ✅ Packet structures are identical
- ✅ No code changes needed (only version references)
- ✅ Tests should pass immediately

### Major Protocol Changes

If there are significant protocol changes:

1. Run the script to create skeleton
2. Compare packet definitions between versions
3. Update handlers systematically
4. Run tests frequently to catch issues early
5. Document changes in `docs/ADDING_NEW_VERSION.md`

## Troubleshooting

### Script Fails: "Source version directory not found"

```bash
# Check available versions
ls -1 versions/ | grep '^v[0-9]'

# Use explicit source version
./scripts/add_version.sh 1.21.9 773 1.21.8
```

### Compilation Fails After Script

This is **expected** if there are protocol changes. Review error messages:

```bash
# See what failed
go build ./versions/v1_21_9/...

# Common errors:
# - "undefined: field X" → Field removed from packet
# - "cannot use X as type Y" → Type changed
# - "method GetFoo not found" → API changed (try Foo() without Get)
```

### Tests Fail

```bash
# Run with verbose output
go test -v ./versions/v1_21_9/...

# Common test failures:
# - Packet ID mismatch → Update using GetClientboundPacketID()
# - Method not found → Check bitflag API changes
# - Type error → Check for Option wrapping or enum type changes
```

## Summary Checklist

- [ ] Run script: `./scripts/add_version.sh VERSION PROTOCOL` (e.g., `1.21.9 773` or `26.1.0 800`)
- [ ] Check protocol changes in mc-protocol-go
- [ ] Update handler implementations
- [ ] Run unit tests: `go test ./versions/vVERSION/...` (with dots replaced by underscores)
- [ ] Add to integration tests
- [ ] Update documentation Quick Reference
- [ ] Test with real server (if available)
- [ ] Commit changes

## Need Help?

See full documentation:
- `scripts/README.md` - Script usage and troubleshooting
- `docs/ADDING_NEW_VERSION.md` - Comprehensive version addition guide
- `versions/common/interfaces.go` - Handler interfaces to implement
