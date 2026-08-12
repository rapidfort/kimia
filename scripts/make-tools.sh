#!/usr/bin/env bash
# make-tools.sh — Report latest upstream versions for pinned external tools
# (BuildKit, RootlessKit, Cosign, credential helpers).
#
# Usage:
#   scripts/make-tools.sh           # compare Dockerfile ARGs vs GitHub latest
#   scripts/make-tools.sh --apply   # rewrite ARG versions

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APPLY="${APPLY:-0}"
DF_BUILDKIT=Dockerfile.buildkit
DF_BUILDAH=Dockerfile.buildah

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) APPLY=1; shift ;;
    -h|--help)
      sed -n '2,10p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[ERROR] required command not found: $1" >&2
    exit 1
  }
}
need_cmd curl
need_cmd grep
need_cmd sed

get_arg() {
  local file="$1" name="$2"
  grep -E "^ARG ${name}=" "$file" | tail -1 | sed -E "s/^ARG ${name}=//" | sed -E 's/^"//; s/"$//'
}

gh_latest_tag() {
  local repo="$1"
  curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1
}

strip_v() {
  local t="$1"
  echo "${t#v}"
}

echo ""
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║  Kimia external tool versions                                    ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo ""
printf "  %-18s %-16s %-16s %-12s %s\n" "TOOL" "PINNED" "LATEST" "STATUS" "SOURCE"
printf "  %-18s %-16s %-16s %-12s %s\n" "----" "------" "------" "------" "------"

declare -a APPLY_OPS=()

check_one() {
  local name="$1" file="$2" arg="$3" repo="$4" strip="${5:-0}"
  local pinned latest status show_latest
  pinned="$(get_arg "$file" "$arg" 2>/dev/null || true)"
  [[ -n "$pinned" ]] || return 0
  latest="$(gh_latest_tag "$repo" || true)"
  if [[ -z "$latest" ]]; then
    printf "  %-18s %-16s %-16s %-12s %s\n" "$name" "$pinned" "?" "UNKNOWN" "$repo"
    return
  fi
  show_latest="$latest"
  local compare_latest="$latest"
  local compare_pinned="$pinned"
  if [[ "$strip" == "1" ]]; then
    compare_latest="$(strip_v "$latest")"
    show_latest="$(strip_v "$latest")"
  fi
  if [[ "$compare_pinned" == "$compare_latest" || "$pinned" == "$show_latest" || "v${pinned}" == "$latest" ]]; then
    status="OK"
  else
    status="STALE"
    APPLY_OPS+=("${file}|${arg}|${show_latest}|${strip}")
  fi
  printf "  %-18s %-16s %-16s %-12s %s\n" "$name" "$pinned" "$show_latest" "$status" "$repo"
}

check_one "BuildKit"     "$DF_BUILDKIT" BUILDKIT_VERSION     "moby/buildkit" 0
check_one "RootlessKit"  "$DF_BUILDKIT" ROOTLESSKIT_VERSION  "rootless-containers/rootlesskit" 0
check_one "Cosign"       "$DF_BUILDKIT" COSIGN_VERSION       "sigstore/cosign" 0
check_one "ECR helper"   "$DF_BUILDKIT" ECR_HELPER_VERSION   "awslabs/amazon-ecr-credential-helper" 1
check_one "GCR helper"   "$DF_BUILDKIT" GCR_HELPER_VERSION   "GoogleCloudPlatform/docker-credential-gcr" 1

if [[ -f "$DF_BUILDAH" ]]; then
  ecr_b="$(get_arg "$DF_BUILDAH" ECR_HELPER_VERSION)"
  ecr_k="$(get_arg "$DF_BUILDKIT" ECR_HELPER_VERSION)"
  if [[ "$ecr_b" != "$ecr_k" ]]; then
    echo ""
    echo "  [WARN] ECR_HELPER_VERSION differs: buildkit=${ecr_k} buildah=${ecr_b}"
  fi
fi

if [[ "$APPLY" -eq 1 && ${#APPLY_OPS[@]} -gt 0 ]]; then
  echo ""
  echo "  Applying version ARG updates..."
  # Deduplicate by arg name, apply to both Dockerfiles
  declare -A SEEN=()
  for op in "${APPLY_OPS[@]}"; do
    IFS='|' read -r file arg val strip <<<"$op"
    [[ -z "${SEEN[$arg]:-}" ]] || continue
    SEEN[$arg]=1
    for f in "$DF_BUILDKIT" "$DF_BUILDAH"; do
      [[ -f "$f" ]] || continue
      if grep -qE "^ARG ${arg}=" "$f"; then
        if [[ "$arg" == *HELPER* ]]; then
          sed -i -E "s|^ARG ${arg}=.*|ARG ${arg}=\"${val}\"|" "$f"
        else
          sed -i -E "s|^ARG ${arg}=.*|ARG ${arg}=${val}|" "$f"
        fi
        echo "    $f: ${arg} → ${val}"
      fi
    done
  done
  echo ""
  echo "  Note: rebuild images so provenance/checksum verification runs against new releases."
elif [[ "$APPLY" -eq 0 ]]; then
  echo ""
  echo "  Re-run with:  make tools APPLY=1   to rewrite version ARGs."
fi
echo ""
