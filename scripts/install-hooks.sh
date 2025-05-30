#!/usr/bin/env bash
# Install pre-commit hooks for CloudPull

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "CloudPull Pre-commit Hook Installer"
echo "==================================="
echo ""

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
  echo "Error: Not in a git repository"
  exit 1
fi

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Function to install pre-commit
install_precommit() {
  echo "Installing pre-commit..."
  
  if command_exists pip3; then
    pip3 install --user pre-commit
  elif command_exists pip; then
    pip install --user pre-commit
  elif command_exists brew; then
    brew install pre-commit
  else
    echo "Error: Could not find pip, pip3, or brew to install pre-commit"
    echo "Please install pre-commit manually: https://pre-commit.com/#install"
    exit 1
  fi
}

# Function to install golangci-lint
install_golangci_lint() {
  echo "Installing golangci-lint..."
  
  # Try to install using go install
  if command_exists go; then
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  elif command_exists brew; then
    brew install golangci-lint
  else
    echo "Error: Could not install golangci-lint"
    echo "Please install manually: https://golangci-lint.run/usage/install/"
    exit 1
  fi
}

# Function to install gosec
install_gosec() {
  echo "Installing gosec..."
  
  if command_exists go; then
    go install github.com/securego/gosec/v2/cmd/gosec@latest
  else
    echo "Warning: Could not install gosec (Go not found)"
    echo "Security checks will be skipped"
  fi
}

# Function to install markdownlint
install_markdownlint() {
  echo "Installing markdownlint..."
  
  if command_exists gem; then
    gem install --user-install mdl
  elif command_exists brew; then
    brew install markdownlint
  else
    echo "Warning: Could not install markdownlint"
    echo "Markdown linting will be skipped"
  fi
}

# Function to install shellcheck
install_shellcheck() {
  echo "Installing shellcheck..."
  
  if command_exists brew; then
    brew install shellcheck
  elif command_exists apt-get; then
    sudo apt-get update && sudo apt-get install -y shellcheck
  elif command_exists yum; then
    sudo yum install -y shellcheck
  else
    echo "Warning: Could not install shellcheck"
    echo "Shell script linting will be skipped"
  fi
}

# Check and install dependencies
echo "Checking dependencies..."
echo ""

# Check for pre-commit
if ! command_exists pre-commit; then
  echo "pre-commit not found"
  install_precommit
else
  echo "✓ pre-commit is installed"
fi

# Check for golangci-lint
if ! command_exists golangci-lint; then
  echo "golangci-lint not found"
  install_golangci_lint
else
  echo "✓ golangci-lint is installed"
fi

# Check for gosec
if ! command_exists gosec; then
  echo "gosec not found"
  install_gosec
else
  echo "✓ gosec is installed"
fi

# Check for markdownlint
if ! command_exists mdl; then
  echo "markdownlint not found"
  install_markdownlint
else
  echo "✓ markdownlint is installed"
fi

# Check for shellcheck
if ! command_exists shellcheck; then
  echo "shellcheck not found"
  install_shellcheck
else
  echo "✓ shellcheck is installed"
fi

echo ""
echo "Installing pre-commit hooks..."

# Install the pre-commit hooks
cd "$PROJECT_ROOT"
pre-commit install
pre-commit install --hook-type commit-msg

# Create markdownlint config if it doesn't exist
if [ ! -f "$PROJECT_ROOT/.markdownlint.yaml" ]; then
  echo "Creating markdownlint configuration..."
  cat > "$PROJECT_ROOT/.markdownlint.yaml" << 'EOF'
# Markdownlint configuration
# https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md

# Default state for all rules
default: true

# Path to configuration file to extend
extends: null

# MD013/line-length - Line length
MD013:
  # Number of characters
  line_length: 120
  # Number of characters for headings
  heading_line_length: 80
  # Number of characters for code blocks
  code_block_line_length: 120
  # Include tables
  tables: true
  # Include headings
  headings: true
  # Strict length checking
  strict: false
  # Stern length checking
  stern: false

# MD033/no-inline-html - Inline HTML
MD033:
  # Allowed elements
  allowed_elements:
    - br
    - p
    - a
    - img
    - details
    - summary

# MD041/first-line-heading - First line in file should be a heading
MD041: false

# MD024/no-duplicate-heading - Multiple headings with the same content
MD024:
  # Only check sibling headings
  siblings_only: true
EOF
fi

# Create .gitignore entries if needed
if ! grep -q "coverage.out" "$PROJECT_ROOT/.gitignore" 2>/dev/null; then
  echo "" >> "$PROJECT_ROOT/.gitignore"
  echo "# Test coverage" >> "$PROJECT_ROOT/.gitignore"
  echo "coverage.out" >> "$PROJECT_ROOT/.gitignore"
  echo "coverage.html" >> "$PROJECT_ROOT/.gitignore"
fi

# Run pre-commit on all files to check current state
echo ""
echo "Running initial checks..."
echo ""

if pre-commit run --all-files; then
  echo ""
  echo "✓ All checks passed!"
else
  echo ""
  echo "⚠ Some checks failed. Please fix the issues above."
  echo "You can run 'pre-commit run --all-files' to re-check."
fi

echo ""
echo "Installation complete!"
echo ""
echo "Pre-commit hooks are now active. They will run automatically before each commit."
echo ""
echo "Useful commands:"
echo "  pre-commit run --all-files  # Run all hooks on all files"
echo "  pre-commit run <hook-id>   # Run a specific hook"
echo "  pre-commit autoupdate       # Update hook versions"
echo ""
echo "To bypass hooks temporarily (not recommended):"
echo "  git commit --no-verify"
echo ""