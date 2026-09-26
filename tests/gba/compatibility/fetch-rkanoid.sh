#!/usr/bin/env bash
set -euo pipefail

revision="b61cb1e8d6646789c833522a7d3d64adda27581a"
sha256="99ae825ffb1b7e8b48b33cd2715ceb8ca524858f99fee4b22938489460083d94"
url="https://raw.githubusercontent.com/benoror/gbadev/${revision}/public/roms/%40rkanoid%20MODERN%20-%20RECOMPILED.gba"
out="tests/gba/compatibility/roms/rkanoid-modern.gba"

mkdir -p "$(dirname "$out")"
tmp="${out}.tmp"
trap 'rm -f "$tmp"' EXIT

curl --fail --location --retry 3 --silent --show-error "$url" --output "$tmp"
echo "${sha256}  ${tmp}" | sha256sum --check --status
mv "$tmp" "$out"
trap - EXIT

echo "fetched @rkanoid modern ${revision}"
