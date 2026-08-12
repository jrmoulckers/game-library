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
# The pinned ref is read out of .engineering-ref, the single pin shared with the
# ENG-* citations in the docs. Two literals would let make advertise one ref
# while the script fetched another, which is the exact failure this stamp exists
# to prevent; a ref repeated per citation drifts the same way but silently,
# because a stale citation URL still returns 200.
ENGINEERING_REF ?= $(shell bash -c "tr -d ' \t\r\n' < .engineering-ref")
# The ref is carried in the stamp's *filename*, not its contents. Make compares
# timestamps and never reads a file, so a stamp recording the ref inside itself
# is up to date whatever it says, and asking for a different ref would refetch
# nothing while lint reported success against the old rules. As a filename, a
# new ref is instead a missing prerequisite, which is the condition make acts
# on.
CONFIG_STAMP := .golangci.$(ENGINEERING_REF).ref
# Shell-dependent recipes are invoked through `bash -c` explicitly. On Windows
# make runs simple commands via CreateProcess without a shell, so neither a
# shebang script nor `rm` resolves; this repository is built and tested on
# Windows as well as Linux.

.PHONY: all lint lint-config test build fmt tidy clean

# Without this, a recipe that fails after creating its target leaves the partial
# file behind, and the next run reports "up to date" -- a red build followed by a
# green one against a truncated config. The fetch script already stages to a
# temporary file and moves only after validating, so this is defence in depth
# rather than the primary guard.
.DELETE_ON_ERROR:

all: lint test

# Three separate conditions must refetch: the pin changed (stamp), the fetch
# logic changed (script), and the config is missing (the target itself).
$(CONFIG): scripts/fetch-engineering-config.sh $(CONFIG_STAMP)
	bash -c 'test -n "$(ENGINEERING_REF)" || { echo "make: could not read the pinned ref from .engineering-ref" >&2; exit 1; }'
	bash -c 'ENGINEERING_REF=$(ENGINEERING_REF) scripts/fetch-engineering-config.sh'

# Stamps for other refs are cleared so the directory cannot accumulate a
# misleading history of pins that were never fetched.
$(CONFIG_STAMP):
	bash -c 'rm -f .golangci.*.ref && touch $@'

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
	bash -c 'rm -f $(CONFIG) .golangci.*.ref'
