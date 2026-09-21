# hotaru — see specs/001-scope-migration-and-lighting-core.md
#
# Tests need no hardware, no OpenRGB, no liquidctl and no root. That is a
# packaging property as much as a testing one: a PKGBUILD's check() runs this.

TAG        := $(shell cat .tag)
MODULE     := github.com/ushineko/hotaru
LDFLAGS    := -X $(MODULE)/internal/version.Version=$(TAG)
GOFLAGS    := -trimpath
# Pinned, like the other ushineko repositories: a linter that moves under you
# turns an unrelated commit into a day of style changes.
LINT_VERSION       := v2.12.2
GOLANGCI           := $(HOME)/go/bin/golangci-lint-$(LINT_VERSION)
# The linter is a Go program, and it can only parse source its own toolchain
# understands. Pinned to what it was built with, or a newer local Go makes it
# panic on files it thinks are from the future.
LINT_GO_TOOLCHAIN  ?= go1.26.0

.PHONY: all test race lint vuln build gui tidy clean

all: test build

test:
	go test ./...

race:
	go test -race ./...

lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint:
	@go version
	$(GOLANGCI) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

vuln:
	govulncheck ./...

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/hotaru ./cmd/hotaru

# The window. Separate from `build` because it needs cgo and the graphics
# stack, which the service deliberately does not: a headless box builds and
# runs everything above without any of this.
gui:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/hotaru-gui ./cmd/hotaru-gui

tidy:
	go mod tidy

clean:
	rm -rf bin
