#!/usr/bin/env bash
# Vendor the dependency-free shared Prettier configuration from
# jrmoulckers/engineering at the ref pinned in .engineering-ref.
#
# Why vendored rather than installed: GitHub Packages authenticates every read,
# including reads of a public package, so `npm install @jrmoulckers/prettier-config`
# requires a classic PAT with read:packages for every contributor and for CI.
# Measured against the registry from this machine:
#
#   anonymous            -> 401   (no credential)
#   gh token, no scope   -> 403   (credential, not authorised)
#
# 401 and 403 are different failures and only the second one means "you need a
# grant". The package has no runtime dependencies, so fetching the files over
# plain HTTPS from the public repository avoids the token entirely.
#
# @jrmoulckers/eslint-config is deliberately NOT vendored: upstream documents
# that it depends on @eslint/js, typescript-eslint, eslint-config-prettier and
# globals at runtime, so copying it would push four version choices back onto
# this repository. That one needs the registry, and it is not adopted here.
#
# Usage:
#   scripts/vendor-engineering-configs.sh                  # fetch and write the lock
#   scripts/vendor-engineering-configs.sh --check          # verify, change nothing
#   scripts/vendor-engineering-configs.sh --notify-drift   # advisory only, never fails

set -euo pipefail

REPO="jrmoulckers/engineering"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

DEST="config/engineering/prettier-config"
LOCK="engineering-configs.lock.json"
FILES=(index.js index.d.ts svelte.js svelte.d.ts)

MODE="fetch"
case "${1:-}" in
  "")              MODE="fetch" ;;
  --check)         MODE="check" ;;
  --notify-drift)  MODE="notify" ;;
  # An unrecognised flag must not fall through to fetch. `--chekc` silently
  # rewriting the vendored tree is the failure that looks like success: the
  # command exits 0 and the files it was asked to merely inspect are replaced.
  *) echo "error: unknown argument '${1}' (expected --check or --notify-drift)" >&2; exit 2 ;;
esac

[ -f .engineering-ref ] || { echo "error: .engineering-ref is missing; it is the single pin" >&2; exit 1; }
REF="$(tr -d ' \t\r\n' < .engineering-ref)"
[ -n "${REF}" ] || { echo "error: .engineering-ref is empty" >&2; exit 1; }

# The declarations do not exist below v0.112.0 -- the fetch 404s rather than
# degrading, which is the good direction, but the floor is named here so the
# failure explains itself instead of looking like a network fault.
MIN_REF="v0.112.0"
lowest="$(printf '%s\n%s\n' "${REF}" "${MIN_REF}" | sort -V | head -1)"
if [ "${REF}" != "${MIN_REF}" ] && [ "${lowest}" = "${REF}" ]; then
  echo "error: .engineering-ref is ${REF}, below the ${MIN_REF} floor where" >&2
  echo "       packages/prettier-config declarations first exist." >&2
  exit 1
fi

hash_of() { sha256sum "$1" | cut -d' ' -f1; }

if [ "${MODE}" = "notify" ]; then
  # Advisory. Every path below exits 0: a tag published upstream must not redden
  # an unrelated PR, and neither must a rate limit or a DNS failure.
  tmpn="$(mktemp -d)"
  trap 'rm -rf "${tmpn}"' EXIT
  # Anonymous api.github.com is rate limited per IP, and Actions runners share
  # addresses, so the unauthenticated call fails often enough that this step
  # would be blind most of the time. The workflow's own GITHUB_TOKEN is enough to
  # read tags of a public repository; it is optional so the script still runs
  # locally without one.
  auth=()
  tok="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
  [ -n "${tok}" ] && auth=(-H "Authorization: Bearer ${tok}")
  tags="$(curl -sSL "${auth[@]}" "https://api.github.com/repos/${REPO}/tags?per_page=100" 2>/dev/null |
          grep -o '"name": *"v[0-9][^"]*"' | sed 's/.*"v/v/; s/"$//' || true)"
  if [ -z "${tags}" ]; then
    echo "::notice::could not reach ${REPO} to check for drift; pinned at ${REF}"
    exit 0
  fi
  # Version-sorted, not publication-ordered. `releases/latest` returns the most
  # recently PUBLISHED release, so a patch backported to an older line and
  # published after a newer minor is reported as latest -- which advises every
  # consumer to move their pin backwards, confidently and wrongly.
  latest="$(printf '%s\n' "${tags}" | sort -V | tail -1)"
  newer="$(printf '%s\n%s\n' "${REF}" "${latest}" | sort -V | tail -1)"
  if [ "${newer}" = "${REF}" ]; then
    echo "pin ${REF} is at or above the newest tag ${latest}; nothing to report"
    exit 0
  fi
  # Compare bytes, not tags. Most releases here touch none of these files, and a
  # notice that fires on every release is one you stop reading -- at which point
  # it hides the release that mattered.
  drifted=""
  unreachable=""
  for f in "${FILES[@]}"; do
    url="https://raw.githubusercontent.com/${REPO}/${latest}/packages/prettier-config/${f}"
    # Fetch to a file and test curl's own exit status. Piping into sha256sum
    # would hash the empty stream on failure, producing a real-looking hash that
    # differs from ours and reporting drift on a network blip.
    if ! curl -fsSL -o "${tmpn}/${f}" "${url}" 2>/dev/null; then
      unreachable="${unreachable} ${f}"
      continue
    fi
    new_sum="$(hash_of "${tmpn}/${f}")"
    cur_sum="$(hash_of "${DEST}/${f}" 2>/dev/null || echo missing)"
    [ "${new_sum}" = "${cur_sum}" ] || drifted="${drifted} ${f}"
  done
  if [ -n "${unreachable}" ]; then
    echo "::notice::could not fetch${unreachable} from ${latest}; drift not determined"
  fi
  if [ -n "${drifted}" ]; then
    echo "::notice::vendored prettier config is behind: pinned ${REF}, newest ${latest}, changed:${drifted}"
  elif [ -z "${unreachable}" ]; then
    echo "pinned ${REF}, newest ${latest}, but the vendored files are byte-identical; no re-pin needed"
  fi
  exit 0
