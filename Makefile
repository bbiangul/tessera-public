BIN := bin/tessera
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/bbiangul/tessera/internal/project.Version=$(VERSION)

.PHONY: build image test test-integration test-evals schema lint lint-extra ci clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/tessera

# The Tessera image for this machine's architecture, as tessera:dev
# (a release builds it with goreleaser: .goreleaser.yaml).
image:
	rm -rf build/image
	CGO_ENABLED=0 GOOS=linux GOARCH=$(shell go env GOARCH) go build -ldflags "$(LDFLAGS)" -o build/image/linux/$(shell go env GOARCH)/tessera ./cmd/tessera
	docker build --platform linux/$(shell go env GOARCH) -t tessera:dev -f Dockerfile build/image

test:
	go test -race -count=1 ./...

# Calls real providers. Needs TYPESAFE_API_KEY and/or OPENROUTER_API_KEY;
# tests without their key skip.
test-integration:
	go test -race -count=1 -tags integration ./...

# Conversation tests (RFC-0002 level 4), replayed from recorded Jev answers:
# offline, free, deterministic. Re-record after changing the example or its
# suite: add --cassette-mode record with TYPESAFE_API_KEY set.
test-evals: build
	./$(BIN) test examples/traveler --generate openrouter:deepseek/deepseek-v4.1-flash \
		--cassette bench/cassettes/traveler-evals.jsonl --cassette-mode replay

# Regenerate JSON Schemas from the API types (TestSchemasAreUpToDate fails
# when this was forgotten).
schema:
	go run ./internal/tools/genschema

lint: build
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	./scripts/check-file-size.sh
	./$(BIN) validate examples

# Pinned analyzers; staticcheck v0.7.0 is the newest that runs on Go 1.25.
lint-extra:
	GOTOOLCHAIN=local go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
	go run github.com/kisielk/errcheck@v1.20.0 -exclude .errcheck-excludes ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

ci: build lint test test-evals

clean:
	rm -rf bin
