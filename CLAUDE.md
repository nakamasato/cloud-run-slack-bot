# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Test/Lint Commands
- Build: `go build -o cloud-run-slack-bot`
- Run all tests: `go test -v ./...`
- Run tests for specific package: `go test -v ./pkg/package_name/...` or `make test-pkg pkg=pkg/package_name`
- Run single test: `go test -v ./pkg/package/file_test.go -run TestFunctionName`
- Lint: `golangci-lint run` or `make lint`
- Test with coverage: `make test-coverage`

## Code Style Guidelines
- Imports: Group standard library, third-party, and project imports with blank line separators
- Error handling: Check errors immediately and provide context in error messages
- Naming: Use CamelCase for exported names, camelCase for internal names
- Error returns: Return errors as the last return value
- Environment variables: Validate required env vars at startup with helpful error messages
- Testing: Use table-driven tests with descriptive test case names
- Context: Pass context.Context as the first parameter to functions that perform I/O
- Package structure: Organize code by domain functionality in pkg/ directory
