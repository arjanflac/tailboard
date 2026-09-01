VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO_BUILD_FLAGS := -trimpath -buildvcs=false
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
.PHONY: all tailboard-engine tailboard-engine-app tailboard tg-clipd tg-clip test test-race lint clean

all: tailboard-engine tailboard

tailboard-engine:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o "bin/Tailboard Engine" ./cmd/tg-clipd

tailboard-engine-app: tailboard-engine
	./scripts/prepare-macos-engine-bundle.sh

tailboard:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/tailboard ./cmd/tg-clip

tg-clipd:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/tg-clipd ./cmd/tg-clipd

tg-clip:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/tg-clip ./cmd/tg-clip

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin/ dist/ .tailboard-build/
