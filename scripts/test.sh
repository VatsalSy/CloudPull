#!/bin/bash

# CloudPull Comprehensive Test Script
# This script runs all tests with various options and generates reports

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"
COVERAGE_FILE="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/coverage.html"

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

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"
    
    if ! command -v go &> /dev/null; then
        print_msg "$RED" "❌ Go is not installed"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    print_msg "$GREEN" "✅ Go version: $go_version"
    
    # Check if we're in the right directory
    if [ ! -f "${PROJECT_ROOT}/go.mod" ]; then
        print_msg "$RED" "❌ Not in CloudPull project root"
        exit 1
    fi
    
    print_msg "$GREEN" "✅ In CloudPull project root"
}

# Clean up previous test artifacts
cleanup() {
    print_header "Cleaning Up Previous Test Artifacts"
    
    rm -rf "$COVERAGE_DIR"
    mkdir -p "$COVERAGE_DIR"
    
    print_msg "$GREEN" "✅ Cleaned up test artifacts"
}

# Run unit tests
run_unit_tests() {
    print_header "Running Unit Tests"
    
    print_msg "$YELLOW" "Running tests with race detection and coverage..."
    print_msg "$YELLOW" "This may take a few minutes due to rate limiter tests..."
    
    # Run tests with coverage and timeout
    if go test -v -race -coverprofile="$COVERAGE_FILE" -covermode=atomic -timeout 5m ./...; then
        print_msg "$GREEN" "✅ All unit tests passed"
    else
        print_msg "$RED" "❌ Unit tests failed"
        return 1
    fi
}

