#!/usr/bin/env bash
# Go generate check script for pre-commit

set -e

# Find all files with go:generate comments
files_with_generate=$(grep -r "^//go:generate" --include="*.go" . 2>/dev/null | cut -d: -f1 | sort -u)

if [ -z "$files_with_generate" ]; then
  echo "No files with go:generate directives"
  exit 0
fi

# Save current state
echo "Checking go:generate directives..."
git stash push -q --keep-index

# Run go generate
go generate ./...

# Check if any files changed
if ! git diff --quiet; then
  echo "Files were modified by 'go generate'"
  echo "Please run 'go generate ./...' and commit the changes"
  git stash pop -q
  exit 1
fi

# Restore state
git stash pop -q || true

echo "All generated files are up to date"
