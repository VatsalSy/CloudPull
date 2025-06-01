#!/usr/bin/env bash
# Go security check script for pre-commit

set -e

echo "Running security checks..."

# Check if gosec is installed
if ! command -v gosec &> /dev/null; then
  echo "gosec not installed. Installing..."
  go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

# Run gosec with exclusions matching our .golangci.yml config
# Exclude: G204, G304, G404, G501, G401, G107, G302 as per golangci config and app requirements
if ! gosec -fmt json -out /tmp/gosec-report.json -exclude=G204,G304,G404,G501,G401,G107,G302 -severity=medium -confidence=medium ./... 2>/dev/null; then
  echo "Security issues found!"

  # Parse and display issues
  if command -v jq &> /dev/null; then
    jq -r '.Issues[] | "[\(.severity)] \(.file):\(.line):\(.column) - \(.rule_id): \(.details)"' /tmp/gosec-report.json
  else
    cat /tmp/gosec-report.json
  fi

  rm -f /tmp/gosec-report.json
  echo ""
  echo "Please fix security issues before committing"
  exit 1
fi

rm -f /tmp/gosec-report.json
echo "No security issues found"
