#!/bin/bash

# Minecraft Agent Integration Test Runner
# This script helps run integration tests with proper configuration

set -e
set -o pipefail

# Change to script directory (testing/)
cd "$(dirname "${BASH_SOURCE[0]}")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
TEST_PATTERN=""
PARALLEL=1  # IMPORTANT: Tests MUST run sequentially to prevent OOM
TIMEOUT="60m"
VERBOSE="-v"
PULL_IMAGE=false

# Memory warning
if [ "$(free -g | awk '/^Mem:/{print $2}')" -lt 8 ]; then
    echo -e "${YELLOW}WARNING: System has less than 8GB RAM. Tests may fail with OOM.${NC}"
    echo -e "${YELLOW}Each test can use 2-4GB of memory.${NC}"
    echo ""
fi

# Print usage
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -t, --test PATTERN      Run specific test (e.g., TestNavigationSingleAgent)"
    echo "  -p, --parallel N        Run N tests in parallel (default: 1)"
    echo "  -T, --timeout DURATION  Set test timeout (default: 60m)"
    echo "  -q, --quiet             Reduce verbosity"
    echo "  -P, --pull              Pull latest Minecraft server image before testing"
    echo "  -h, --help              Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Run all tests"
    echo "  $0 -t TestNavigationSingleAgent       # Run specific test"
    echo "  $0 -p 2 -T 90m                        # Run 2 tests in parallel with 90min timeout"
    echo "  $0 -P                                 # Pull image and run all tests"
    exit 1
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--test)
            TEST_PATTERN="$2"
            shift 2
            ;;
        -p|--parallel)
            PARALLEL="$2"
            shift 2
            ;;
        -T|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -q|--quiet)
            VERBOSE=""
            shift
            ;;
        -P|--pull)
            PULL_IMAGE=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            ;;
    esac
done

echo -e "${GREEN}=== Minecraft Agent Integration Tests ===${NC}"
echo ""

