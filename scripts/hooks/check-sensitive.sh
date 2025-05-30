#!/usr/bin/env bash
# Check for sensitive data in files

set -e

# Patterns to check for sensitive data
patterns=(
  # API keys and tokens
  "api[_-]?key"
  "api[_-]?secret"
  "access[_-]?token"
  "auth[_-]?token"
  "private[_-]?key"
  "secret[_-]?key"
  
  # Credentials
  "password"
  "passwd"
  "pwd"
  "credential"
  
  # Google specific
  "client[_-]?secret"
  "refresh[_-]?token"
  "oauth[_-]?token"
  
  # AWS
  "aws[_-]?access[_-]?key"
  "aws[_-]?secret"
  
  # Other providers
  "github[_-]?token"
  "gitlab[_-]?token"
  
  # Private keys
  "BEGIN RSA PRIVATE KEY"
  "BEGIN DSA PRIVATE KEY"
  "BEGIN EC PRIVATE KEY"
  "BEGIN PRIVATE KEY"
  "BEGIN OPENSSH PRIVATE KEY"
)

# Files to check
files="$@"

if [ -z "$files" ]; then
  exit 0
fi

found_sensitive=false

for file in $files; do
  # Skip binary files
  if file "$file" | grep -q "binary"; then
    continue
  fi
  
  # Skip certain file types
  case "$file" in
    *.json|*.yaml|*.yml|*.env|*.conf|*.config|*.ini)
      # Check these files more carefully
      for pattern in "${patterns[@]}"; do
        if grep -i -E "$pattern\s*[:=]" "$file" 2>/dev/null | grep -v -E "example|sample|test|fake|dummy|xxx|placeholder|\\\$\{.*\}|<.*>"; then
          echo "Potential sensitive data found in $file"
          found_sensitive=true
        fi
      done
      ;;
    *)
      # For other files, just check for obvious patterns
      if grep -i -E "(password|secret|token|key)\s*[:=]\s*[\"'][^\"']+[\"']" "$file" 2>/dev/null | grep -v -E "example|sample|test|fake|dummy|xxx|placeholder"; then
        echo "Potential sensitive data found in $file"
        found_sensitive=true
      fi
      ;;
  esac
done

if [ "$found_sensitive" = true ]; then
  echo ""
  echo "Please review the files above for sensitive data"
  echo "If the data is intentional (e.g., example configs), consider adding it to .gitignore"
  exit 1
fi

echo "No sensitive data detected"