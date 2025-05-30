#!/usr/bin/env bash
# Go fmt check script for pre-commit

set -e

# Get the list of changed Go files
files="$@"

if [ -z "$files" ]; then
  echo "No Go files to check"
  exit 0
fi

# Check if files are formatted
unformatted=$(gofmt -l $files)

if [ -n "$unformatted" ]; then
  echo "The following files are not formatted:"
  echo "$unformatted"
  echo ""
  echo "Run 'go fmt ./...' to format them"
  exit 1
fi

echo "All Go files are properly formatted"