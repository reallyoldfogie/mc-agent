#!/usr/bin/env bash
# Script to automate adding a new Minecraft version handler to mc-agent
# Usage: ./scripts/add_version.sh <version> <protocol> [source_version]
# Example: ./scripts/add_version.sh 1.21.9 773 1.21.8

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
error() {
    echo -e "${RED}Error: $1${NC}" >&2
    exit 1
}

info() {
    echo -e "${BLUE}Info: $1${NC}"
}

success() {
    echo -e "${GREEN}Success: $1${NC}"
}

warn() {
    echo -e "${YELLOW}Warning: $1${NC}"
}

# Parse arguments
DRY_RUN=false
if [[ "$1" == "--dry-run" ]]; then
    DRY_RUN=true
    shift
fi

if [ $# -lt 2 ]; then
    echo "Usage: $0 [--dry-run] <version> <protocol> [source_version]"
    echo ""
    echo "Options:"
    echo "  --dry-run       - Show what would be done without making changes"
    echo ""
    echo "Arguments:"
    echo "  version         - Minecraft version (e.g., 1.21.9 or 26.1.0)"
    echo "                    Format: numbers and dots (converted to v1_21_9 or v26_1_0)"
    echo "  protocol        - Protocol version number (e.g., 773)"
    echo "  source_version  - Source version to copy from (default: latest version)"
    echo ""
    echo "Examples:"
    echo "  $0 --dry-run 1.21.9 773       # Preview what would be created"
    echo "  $0 1.21.9 773                 # Copy from latest version (legacy format)"
    echo "  $0 26.1.0 800 1.21.8          # 2026+ format version"
    echo "  $0 1.21.9 773 1.21.8          # Copy from specific version"
    exit 1
fi

NEW_VERSION="$1"
PROTOCOL_VERSION="$2"
SOURCE_VERSION="${3:-}"

# Validate version format (allow any version with dots and numbers)
if ! [[ "$NEW_VERSION" =~ ^[0-9][0-9.]*$ ]]; then
    error "Invalid version format: $NEW_VERSION (expected format: numbers and dots, e.g., 1.21.9 or 26.1.0)"
fi

# Validate protocol version
if ! [[ "$PROTOCOL_VERSION" =~ ^[0-9]+$ ]]; then
    error "Invalid protocol version: $PROTOCOL_VERSION (expected numeric value)"
fi

# Convert version to package name (e.g., 1.21.9 -> v1_21_9, 26.1.0 -> v26_1_0)
VERSION_PKG=$(echo "v$NEW_VERSION" | tr '.' '_')

# Get project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VERSIONS_DIR="$PROJECT_ROOT/versions"
NEW_VERSION_DIR="$VERSIONS_DIR/$VERSION_PKG"

if [ "$DRY_RUN" = true ]; then
    warn "DRY RUN MODE - No changes will be made"
fi

info "Adding Minecraft version $NEW_VERSION (protocol $PROTOCOL_VERSION)"
info "Package name: $VERSION_PKG"

# Check if version already exists
if [ -d "$NEW_VERSION_DIR" ]; then
    error "Version $NEW_VERSION already exists at $NEW_VERSION_DIR"
fi

# Determine source version
if [ -z "$SOURCE_VERSION" ]; then
    # Find the latest version directory
    LATEST_VERSION=$(ls -1 "$VERSIONS_DIR" | grep '^v[0-9]' | sort -V | tail -n 1)
    if [ -z "$LATEST_VERSION" ]; then
        error "No existing version directories found in $VERSIONS_DIR"
    fi
    SOURCE_VERSION=$(echo "$LATEST_VERSION" | sed 's/^v//; s/_/./g')
    info "Auto-detected source version: $SOURCE_VERSION"
else
    info "Using specified source version: $SOURCE_VERSION"
fi

SOURCE_PKG=$(echo "v$SOURCE_VERSION" | tr '.' '_')
SOURCE_VERSION_DIR="$VERSIONS_DIR/$SOURCE_PKG"

if [ ! -d "$SOURCE_VERSION_DIR" ]; then
    error "Source version directory not found: $SOURCE_VERSION_DIR"
fi

info "Copying from: $SOURCE_VERSION_DIR"
info "Creating: $NEW_VERSION_DIR"

if [ "$DRY_RUN" = true ]; then
    info "Would create directory: $NEW_VERSION_DIR"
    info "Would copy files from: $SOURCE_VERSION_DIR"
    info "Would update: package names, version strings, protocol version"
    info "Would add import to: $VERSIONS_DIR/init.go"
    success "Dry run complete - no changes made"
    exit 0
fi

# Create new version directory
mkdir -p "$NEW_VERSION_DIR"

# Copy all .go files from source version
info "Copying source files..."
cp "$SOURCE_VERSION_DIR"/*.go "$NEW_VERSION_DIR/"

# Replace version strings in all files
info "Updating version references..."
cd "$NEW_VERSION_DIR"

for file in *.go; do
    # Skip if file doesn't exist (shouldn't happen, but safety check)
    [ -f "$file" ] || continue
    
    # Replace package name
    sed -i "s/package $SOURCE_PKG/package $VERSION_PKG/g" "$file"
    
    # Replace version string constant
    sed -i "s/versionString[[:space:]]*=[[:space:]]*\"$SOURCE_VERSION\"/versionString   = \"$NEW_VERSION\"/g" "$file"
    
    # Replace protocol version constant
    SOURCE_PROTOCOL=$(grep -oP 'protocolVersion\s*=\s*\K[0-9]+' handler.go 2>/dev/null | head -n 1 || echo "")
    if [ -n "$SOURCE_PROTOCOL" ]; then
        sed -i "s/protocolVersion[[:space:]]*=[[:space:]]*$SOURCE_PROTOCOL/protocolVersion = $PROTOCOL_VERSION/g" "$file"
    fi
    
    # Replace version in log messages
    sed -i "s/\\[versions\\/$SOURCE_PKG\\]/[versions\\/$VERSION_PKG]/g" "$file"
    
    # Replace version in test names (e.g., TestV1_21_8 -> TestV1_21_9)
    SOURCE_TEST_PREFIX=$(echo "$SOURCE_PKG" | tr 'a-z' 'A-Z' | sed 's/_//g')
    NEW_TEST_PREFIX=$(echo "$VERSION_PKG" | tr 'a-z' 'A-Z' | sed 's/_//g')
    sed -i "s/Test${SOURCE_TEST_PREFIX}/Test${NEW_TEST_PREFIX}/g" "$file"
done

success "Created version handler at $NEW_VERSION_DIR"

# Add import to versions/init.go
info "Updating versions/init.go..."
INIT_FILE="$VERSIONS_DIR/init.go"

if [ ! -f "$INIT_FILE" ]; then
    error "versions/init.go not found at $INIT_FILE"
fi

# Check if import already exists
if grep -q "_ \"github.com/reallyoldfogie/mc-agent/versions/$VERSION_PKG\"" "$INIT_FILE"; then
    warn "Import for $VERSION_PKG already exists in versions/init.go"
else
    # Add import before the closing parenthesis
    # Find the last import line and add after it
    awk -v new_import="\t_ \"github.com/reallyoldfogie/mc-agent/versions/$VERSION_PKG\"" '
        /^import \(/ { in_import=1 }
        in_import && /^\)/ { 
            print new_import
            in_import=0
        }
        { print }
    ' "$INIT_FILE" > "$INIT_FILE.tmp"
    mv "$INIT_FILE.tmp" "$INIT_FILE"
    success "Added import to versions/init.go"
fi

# Run go fmt on new files
info "Running go fmt..."
cd "$PROJECT_ROOT"
go fmt "./versions/$VERSION_PKG/..." || warn "go fmt failed (may need manual cleanup)"

# Verify the package compiles
info "Verifying package compiles..."
if go build "./versions/$VERSION_PKG/..."; then
    success "Package compiles successfully"
else
    error "Package compilation failed - manual fixes may be needed"
fi

# Summary
echo ""
echo "=========================================="
success "Version $NEW_VERSION setup complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Review protocol changes for $NEW_VERSION:"
echo "   - Check wiki.vg or Minecraft protocol documentation"
echo "   - Compare packet structures with $SOURCE_VERSION"
echo ""
echo "2. Update implementation files as needed:"
echo "   - $NEW_VERSION_DIR/movement.go"
echo "   - $NEW_VERSION_DIR/entities.go"
echo "   - $NEW_VERSION_DIR/configuration.go"
echo "   - $NEW_VERSION_DIR/login.go"
echo "   - Other handlers as needed"
echo ""
echo "3. Run tests:"
echo "   go test ./versions/$VERSION_PKG/..."
echo ""
echo "4. Update integration tests:"
echo "   - testing/navigation_pathfinding_test.go"
echo "   - Add $NEW_VERSION to version test arrays"
echo ""
echo "5. Update documentation:"
echo "   - docs/ADDING_NEW_VERSION.md (Quick Reference table)"
echo ""
echo "Files created:"
find "$NEW_VERSION_DIR" -name "*.go" | sort | sed 's/^/  - /'
echo ""
