# MC-Agent Automation Scripts

This directory contains scripts to automate common development tasks.

## add_version.sh

Automates the process of adding a new Minecraft version handler to mc-agent.

**Note:** Supports both legacy (1.X.X) and modern (YY.X.Z) version formats. See `VERSION_FORMATS.md` for details.

### Usage

```bash
./scripts/add_version.sh [--dry-run] <version> <protocol> [source_version]
```

### Options

- `--dry-run` - Preview what the script would do without making any changes

### Arguments

- `version` - Minecraft version (e.g., `1.21.9` for legacy versions, `26.1.0` for 2026+ versions)
- `protocol` - Protocol version number (e.g., `773`)
- `source_version` - (Optional) Source version to copy from. If not specified, copies from the latest version.

### Examples

```bash
# Preview what would be created (dry run)
./scripts/add_version.sh --dry-run 1.21.9 773

# Add version 1.21.9 with protocol 773, copying from latest version
./scripts/add_version.sh 1.21.9 773

# Add version 1.21.9 with protocol 773, copying from 1.21.8 specifically
./scripts/add_version.sh 1.21.9 773 1.21.8

# Add version 1.21.7 that shares protocol 772 with 1.21.8
./scripts/add_version.sh 1.21.7 772 1.21.8

# Add 2026 format version (26.1.0 -> v26_1_0)
./scripts/add_version.sh 26.1.0 800 1.21.8
```

### What It Does

1. **Creates version package directory** - `versions/v1_21_9/` or `versions/v26_1_0/`
2. **Copies source files** - All `.go` files from the source version
3. **Updates references** - Replaces:
   - Package names (`v1_21_8` → `v1_21_9` or `v26_1_0`)
   - Version strings (`"1.21.8"` → `"1.21.9"` or `"26.1.0"`)
   - Protocol versions (`772` → `773`)
   - Log messages and test names
4. **Updates registry** - Adds import to `versions/init.go`
5. **Formats code** - Runs `go fmt` on new files
6. **Verifies compilation** - Runs `go build` to check for errors

### What You Still Need to Do

The script creates a working skeleton, but you must:

1. **Review protocol changes** - Check wiki.vg or Minecraft protocol documentation
2. **Update handlers** - Modify implementation files for protocol differences:
   - Packet structure changes (field additions/removals)
   - Data type changes (int → string, etc.)
   - New packet types
   - Bitflag API changes
3. **Update tests** - Modify test files to handle protocol changes
4. **Run tests** - `go test ./versions/v1_XX_X/...`
5. **Update integration tests** - Add version to `testing/navigation_pathfinding_test.go`
6. **Update documentation** - Add to Quick Reference table in `docs/ADDING_NEW_VERSION.md`

### Finding Protocol Version Numbers

Protocol version numbers can be found at:
- https://wiki.vg/Protocol_version_numbers
- https://minecraft.wiki/w/Protocol_version

### Common Protocol Changes

Between Minecraft versions, common changes include:

- **Packet structure changes** - Fields added, removed, or reordered
- **Data type changes** - `int` → `string` for enums, `Option<T>` → nullable types
- **API changes** - Method renames (e.g., `GetOnGround()` → `OnGround()`)
- **New packets** - Entirely new packet types
- **Shared protocols** - Multiple versions using the same protocol number

See `docs/ADDING_NEW_VERSION.md` for detailed information on handling these changes.

### Troubleshooting

**Script fails with "Source version directory not found"**
- Ensure the source version exists in `versions/`
- Check version format (use dots: `1.21.8`, not underscores)

**Compilation fails after running script**
- This is expected if there are protocol changes
- Review error messages and update handler implementations
- See `docs/ADDING_NEW_VERSION.md` for common issues

**Import not added to versions/init.go**
- Script may have detected existing import
- Check `versions/init.go` manually and add if needed

### Example Workflow

```bash
# 1. Run the script to create skeleton
./scripts/add_version.sh 1.21.9 773

# 2. Review what changed in this protocol version
# Check: https://wiki.vg/Protocol

# 3. Update handlers for protocol changes
vim versions/v1_21_9/movement.go
vim versions/v1_21_9/entities.go
# ... etc

# 4. Run tests
go test ./versions/v1_21_9/...

# 5. Update integration tests
vim testing/navigation_pathfinding_test.go
# Add 1.21.9 to versionTests array

# 6. Update documentation
vim docs/ADDING_NEW_VERSION.md
# Add 1.21.9 to Quick Reference table

# 7. Test with real server
go test -v ./testing -run TestPathfindingVerticalMovement/1.21.9
```

## verify_versions.sh

Verifies that all version handlers are properly configured, compile, and have tests.

### Usage

```bash
./scripts/verify_versions.sh
```

### What It Checks

For each version handler:
1. **Required files** - Ensures all handler files exist (handler.go, init.go, etc.)
2. **Registration** - Verifies version is imported in `versions/init.go`
3. **Compilation** - Checks that the package builds without errors
4. **Tests** - Looks for test files and attempts to run them
5. **Integration** - Checks if version appears in integration tests

### Output

```
==========================================
MC-Agent Version Handler Verification
==========================================

Checking 1.21.4 (v1_21_4)...
✓ 1.21.4: All required files present
✓ 1.21.4: Registered in versions/init.go
✓ 1.21.4: Package compiles
✓ 1.21.4: Has 4 test files
⚠ 1.21.4: Some tests fail (may need server connection)
✓ 1.21.4: Found in integration tests

==========================================
Verification Summary
==========================================
Total checks: 42
✓ Passed: 38
⚠ Warnings: 4

✓ All critical checks passed!
```

### Use Cases

- **After adding a new version** - Verify it's properly configured
- **Before committing** - Check all versions still work
- **CI/CD pipeline** - Automated verification in build process
- **Troubleshooting** - Identify missing files or configuration issues

### Exit Codes

- `0` - All critical checks passed (warnings are OK)
- `1` - One or more critical checks failed

## Future Scripts

Other automation scripts that could be added:
- `update_tests.sh` - Update integration tests for all versions
- `generate_docs.sh` - Auto-generate version compatibility documentation
