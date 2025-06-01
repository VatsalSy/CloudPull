#!/usr/bin/env bash
# Go test script for pre-commit

set -e

test_type="${1:-unit}"

echo "Running $test_type tests..."

case "$test_type" in
  unit)
    # Run unit tests (short mode)
    if ! go test -short -race ./...; then
      echo "Unit tests failed!"
      echo "Please fix test failures before committing"
      exit 1
    fi
    ;;

  integration)
    # Run integration tests
    if ! go test -race -run Integration ./...; then
      echo "Integration tests failed!"
      echo "Please fix test failures before committing"
      exit 1
    fi
    ;;

  all)
    # Run all tests
    if ! go test -race ./...; then
      echo "Tests failed!"
      echo "Please fix test failures before committing"
      exit 1
    fi
    ;;

  *)
    echo "Unknown test type: $test_type"
    echo "Valid types: unit, integration, all"
    exit 1
    ;;
esac

echo "$test_type tests passed"
