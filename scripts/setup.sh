#!/bin/bash
# CloudPull Setup Script

set -e

echo "🚀 CloudPull Setup"
echo "=================="

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21+ from https://golang.org/dl/"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
MIN_VERSION="1.21"
if [ "$(printf '%s\n' "$MIN_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_VERSION" ]; then
    echo "❌ Go version $GO_VERSION is too old. Please install Go 1.21+"
    exit 1
fi

echo "✅ Go $GO_VERSION detected"

# Download dependencies
echo "📦 Downloading dependencies..."
go mod download
go mod verify

# Build CloudPull
echo "🔨 Building CloudPull..."
make build

echo ""
echo "✨ Setup complete!"
echo ""
echo "Next steps:"
echo "1. Run './build/cloudpull init' to set up authentication"
echo "2. Run './build/cloudpull sync <folder-id> <local-path>' to start syncing"
echo ""
echo "For more help, run './build/cloudpull --help'"
