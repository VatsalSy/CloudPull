#!/bin/bash

# CloudPull Test Dependencies Installer
# This script installs all required testing tools

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored message
print_msg() {
    local color=$1
    local msg=$2
    echo -e "${color}${msg}${NC}"
}

# Print section header
print_header() {
    local msg=$1
    echo ""
    print_msg "$BLUE" "=========================================="
    print_msg "$BLUE" "$msg"
    print_msg "$BLUE" "=========================================="
    echo ""
}

# Install a Go tool
install_go_tool() {
    local tool=$1
    local name
    name=$(echo "$tool" | awk -F'/' '{print $NF}' | awk -F'@' '{print $1}')

    print_msg "$YELLOW" "Installing $name..."

    if go install "$tool"; then
        print_msg "$GREEN" "✅ $name installed successfully"
        return 0
    else
        print_msg "$RED" "❌ Failed to install $name"
        return 1
    fi
}

# Check if a tool is already installed
check_tool() {
    local cmd=$1
    if command -v "$cmd" &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# Main installation function
main() {
    print_header "CloudPull Test Dependencies Installer"

    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_msg "$RED" "❌ Go is not installed. Please install Go first."
        exit 1
    fi

    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    print_msg "$GREEN" "✅ Go version: $go_version"

    # Check Go module support
    if ! go env GOMOD &> /dev/null; then
        print_msg "$YELLOW" "⚠️  Go modules not enabled. Enabling..."
        export GO111MODULE=on
    fi

    print_header "Installing Required Tools"

    # Install golangci-lint
    if check_tool "golangci-lint"; then
        print_msg "$GREEN" "✅ golangci-lint already installed"
    else
        install_go_tool "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2"
    fi

    # Install gosec
    if check_tool "gosec"; then
        print_msg "$GREEN" "✅ gosec already installed"
    else
        install_go_tool "github.com/securego/gosec/v2/cmd/gosec@latest"
    fi

    print_header "Verifying Installations"

    # Verify all tools are accessible
    local all_installed=true

    if check_tool "golangci-lint"; then
        local lint_version
        lint_version=$(golangci-lint --version 2>/dev/null | head -n1)
        print_msg "$GREEN" "✅ golangci-lint: $lint_version"
    else
        print_msg "$RED" "❌ golangci-lint not found in PATH"
        all_installed=false
    fi

    if check_tool "gosec"; then
        local gosec_version
        gosec_version=$(gosec --version 2>/dev/null | head -n1)
        print_msg "$GREEN" "✅ gosec: $gosec_version"
    else
        print_msg "$RED" "❌ gosec not found in PATH"
        all_installed=false
    fi

    echo ""

    # Check if tools are in GOPATH/bin
    local go_bin
    go_bin=$(go env GOPATH)/bin
    print_msg "$BLUE" "Tools installed to: $go_bin"

    # Re-check with explicit path
    if [ -f "$go_bin/golangci-lint" ] && [ -f "$go_bin/gosec" ]; then
        print_msg "$GREEN" "✅ All test dependencies installed successfully!"

        if [[ ":$PATH:" != *":$go_bin:"* ]]; then
            print_msg "$YELLOW" ""
            print_msg "$YELLOW" "⚠️  Note: Add Go's bin directory to your PATH for command-line access:"
            print_msg "$YELLOW" "   export PATH=\$PATH:$go_bin"
            print_msg "$YELLOW" ""
            print_msg "$YELLOW" "The test script will automatically use these tools."
        fi
    else
        if [ "$all_installed" = true ]; then
            print_msg "$GREEN" "✅ All test dependencies installed successfully!"

            # Show PATH reminder if needed
            if [[ ":$PATH:" != *":$go_bin:"* ]]; then
                print_msg "$YELLOW" ""
                print_msg "$YELLOW" "⚠️  Note: Make sure $(go env GOPATH)/bin is in your PATH:"
                print_msg "$YELLOW" "   export PATH=\$PATH:$(go env GOPATH)/bin"
            fi
        else
            print_msg "$RED" "❌ Some tools failed to install or are not in PATH"
            print_msg "$YELLOW" ""
            print_msg "$YELLOW" "Try adding Go's bin directory to your PATH:"
            print_msg "$YELLOW" "   export PATH=\$PATH:$(go env GOPATH)/bin"
            exit 1
        fi
    fi
}

# Run main function
main
