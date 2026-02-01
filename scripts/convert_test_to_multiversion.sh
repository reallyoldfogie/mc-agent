#!/usr/bin/env bash
# Script to convert a single test function to multi-version format
# Usage: ./convert_test_to_multiversion.sh <test_file> <test_function_name>

set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <test_file> <test_function_name>"
    echo "Example: $0 testing/navigation_test.go TestNavigationSingleAgent"
    exit 1
fi

TEST_FILE="$1"
TEST_FUNC="$2"

if [ ! -f "$TEST_FILE" ]; then
    echo "Error: Test file not found: $TEST_FILE"
    exit 1
fi

echo "Converting $TEST_FUNC in $TEST_FILE to multi-version format..."
echo ""
echo "Steps:"
echo "1. Wrap function body in: for _, tt := range standardVersionTests { t.Run(tt.name, func(t *testing.T) {"
echo "2. Add closing braces: }) }"
echo "3. Replace: serverCfg.Version = \"1.21.X\" with serverCfg.Version = tt.mcVersion"
echo "4. Add version handler setup after agentCfg creation:"
echo "   if tt.useVersionHandler {"
echo "       versionHandler, err := common.GetVersionHandler(tt.mcVersion)"
echo "       if err == nil && versionHandler != nil {"
echo "           agentCfg.VersionHandler = versionHandler"
echo "       }"
echo "   } else {"
echo "       agentCfg.DisableVersionHandlerAutoDetect = true"
echo "   }"
echo ""
echo "This requires manual editing. Use the pattern from TestPathfindingVerticalMovement as reference."
