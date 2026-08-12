#!/usr/bin/env bash
# make-apk.sh — Discover (and optionally apply) latest Alpine OS package pins
# used by Dockerfile.buildkit and Dockerfile.buildah.
#
# Usage:
#   scripts/make-apk.sh              # report: pinned vs latest for each Dockerfile
#   scripts/make-apk.sh --apply      # rewrite package pins in Dockerfiles
#   scripts/make-apk.sh --json       # machine-readable summary
#   scripts/make-apk.sh --dockerfile Dockerfile.buildkit
#
# Env:
#   ALPINE_TAG   Override Alpine tag used for package lookup (e.g. 3.24.1).
#                Default: derive from each Dockerfile's ALPINE_IMAGE.
#   APPLY=1      Same as --apply

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APPLY="${APPLY:-0}"
JSON=0
FILTER_DF=""
DOCKERFILES=(Dockerfile.buildkit Dockerfile.buildah)

# Packages that must never be treated as apk pins (false positives from grepping)
SKIP_NAMES_RE='^(CGO_ENABLED|GOOS|KIMIA_UID|KIMIA_USER|VERSION|BUILD_DATE|COMMIT|BRANCH|RELEASE|PATH|HOME|TARGETARCH)$'

usage() {
  sed -n '2,16p' "$0" | sed 's/^# \?//'
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage ;;
    --apply) APPLY=1; shift ;;
    --json) JSON=1; shift ;;
    --dockerfile) FILTER_DF="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

if [[ -n "$FILTER_DF" ]]; then
  DOCKERFILES=("$FILTER_DF")
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[ERROR] required command not found: $1" >&2
    exit 1
  }
}

need_cmd docker
need_cmd awk
need_cmd sed
need_cmd grep

# Extract ARG NAME="value" or ARG NAME=value from a Dockerfile
get_arg() {
  local file="$1" name="$2"
  # Prefer last matching ARG (in case of overrides)
  grep -E "^ARG ${name}=" "$file" | tail -1 | sed -E "s/^ARG ${name}=//" | sed -E 's/^"//; s/"$//'
}

# Resolve which Alpine release an image provides
alpine_release_of() {
  local image="$1"
  docker run --rm --entrypoint cat "$image" /etc/alpine-release 2>/dev/null | tr -d '[:space:]'
}

