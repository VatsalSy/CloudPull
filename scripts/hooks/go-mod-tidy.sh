#!/usr/bin/env bash
# Go mod tidy check script for pre-commit

set -e

# Save current go.mod and go.sum
cp go.mod go.mod.backup
cp go.sum go.sum.backup 2>/dev/null || true

# Run go mod tidy
go mod tidy

# Check if go.mod or go.sum changed
if ! diff -q go.mod go.mod.backup >/dev/null 2>&1; then
  echo "go.mod was modified by 'go mod tidy'"
  echo "Please run 'go mod tidy' and commit the changes"
  mv go.mod.backup go.mod
  mv go.sum.backup go.sum 2>/dev/null || true
  exit 1
fi

if [ -f go.sum.backup ] && ! diff -q go.sum go.sum.backup >/dev/null 2>&1; then
  echo "go.sum was modified by 'go mod tidy'"
  echo "Please run 'go mod tidy' and commit the changes"
  mv go.mod.backup go.mod
  mv go.sum.backup go.sum
  exit 1
fi

# Clean up backups
rm -f go.mod.backup go.sum.backup

echo "go.mod and go.sum are tidy"