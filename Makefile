BINARY  := thop
INSTALL := /usr/local/bin/$(BINARY)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build install uninstall vet lint test clean release

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/thop

install: build
	mv $(BINARY) $(INSTALL)

uninstall:
	rm -f $(INSTALL)

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test -race ./...

clean:
	rm -f $(BINARY)

release:
ifndef version
	$(error version is required, e.g., 'make release version=0.4.1')
endif
ifneq ($(shell git status --porcelain),)
	$(error Git working directory is dirty. Please commit or stash your changes before releasing.)
endif
	nix run nixpkgs#nix-update -- --flake --version $(version) default
	git add flake.nix flake.lock
	git commit -m "chore: release v$(version)"
	git tag v$(version)
	@echo "\nRelease v$(version) prepared and tagged locally."
	@echo "Run the following to publish to GitHub:\n"
	@echo "    git push origin main --tags\n"
