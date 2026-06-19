# Canonical Go Makefile — fjacquet/ci standard interface (do not rename targets)
.DEFAULT_GOAL := all
BIN  := bin/pmax_exporter
DIST ?= dist
COVER ?= coverage.out
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Pinned tool versions (installed by `make tools`).
# Go 1.26.x: canonical versions (goreleaser v2.16+ requires Go 1.26).
GOLANGCI_VERSION      ?= v2.12.2
GORELEASER_VERSION    ?= v2.16.0
CYCLONEDX_GOMOD_VERSION ?= latest

.PHONY: all clean install tools lint format test build vuln sbom security docs coverage-upload release ci \
        tools-sbom test-race test-coverage vet fmt fmt-check sure release-snapshot docker run-cli clean-dist

all: clean lint test build

clean:
	rm -rf $(DIST) site $(COVER) *.sarif
	rm -f $(BIN)

install:
	go mod download

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

# Just the SBOM generator — used by the release pipeline (GoReleaser sboms hook).
tools-sbom:
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

lint:
	golangci-lint run --timeout=5m

format:
	golangci-lint fmt

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)"

vet:
	go vet ./...

test:
	go test -race -coverprofile=$(COVER) -covermode=atomic ./...

test-race:
	go test -race -cover ./...

test-coverage:
	@mkdir -p $(DIST)
	go test -coverprofile=$(DIST)/coverage.out ./... && go tool cover -func=$(DIST)/coverage.out

build:
	go build -v ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

sbom:
	@mkdir -p $(DIST)
	go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest mod -json -output $(DIST)/sbom.cdx.json

security:
	uvx semgrep scan --config auto --error --skip-unknown-extensions

docs:
	uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict --site-dir site

coverage-upload:
	uvx --from codecov-cli codecov upload-process --file $(COVER) || true

release:
	goreleaser release --clean

# Local dry-run: full pipeline (build, archive, SBOM, checksums) without publishing.
release-snapshot: tools-sbom
	goreleaser release --snapshot --clean
	@echo "release artifacts in $(DIST)/"

# --- repo-specific targets ---

# Compile a single binary with version ldflags.
cli:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

run-cli: cli
	./$(BIN) --config config.yaml --debug

docker:
	docker build -t pmax_exporter:$(VERSION) .

# Convenience gates
sure: fmt-check vet test cli
ci: lint test build vuln

clean-dist:
	rm -rf $(DIST)
