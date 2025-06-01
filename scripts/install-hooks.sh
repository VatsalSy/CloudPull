#!/usr/bin/env bash
# Install pre-commit hooks for CloudPull

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Ensure Go bin directory is in PATH
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="$PATH:$GOPATH_BIN"

# Ensure Ruby gem bin directory is in PATH
if command -v gem >/dev/null 2>&1; then
  GEM_BIN="$(gem environment gemdir)/bin"
  export PATH="$PATH:$GEM_BIN"
  # Also add user gem path
  USER_GEM_BIN="$(ruby -r rubygems -e 'puts Gem.user_dir')/bin"
  export PATH="$PATH:$USER_GEM_BIN"
fi

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
  # Re-check after installation
  if ! command_exists pre-commit; then
    echo "Error: pre-commit installation failed"
    exit 1
  fi
else
  echo "✓ pre-commit is installed"
fi

# Check for golangci-lint
if ! command_exists golangci-lint; then
  echo "golangci-lint not found"
  install_golangci_lint
  # Re-check after installation
  if ! command_exists golangci-lint; then
    echo "Warning: golangci-lint not in PATH after installation"
    echo "It may be installed at: $(go env GOPATH)/bin/golangci-lint"
  fi
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
  # Re-check after installation
  if ! command_exists mdl; then
    echo "Warning: markdownlint not in PATH after installation"
    echo "You may need to add Ruby gems to your PATH"
  fi
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
  {
    echo ""
    echo "# Test coverage"
    echo "coverage.out"
    echo "coverage.html"
  } >> "$PROJECT_ROOT/.gitignore"
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

# Check if any tools might not be in PATH
if ! command_exists golangci-lint || ! command_exists gosec || ! command_exists mdl; then
  echo "⚠️  Some tools may not be in your PATH. Add these to your shell profile:"
  echo ""
  if ! command_exists golangci-lint || ! command_exists gosec; then
    echo "  export PATH=\"\$PATH:$(go env GOPATH)/bin\""
  fi
  if ! command_exists mdl; then
    echo "  export PATH=\"\$PATH:$(ruby -r rubygems -e 'puts Gem.user_dir')/bin\""
  fi
  echo ""
fi
