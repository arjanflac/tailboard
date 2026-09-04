VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO_BUILD_FLAGS := -trimpath -buildvcs=false
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
.PHONY: all tailboard-engine tailboard-engine-app tailboard test test-race lint clean

all: tailboard-engine tailboard

tailboard-engine:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o "bin/Tailboard Engine" ./cmd/tailboard-engine

tailboard-engine-app: tailboard-engine
	./scripts/prepare-macos-engine-bundle.sh

tailboard:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/tailboard ./cmd/tailboard

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin/ dist/ .tailboard-build/
