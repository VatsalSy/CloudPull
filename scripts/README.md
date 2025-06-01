# CloudPull Scripts

This directory contains scripts for setting up, developing, and maintaining the CloudPull project.

## Quick Start

```bash
# Basic setup (just build the project)
./scripts/setup.sh

# Full developer setup (tools, linters, hooks)
./scripts/setup-dev.sh

# Run tests
./scripts/test.sh
```

## Main Scripts

### setup.sh
Basic project setup for building and running CloudPull.

**Purpose:** Minimal setup to get CloudPull running
- Verifies Go installation (1.21+ required)
- Downloads and verifies dependencies
- Builds the CloudPull binary

**Usage:**
```bash
./scripts/setup.sh
```

**When to use:**
- First time cloning the repository
- Just want to build and use CloudPull
- Don't need development tools

### setup-dev.sh
Complete development environment setup.

**Purpose:** Full developer setup with all tools and hooks
- Everything from `setup.sh` plus:
- Installs Go development tools (golangci-lint, gosec)
- Sets up pre-commit framework and hooks
- Installs additional linters (markdownlint, shellcheck)
- Configures git hooks for code quality

**Usage:**
```bash
./scripts/setup-dev.sh
```

**When to use:**
- Setting up for development work
- Need linting and code quality tools
- Want automatic pre-commit checks

### fix-precommit.sh
Troubleshooting tool for pre-commit issues.

**Purpose:** Fix broken pre-commit configurations
- Reinstalls required tools
- Cleans pre-commit cache
- Reinstalls all hooks
- Tests the configuration

**Usage:**
```bash
./scripts/fix-precommit.sh
```

**When to use:**
- Pre-commit hooks are failing unexpectedly
- After updating pre-commit configuration
- Tools seem to be missing or corrupted

### test.sh
Comprehensive test suite runner.

**Purpose:** Run tests with various options and generate reports

**Usage:**
```bash
./scripts/test.sh [command]
```

**Commands:**
- `<none>` - Run full test suite with coverage
- `unit` - Run unit tests only
- `bench` - Run benchmarks only
- `coverage` - Generate detailed coverage report
- `lint` - Run linters only
- `security` - Run security checks only
- `quick` - Quick tests without coverage
- `help` - Show help message

**Features:**
- Race detection enabled by default
- HTML coverage reports in `coverage/` directory
- Cross-platform compilation testing
- Comprehensive test summary
- Automatic dependency checking

## Git Hooks (hooks/ directory)

Pre-commit hooks that run automatically:

| Hook | Purpose | When it runs |
|------|---------|--------------|
| `check-sensitive.sh` | Scans for API keys, passwords, tokens | Before commit |
| `go-build.sh` | Ensures code compiles | Before commit |
| `go-fmt.sh` | Checks Go code formatting | Before commit |
| `go-generate.sh` | Updates generated code | Before commit |
| `go-mod-tidy.sh` | Keeps dependencies clean | Before commit |
| `go-security.sh` | Scans for security issues | Before commit |
| `go-test.sh` | Runs relevant tests | Before commit |

## Script Dependencies

### Required
- Go 1.21+
- Git

### Optional (installed by setup-dev.sh)
- Python 3 + pip (for pre-commit)
- Node.js + npm (for markdownlint)
- golangci-lint
- gosec
- shellcheck (manual install required)

## Workflow Examples

### New Developer Setup
```bash
# 1. Clone the repository
git clone https://github.com/yourusername/cloudpull.git
cd cloudpull

# 2. Run developer setup
./scripts/setup-dev.sh

# 3. Make your changes, hooks will run automatically
git add .
git commit -m "Your change"
```

### Running Tests Locally
```bash
# Quick test run
./scripts/test.sh quick

# Full test suite with coverage
./scripts/test.sh

# Just linting
./scripts/test.sh lint

# View coverage report
open coverage/coverage.html
```

### Fixing Pre-commit Issues
```bash
# If pre-commit fails
./scripts/fix-precommit.sh

# Run hooks manually on all files
pre-commit run --all-files

# Skip hooks temporarily (not recommended)
git commit --no-verify
```

## Troubleshooting

### "command not found" errors
Add Go binaries to your PATH:
```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Pre-commit hooks failing
1. Run `./scripts/fix-precommit.sh`
2. Check specific tool versions
3. Ensure all dependencies are installed

### Permission denied
Make scripts executable:
```bash
chmod +x scripts/*.sh scripts/hooks/*.sh
```

## Notes

- All scripts are designed to be run from the project root
- Scripts use colored output for better readability
- CI environments are detected and handled appropriately
- Test artifacts are saved in the `coverage/` directory
- Scripts follow strict error handling (`set -e`)
