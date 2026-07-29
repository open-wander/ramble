#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)"
cd "$root_dir"

identity="${MACOS_SIGN_IDENTITY:?MACOS_SIGN_IDENTITY is required}"
profile="${MACOS_NOTARY_PROFILE:?MACOS_NOTARY_PROFILE is required}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: signed CLI releases must run on macOS" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree must be clean before releasing" >&2
  exit 1
fi

tag="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: HEAD must be an exact semantic-version tag (for example v0.5.10)" >&2
  exit 1
fi

if ! security find-identity -v -p codesigning |
  grep -Fq "\"$identity\""; then
  echo "error: Developer ID identity is unavailable: $identity" >&2
  exit 1
fi

xcrun notarytool history \
  --keychain-profile "$profile" \
  --output-format json >/dev/null

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  GITHUB_TOKEN="$(gh auth token)"
  export GITHUB_TOKEN
fi

if [[ -z "${HOMEBREW_TAP_TOKEN:-}" ]]; then
  HOMEBREW_TAP_TOKEN="$GITHUB_TOKEN"
  export HOMEBREW_TAP_TOKEN
fi

echo "Publishing signed CLI release $tag..."
goreleaser release --clean
