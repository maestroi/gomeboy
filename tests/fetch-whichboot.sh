#!/usr/bin/env bash
set -euo pipefail

# nitro2k01/whichboot.gb v1.1. The release is built from the same source tree
# whose hardware reference table is pinned in issue #55.
VERSION="v1.1"
SOURCE_REVISION="545436fb485f9006d47e0f26f0ceec76cd8e3a07"
ARCHIVE_NAME="whichboot-v1_1.zip"
ARCHIVE_URL="https://github.com/nitro2k01/whichboot.gb/releases/download/${VERSION}/${ARCHIVE_NAME}"
# Filled after the first CI fetch; the script prints the observed digest.
EXPECTED_ARCHIVE_SHA256="c2d1d064ce8871c476043f486be51f478650709943dbfab5052c77b34ce48569"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${ROOT_DIR}/tests/roms/whichboot"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

command -v curl >/dev/null || { echo "fetch-whichboot: curl is required" >&2; exit 2; }
command -v unzip >/dev/null || { echo "fetch-whichboot: unzip is required" >&2; exit 2; }

echo "fetch-whichboot: downloading ${ARCHIVE_NAME} (${SOURCE_REVISION})"
curl --fail --location --silent --show-error "${ARCHIVE_URL}" -o "${TMP_DIR}/${ARCHIVE_NAME}"
ARCHIVE_SHA256="$(sha256sum "${TMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')"
echo "fetch-whichboot: archive sha256=${ARCHIVE_SHA256}"
if [[ -n "${EXPECTED_ARCHIVE_SHA256}" && "${ARCHIVE_SHA256}" != "${EXPECTED_ARCHIVE_SHA256}" ]]; then
  echo "fetch-whichboot: checksum mismatch: expected ${EXPECTED_ARCHIVE_SHA256}, got ${ARCHIVE_SHA256}" >&2
  exit 1
fi

mkdir -p "${TMP_DIR}/unpacked"
unzip -q "${TMP_DIR}/${ARCHIVE_NAME}" -d "${TMP_DIR}/unpacked"
ROM_PATH="$(find "${TMP_DIR}/unpacked" -type f -name 'whichboot.gb' -print -quit)"
if [[ -z "${ROM_PATH}" ]]; then
  echo "fetch-whichboot: release does not contain whichboot.gb" >&2
  unzip -l "${TMP_DIR}/${ARCHIVE_NAME}" >&2
  exit 1
fi

rm -rf "${DEST_DIR}"
mkdir -p "${DEST_DIR}"
cp "${ROM_PATH}" "${DEST_DIR}/whichboot.gb"
echo "fetch-whichboot: installed tests/roms/whichboot/whichboot.gb"