# Multi-arch index digest for alpine:<tag> (sha256:… only)
alpine_index_digest() {
  local tag="$1" digest=""
  # Prefer buildx text output — --format templates vary across buildx versions
  digest="$(docker buildx imagetools inspect "alpine:${tag}" 2>/dev/null     | awk '/^Digest:/ {print $2; exit}')"
  if [[ -z "$digest" || "$digest" != sha256:* ]]; then
    # docker manifest inspect fallback
    digest="$(docker manifest inspect "alpine:${tag}" 2>/dev/null       | sed -n 's/.*"digest"[[:space:]]*:[[:space:]]*"\(sha256:[^"]*\)".*/\1/p'       | head -1)"
  fi
  if [[ -n "$digest" && "$digest" == sha256:* ]]; then
    printf '%s\n' "$digest"
    return 0
  fi
  return 1
}

# Latest known patch of the same Alpine major.minor (e.g. 3.23.3 → 3.23.5).
# Uses the floating alpine:N.M tag annotation when available.
latest_patch_tag() {
  local release="$1"
  local major minor patch floated ver
  if [[ ! "$release" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo "$release"
    return 0
  fi
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  floated="${major}.${minor}"
  ver="$(docker buildx imagetools inspect "alpine:${floated}" 2>/dev/null     | awk '/org.opencontainers.image.version:/ {
        gsub(/^[ \t]+/, "", $2);
        if ($2 ~ /^[0-9]+\.[0-9]+\.[0-9]+$/) { print $2; exit }
      }')"
  if [[ -n "$ver" ]]; then
    echo "$ver"
    return 0
  fi
  echo "$release"
}

# Query latest exact apk package versions from a given Alpine image.
# Prints lines: name=version  (version includes -rX release suffix, no name prefix)
query_apk_versions() {
  local image="$1"
  shift
  local pkgs=("$@")
  # shellcheck disable=SC2086
  docker run --rm --entrypoint sh "$image" -c '
    set -e
    apk update -q >/dev/null
    for p in '"${pkgs[*]}"'; do
      # apk search -e prints name-version (version may contain hyphens)
      line=$(apk search -e "$p" 2>/dev/null | head -1)
      if [ -z "$line" ]; then
        echo "${p}=NOT_FOUND" >&2
        echo "${p}="
        continue
      fi
      # Strip exact package name prefix + hyphen → remaining is version
      ver=${line#"${p}-"}
      if [ "$ver" = "$line" ]; then
        echo "${p}=NOT_FOUND" >&2
        echo "${p}="
        continue
      fi
      echo "${p}=${ver}"
    done
  '
}

# Collect unique apk package names currently pinned in a Dockerfile
pinned_packages_in() {
  local file="$1"
  # Match name=version-rN style pins inside apk add blocks / lines
  grep -oE '[a-zA-Z0-9_+.-]+=[0-9][a-zA-Z0-9._:+~-]*-r[0-9]+' "$file" \
    | awk -F= '{print $1}' \
    | while read -r n; do
        [[ "$n" =~ $SKIP_NAMES_RE ]] && continue
        echo "$n"
      done \
    | sort -u
}

# Current pin for package in file (first match)
current_pin() {
  local file="$1" pkg="$2"
  grep -oE "${pkg}=[0-9][a-zA-Z0-9._:+~-]*-r[0-9]+" "$file" | head -1 | cut -d= -f2-
}

# Replace all pkg=old with pkg=new in file
apply_pin() {
  local file="$1" pkg="$2" newver="$3"
  # Use | as sed delimiter; versions never contain |
  sed -i -E "s|${pkg}=[0-9][a-zA-Z0-9._:+~-]*-r[0-9]+|${pkg}=${newver}|g" "$file"
}

# Optional: rewrite ALPINE_IMAGE ARG to alpine:<tag>@<digest>
apply_alpine_image() {
  local file="$1" tag="$2" digest="$3"
  local new="alpine:${tag}@${digest}"
  sed -i -E "s|^ARG ALPINE_IMAGE=.*|ARG ALPINE_IMAGE=\"${new}\"|" "$file"
}

report_header() {
  if [[ "$JSON" -eq 1 ]]; then
    return
  fi
  echo ""
  echo "╔══════════════════════════════════════════════════════════════════╗"
  echo "║  Kimia OS package audit (apk)                                    ║"
  echo "╚══════════════════════════════════════════════════════════════════╝"
}

# --- main per-dockerfile processing ---
report_header

JSON_PARTS=()
TOTAL_STALE=0
TOTAL_OK=0
TOTAL_MISSING=0

for df in "${DOCKERFILES[@]}"; do
  if [[ ! -f "$df" ]]; then
    echo "[WARN] skipping missing file: $df" >&2
    continue
  fi

  alpine_image="$(get_arg "$df" ALPINE_IMAGE)"
  if [[ -z "$alpine_image" ]]; then
    echo "[ERROR] $df: no ARG ALPINE_IMAGE" >&2
    exit 1
  fi

  echo ""
  echo "━━━ $df ━━━"
  echo "  ALPINE_IMAGE: ${alpine_image}"

  # Pull base so release detection and apk queries work offline-ish after first pull
  if ! docker image inspect "$alpine_image" >/dev/null 2>&1; then
    echo "  Pulling ${alpine_image} ..."
    docker pull "$alpine_image" >/dev/null
  fi

  current_release="$(alpine_release_of "$alpine_image")"
  if [[ -n "${ALPINE_TAG:-}" ]]; then
    lookup_tag="${ALPINE_TAG}"
  elif [[ "${BUMP_ALPINE_PATCH:-1}" == "1" ]]; then
    lookup_tag="$(latest_patch_tag "$current_release")"
  else
    lookup_tag="$current_release"
  fi
  lookup_image="alpine:${lookup_tag}"

  echo "  Alpine release (image): ${current_release}"
  if [[ "$lookup_tag" != "$current_release" ]]; then
    echo "  Alpine lookup tag:      ${lookup_tag}  (newer patch in same minor)"
  fi
  if [[ -n "${ALPINE_TAG:-}" ]]; then
    echo "  Lookup override ALPINE_TAG: ${lookup_tag}"
  fi

  # Ensure lookup image exists
  if ! docker image inspect "$lookup_image" >/dev/null 2>&1; then
    echo "  Pulling ${lookup_image} for package index..."
    docker pull "$lookup_image" >/dev/null
  fi

  mapfile -t pkgs < <(pinned_packages_in "$df")
  if [[ ${#pkgs[@]} -eq 0 ]]; then
    echo "  [WARN] no pinned apk packages found"
    continue
  fi

  echo "  Packages pinned: ${#pkgs[@]}"
  echo ""
  printf "  %-22s %-22s %-22s %s\n" "PACKAGE" "PINNED" "LATEST" "STATUS"
  printf "  %-22s %-22s %-22s %s\n" "-------" "------" "------" "------"

  # Batch query
  mapfile -t latest_lines < <(query_apk_versions "$lookup_image" "${pkgs[@]}")
  declare -A LATEST=()
  for line in "${latest_lines[@]}"; do
    name="${line%%=*}"
    ver="${line#*=}"
    LATEST["$name"]="$ver"
  done

  file_stale=0
  file_ok=0
  file_missing=0
  updates=()

  for pkg in "${pkgs[@]}"; do
    pinned="$(current_pin "$df" "$pkg" || true)"
    latest="${LATEST[$pkg]:-}"
    if [[ -z "$latest" ]]; then
      status="MISSING"
      file_missing=$((file_missing + 1))
      TOTAL_MISSING=$((TOTAL_MISSING + 1))
    elif [[ "$pinned" == "$latest" ]]; then
      status="OK"
      file_ok=$((file_ok + 1))
      TOTAL_OK=$((TOTAL_OK + 1))
    else
      status="STALE"
      file_stale=$((file_stale + 1))
      TOTAL_STALE=$((TOTAL_STALE + 1))
      updates+=("${pkg}=${latest}")
    fi
    printf "  %-22s %-22s %-22s %s\n" "$pkg" "${pinned:-?}" "${latest:-?}" "$status"
  done

  echo ""
  echo "  Summary: ${file_ok} ok, ${file_stale} stale, ${file_missing} missing"

  # Suggest alpine digest pin for the lookup tag
  digest="$(alpine_index_digest "$lookup_tag" || true)"
  if [[ -n "$digest" && "$digest" == sha256:* ]]; then
    suggested="alpine:${lookup_tag}@${digest}"
    echo "  Suggested ALPINE_IMAGE: ${suggested}"
  else
    echo "  Suggested ALPINE_IMAGE: alpine:${lookup_tag}  (digest unresolved)"
    digest=""
  fi

  if [[ "$APPLY" -eq 1 ]]; then
    if [[ ${#updates[@]} -gt 0 ]]; then
      echo "  Applying ${#updates[@]} package pin update(s)..."
      for u in "${updates[@]}"; do
        pkg="${u%%=*}"
        ver="${u#*=}"
        apply_pin "$df" "$pkg" "$ver"
        echo "    ${pkg} → ${ver}"
      done
    else
      echo "  No package pin changes needed."
    fi
    # Keep ALPINE_IMAGE in sync with lookup tag when digest is a real sha256
    if [[ -n "${digest:-}" && "$digest" == sha256:* ]]; then
      apply_alpine_image "$df" "$lookup_tag" "$digest"
      echo "  ALPINE_IMAGE → alpine:${lookup_tag}@${digest}"
    else
      echo "  [WARN] could not resolve a clean multi-arch digest for alpine:${lookup_tag}; left ALPINE_IMAGE unchanged"
    fi
  fi

  # JSON fragment
  if [[ "$JSON" -eq 1 ]]; then
    pkgs_json="["
    first=1
    for pkg in "${pkgs[@]}"; do
      pinned="$(current_pin "$df" "$pkg" || true)"
      latest="${LATEST[$pkg]:-}"
      [[ $first -eq 1 ]] || pkgs_json+=","
      first=0
      pkgs_json+=$(printf '{"name":"%s","pinned":"%s","latest":"%s"}' \
        "$pkg" "$pinned" "$latest")
    done
    pkgs_json+="]"
    JSON_PARTS+=("$(printf '{"dockerfile":"%s","alpine_image":"%s","alpine_release":"%s","lookup_tag":"%s","packages":%s}' \
      "$df" "$alpine_image" "$current_release" "$lookup_tag" "$pkgs_json")")
  fi

  unset LATEST
done

echo ""
echo "━━━ Totals ━━━"
echo "  OK: ${TOTAL_OK}  STALE: ${TOTAL_STALE}  MISSING: ${TOTAL_MISSING}"
if [[ "$APPLY" -eq 0 && "$TOTAL_STALE" -gt 0 ]]; then
  echo ""
  echo "  Re-run with:  make apk APPLY=1"
  echo "  to rewrite pins (and ALPINE_IMAGE digests) in the Dockerfiles."
fi
echo ""

if [[ "$JSON" -eq 1 ]]; then
  echo -n '{"results":['
  first=1
  for part in "${JSON_PARTS[@]:-}"; do
    [[ $first -eq 1 ]] || echo -n ','
    first=0
    echo -n "$part"
  done
  echo ']}'
fi

# Exit non-zero if stale when CHECK=1 (for CI)
if [[ "${CHECK:-0}" -eq 1 && "$TOTAL_STALE" -gt 0 ]]; then
  exit 1
fi
