#!/usr/bin/env bash
# make-go.sh — Report (and optionally apply) Go toolchain / module updates
# for Kimia.
#
# Context:
#   - Runtime image does NOT ship a Go toolchain; Go is build-stage only.
#   - Kimia currently uses the Go standard library only (no third-party modules
#     in go.mod). This script still handles modules when they appear later.
#
# Usage:
#   scripts/make-go.sh                 # report toolchain + modules
#   scripts/make-go.sh --apply         # rewrite GO_IMAGE digests in Dockerfiles
#   scripts/make-go.sh --modules       # also run go list -m -u (needs network + go)
#   scripts/make-go.sh --apply-modules # go get -u ./... && go mod tidy
#
# Env:
#   GO_TAG   Override golang tag to pin (default: keep each Dockerfile's
#            major.minor patch series, resolve latest digest for that tag family)
#   APPLY=1  Same as --apply

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APPLY="${APPLY:-0}"
APPLY_MODULES=0
CHECK_MODULES=0
DOCKERFILES=(Dockerfile.buildkit Dockerfile.buildah)
GOMOD="src/go.mod"

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \?//'
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage ;;
    --apply) APPLY=1; shift ;;
    --modules) CHECK_MODULES=1; shift ;;
    --apply-modules) APPLY_MODULES=1; CHECK_MODULES=1; shift ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[ERROR] required command not found: $1" >&2
    exit 1
  }
}

need_cmd docker

get_arg() {
  local file="$1" name="$2"
  grep -E "^ARG ${name}=" "$file" | tail -1 | sed -E "s/^ARG ${name}=//" | sed -E 's/^"//; s/"$//'
}

# Parse golang image ref into tag (without digest)
# e.g. golang:1.26.5-alpine3.24@sha256:abc → 1.26.5-alpine3.24
go_tag_of() {
  local ref="$1"
  local no_digest="${ref%%@*}"
  echo "${no_digest#golang:}"
}

