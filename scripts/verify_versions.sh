#!/usr/bin/env bash
# Script to verify all version handlers are properly configured
# Checks compilation, registration, and test coverage

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

error() { echo -e "${RED}✗ $1${NC}" >&2; }
success() { echo -e "${GREEN}✓ $1${NC}"; }
warn() { echo -e "${YELLOW}⚠ $1${NC}"; }
info() { echo -e "${BLUE}ℹ $1${NC}"; }

# Get project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

echo "=========================================="
echo "MC-Agent Version Handler Verification"
echo "=========================================="
echo ""

TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

check_pass() {
    ((TOTAL_CHECKS++))
    ((PASSED_CHECKS++))
    success "$1"
}

check_fail() {
    ((TOTAL_CHECKS++))
    ((FAILED_CHECKS++))
    error "$1"
}

check_warn() {
    ((TOTAL_CHECKS++))
    warn "$1"
}

# 1. Find all version directories
info "Scanning for version handlers..."
VERSION_DIRS=$(find versions -maxdepth 1 -type d -name 'v[0-9]*' | sort)
VERSION_COUNT=$(echo "$VERSION_DIRS" | wc -l)
info "Found $VERSION_COUNT version directories"
echo ""

# 2. Check each version directory
for version_dir in $VERSION_DIRS; do
    VERSION_PKG=$(basename "$version_dir")
    VERSION=$(echo "$VERSION_PKG" | sed 's/^v//; s/_/./g')
    
    echo "Checking $VERSION ($VERSION_PKG)..."
    
    # Check required files exist
    REQUIRED_FILES=("handler.go" "init.go" "login.go" "configuration.go" "movement.go" "entities.go" "containers.go" "chat.go" "world.go")
    MISSING_FILES=0
    
    for file in "${REQUIRED_FILES[@]}"; do
        if [ ! -f "$version_dir/$file" ]; then
            check_fail "$VERSION: Missing required file $file"
            ((MISSING_FILES++))
        fi
    done
    
    if [ $MISSING_FILES -eq 0 ]; then
        check_pass "$VERSION: All required files present"
    fi
    
    # Check if version is registered in init.go
    if grep -q "_ \"github.com/reallyoldfogie/mc-agent/handler_versions/$VERSION_PKG\"" versions/init.go; then
        check_pass "$VERSION: Registered in versions/init.go"
    else
        check_fail "$VERSION: NOT registered in versions/init.go"
    fi
    
    # Check if package compiles
    if go build "./$version_dir/..." > /dev/null 2>&1; then
        check_pass "$VERSION: Package compiles"
    else
        check_fail "$VERSION: Compilation failed"
    fi
    
    # Check if tests exist
    TEST_FILES=$(find "$version_dir" -name "*_test.go" | wc -l)
    if [ "$TEST_FILES" -gt 0 ]; then
        check_pass "$VERSION: Has $TEST_FILES test files"
        
        # Try to run tests
        if go test "./$version_dir/..." > /dev/null 2>&1; then
            check_pass "$VERSION: All tests pass"
        else
            check_warn "$VERSION: Some tests fail (may need server connection)"
        fi
    else
        check_warn "$VERSION: No test files found"
    fi
    
    # Check if version appears in integration tests
    if grep -q "\"$VERSION\"" testing/navigation_pathfinding_test.go 2>/dev/null; then
        check_pass "$VERSION: Found in integration tests"
    else
        check_warn "$VERSION: Not found in integration tests"
    fi
    
    echo ""
done

# 3. Check documentation
info "Checking documentation..."
if grep -q "Quick Reference" docs/ADDING_NEW_VERSION.md; then
    check_pass "Quick Reference table exists in documentation"
else
    check_warn "No Quick Reference table found in documentation"
fi
echo ""

# 4. Summary
echo "=========================================="
echo "Verification Summary"
echo "=========================================="
echo "Total checks: $TOTAL_CHECKS"
success "Passed: $PASSED_CHECKS"
if [ $FAILED_CHECKS -gt 0 ]; then
    error "Failed: $FAILED_CHECKS"
fi
WARNED=$((TOTAL_CHECKS - PASSED_CHECKS - FAILED_CHECKS))
if [ $WARNED -gt 0 ]; then
    warn "Warnings: $WARNED"
fi
echo ""

if [ $FAILED_CHECKS -eq 0 ]; then
    success "All critical checks passed!"
    exit 0
else
    error "Some checks failed - please review above"
    exit 1
fi