# Run tests by package with detailed output
run_package_tests() {
    print_header "Running Tests by Package"
    
    local packages=(
        "cmd/cloudpull"
        "internal/api"
        "internal/app"
        "internal/config"
        "internal/errors"
        "internal/logger"
        "internal/state"
        "internal/sync"
        "pkg/progress"
    )
    
    local failed_packages=()
    
    for pkg in "${packages[@]}"; do
        print_msg "$YELLOW" "Testing $pkg..."
        
        if go test -v -race "./$pkg/..." 2>&1 | tee "${COVERAGE_DIR}/${pkg//\//_}.log"; then
            print_msg "$GREEN" "  ✅ $pkg tests passed"
        else
            print_msg "$RED" "  ❌ $pkg tests failed"
            failed_packages+=("$pkg")
        fi
    done
    
    if [ ${#failed_packages[@]} -gt 0 ]; then
        print_msg "$RED" "\n❌ Failed packages:"
        for pkg in "${failed_packages[@]}"; do
            print_msg "$RED" "  - $pkg"
        done
        return 1
    fi
}

# Run benchmark tests
run_benchmarks() {
    print_header "Running Benchmark Tests"
    
    print_msg "$YELLOW" "Running benchmarks..."
    
    # Find and run benchmark tests
    if go test -bench=. -benchmem -run=^$ ./... > "${COVERAGE_DIR}/benchmarks.txt" 2>&1; then
        print_msg "$GREEN" "✅ Benchmarks completed"
        print_msg "$BLUE" "Benchmark results saved to: ${COVERAGE_DIR}/benchmarks.txt"
    else
        print_msg "$YELLOW" "⚠️  No benchmarks found or benchmarks failed"
    fi
}

# Generate coverage report
generate_coverage_report() {
    print_header "Generating Coverage Report"
    
    if [ -f "$COVERAGE_FILE" ]; then
        # Generate HTML coverage report
        go tool cover -html="$COVERAGE_FILE" -o "$COVERAGE_HTML"
        
        # Calculate total coverage
        local total_coverage=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print $3}')
        
        print_msg "$GREEN" "✅ Coverage report generated"
        print_msg "$BLUE" "Total coverage: $total_coverage"
        print_msg "$BLUE" "HTML report: $COVERAGE_HTML"
        
        # Show coverage by package
        print_msg "$YELLOW" "\nCoverage by package:"
        go tool cover -func="$COVERAGE_FILE" | grep -v "total"
    else
        print_msg "$YELLOW" "⚠️  No coverage file found"
    fi
}

# Run linters (if available)
run_linters() {
    print_header "Running Linters"
    
    # Check if golangci-lint is installed
    if command -v golangci-lint &> /dev/null; then
        print_msg "$YELLOW" "Running golangci-lint..."
        
        if golangci-lint run ./... > "${COVERAGE_DIR}/lint.log" 2>&1; then
            print_msg "$GREEN" "✅ Linting passed"
        else
            print_msg "$RED" "❌ Linting issues found"
            print_msg "$YELLOW" "Check ${COVERAGE_DIR}/lint.log for details"
        fi
    else
        print_msg "$YELLOW" "⚠️  golangci-lint not installed, skipping linting"
    fi
    
    # Run go vet
    print_msg "$YELLOW" "Running go vet..."
    if go vet ./... 2>&1 | tee "${COVERAGE_DIR}/vet.log"; then
        print_msg "$GREEN" "✅ go vet passed"
    else
        print_msg "$RED" "❌ go vet found issues"
    fi
}

# Run security checks
run_security_checks() {
    print_header "Running Security Checks"
    
    # Check if gosec is installed
    if command -v gosec &> /dev/null; then
        print_msg "$YELLOW" "Running gosec..."
        
        if gosec -fmt json -out "${COVERAGE_DIR}/security.json" ./... 2>/dev/null; then
            print_msg "$GREEN" "✅ Security scan completed"
            print_msg "$BLUE" "Security report: ${COVERAGE_DIR}/security.json"
        else
            print_msg "$YELLOW" "⚠️  Security issues found or gosec failed"
        fi
    else
        print_msg "$YELLOW" "⚠️  gosec not installed, skipping security scan"
    fi
}

# Test compilation for different platforms
test_cross_compilation() {
    print_header "Testing Cross-Platform Compilation"
    
    local platforms=(
        "darwin/amd64"
        "darwin/arm64"
        "linux/amd64"
        "linux/arm64"
        "windows/amd64"
    )
    
    for platform in "${platforms[@]}"; do
        IFS='/' read -r goos goarch <<< "$platform"
        
        print_msg "$YELLOW" "Building for $goos/$goarch..."
        
        if GOOS=$goos GOARCH=$goarch go build -o /dev/null ./cmd/cloudpull 2>/dev/null; then
            print_msg "$GREEN" "  ✅ $platform build successful"
        else
            print_msg "$RED" "  ❌ $platform build failed"
        fi
    done
}

# Generate test summary
generate_summary() {
    print_header "Test Summary"
    
    local summary_file="${COVERAGE_DIR}/summary.txt"
    
    {
        echo "CloudPull Test Summary"
        echo "====================="
        echo "Date: $(date)"
        echo ""
        echo "Test Results:"
        echo "- Unit Tests: Check logs above"
        echo "- Coverage: $(go tool cover -func="$COVERAGE_FILE" 2>/dev/null | grep total | awk '{print $3}' || echo 'N/A')"
        echo "- Benchmarks: ${COVERAGE_DIR}/benchmarks.txt"
        echo "- Lint Report: ${COVERAGE_DIR}/lint.log"
        echo "- Security Report: ${COVERAGE_DIR}/security.json"
        echo ""
        echo "Reports generated in: $COVERAGE_DIR"
    } > "$summary_file"
    
    cat "$summary_file"
}

# Main execution
main() {
    cd "$PROJECT_ROOT"
    
    print_msg "$BLUE" "CloudPull Comprehensive Test Suite"
    print_msg "$BLUE" "=================================="
    
    check_prerequisites
    cleanup
    
    # Run different test suites
    local exit_code=0
    
    if ! run_unit_tests; then
        exit_code=1
    fi
    
    if ! run_package_tests; then
        exit_code=1
    fi
    
    run_benchmarks
    generate_coverage_report
    run_linters
    run_security_checks
    test_cross_compilation
    generate_summary
    
    # Final result
    echo ""
    if [ $exit_code -eq 0 ]; then
        print_msg "$GREEN" "✅ All tests completed successfully!"
    else
        print_msg "$RED" "❌ Some tests failed. Check the logs above."
    fi
    
    print_msg "$BLUE" "\nTest artifacts saved in: $COVERAGE_DIR"
    
    # Open coverage report if on macOS and tests passed
    if [ $exit_code -eq 0 ] && [ "$(uname)" == "Darwin" ] && [ -f "$COVERAGE_HTML" ]; then
        print_msg "$YELLOW" "\nOpening coverage report in browser..."
        open "$COVERAGE_HTML"
    fi
    
    return $exit_code
}

# Handle script arguments
case "${1:-}" in
    "unit")
        check_prerequisites
        cleanup
        run_unit_tests
        ;;
    "bench")
        check_prerequisites
        cleanup
        run_benchmarks
        ;;
    "coverage")
        check_prerequisites
        cleanup
        run_unit_tests
        generate_coverage_report
        ;;
    "lint")
        check_prerequisites
        run_linters
        ;;
    "security")
        check_prerequisites
        run_security_checks
        ;;
    "quick")
        # Quick test without coverage or extra checks
        check_prerequisites
        print_header "Running Quick Tests"
        print_msg "$YELLOW" "Running tests without coverage (faster)..."
        if go test -short -timeout 2m ./...; then
            print_msg "$GREEN" "✅ Quick tests passed"
        else
            print_msg "$RED" "❌ Quick tests failed"
            exit 1
        fi
        ;;
    "help"|"--help"|"-h")
        echo "CloudPull Test Script"
        echo ""
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  <none>     Run comprehensive test suite"
        echo "  unit       Run unit tests only"
        echo "  bench      Run benchmarks only"
        echo "  coverage   Run tests and generate coverage report"
        echo "  lint       Run linters only"
        echo "  security   Run security checks only"
        echo "  quick      Run quick tests without coverage"
        echo "  help       Show this help message"
        ;;
    *)
        main
        ;;
esac