# Agent Instructions

## On each session

- Update documentation
- Update webapp on backend changes
- Update public/bridgeclient if needed
- Add/Update unit tests
- Run `/coderabbit:review`

## Build Commands

- `make all` — Check style, run tests, and build the plugin bundle
- `make check-style` — Run linters (golangci-lint + eslint)
- `make test` — Run server and webapp unit tests
- `make server` — Build the server binary
- `make webapp` — Build the webapp bundle
- `make dist` — Build and bundle the plugin (.tar.gz)
- `make clean` — Remove all build artifacts
- `make mock` — Regenerate mock files

## Quick Checks

- `make check-style` — Run all linters (Go + webapp)
- `make test` — Run all tests (Go + webapp)
- `make dist` — Build all assets
