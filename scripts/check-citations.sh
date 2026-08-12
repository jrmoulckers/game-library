#!/usr/bin/env bash
# Verify that every citation of jrmoulckers/engineering in this repository
# resolves at the single ref pinned in .engineering-ref.
#
# Why this exists: a citation URL carrying a stale tag is still a valid URL. It
# returns 200, renders normally, and reviews clean, so nothing in an ordinary
# toolchain can report it. Before the pin was unified this repository had
# drifted to two refs without anyone noticing, and the drift was not carelessness
# -- one cited document did not exist at the older tag, so adding the citation
# silently required a second pin. One pin makes that case a visible edit to
# .engineering-ref instead of an invisible divergence inside a URL.
#
# Two checks, deliberately separated by what can make them fail:
#
#   ref conformance  offline, deterministic, always fatal. Every citation must
#                    carry exactly the pinned ref.
#   anchor resolution  network. A 404 path or a heading that no longer exists is
#                    fatal; an unreachable network is not, because an offline
#                    runner is not evidence about a citation.
#
# Usage: scripts/check-citations.sh [--anchors]

set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

REPO="jrmoulckers/engineering"
PIN_FILE=".engineering-ref"
WITH_ANCHORS=0
[ "${1:-}" = "--anchors" ] && WITH_ANCHORS=1

[ -f "${PIN_FILE}" ] || { echo "FAIL: ${PIN_FILE} is missing; it is the single pin" >&2; exit 1; }
PIN="$(tr -d ' \t\r\n' < "${PIN_FILE}")"
[ -n "${PIN}" ] || { echo "FAIL: ${PIN_FILE} is empty" >&2; exit 1; }

echo "pinned ref: ${PIN}"

# Only tracked files are considered. Untracked scratch files are not citations,
# and the generated .golangci.yml carries an upstream header that is not ours to
# conform.
mapfile -t hits < <(
  git grep -n -o -E "https://github\.com/${REPO//\//\\/}/blob/[^)\"'[:space:]]+" -- . \
    | grep -v '\\(' \
    | sort -u
)

total="${#hits[@]}"

# A checker that examines nothing prints the same "no problems" as a checker that
# examined everything and approved it. The count is reported on every run, and
# examining nothing is itself a failure, so the two cannot be confused.
if [ "${total}" -eq 0 ]; then
  echo "FAIL: examined 0 citations; the extraction pattern has stopped matching" >&2
  exit 1
fi

# The count guard above only catches the extraction collapsing to nothing. A
# single citation the pattern cannot parse is worse, because it is skipped in
# silence: the total quietly drops from 25 to 24, every remaining citation still
# conforms, and the run passes. Re-pinning rewrites all of these URLs at once,
# so a malformed rewrite is exactly the plausible way to produce one.
#
# So count blob URLs a second time with a deliberately permissive pattern and
# require the two to agree. The only legitimate difference is the ref-extraction
# regex in the CI workflow, which contains a literal `\(` and is not a citation.
#
# git grep selects the lines but does not do the counting: `git grep -o -E` with
# a pattern ending at a literal `/` matches nothing here, while the same pattern
# extended by one character class matches every occurrence. Plain grep agrees
# with itself.
mention_lines="$(git grep -h -E "github\.com/${REPO}" -- . || true)"
loose=$(printf '%s\n' "${mention_lines}" | grep -o -E "https://github\.com/${REPO}/blob/" | wc -l)
parsed=$(printf '%s\n' "${mention_lines}" | grep -o -E "https://github\.com/${REPO}/blob/[^)\"'[:space:]]+" | grep -vc '\\(' || true)
excluded=$(printf '%s\n' "${mention_lines}" | grep -o -E "https://github\.com/${REPO}/blob/[^)\"'[:space:]]+" | grep -c '\\(' || true)
unaccounted=$(( loose - parsed - excluded ))
if [ "${unaccounted}" -ne 0 ]; then
  echo "FAIL: ${unaccounted} citation URL(s) are present but unparseable, so they would be skipped without being reported" >&2
  echo "      ${loose} blob URL(s) found, ${parsed} parsed as citations, ${excluded} deliberately excluded" >&2
  exit 1
fi

fail=0
declare -a targets=()

for hit in "${hits[@]}"; do
  loc="${hit%%:https://*}"
  url="https://${hit#*:https://}"
  rest="${url#https://github.com/${REPO}/blob/}"
  ref="${rest%%/*}"
  path_and_anchor="${rest#*/}"

  if [ "${ref}" != "${PIN}" ]; then
    echo "STALE REF  ${loc}: cites ${ref}, pin is ${PIN}"
    fail=1
  fi
  targets+=("${path_and_anchor}")
done

echo "checked ${total} citation(s) for ref conformance"

if [ "${WITH_ANCHORS}" -eq 0 ]; then
  [ "${fail}" -eq 0 ] && echo "OK: every citation is at ${PIN}"
  exit "${fail}"
fi

# --- anchor resolution ------------------------------------------------------
# A fragment cannot 404. GitHub serves file.md#nonexistent with status 200, so
# the only way to validate one is to fetch the document, extract its headings,
# apply GitHub's slug algorithm and compare.

slugify() {
  printf '%s' "$1" \
    | sed -E 's/`([^`]*)`/\1/g; s/\*\*([^*]*)\*\*/\1/g; s/\*([^*]*)\*/\1/g' \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9 _-]//g' \
    | sed -E 's/ /-/g'
}

declare -A CACHE
body="$(mktemp)"
trap 'rm -f "${body}"' EXIT
anchors_checked=0
paths_checked=0

for t in $(printf '%s\n' "${targets[@]}" | sort -u); do
  path="${t%%#*}"
  anchor=""
  case "${t}" in *#*) anchor="${t#*#}";; esac

  if [ -z "${CACHE[${path}]+x}" ]; then
    url="https://raw.githubusercontent.com/${REPO}/${PIN}/${path}"
    code="$(curl -sS -o "${body}" -w '%{http_code}' "${url}")"
    curl_status=$?
    if [ "${curl_status}" -ne 0 ]; then
      echo "SKIP: network unreachable while fetching ${path}; anchors not verified"
      exit "${fail}"
    fi
    paths_checked=$((paths_checked + 1))
    if [ "${code}" != "200" ]; then
      echo "MISSING    ${path} -> HTTP ${code} at ${PIN}"
      CACHE[${path}]="__MISSING__"
      fail=1
      continue
    fi
    # Headings inside fenced blocks are not headings; a shell comment in an
    # example is the common case.
    CACHE[${path}]="$(
      awk '
        /^```/ { infence = !infence; next }
        !infence && /^#{1,6}[ \t]+/ { sub(/^#{1,6}[ \t]+/, ""); print }
      ' "${body}" \
        | while IFS= read -r h; do slugify "${h}"; echo; done \
        | paste -sd'|' -
    )"
  fi

  [ "${CACHE[${path}]}" = "__MISSING__" ] && continue
  [ -n "${anchor}" ] || continue

  anchors_checked=$((anchors_checked + 1))
  if ! printf '%s' "${CACHE[${path}]}" | tr '|' '\n' | grep -qxF "${anchor}"; then
    echo "BAD ANCHOR ${path}#${anchor} does not exist at ${PIN}"
    fail=1
  fi
done

echo "fetched ${paths_checked} document(s), resolved ${anchors_checked} anchor(s) at ${PIN}"
[ "${fail}" -eq 0 ] && echo "OK: every citation is at ${PIN} and resolves"
exit "${fail}"
