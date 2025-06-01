#!/usr/bin/env bash
# Go build check script for pre-commit

set -e

echo "Building CloudPull..."

# Try to build the main binary
if ! go build -o /tmp/cloudpull-test ./cmd/cloudpull; then
  echo "Build failed!"
  echo "Please fix build errors before committing"
  exit 1
fi

# Clean up
rm -f /tmp/cloudpull-test

echo "Build successful"