# Check Docker
echo -e "${YELLOW}Checking Docker...${NC}"
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}Error: Docker is not running or not accessible${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker is running${NC}"
echo ""

# Clean up leftover test containers
echo -e "${YELLOW}Cleaning up leftover test containers...${NC}"
LEFTOVER_CONTAINERS=$(docker ps -aq --filter 'name=mc-agent-test' 2>/dev/null)
if [ -n "$LEFTOVER_CONTAINERS" ]; then
    echo -e "${YELLOW}Found leftover containers, removing...${NC}"
    docker rm -f $LEFTOVER_CONTAINERS 2>/dev/null || true
    echo -e "${GREEN}✓ Removed $(echo "$LEFTOVER_CONTAINERS" | wc -w) leftover container(s)${NC}"
else
    echo -e "${GREEN}✓ No leftover containers found${NC}"
fi
echo ""

# Pull image if requested
if [ "$PULL_IMAGE" = true ]; then
    echo -e "${YELLOW}Pulling latest Minecraft server image...${NC}"
    docker pull itzg/minecraft-server:latest
    echo -e "${GREEN}✓ Image pulled${NC}"
    echo ""
fi

# Create directories
echo -e "${YELLOW}Setting up directories...${NC}"
mkdir -p ./replays
mkdir -p ./logs/servers
mkdir -p ./logs/agents
echo -e "${GREEN}✓ Directories ready${NC}"
echo ""

# Build test command (run from testing directory)
TEST_CMD="go test . -count 1 "
if [ -n "$TEST_PATTERN" ]; then
    TEST_CMD="$TEST_CMD -run "${TEST_PATTERN@Q}
fi
TEST_CMD="$TEST_CMD -parallel $PARALLEL"
TEST_CMD="$TEST_CMD -timeout $TIMEOUT"
if [ -n "$VERBOSE" ]; then
    TEST_CMD="$TEST_CMD $VERBOSE"
fi

# Print configuration
echo -e "${YELLOW}Test Configuration:${NC}"
if [ -n "$TEST_PATTERN" ]; then
    echo "  Test Pattern: "${TEST_PATTERN@Q}
else
    echo "  Test Pattern: All tests"
fi
echo "  Parallel: $PARALLEL"
if [ "$PARALLEL" -gt 1 ]; then
    echo -e "  ${YELLOW}WARNING: Parallel execution may cause OOM. Recommended: -p 1${NC}"
fi
echo "  Timeout: $TIMEOUT"
echo "  Verbosity: $([ -n "$VERBOSE" ] && echo "Verbose" || echo "Quiet")"
echo ""

# Run tests
echo -e "${YELLOW}Running tests...${NC}"
echo -e "${YELLOW}Command: $TEST_CMD${NC}"
echo ""

# Create temporary file for test output
TEST_OUTPUT=$(mktemp)
trap "rm -f $TEST_OUTPUT" EXIT

# Run tests and capture output
if eval $TEST_CMD 2>&1 | tee $TEST_OUTPUT; then
    echo ""

    # Parse test results for summary
    PASS_COUNT=$(grep -c "^--- PASS:" $TEST_OUTPUT 2>/dev/null || echo "0")
    PASS_COUNT=$(echo "$PASS_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    FAIL_COUNT=$(grep -c "^--- FAIL:" $TEST_OUTPUT 2>/dev/null || echo "0")
    FAIL_COUNT=$(echo "$FAIL_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    SKIP_COUNT=$(grep -c "^--- SKIP:" $TEST_OUTPUT 2>/dev/null || echo "0")
    SKIP_COUNT=$(echo "$SKIP_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    TOTAL_COUNT=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))

    echo -e "${GREEN}=== Test Summary ===${NC}"
    if [ "$TOTAL_COUNT" -gt 0 ]; then
        echo -e "${GREEN}  Passed: $PASS_COUNT${NC}"
        if [ "$FAIL_COUNT" -gt 0 ]; then
            echo -e "${RED}  Failed: $FAIL_COUNT${NC}"
        fi
        if [ "$SKIP_COUNT" -gt 0 ]; then
            echo -e "${YELLOW}  Skipped: $SKIP_COUNT${NC}"
        fi
        echo "  Total:  $TOTAL_COUNT"
    else
        echo -e "${GREEN}  All tests passed!${NC}"
    fi

    # Check for leftover containers (shouldn't be any if tests cleaned up properly)
    if [ "$TEST_KEEP_SERVER" != "" ]; then
        echo -e "${YELLOW}NOTE: TEST_KEEP_SERVER is set; skipping leftover container check.${NC}"
    else
        LEFTOVER=$(docker ps -aq --filter 'name=mc-agent-test' 2>/dev/null | wc -l)
        if [ "$LEFTOVER" -gt 0 ]; then
            echo ""
            echo -e "${YELLOW}WARNING: Found $LEFTOVER leftover test container(s)${NC}"
            echo -e "${YELLOW}This indicates tests may not be cleaning up properly.${NC}"
            docker ps -a --filter 'name=mc-agent-test'
        fi
    fi
    # List replay files
    REPLAY_COUNT=$(find ./replays -name "*.mcpr" 2>/dev/null | wc -l)
    if [ "$REPLAY_COUNT" -gt 0 ]; then
        echo ""
        echo -e "${GREEN}Replay recordings ($REPLAY_COUNT files):${NC}"
        ls -lh ./replays/*.mcpr | tail -n 10
        if [ "$REPLAY_COUNT" -gt 10 ]; then
            echo "  ... and $((REPLAY_COUNT - 10)) more files"
        fi
    fi

    # List captured server logs
    SERVER_LOG_COUNT=$(find ./logs/servers -name "*.log" 2>/dev/null | wc -l)
    if [ "$SERVER_LOG_COUNT" -gt 0 ]; then
        echo ""
        echo -e "${GREEN}Server logs captured ($SERVER_LOG_COUNT files):${NC}"
        ls -lh ./logs/servers/*.log | tail -n 5
    fi

    # List captured agent logs
    AGENT_LOG_COUNT=$(find ./logs/agents -name "*.log" 2>/dev/null | wc -l)
    if [ "$AGENT_LOG_COUNT" -gt 0 ]; then
        echo ""
        echo -e "${GREEN}Agent logs captured ($AGENT_LOG_COUNT files):${NC}"
        ls -lh ./logs/agents/*.log | tail -n 5
    fi

    exit 0
else
    echo ""

    # Parse test results for summary
    PASS_COUNT=$(grep -c "^--- PASS:" $TEST_OUTPUT 2>/dev/null || echo "0")
    PASS_COUNT=$(echo "$PASS_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    FAIL_COUNT=$(grep -c "^--- FAIL:" $TEST_OUTPUT 2>/dev/null || echo "0")
    FAIL_COUNT=$(echo "$FAIL_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    SKIP_COUNT=$(grep -c "^--- SKIP:" $TEST_OUTPUT 2>/dev/null || echo "0")
    SKIP_COUNT=$(echo "$SKIP_COUNT" | tr -d '\n' | tr -d ' ' | head -n1)
    TOTAL_COUNT=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))

    echo -e "${RED}=== Test Summary ===${NC}"
    if [ "$TOTAL_COUNT" -gt 0 ]; then
        if [ "$PASS_COUNT" -gt 0 ]; then
            echo -e "${GREEN}  Passed:  $PASS_COUNT${NC}"
        fi
        echo -e "${RED}  Failed:  $FAIL_COUNT${NC}"
        if [ "$SKIP_COUNT" -gt 0 ]; then
            echo -e "${YELLOW}  Skipped: $SKIP_COUNT${NC}"
        fi
        echo "  Total:   $TOTAL_COUNT"
        echo ""
        # List failed tests
        echo -e "${RED}Failed tests:${NC}"
        grep "^--- FAIL:" $TEST_OUTPUT | sed 's/^--- FAIL: /  - /' || echo "  (parse error)"
    else
        echo -e "${RED}  Tests failed${NC}"
    fi

    # Check for leftover containers
    LEFTOVER=$(docker ps -aq --filter 'name=mc-agent-test' 2>/dev/null | wc -l)
    if [ "$LEFTOVER" -gt 0 ]; then
        echo ""
        echo -e "${YELLOW}Found $LEFTOVER leftover test container(s):${NC}"
        docker ps -a --filter 'name=mc-agent-test'
        echo ""
        echo -e "${YELLOW}Clean up with: docker rm -f \$(docker ps -aq --filter 'name=mc-agent-test')${NC}"
    fi

    echo ""
    echo -e "${YELLOW}Troubleshooting:${NC}"
    echo "  1. Review server logs: ls -lh ./logs/servers/"
    echo "  2. Review test output above"
    echo "  3. Examine replay files: ls -lh ./replays/"
    echo "  4. Check Docker containers: docker ps -a | grep mc-agent-test"
    echo ""

    # List any existing replays
    REPLAY_COUNT=$(find ./replays -name "*.mcpr" 2>/dev/null | wc -l)
    if [ "$REPLAY_COUNT" -gt 0 ]; then
        echo -e "${YELLOW}Available replays for debugging ($REPLAY_COUNT files):${NC}"
        ls -lh ./replays/*.mcpr | tail -n 5
    fi

    # List server logs
    SERVER_LOG_COUNT=$(find ./logs/servers -name "*.log" 2>/dev/null | wc -l)
    if [ "$SERVER_LOG_COUNT" -gt 0 ]; then
        echo ""
        echo -e "${YELLOW}Server logs for debugging ($SERVER_LOG_COUNT files):${NC}"
        ls -lh ./logs/servers/*.log | tail -n 5
    fi

    # List agent logs
    AGENT_LOG_COUNT=$(find ./logs/agents -name "*.log" 2>/dev/null | wc -l)
    if [ "$AGENT_LOG_COUNT" -gt 0 ]; then
        echo ""
        echo -e "${YELLOW}Agent logs for debugging ($AGENT_LOG_COUNT files):${NC}"
        ls -lh ./logs/agents/*.log | tail -n 5
    fi

    exit 1
fi
