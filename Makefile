BINARY := fz
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIST := dist
PLATFORMS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint vuln install man dist clean tidy

build: ## Build the fz binary
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/fz

test: ## Run tests with the race detector
	go test -race -count=1 ./...

lint: ## Run go vet
	go vet ./...

vuln: ## Scan for known vulnerabilities
	govulncheck ./...

install: build ## Copy fz to ~/.local/bin
	install -d $(HOME)/.local/bin
	rm -f $(HOME)/.local/bin/$(BINARY)
	cp $(BINARY) $(HOME)/.local/bin/$(BINARY)

man: build ## Write the man page to ~/.local/share/man
	./$(BINARY) man --dir $(HOME)/.local/share/man

tidy: ## Tidy go.mod and go.sum
	go mod tidy

dist: ## Build release archives and checksums.txt in dist/
	rm -rf $(DIST)
	mkdir -p $(DIST)
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		echo "build $$name"; \
		mkdir -p $(DIST)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$$name/$(BINARY) ./cmd/fz; \
		cp LICENSE README.md $(DIST)/$$name/; \
		tar -czf $(DIST)/$$name.tar.gz -C $(DIST) $$name; \
	done
	@cd $(DIST) && if command -v sha256sum >/dev/null 2>&1; then sha256sum *.tar.gz; else shasum -a 256 *.tar.gz; fi > checksums.txt

clean:
	rm -f $(BINARY)
	rm -rf dist
