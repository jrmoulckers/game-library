#!/usr/bin/env bash
# Fetch the Engineering-owned golangci-lint configuration into this working tree.
#
# The config is NOT copied into source control: ADR-0003 in jrmoulckers/.github
# forbids one authority vendoring another's normative content. golangci-lint has
# no config inheritance (`extends` is rejected by its v2 schema), so the config is
# materialised at a pinned tag into a gitignored `.golangci.yml` at the repo root.
# Root placement matters: golangci-lint resolves reported paths relative to the
# config file's directory by default, and it lets `golangci-lint run` and editor
# integrations work with no extra flags.
#
# Usage: scripts/fetch-engineering-config.sh [ref]
#
# Auth: jrmoulckers/engineering is private. `gh` must be authenticated locally.
# In CI, set GH_TOKEN to a token with read access to that repository.

set -euo pipefail

REPO="jrmoulckers/engineering"
REF="${1:-${ENGINEERING_REF:-v0.1.0}}"
SRC="configs/golangci.yml"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="${root}/.golangci.yml"
tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI is required to read the private ${REPO} repository" >&2
  exit 1
fi

if ! gh api "repos/${REPO}/contents/${SRC}?ref=${REF}" \
  -H "Accept: application/vnd.github.raw" >"${tmp}"; then
  echo "error: could not read ${REPO}/${SRC}@${REF}." >&2
  echo "       Authenticate with 'gh auth login', or set GH_TOKEN to a token" >&2
  echo "       with read access to ${REPO}." >&2
  exit 1
fi

{
  echo "# GENERATED - do not edit and do not commit."
  echo "# Source: https://github.com/${REPO}/blob/${REF}/${SRC}"
  echo "# Refresh: scripts/fetch-engineering-config.sh"
  cat "${tmp}"
} >"${dest}"

echo "wrote ${dest} from ${REPO}/${SRC}@${REF}"
