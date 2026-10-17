BINARY := fz
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint vuln install man clean tidy

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
	cp $(BINARY) $(HOME)/.local/bin/$(BINARY)

man: build ## Write the man page to ~/.local/share/man
	./$(BINARY) man --dir $(HOME)/.local/share/man

tidy: ## Tidy go.mod and go.sum
	go mod tidy

clean:
	rm -f $(BINARY)
	rm -rf dist
