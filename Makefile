# hotaru — see specs/001-scope-migration-and-lighting-core.md
#
# Tests need no hardware, no OpenRGB, no liquidctl and no root. That is a
# packaging property as much as a testing one: a PKGBUILD's check() runs this.

TAG        := $(shell cat .tag)
MODULE     := github.com/ushineko/hotaru
LDFLAGS    := -X $(MODULE)/internal/version.Version=$(TAG)
GOFLAGS    := -trimpath

.PHONY: all test race lint vuln build tidy clean

all: test build

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

vuln:
	govulncheck ./...

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/hotaru ./cmd/hotaru

tidy:
	go mod tidy

clean:
	rm -rf bin