# From a concrete tag like 1.26.5-alpine3.24, prefer floating family tags:
#   1.26.5-alpine3.24  → try 1.26-alpine3.24, 1.26-alpine, then exact
# so we can discover newer patch releases on the same minor.
candidate_tags() {
  local tag="$1"
  local major minor rest alpine_part
  # Patterns: 1.26.5-alpine3.24 | 1.25.3-alpine | 1.26-alpine
  if [[ "$tag" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)(-alpine([0-9.]+)?)$ ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    alpine_part="${BASH_REMATCH[4]}"   # e.g. -alpine3.24 or -alpine
    echo "${major}.${minor}${alpine_part}"
    if [[ "$alpine_part" == -alpine* && "$alpine_part" != "-alpine" ]]; then
      echo "${major}.${minor}-alpine"
    fi
    echo "$tag"
  elif [[ "$tag" =~ ^([0-9]+)\.([0-9]+)(-alpine([0-9.]+)?)$ ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    alpine_part="${BASH_REMATCH[3]}"
    echo "${major}.${minor}${alpine_part}"
    if [[ "$alpine_part" == -alpine* && "$alpine_part" != "-alpine" ]]; then
      echo "${major}.${minor}-alpine"
    fi
    echo "$tag"
  else
    echo "$tag"
  fi
}

# Resolve multi-arch index digest + annotated version for golang:<tag>
inspect_golang() {
  local tag="$1"
  # Returns: digest<TAB>version_annotation
  local out digest version
  if ! out="$(docker buildx imagetools inspect "golang:${tag}" 2>/dev/null)"; then
    return 1
  fi
  digest="$(echo "$out" | awk '/^Digest:/ {print $2; exit}')"
  if [[ -z "$digest" || "$digest" != sha256:* ]]; then
    return 1
  fi
  # Prefer the first org.opencontainers.image.version annotation (index or amd64)
  version="$(echo "$out" | awk '
    /org.opencontainers.image.version:/ {
      gsub(/^[ \t]+/, "", $2); print $2; exit
    }')"
  # Prefer a concrete alpine version string when present later in manifests
  local v2
  v2="$(echo "$out" | awk '
    /org.opencontainers.image.version:/ {
      gsub(/^[ \t]+/, "", $2);
      if ($2 ~ /alpine/) { print $2; exit }
    }')"
  if [[ -n "$v2" ]]; then
    version="$v2"
  fi
  printf '%s\t%s\n' "$digest" "${version:-$tag}"
}

echo ""
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║  Kimia Go toolchain / modules                                    ║"
echo "╚══════════════════════════════════════════════════════════════════╝"

# --- go.mod language version ---
if [[ -f "$GOMOD" ]]; then
  go_lang="$(awk '/^go / {print $2; exit}' "$GOMOD")"
  echo ""
  echo "━━━ Module: $GOMOD ━━━"
  echo "  go directive: ${go_lang:-unknown}"
  # Direct requires (if any)
  if grep -qE '^\s*(require |require \()' "$GOMOD" 2>/dev/null || grep -qE '^\t' "$GOMOD"; then
    req_count="$(awk '/^require \(/,/^\)/ {if ($1 ~ /^[a-zA-Z0-9.]/) c++} /^require [^(]/ {c++} END{print c+0}' "$GOMOD")"
    echo "  direct/indirect requires: ${req_count}"
  else
    echo "  third-party modules: none (stdlib only)"
  fi
else
  echo "[WARN] $GOMOD not found"
fi

# --- GO_IMAGE per Dockerfile ---
TOTAL_STALE=0
TOTAL_OK=0

for df in "${DOCKERFILES[@]}"; do
  [[ -f "$df" ]] || continue
  go_image="$(get_arg "$df" GO_IMAGE)"
  [[ -n "$go_image" ]] || continue

  echo ""
  echo "━━━ $df ━━━"
  echo "  GO_IMAGE (pinned): ${go_image}"

  current_tag="$(go_tag_of "$go_image")"
  current_digest=""
  if [[ "$go_image" == *@sha256:* ]]; then
    current_digest="${go_image##*@}"
  fi

  # Choose lookup tag
  if [[ -n "${GO_TAG:-}" ]]; then
    lookup_tags=("$GO_TAG")
  else
    mapfile -t lookup_tags < <(candidate_tags "$current_tag")
  fi

  best_tag=""
  best_digest=""
  best_version=""
  for t in "${lookup_tags[@]}"; do
    if info="$(inspect_golang "$t")"; then
      best_tag="$t"
      best_digest="$(echo "$info" | cut -f1)"
      best_version="$(echo "$info" | cut -f2)"
      break
    fi
  done

  if [[ -z "$best_digest" ]]; then
    echo "  [WARN] could not resolve latest golang image (is docker buildx available?)"
    continue
  fi

  suggested="golang:${best_version}@${best_digest}"
  # If annotation is a floating tag, fall back to best_tag
  if [[ "$best_version" != *.* ]]; then
    suggested="golang:${best_tag}@${best_digest}"
  fi
  # Prefer concrete version from annotation when present
  if [[ -n "$best_version" && "$best_version" == *alpine* ]]; then
    suggested="golang:${best_version}@${best_digest}"
  fi

  echo "  Lookup tag family: ${lookup_tags[*]}"
  echo "  Latest resolved:   ${suggested}"
  echo "  Hub version:       ${best_version}"

  if [[ -n "$current_digest" && "$current_digest" == "$best_digest" ]]; then
    echo "  Status: OK (digest current)"
    TOTAL_OK=$((TOTAL_OK + 1))
  else
    echo "  Status: STALE (or unpinned digest)"
    TOTAL_STALE=$((TOTAL_STALE + 1))
  fi

  if [[ "$APPLY" -eq 1 ]]; then
    # Pin to concrete version from hub annotation when available
    pin_tag="$best_version"
    if [[ -z "$pin_tag" || "$pin_tag" != *alpine* ]]; then
      pin_tag="$best_tag"
    fi
    new_ref="golang:${pin_tag}@${best_digest}"
    sed -i -E "s|^ARG GO_IMAGE=.*|ARG GO_IMAGE=\"${new_ref}\"|" "$df"
    echo "  Applied GO_IMAGE → ${new_ref}"
  fi
done

# --- modules ---
echo ""
echo "━━━ Go modules (src/) ━━━"
if [[ "$CHECK_MODULES" -eq 1 ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "  [WARN] 'go' not installed on host; using Docker golang image for module check"
    # Use builder image from first Dockerfile
    builder="$(get_arg Dockerfile.buildkit GO_IMAGE 2>/dev/null || true)"
    builder="${builder:-golang:1.26-alpine}"
    if [[ "$APPLY_MODULES" -eq 1 ]]; then
      docker run --rm -v "$ROOT/src:/app" -w /app "$builder" \
        sh -c 'go get -u ./... && go mod tidy'
      echo "  Applied: go get -u ./... && go mod tidy (in container)"
    else
      docker run --rm -v "$ROOT/src:/app" -w /app "$builder" \
        sh -c 'go list -m -u all 2>/dev/null || go list -m all'
    fi
  else
    pushd src >/dev/null
    if [[ "$APPLY_MODULES" -eq 1 ]]; then
      go get -u ./...
      go mod tidy
      echo "  Applied: go get -u ./... && go mod tidy"
    else
      echo "  Available updates (go list -m -u all):"
      go list -m -u all 2>/dev/null || go list -m all
    fi
    popd >/dev/null
  fi
else
  echo "  Skipped (pass --modules or run: make go-deps MODULES=1)"
  echo "  Note: kimia currently has no third-party require directives."
fi

echo ""
echo "━━━ Totals (toolchain images) ━━━"
echo "  OK: ${TOTAL_OK}  STALE: ${TOTAL_STALE}"
if [[ "$APPLY" -eq 0 && "$TOTAL_STALE" -gt 0 ]]; then
  echo ""
  echo "  Re-run with:  make go-deps APPLY=1"
  echo "  to rewrite GO_IMAGE digests in the Dockerfiles."
fi
echo ""
echo "  Tips:"
echo "    make go-deps              # toolchain report"
echo "    make go-deps APPLY=1      # pin latest GO_IMAGE digests"
echo "    make go-deps MODULES=1    # also check module updates"
echo "    make apk                  # Alpine OS packages (separate plane)"
echo ""

if [[ "${CHECK:-0}" -eq 1 && "$TOTAL_STALE" -gt 0 ]]; then
  exit 1
fi
