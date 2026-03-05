#!/bin/bash

# Test runner script for LatticeLM Gateway
# Usage: ./run-tests.sh [option]
#
# Options:
#   all       - Run all tests (default)
#   coverage  - Run tests with coverage report
#   verbose   - Run tests with verbose output
#   config    - Run config tests only
#   providers - Run provider tests only
#   conv      - Run conversation tests only
#   watch     - Watch mode (requires entr)

set -e

COLOR_GREEN='\033[0;32m'
COLOR_BLUE='\033[0;34m'
COLOR_YELLOW='\033[1;33m'
COLOR_RED='\033[0;31m'
COLOR_RESET='\033[0m'

print_header() {
    echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
    echo -e "${COLOR_BLUE}$1${COLOR_RESET}"
    echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
}

print_success() {
    echo -e "${COLOR_GREEN}✓ $1${COLOR_RESET}"
}

print_error() {
    echo -e "${COLOR_RED}✗ $1${COLOR_RESET}"
}

print_info() {
    echo -e "${COLOR_YELLOW}ℹ $1${COLOR_RESET}"
}

run_all_tests() {
    print_header "Running All Tests"
    go test ./internal/... || exit 1
    print_success "All tests passed!"
}

run_verbose_tests() {
    print_header "Running Tests (Verbose)"
    go test ./internal/... -v || exit 1
    print_success "All tests passed!"
}

run_coverage_tests() {
    print_header "Running Tests with Coverage"
    go test ./internal/... -cover -coverprofile=coverage.out || exit 1
    print_success "Tests passed! Generating HTML report..."
    go tool cover -html=coverage.out -o coverage.html
    print_success "Coverage report generated: coverage.html"
    print_info "Open coverage.html in your browser to view detailed coverage"
}

run_config_tests() {
    print_header "Running Config Tests"
    go test ./internal/config -v -cover || exit 1
    print_success "Config tests passed!"
}

run_provider_tests() {
    print_header "Running Provider Tests"
    go test ./internal/providers/... -v -cover || exit 1
    print_success "Provider tests passed!"
}

run_conversation_tests() {
    print_header "Running Conversation Tests"
    go test ./internal/conversation -v -cover || exit 1
    print_success "Conversation tests passed!"
}

run_watch_mode() {
    if ! command -v entr &> /dev/null; then
        print_error "entr is not installed. Install it with: apt-get install entr"
        exit 1
    fi
    print_header "Running Tests in Watch Mode"
    print_info "Watching for file changes... (Press Ctrl+C to stop)"
    find ./internal -name '*.go' | entr -c sh -c 'go test ./internal/... || true'
}

# Main script
case "${1:-all}" in
    all)
        run_all_tests
        ;;
    coverage)
        run_coverage_tests
        ;;
    verbose)
        run_verbose_tests
        ;;
    config)
        run_config_tests
        ;;
    providers)
        run_provider_tests
        ;;
    conv)
        run_conversation_tests
        ;;
    watch)
        run_watch_mode
        ;;
    *)
        echo "Usage: $0 {all|coverage|verbose|config|providers|conv|watch}"
        echo ""
        echo "Options:"
        echo "  all       - Run all tests (default)"
        echo "  coverage  - Run tests with coverage report"
        echo "  verbose   - Run tests with verbose output"
        echo "  config    - Run config tests only"
        echo "  providers - Run provider tests only"
        echo "  conv      - Run conversation tests only"
        echo "  watch     - Watch mode (requires entr)"
        exit 1
        ;;
esac
