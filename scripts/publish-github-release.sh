#!/usr/bin/env bash
# Publish a new OWPanel version to GitHub Releases.
#
# Prerequisites:
#   git, go, npm, gh (GitHub CLI, authenticated: gh auth login)
#
# Usage:
#   ./scripts/publish-github-release.sh v0.1.16              # build + tag + push + upload assets
#   ./scripts/publish-github-release.sh v0.1.16 --draft      # draft release
#   ./scripts/publish-github-release.sh v0.1.16 --ci         # build locally, push tag only (GitHub Actions uploads)
#   ./scripts/publish-github-release.sh v0.1.16 --skip-build # reuse dist/*.tar.gz
#   ./scripts/publish-github-release.sh v0.1.16 --notes-file CHANGELOG.md
#   ./scripts/publish-github-release.sh --latest               # use latest git tag
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
REPO="${GITHUB_REPO:-}"

log() { echo "[release] $*"; }
die() { echo "[release] ERROR: $*" >&2; exit 1; }
warn() { echo "[release] WARN: $*" >&2; }

usage() {
  cat <<'EOF'
Publish OWPanel release to GitHub.

Usage:
  publish-github-release.sh <version> [options]

Version:
  v0.1.16          Release tag (must start with v)
  --latest         Use the newest existing git tag

Options:
  --ci             Push tag only; GitHub Actions builds and uploads (see .github/workflows/release.yml)
  --draft          Create a draft release
  --skip-build     Skip build-release.sh (use existing dist/*.tar.gz)
  --skip-tag       Do not create/push git tag (release assets only)
  --no-push        Create local tag but do not push to origin
  --notes TEXT     Release notes body
  --notes-file F   Read release notes from file
  --generate-notes Use gh to generate notes from merged PRs
  --include-windows Upload windows amd64 package too
  --repo OWNER/REPO  Override repository (default: git remote origin)
  -h, --help       Show this help

Examples:
  ./scripts/publish-github-release.sh v0.1.16
  ./scripts/publish-github-release.sh v0.1.16 --ci
  ./scripts/publish-github-release.sh v0.1.16 --draft --notes "Bug fixes"
EOF
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1 (install it first)"
}

normalize_version() {
  local v="$1"
  v="${v#refs/tags/}"
  if [[ ! "$v" =~ ^v[0-9]+(\.[0-9]+)*(-[a-zA-Z0-9.]+)?$ ]]; then
    die "invalid version tag: $v (expected v1.2.3)"
  fi
  echo "$v"
}

detect_repo() {
  if [[ -n "$REPO" ]]; then
    echo "$REPO"
    return
  fi
  local url
  url="$(git -C "$ROOT" remote get-url origin 2>/dev/null || true)"
  [[ -n "$url" ]] || die "cannot detect GitHub repo; set GITHUB_REPO=owner/repo"
  url="${url%.git}"
  url="${url#git@github.com:}"
  url="${url#https://github.com/}"
  url="${url#http://github.com/}"
  [[ "$url" == */* ]] || die "unexpected origin URL: $url"
  echo "$url"
}

ensure_gh_auth() {
  need_cmd gh
  gh auth status >/dev/null 2>&1 || die "gh not authenticated — run: gh auth login"
}

git_is_clean() {
  [[ -z "$(git -C "$ROOT" status --porcelain 2>/dev/null)" ]]
}

create_tag_if_needed() {
  local ver="$1"
  if git -C "$ROOT" rev-parse "$ver" >/dev/null 2>&1; then
    log "Tag $ver already exists"
    return
  fi
  log "Creating annotated tag $ver"
  git -C "$ROOT" tag -a "$ver" -m "Release $ver"
}

push_tag() {
  local ver="$1"
  log "Pushing tag $ver to origin..."
  git -C "$ROOT" push origin "$ver"
}

build_packages() {
  log "Building release packages (VERSION=$VERSION)..."
  need_cmd go
  need_cmd npm
  VERSION="$VERSION" bash "$ROOT/scripts/build-release.sh"
}

collect_assets() {
  ASSETS=()
  local f
  for f in owpanel-linux-amd64.tar.gz owpanel-linux-arm64.tar.gz owpanel-stack-scripts.tar.gz; do
    [[ -f "$DIST/$f" ]] || die "missing asset: $DIST/$f (run without --skip-build)"
    ASSETS+=("$DIST/$f")
  done
  if [[ "$INCLUDE_WINDOWS" == "1" ]]; then
    [[ -f "$DIST/owpanel-windows-amd64.tar.gz" ]] && ASSETS+=("$DIST/owpanel-windows-amd64.tar.gz")
  fi
}

