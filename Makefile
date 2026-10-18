BINARY := fz
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
# The build date is the commit time, or SOURCE_DATE_EPOCH when that is set,
# never the clock, so two builds of one commit give the same bytes.
ifeq ($(strip $(SOURCE_DATE_EPOCH)),)
SOURCE_DATE_EPOCH := $(shell git log -1 --format=%ct 2>/dev/null)
endif
ifeq ($(strip $(SOURCE_DATE_EPOCH)),)
SOURCE_DATE_EPOCH := 0
endif
DATE    ?= $(shell date -u -d @$(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r $(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ)
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

# The archives are reproducible too. Every entry gets the build date as its
# mtime, owner and group 0 with no names, and fixed modes. Entries are
# listed by hand in byte order with no recursion, and gzip -n stores no
# name or time. GNU tar and macOS bsdtar spell the owner options
# differently, and bsdtar must be told to leave out macOS metadata.
dist: ## Build release archives and checksums.txt in dist/
	rm -rf $(DIST)
	mkdir -p $(DIST)
	@set -e; \
	if tar --version 2>/dev/null | grep -q 'GNU tar'; then \
		owner="--owner=0 --group=0 --numeric-owner"; \
	else \
		owner="--uid 0 --gid 0 --uname= --gname= --no-mac-metadata --no-xattrs"; \
	fi; \
	for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		echo "build $$name"; \
		mkdir -p $(DIST)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$$name/$(BINARY) ./cmd/fz; \
		cp LICENSE README.md $(DIST)/$$name/; \
		chmod 755 $(DIST)/$$name $(DIST)/$$name/$(BINARY); \
		chmod 644 $(DIST)/$$name/LICENSE $(DIST)/$$name/README.md; \
		touch -d '$(DATE)' $(DIST)/$$name/* $(DIST)/$$name; \
		(cd $(DIST) && COPYFILE_DISABLE=1 tar --format=ustar $$owner --no-recursion -cf $$name.tar \
			$$name $$name/LICENSE $$name/README.md $$name/$(BINARY)); \
		gzip -n -9 $(DIST)/$$name.tar; \
	done
	@cd $(DIST) && if command -v sha256sum >/dev/null 2>&1; then sha256sum *.tar.gz; else shasum -a 256 *.tar.gz; fi > checksums.txt

clean:
	rm -f $(BINARY)
	rm -rf dist
