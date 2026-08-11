# Development tasks for game-library.
#
# The lint target exists to make the fetch step unskippable. golangci-lint
# silently falls back to its built-in defaults when no configuration file is
# found -- it reports "0 issues" and exits 0 -- so a contributor who has not run
# the fetch script gets a green result from a weaker rule set than CI enforces.
# Depending lint on the fetch removes the ordering mistake, and naming the file
# with --config turns a missing config into a hard error rather than a silent
# downgrade. See scripts/fetch-engineering-config.sh.

GO ?= go
GOLANGCI_LINT ?= golangci-lint
CONFIG := .golangci.yml
# Shell-dependent recipes are invoked through `bash -c` explicitly. On Windows
# make runs simple commands via CreateProcess without a shell, so neither a
# shebang script nor `rm` resolves; this repository is built and tested on
# Windows as well as Linux.

.PHONY: all lint lint-config test build fmt tidy clean

all: lint test

# The config depends on the fetch script, not merely on its own existence: the
# pinned ref lives in the script, so editing the pin must invalidate a config
# fetched at the old one. Without this, `make lint` would keep using a stale
# config after a re-pin.
$(CONFIG): scripts/fetch-engineering-config.sh
	bash -c 'scripts/fetch-engineering-config.sh'

lint-config: $(CONFIG)

lint: $(CONFIG)
	$(GOLANGCI_LINT) run --config $(CONFIG) ./...

test:
	$(GO) test ./...

build:
	$(GO) build ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

# The config is generated and gitignored, so removing it is safe; the next
# lint run re-fetches it at the pinned ref.
clean:
	bash -c 'rm -f $(CONFIG)'