publish_with_gh() {
  local ver="$1"
  local repo="$2"
  local args=(release create "$ver")
  args+=(--repo "$repo")
  args+=(--title "OWPanel $ver")

  if [[ "$DRAFT" == "1" ]]; then
    args+=(--draft)
  fi
  if [[ -n "$NOTES_FILE" ]]; then
    args+=(--notes-file "$NOTES_FILE")
  elif [[ -n "$NOTES" ]]; then
    args+=(--notes "$NOTES")
  elif [[ "$GENERATE_NOTES" == "1" ]]; then
    args+=(--generate-notes)
  else
    local prev=""
    prev="$(git -C "$ROOT" describe --tags --abbrev=0 "${ver}^" 2>/dev/null || true)"
    if [[ -n "$prev" ]]; then
      args+=(--notes "Changes since ${prev}:
$(git -C "$ROOT" log --pretty=format:'- %s (%h)' "${prev}..${ver}" 2>/dev/null || echo '- see commit history')")
    else
      args+=(--notes "OWPanel release $ver")
    fi
  fi

  for f in "${ASSETS[@]}"; do
    args+=("$f")
  done

  log "Creating GitHub release $ver on $repo ..."
  gh "${args[@]}"
}

wait_for_ci_release() {
  local ver="$1" repo="$2" i
  log "Waiting for GitHub Actions to publish $ver (up to 20 min)..."
  for i in $(seq 1 120); do
    if gh release view "$ver" --repo "$repo" >/dev/null 2>&1; then
      log "Release $ver is live"
      gh release view "$ver" --repo "$repo"
      return 0
    fi
    sleep 10
  done
  die "timed out waiting for CI release — check Actions: https://github.com/${repo}/actions"
}

# --- parse args ---
VERSION=""
DRAFT=0
CI_MODE=0
SKIP_BUILD=0
SKIP_TAG=0
NO_PUSH=0
INCLUDE_WINDOWS=0
GENERATE_NOTES=0
NOTES=""
NOTES_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --latest)
      VERSION="$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null || die "no git tags found")"
      shift
      ;;
    --draft) DRAFT=1; shift ;;
    --ci) CI_MODE=1; shift ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --skip-tag) SKIP_TAG=1; shift ;;
    --no-push) NO_PUSH=1; shift ;;
    --generate-notes) GENERATE_NOTES=1; shift ;;
    --include-windows) INCLUDE_WINDOWS=1; shift ;;
    --notes) NOTES="${2:-}"; shift 2 ;;
    --notes-file) NOTES_FILE="${2:-}"; shift 2 ;;
    --repo) REPO="${2:-}"; shift 2 ;;
    v*) VERSION="$1"; shift ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

[[ -n "$VERSION" ]] || die "version required (e.g. v0.1.16) — try --help"
VERSION="$(normalize_version "$VERSION")"
REPO="$(detect_repo)"

log "Repository: $REPO"
log "Version:    $VERSION"

need_cmd git
if ! git_is_clean; then
  warn "working tree is not clean — commit or stash changes before releasing"
  if [[ "${ALLOW_DIRTY:-}" != "1" ]]; then
    die "aborting (set ALLOW_DIRTY=1 to override)"
  fi
fi

if [[ "$SKIP_BUILD" != "1" ]]; then
  build_packages
else
  log "Skipping build (--skip-build)"
fi

if [[ "$CI_MODE" == "1" ]]; then
  ensure_gh_auth
  if [[ "$SKIP_TAG" != "1" ]]; then
    create_tag_if_needed "$VERSION"
    if [[ "$NO_PUSH" != "1" ]]; then
      push_tag "$VERSION"
    else
      warn "tag not pushed (--no-push)"
    fi
  fi
  log "Tag pushed — GitHub Actions will build and upload assets"
  log "Monitor: https://github.com/${REPO}/actions"
  if [[ "$NO_PUSH" != "1" ]]; then
    wait_for_ci_release "$VERSION" "$REPO"
  fi
  exit 0
fi

ensure_gh_auth
collect_assets

if gh release view "$VERSION" --repo "$REPO" >/dev/null 2>&1; then
  log "Release $VERSION exists — uploading/replacing assets"
  gh release upload "$VERSION" "${ASSETS[@]}" --repo "$REPO" --clobber
  gh release view "$VERSION" --repo "$REPO"
else
  if [[ "$SKIP_TAG" != "1" ]]; then
    create_tag_if_needed "$VERSION"
  fi
  publish_with_gh "$VERSION" "$REPO"
  if [[ "$SKIP_TAG" != "1" && "$NO_PUSH" != "1" ]]; then
    push_tag "$VERSION" || warn "could not push tag (it may already exist on remote)"
  fi
fi

log "Done."
log "Release URL: https://github.com/${REPO}/releases/tag/${VERSION}"
log "Panel auto-update and install.sh will pick up this version once it is the latest release."
