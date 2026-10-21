#!/usr/bin/env bash
# Cross-builds style.exe for Windows and packages one zip per architecture,
# plus a SHA-256 checksum file. Used by the release workflow.
#
# Usage: scripts/package_windows.sh VERSION OUT_DIR
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 VERSION OUT_DIR" >&2
  exit 2
fi

version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for arch in amd64 arm64; do
  name="style-cli_${version}_windows_${arch}"
  stage="$work/$name"
  mkdir -p "$stage"
  (
    cd "$root"
    CGO_ENABLED=0 GOOS=windows GOARCH=$arch GOWORK=off go build -trimpath \
      -ldflags "-s -w -X github.com/inxups/style-cli/internal/app.Version=${version}" \
      -o "$stage/style.exe" ./cmd/style
  )
  cp "$root/LICENSE" "$stage/"
  cp -R "$root/third_party" "$stage/"
  rm -f "$out/$name.zip"
  (cd "$work" && zip -qr "$out/$name.zip" "$name")
done

sums="style-cli_${version}_windows_checksums.txt"
(
  cd "$out"
  if command -v sha256sum >/dev/null; then
    sha256sum style-cli_"${version}"_windows_*.zip > "$sums"
  else
    shasum -a 256 style-cli_"${version}"_windows_*.zip > "$sums"
  fi
)
