# Minecraft Version Format Support

The automation scripts support both legacy and modern Minecraft version formats.

## Supported Formats

### Legacy Format (pre-2026)
- **Pattern**: `X.Y.Z` where X, Y, Z are numbers
- **Examples**: `1.21.4`, `1.21.5`, `1.21.8`
- **Package name**: `vX_Y_Z` (e.g., `v1_21_8`)

### Modern Format (2026+)
- **Pattern**: `YY.X.Z` where YY is year, X and Z are numbers
- **Examples**: `26.1.0`, `26.2.3`, `27.0.1`
- **Package name**: `vYY_X_Z` (e.g., `v26_1_0`)

### Flexible Format
- **Pattern**: Any sequence of numbers and dots starting with a number
- **Examples**: `26.0`, `27.1.2.5`, `2.0`
- **Package name**: Dots replaced with underscores (e.g., `v26_0`, `v27_1_2_5`)

## Examples

```bash
# Legacy versions (1.X.X format)
./scripts/add_version.sh 1.21.9 773          # Creates v1_21_9
./scripts/add_version.sh 1.22.0 780          # Creates v1_22_0

# 2026+ versions (year-based format)
./scripts/add_version.sh 26.1.0 800          # Creates v26_1_0
./scripts/add_version.sh 26.2.3 802          # Creates v26_2_3
./scripts/add_version.sh 27.0.1 810          # Creates v27_0_1

# Short versions
./scripts/add_version.sh 26.0 801            # Creates v26_0
./scripts/add_version.sh 2.0 100             # Creates v2_0

# Extended versions
./scripts/add_version.sh 27.1.2.5 815        # Creates v27_1_2_5
```

## Package Name Conversion

The version-to-package-name conversion follows these rules:

1. Prefix with `v`
2. Replace all dots (`.`) with underscores (`_`)
3. Keep all numeric segments

| Version | Package Name |
|---------|--------------|
| 1.21.8  | v1_21_8      |
| 26.1.0  | v26_1_0      |
| 26.0    | v26_0        |
| 27.1.2  | v27_1_2      |
| 2.0.1   | v2_0_1       |

## Validation

The script validates version format to ensure:
- Must start with a number
- Can only contain numbers and dots
- No letters or special characters allowed

### Valid Examples
✓ `1.21.9`
✓ `26.1.0`
✓ `26.0`
✓ `27.1.2.5`
✓ `2.0`

### Invalid Examples
✗ `v1.21.9` (starts with 'v')
✗ `1.21.9-rc1` (contains letters)
✗ `1.21.9_beta` (contains underscore/letters)
✗ `.21.9` (starts with dot)

## Directory Structure

```
versions/
├── v1_21_4/    # Legacy format
├── v1_21_5/    # Legacy format
├── v1_21_8/    # Legacy format
├── v26_1_0/    # 2026 format
├── v26_2_3/    # 2026 format
└── v27_0_1/    # 2027 format
```

## Protocol Version Numbers

Protocol version numbers are independent of Minecraft version format:
- Protocol numbers are sequential integers (e.g., 769, 770, 771, 772, 773...)
- Multiple Minecraft versions can share the same protocol (e.g., 1.21.7 and 1.21.8 both use 772)
- Find protocol numbers at: https://wiki.vg/Protocol_version_numbers

## Migration Notes

If migrating from an older automation system:
- All existing version formats continue to work
- No changes needed to existing version handlers
- New 2026+ versions will automatically use the year-based format
- The script auto-detects the appropriate naming convention