fi

if [ "${MODE}" = "check" ]; then
  # A checker that examines nothing prints the same silence as one that examined
  # everything and approved it, so the count is reported and examining nothing is
  # itself a failure.
  [ -f "${LOCK}" ] || { echo "error: ${LOCK} is missing; run without --check to vendor" >&2; exit 1; }
  fail=0
  checked=0
  for f in "${FILES[@]}"; do
    want="$(sed -n "s|.*\"${DEST}/${f}\": *\"\([a-f0-9]\{64\}\)\".*|\1|p" "${LOCK}")"
    if [ -z "${want}" ]; then
      echo "no usable hash recorded for ${DEST}/${f}; nothing can be verified"
      fail=1
      continue
    fi
    if [ ! -f "${DEST}/${f}" ]; then
      echo "missing ${DEST}/${f}, which the lock records"
      fail=1
      continue
    fi
    checked=$((checked + 1))
    got="$(hash_of "${DEST}/${f}")"
    [ "${got}" = "${want}" ] || { echo "content differs from the lock: ${DEST}/${f}"; fail=1; }
  done
  echo "verified ${checked} of ${#FILES[@]} vendored file(s) against ${LOCK} at ${REF}"
  if [ "${checked}" -eq 0 ]; then
    echo "error: verified nothing" >&2
    exit 1
  fi
  [ "${fail}" -eq 0 ] && echo "OK: vendored configuration matches the lock"
  exit "${fail}"
fi

mkdir -p "${DEST}"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# Every file is fetched and validated before any of them is moved into place. A
# partial set is worse than none: prettier would load a config that resolves to
# something other than what the lock describes.
for f in "${FILES[@]}"; do
  url="https://raw.githubusercontent.com/${REPO}/${REF}/packages/prettier-config/${f}"
  code="$(curl -sSL -o "${tmp}/${f}" -w '%{http_code}' "${url}")"
  if [ "${code}" != "200" ]; then
    echo "error: ${url} returned HTTP ${code}" >&2
    echo "       Check that ref '${REF}' exists and is at or above ${MIN_REF}." >&2
    exit 1
  fi
  [ -s "${tmp}/${f}" ] || { echo "error: ${url} returned an empty file" >&2; exit 1; }
done

# Files are written byte-identical to source, with no generated header, so a
# re-run shows exactly what upstream changed and nothing else. Provenance lives
# in the lock instead.
for f in "${FILES[@]}"; do
  mv "${tmp}/${f}" "${DEST}/${f}"
done

# Upstream publishes these as ESM via "type": "module" in its package manifest,
# and vendoring copies the files but leaves the manifest behind. Without this
# marker the files are nominally CommonJS and `export default` is a syntax
# error. Node >=22.7 masks that by retrying a failed CJS parse as ESM, so the
# defect is invisible on a new runtime and fatal on an older one. It is also
# invisible to the hash check: every byte can be correct and the config still
# not load.
cat > "${DEST}/package.json" <<'JSON'
{
  "type": "module"
}
JSON

{
  echo '{'
  echo '  "$comment": "Generated by scripts/vendor-engineering-configs.sh. Do not edit by hand.",'
  echo "  \"repository\": \"${REPO}\","
  echo "  \"ref\": \"${REF}\","
  echo '  "files": {'
  last="${FILES[${#FILES[@]} - 1]}"
  for f in "${FILES[@]}"; do
    sep=","
    [ "${f}" = "${last}" ] && sep=""
    echo "    \"${DEST}/${f}\": \"$(hash_of "${DEST}/${f}")\"${sep}"
  done
  echo '  }'
  echo '}'
} > "${LOCK}"

echo "vendored ${#FILES[@]} file(s) from ${REPO}/packages/prettier-config@${REF} into ${DEST}"
echo "wrote ${LOCK}"
