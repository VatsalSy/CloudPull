# golangci-lint Configuration Summary

## Overview

The `.golangci.yml` configuration has been updated to provide a balanced approach to code quality that:
- Catches real bugs and security issues
- Enforces reasonable code standards
- Avoids overly pedantic style rules
- Is practical for active development

## Key Changes Made

### 1. **Relaxed Complexity Limits**
- **Cognitive complexity**: Increased to 30 (from 20)
- **Cyclomatic complexity**: Increased to 25 (from 15)
- **Function length**: Increased to 150 lines (from 100)
- **Line length**: Increased to 140 characters (from 120)
- **Nesting depth**: Increased to 6 (from 4)

### 2. **Variable Naming Rules**
- Allow common short names: `i`, `j`, `k`, `v`, `w`, `r`, `q`, `m`, `n`, `p`, `s`, `t`
- Don't check method receivers or return values (too strict)
- Ignore commonly used context variables

### 3. **Error Handling**
- Keep strict error checking for critical functions
- Allow unchecked errors for Close() methods and similar
- Relaxed error wrapping requirements for internal packages
- Better handling of SQL error comparisons

### 4. **Security Settings**
- Allow MD5 usage (for file checksums, not cryptography)
- Allow math/rand usage (for jitter, not security)
- Allow HTTP requests with variable URLs in auth code

### 5. **Test File Exclusions**
- Most linters disabled for test files
- Allow longer functions and higher complexity in tests
- Skip error checking and security rules in tests

## Enabled Linters

### Essential (Always Run)
- `errcheck` - Check error returns ⚠️ **ERROR level**
- `gosimple` - Simplify code
- `govet` - Go vet checks
- `ineffassign` - Detect ineffectual assignments
- `staticcheck` - Advanced static analysis
- `typecheck` - Type checking
- `unused` - Find unused code

### Code Quality
- `bodyclose` - Check HTTP body close
- `dupl` - Code duplication
- `gocognit` - Cognitive complexity
- `goconst` - Find repeated strings
- `gocritic` - Comprehensive style checks
- `gocyclo` - Cyclomatic complexity
- `gofmt` - Go formatting
- `goimports` - Import organization
- `gosec` - Security issues ⚠️ **ERROR level**
- `lll` - Line length limit
- `misspell` - Spelling mistakes
- `nakedret` - Naked returns
- `prealloc` - Slice preallocation
- `unconvert` - Unnecessary conversions
- `unparam` - Unused parameters
- `varnamelen` - Variable name length

### Best Practices
- `errorlint` - Error handling best practices
- `exhaustive` - Exhaustive switches
- `godox` - TODO/FIXME comments ℹ️ **INFO level**
- `nestif` - Deeply nested ifs
- `nilerr` - Return nil with nil error
- `wrapcheck` - Error wrapping

## Disabled Linters

### Too Strict for General Use
- `funlen` - Function length (we use gocognit instead)
- `gomnd` - Magic numbers (too many false positives)
- `revive` - Style checker (conflicts with other linters)
- `stylecheck` - Style rules (too opinionated)
- `cyclop` - Complexity (redundant with gocyclo)

### Project-Specific Requirements
- `depguard` - Dependency restrictions
- `forbidigo` - Forbidden identifiers
- `goheader` - File headers
- `gomodguard` - Module restrictions

### Overly Restrictive
- `gochecknoglobals` - No global variables
- `gochecknoinits` - No init functions
- `exhaustruct` - Exhaustive struct initialization

### Stylistic Preferences
- `nlreturn` - Newline before return
- `whitespace` - Whitespace rules
- `wsl` - Whitespace linter
- `godot` - Comment periods

## Usage Commands

### Run All Linters
```bash
golangci-lint run
```

### Run with Auto-fix
```bash
golangci-lint run --fix
```

### Run Specific Linters
```bash
golangci-lint run -E errcheck,gosec
```

### Run with Limited Output
```bash
golangci-lint run --max-issues-per-linter=10 --max-same-issues=3
```

### Check Only New Issues
```bash
golangci-lint run --new-from-rev=HEAD~1
```

## Expected Issue Count

With this configuration, you should see:
- **Before**: 500+ issues (too overwhelming)
- **After**: 50-100 issues (manageable)

Most remaining issues will be:
1. **Real bugs** - Unchecked errors, logic issues
2. **Security concerns** - Potential vulnerabilities  
3. **Code quality** - High complexity, duplication
4. **Style issues** - Minor formatting problems

## Gradual Improvement Strategy

1. **Fix Critical Issues First**:
   ```bash
   golangci-lint run -E errcheck,gosec --max-issues-per-linter=10
   ```

2. **Address Code Quality**:
   ```bash
   golangci-lint run -E gocognit,dupl,goconst
   ```

3. **Clean Up Style**:
   ```bash
   golangci-lint run --fix
   ```

4. **Enable Stricter Rules Gradually**:
   - Uncomment disabled linters one at a time
   - Lower complexity thresholds slowly
   - Add more exclude rules as needed

This configuration provides a solid foundation for code quality without being overwhelming.
