#!/usr/bin/env bash
set -euo pipefail

SMX="./bin/smx"

# Build first
make build

# Test 1: Basic lifecycle
echo "=== Test 1: Basic lifecycle ==="
$SMX sandbox create -b blueprints/python-dev.yaml -n e2e-test
$SMX sandbox list
$SMX sandbox exec e2e-test -- python -c "print('lifecycle ok')"
$SMX sandbox stop e2e-test
$SMX sandbox start e2e-test
$SMX sandbox inspect e2e-test
$SMX sandbox destroy e2e-test
echo "PASS: Basic lifecycle"

# Test 2: Workspace mount
echo "=== Test 2: Workspace mount ==="
TMPDIR=$(mktemp -d)
echo 'print("workspace works")' > "$TMPDIR/test.py"
$SMX sandbox create -b blueprints/python-dev.yaml -n e2e-ws -w "$TMPDIR"
$SMX sandbox exec e2e-ws -- python /workspace/test.py
$SMX sandbox destroy e2e-ws
rm -rf "$TMPDIR"
echo "PASS: Workspace mount"

# Test 3: Multiple sandboxes
echo "=== Test 3: Multiple sandboxes ==="
$SMX sandbox create -b blueprints/python-dev.yaml -n e2e-multi-1
$SMX sandbox create -b blueprints/python-dev.yaml -n e2e-multi-2
$SMX sandbox list
$SMX sandbox destroy e2e-multi-1
$SMX sandbox destroy e2e-multi-2
echo "PASS: Multiple sandboxes"

echo ""
echo "All E2E tests passed!"
