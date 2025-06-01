#!/usr/bin/env bash
# Fix pre-commit configuration issues

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Ensure Go bin directory is in PATH
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="$PATH:$GOPATH_BIN"

echo "Fixing pre-commit configuration..."
echo ""

# 1. Check if golangci-lint and gosec are available
if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "Installing golangci-lint..."
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
fi

if ! command -v gosec >/dev/null 2>&1; then
  echo "Installing gosec..."
  go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

# 2. Ensure pre-commit is installed
if ! command -v pre-commit >/dev/null 2>&1; then
  echo "Installing pre-commit..."
  pip3 install --user pre-commit || pip install --user pre-commit
fi

# 3. Clean pre-commit cache to remove problematic downloads
echo "Cleaning pre-commit cache..."
pre-commit clean

# 4. Reinstall hooks
echo "Reinstalling hooks..."
cd "$PROJECT_ROOT"
pre-commit uninstall
pre-commit install
pre-commit install --hook-type commit-msg
pre-commit install --hook-type pre-push

echo ""
echo "Testing configuration..."
echo ""

# 5. Test specific hooks
echo "Testing golangci-lint hook..."
if pre-commit run golangci-lint --all-files; then
  echo "✓ golangci-lint hook is working"
else
  echo "⚠ golangci-lint found issues (this is normal)"
fi

echo ""
echo "Fix complete!"
echo ""
echo "You can now run:"
echo "  pre-commit run --all-files"
echo ""

# Show PATH configuration if needed
if [[ ":$PATH:" != *":$(go env GOPATH)/bin:"* ]]; then
  echo "Don't forget to add Go bin to your PATH permanently:"
  echo "  export PATH=\"\$PATH:$(go env GOPATH)/bin\""
  echo ""
fi
