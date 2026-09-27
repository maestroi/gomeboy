#!/usr/bin/env bash
set -euo pipefail

revision="v0-r76"
source_commit="7320640f9aad4e48418324d1243cb4402a03cfaa"
url="https://s3.amazonaws.com/mgba/tests/suite-v0-r76.zip"
expected_zip_sha256="3dad215516545317201224c85bb09b1fc9d6a73ac927153c874e351042d3de7f"
expected_rom_sha256="3dc46e69d36a60f0db55e72d138de07e2ba79456fc43e9a2416a2529c538bf41"

root="$(cd "$(dirname "$0")" && pwd)"
rom_dir="$root/roms"
mkdir -p "$rom_dir"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$url" -o "$tmp/suite.zip"
zip_sha256="$(sha256sum "$tmp/suite.zip" | awk '{print $1}')"
unzip -p "$tmp/suite.zip" suite.gba > "$rom_dir/suite.gba"
rom_sha256="$(sha256sum "$rom_dir/suite.gba" | awk '{print $1}')"

echo "mGBA suite revision: $revision"
echo "mGBA suite source commit: $source_commit"
echo "archive SHA-256: $zip_sha256"
echo "ROM SHA-256: $rom_sha256"

if [[ -n "$expected_zip_sha256" && "$zip_sha256" != "$expected_zip_sha256" ]]; then
  echo "archive hash mismatch: got $zip_sha256 want $expected_zip_sha256" >&2
  exit 1
fi
if [[ -n "$expected_rom_sha256" && "$rom_sha256" != "$expected_rom_sha256" ]]; then
  echo "ROM hash mismatch: got $rom_sha256 want $expected_rom_sha256" >&2
  exit 1
fi
