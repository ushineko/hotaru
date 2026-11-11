# hotaru — see specs/001-scope-migration-and-lighting-core.md
#
# Tests need no hardware, no OpenRGB, no liquidctl and no root. That is a
# packaging property as much as a testing one: a PKGBUILD's check() runs this.

TAG        := $(shell cat .tag)
MODULE     := github.com/ushineko/hotaru

# A build that is not the release says so.
#
# .tag is the version the release flow sets, and stamping it unconditionally
# meant a build from a working tree reported the same string as the package --
# so a window open beside a terminal could not be told apart from the one
# pacman installed, which is exactly the confusion a dev build creates.
#
# Released is HEAD sitting exactly on this version's tag with nothing
# modified. Anything else is `0.1.3-1a2b3c4-dev`, which names the commit it
# came from. The PKGBUILD stamps pkgver itself and never runs this, so a
# package is always the plain version.
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null)
DIRTY      := $(shell git status --porcelain 2>/dev/null | head -1)
ATTAG      := $(shell git describe --exact-match --tags --match 'v$(TAG)' HEAD 2>/dev/null)
VERSION    := $(if $(and $(ATTAG),$(if $(DIRTY),,x)),$(TAG),$(TAG)-$(COMMIT)-dev)

LDFLAGS    := -X $(MODULE)/internal/version.Version=$(VERSION)
GOFLAGS    := -trimpath
# `.exe` on Windows, empty elsewhere. `go build -o` writes the name it is
# given, and Windows runs a binary by name only with the suffix.
EXE        := $(shell go env GOEXE)

# The window is built without Fyne's thread-safety check.
#
# Fyne calls EnsureMain on every canvas refresh, and to find out which
# goroutine it is on it calls runtime.Stack -- which formats a whole traceback
# and then keeps the first thirty bytes of it to parse the ID out. A CPU
# profile of a 25-second window drag put that at 82% of all samples.
#
# The check exists for programs that have not moved to fyne.Do. This one has:
# internal/gui/thread_test.go walks the AST and fails the build if anything
# inside a Perform callback touches the interface unwrapped. The tag is only
# safe because of that test, and removing either means removing both.
GUITAGS    := -tags migrated_fynedo
# On Windows the window is a program with a window, not a console program:
# started from the Start Menu it opens no console beside it (spec 072 R6.3).
GUILDFLAGS := $(LDFLAGS)$(if $(filter windows,$(shell go env GOOS)), -H windowsgui)
# The Windows installer is built by fynstall (github.com/ushineko/fynstall)
# from fynstall.yaml, and an installer is pinned to the fynstall that built
# it. Run with `go run` at the pinned release, so nothing is installed. A
# build tool, not a dependency of either program, so go.mod does not name it.
FYNSTALL_VERSION := v0.2.7
FYNSTALL   := go run github.com/ushineko/fynstall/cmd/fynstall@$(FYNSTALL_VERSION)
# Pinned, like the other ushineko repositories: a linter that moves under you
# turns an unrelated commit into a day of style changes.
LINT_VERSION       := v2.12.2
GOLANGCI           := $(HOME)/go/bin/golangci-lint-$(LINT_VERSION)
# The linter is a Go program, and it can only parse source its own toolchain
# understands. Pinned to what it was built with, or a newer local Go makes it
# panic on files it thinks are from the future.
LINT_GO_TOOLCHAIN  ?= go1.26.0

.PHONY: all test race lint lint-windows vuln build gui install installer generate screenshots tidy clean

all: test build

test:
	go test ./...

race:
	go test -race ./...

lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint:
	@go version
	$(GOLANGCI) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

# The files only Windows builds, checked from whichever system this runs on.
# A linter sees one system's files at a time, so `lint` alone never reads a
# _windows.go file on Linux, or the other way about.
#
# Without the window: it needs cgo, and a cross-build has none. The linter
# takes directories, not import paths, so the module prefix becomes `.`.
lint-windows: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint-windows: export GOOS = windows
lint-windows: export CGO_ENABLED = 0
lint-windows:
	$(GOLANGCI) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml \
		$$(go list -e ./... | grep -v -e /internal/gui -e /cmd/hotaru-gui | sed 's|^$(MODULE)|.|')

vuln:
	govulncheck ./...

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/hotaru$(EXE) ./cmd/hotaru

# The window. Separate from `build` because it needs cgo and the graphics
# stack, which the service deliberately does not: a headless box builds and
# runs everything above without any of this.
gui:
	go build $(GOFLAGS) $(GUITAGS) -ldflags "$(GUILDFLAGS)" -o bin/hotaru-gui$(EXE) ./cmd/hotaru-gui

# The Windows installer, into dist/: the service, the CLI and the window, and
# an uninstaller (spec 072 R6). Windows only; Linux is packaged for the AUR.
# --windowed: the installer and the uninstaller never show a console.
installer: build gui
	@mkdir -p dist
	go run ./scripts/icon packaging/hotaru.svg dist/hotaru.png 512
	$(FYNSTALL) validate
	$(FYNSTALL) build --windowed

# What a developer runs. `go install ./cmd/...` builds the window without
# GUITAGS, which is a window most of whose refresh time is a debug check.
install:
	go install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/hotaru
	go install $(GOFLAGS) $(GUITAGS) -ldflags "$(LDFLAGS)" ./cmd/hotaru-gui

# The gallery in docs/gallery.md, recaptured. Starts a window per image and
# takes over the screen for about a minute, so it is a target somebody runs
# deliberately rather than part of a build. Needs kdotool, spectacle and
# Pillow, a KDE/Wayland session, and the service running: the window is a
# client, and a picture of "the service is not running" is a picture of a
# machine nobody has.
screenshots: gui
	tools/screenshot.sh --all

# The README's mermaid diagrams, rendered to the PNGs the window embeds.
# Build-time only: a program that drew a diagram at runtime would need node,
# a browser and the network to draw a box with an arrow in it. Needs mmdc
# (mermaid-cli) on PATH; the test suite fails when a diagram and its image
# have drifted apart.
generate:
	go generate ./...

tidy:
	go mod tidy

clean:
	rm -rf bin
