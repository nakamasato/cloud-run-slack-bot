.PHONY: test test-verbose test-coverage build clean

# Default target
all: test build

# Run tests
test:
	go test -v ./...

# Run tests with verbose output
test-verbose:
	go test -v -count=1 ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Build the application
build:
	go build -o cloud-run-slack-bot

# Clean build artifacts
clean:
	rm -f cloud-run-slack-bot
	rm -f coverage.out
	rm -f coverage.html

# Run linter
lint:
	golangci-lint run

# Run tests for a specific package
test-pkg:
	@if [ "$(pkg)" = "" ]; then \
		echo "Usage: make test-pkg pkg=<package_name>"; \
		exit 1; \
	fi
	go test -v ./$(pkg)/...

# Run tests with race detection
test-race:
	go test -race ./...

# Run tests with benchmarks
test-bench:
	go test -bench=. ./...

# Run tests with benchmarks and memory allocation statistics
test-bench-mem:
	go test -bench=. -benchmem ./...
