#!/usr/bin/env bash
set -euo pipefail

# c-sp/game-boy-test-roms v7.0 is pinned to aggregate commit
# 8e1f6d7f3a1d8683f11fdf23008d1b1b26e51b52 and builds AGE from
# c-sp/age-test-roms cd3f654d13bf7137fa4f7fb7e5dd041d10f7098e.
VERSION="v7.0"
AGGREGATE_REVISION="8e1f6d7f3a1d8683f11fdf23008d1b1b26e51b52"
AGE_REVISION="cd3f654d13bf7137fa4f7fb7e5dd041d10f7098e"
ARCHIVE_NAME="game-boy-test-roms-v7.0.zip"
ARCHIVE_URL="https://github.com/c-sp/game-boy-test-roms/releases/download/${VERSION}/${ARCHIVE_NAME}"
EXPECTED_ROM_COUNT=32

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${ROOT_DIR}/tests/roms/age"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

command -v curl >/dev/null || { echo "fetch-age: curl is required" >&2; exit 2; }
command -v unzip >/dev/null || { echo "fetch-age: unzip is required" >&2; exit 2; }

echo "fetch-age: downloading ${ARCHIVE_NAME} (aggregate ${AGGREGATE_REVISION}, AGE ${AGE_REVISION})"
curl --fail --location --silent --show-error "${ARCHIVE_URL}" -o "${TMP_DIR}/${ARCHIVE_NAME}"

ARCHIVE_SHA256="$(sha256sum "${TMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')"
echo "fetch-age: archive sha256=${ARCHIVE_SHA256}"

mkdir -p "${TMP_DIR}/unpacked"
unzip -q "${TMP_DIR}/${ARCHIVE_NAME}" -d "${TMP_DIR}/unpacked"
SOURCE_DIR="$(find "${TMP_DIR}/unpacked" -type d -name age-test-roms -print -quit)"
if [[ -z "${SOURCE_DIR}" ]]; then
  echo "fetch-age: release does not contain age-test-roms" >&2
  exit 1
fi

rm -rf "${DEST_DIR}"
mkdir -p "${DEST_DIR}"
for group in halt lcd-align-ly ly oam stat-interrupt stat-mode stat-mode-sprites stat-mode-window vram; do
  if [[ ! -d "${SOURCE_DIR}/${group}" ]]; then
    echo "fetch-age: missing expected group ${group}" >&2
    exit 1
  fi
  cp -R "${SOURCE_DIR}/${group}" "${DEST_DIR}/${group}"
done

ROM_COUNT="$(find "${DEST_DIR}" -type f -name '*.gb' | wc -l | tr -d ' ')"
if [[ "${ROM_COUNT}" != "${EXPECTED_ROM_COUNT}" ]]; then
  echo "fetch-age: expected ${EXPECTED_ROM_COUNT} ROMs, found ${ROM_COUNT}" >&2
  exit 1
fi

echo "fetch-age: installed ${ROM_COUNT} AGE ROMs into tests/roms/age"
