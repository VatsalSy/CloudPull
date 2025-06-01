#!/usr/bin/env bash
# CloudPull Development Environment Setup
# Sets up complete development environment including all tools, linters, and pre-commit hooks

set -e

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔧 CloudPull Development Environment Setup${NC}"
echo -e "${BLUE}===========================================${NC}"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go 1.21+ from https://golang.org/dl/${NC}"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
MIN_VERSION="1.21"
if [ "$(printf '%s\n' "$MIN_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_VERSION" ]; then
    echo -e "${RED}❌ Go version $GO_VERSION is too old. Please install Go 1.21+${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Go $GO_VERSION detected${NC}"

# Ensure Go bin directory is in PATH
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="$PATH:$GOPATH_BIN"

# Install Go development tools
echo ""
echo -e "${BLUE}📦 Installing Go development tools...${NC}"

# Install golangci-lint
if ! command -v golangci-lint >/dev/null 2>&1; then
    echo -e "${YELLOW}Installing golangci-lint...${NC}"
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
else
    echo -e "${GREEN}✓ golangci-lint already installed${NC}"
fi

# Install gosec
if ! command -v gosec >/dev/null 2>&1; then
    echo -e "${YELLOW}Installing gosec...${NC}"
    go install github.com/securego/gosec/v2/cmd/gosec@latest
else
    echo -e "${GREEN}✓ gosec already installed${NC}"
fi

# Install pre-commit framework
echo ""
echo -e "${BLUE}🪝 Setting up pre-commit framework...${NC}"

if ! command -v pre-commit >/dev/null 2>&1; then
    echo -e "${YELLOW}Installing pre-commit...${NC}"
    if command -v pip3 >/dev/null 2>&1; then
        pip3 install --user pre-commit
    elif command -v pip >/dev/null 2>&1; then
        pip install --user pre-commit
    else
        echo -e "${RED}❌ Neither pip nor pip3 found. Please install Python pip to continue.${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ pre-commit already installed${NC}"
fi

# Install additional linters if needed
echo ""
echo -e "${BLUE}📋 Installing additional linters...${NC}"

# Install markdownlint-cli if Node.js is available
if command -v npm >/dev/null 2>&1; then
    if ! command -v markdownlint >/dev/null 2>&1; then
        echo -e "${YELLOW}Installing markdownlint-cli...${NC}"
        npm install -g markdownlint-cli
    else
        echo -e "${GREEN}✓ markdownlint already installed${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Node.js not found, skipping markdownlint installation${NC}"
fi

# Install shellcheck if not present
if ! command -v shellcheck >/dev/null 2>&1; then
    echo -e "${YELLOW}⚠ shellcheck not found. Install it for shell script linting:${NC}"
    echo "  macOS: brew install shellcheck"
    echo "  Ubuntu: apt-get install shellcheck"
    echo "  Other: https://github.com/koalaman/shellcheck#installing"
else
    echo -e "${GREEN}✓ shellcheck already installed${NC}"
fi

# Create markdownlint config if it doesn't exist
if [ ! -f "$PROJECT_ROOT/.markdownlint.yml" ]; then
    echo -e "${YELLOW}Creating .markdownlint.yml...${NC}"
    cat > "$PROJECT_ROOT/.markdownlint.yml" << EOF
# Markdownlint configuration
# https://github.com/DavidAnson/markdownlint

default: true

# Line length
MD013:
  line_length: 120
  code_blocks: false
  tables: false

# Allow inline HTML
MD033: false

# Allow duplicate headings
MD024:
  siblings_only: true
EOF
fi

# Install pre-commit hooks
echo ""
echo -e "${BLUE}🪝 Installing pre-commit hooks...${NC}"

cd "$PROJECT_ROOT"

# Remove existing hooks and reinstall
pre-commit uninstall >/dev/null 2>&1 || true
pre-commit install --install-hooks
pre-commit install --hook-type commit-msg
pre-commit install --hook-type pre-push

# Run initial checks
echo ""
echo -e "${BLUE}🔍 Running initial checks...${NC}"

# Test if hooks are working
if pre-commit run --all-files; then
    echo -e "${GREEN}✅ All checks passed!${NC}"
else
    echo -e "${YELLOW}⚠ Some checks failed (this is normal for first run)${NC}"
    echo -e "${YELLOW}  Review the output above and fix any issues.${NC}"
fi

# Show PATH configuration reminder if needed
echo ""
if [[ ":$PATH:" != *":$(go env GOPATH)/bin:"* ]]; then
    echo -e "${YELLOW}📝 Don't forget to add Go bin to your PATH permanently:${NC}"
    echo ""
    echo "  Add this line to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
    echo "  export PATH=\"\$PATH:$(go env GOPATH)/bin\""
    echo ""
fi

echo -e "${GREEN}✨ Development environment setup complete!${NC}"
echo ""
echo "Next steps:"
echo "  • Your git hooks are now active"
echo "  • Run 'pre-commit run --all-files' to check all files"
echo "  • Run './scripts/test.sh' to run the full test suite"
echo "  • Check './scripts/README.md' for more information"
