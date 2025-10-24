#!/bin/bash
# Auto-generated Docker test script
# Builder: buildkit
# Type: happy
# Test: version
# Mode: rootless
# Driver: native
# Generated: Thu Oct 23 07:50:10 PM PDT 2025

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo ""
echo -e "${CYAN}╔═══════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║  Docker Test: version${NC}"
echo -e "${CYAN}║  Builder: buildkit${NC}"
echo -e "${CYAN}║  Type: happy${NC}"
echo -e "${CYAN}║  Mode: rootless${NC}"
echo -e "${CYAN}║  Driver: native${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════════════════╝${NC}"
echo ""

# Test execution
echo "Running test command..."
echo ""

if docker run --rm --user 1000:1000 --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined -e HOME=/home/smithy -e DOCKER_CONFIG=/home/smithy/.docker -v /tmp/smithy-test-yrinE1:/home/smithy/workspace:ro 100.92.16.57:5000/rapidfort/smithy:latest --version; then
    echo ""
    echo -e "${GREEN}✓ Test PASSED${NC}"
    exit 0
else
    exit_code=$?
    echo ""
    echo -e "${RED}✗ Test FAILED (exit code: ${exit_code})${NC}"
    exit $exit_code
fi
