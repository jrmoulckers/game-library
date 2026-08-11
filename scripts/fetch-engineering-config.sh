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
# jrmoulckers/engineering is public, so this needs no token and no gh CLI.
#
# Usage: scripts/fetch-engineering-config.sh [ref]

set -euo pipefail

REPO="jrmoulckers/engineering"
REF="${1:-${ENGINEERING_REF:-v0.27.0}}"
SRC="configs/golangci.yml"
URL="https://raw.githubusercontent.com/${REPO}/${REF}/${SRC}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="${root}/.golangci.yml"
tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

# Fetching config over the network can silently yield an empty file or an error
# body, and lint would then "pass" against nothing. Every failure mode below is
# fatal, and the destination is only written once the payload is known good.
if ! status="$(curl --silent --show-error --location --max-time 30 \
  --write-out '%{http_code}' --output "${tmp}" "${URL}")"; then
  echo "error: could not reach ${URL}" >&2
  exit 1
fi

if [[ "${status}" != "200" ]]; then
  echo "error: ${URL} returned HTTP ${status}" >&2
  echo "       Check that ref '${REF}' exists in ${REPO}." >&2
  exit 1
fi

if [[ ! -s "${tmp}" ]]; then
  echo "error: ${URL} returned an empty body" >&2
  exit 1
fi

# A 200 carrying an unexpected payload (an HTML error page, a redirect landing
# page) would otherwise reach golangci-lint as a config it cannot parse.
if ! grep -qE '^version:' "${tmp}" || ! grep -qE '^linters:' "${tmp}"; then
  echo "error: ${URL} did not return a golangci-lint configuration" >&2
  echo "       (no top-level 'version:' and 'linters:' keys found)" >&2
  exit 1
fi

{
  echo "# GENERATED - do not edit and do not commit."
  echo "# Source: https://github.com/${REPO}/blob/${REF}/${SRC}"
  echo "# Refresh: scripts/fetch-engineering-config.sh"
  cat "${tmp}"
} >"${dest}"

echo "wrote ${dest} from ${REPO}/${SRC}@${REF}"
