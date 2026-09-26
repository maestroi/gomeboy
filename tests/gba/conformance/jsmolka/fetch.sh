#!/usr/bin/env bash
set -euo pipefail

revision="a7113b67e63f83a9b321696ddd7042ccfad6c881"
base="https://raw.githubusercontent.com/jsmolka/gba-tests/${revision}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
rom_dir="${dir}/roms"
mkdir -p "${rom_dir}"

fetch_rom() {
  local path="$1"
  local expected_blob="$2"
  local dest="${rom_dir}/$(basename "${path}")"

  curl --fail --location --retry 3 --silent --show-error     "${base}/${path}"     --output "${dest}"

  local actual_blob
  actual_blob="$(git hash-object "${dest}")"
  if [[ "${actual_blob}" != "${expected_blob}" ]]; then
    echo "${path}: git blob mismatch: got ${actual_blob}, want ${expected_blob}" >&2
    rm -f "${dest}"
    exit 1
  fi

  echo "${path}: blob=${actual_blob} sha256=$(sha256sum "${dest}" | awk '{print $1}')"
}

fetch_rom "arm/arm.gba" "dd0c023da55cc9a62afc90bc6917ceca689d1e29"
fetch_rom "thumb/thumb.gba" "47e50db6920d5a996051d0902365e5cdf7b166b6"
