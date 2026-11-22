#!/usr/bin/env bash
set -euo pipefail

# mattcurrie/mealybug-tearoom-tests is MIT licensed. Pin the upstream commit
# used by c-sp/game-boy-test-roms v7.0; do not follow moving branches/tags.
REVISION="70e88fb90b59d19dfbb9c3ac36c64105202bb1f4"
ZIP_SHA256="e2fa6ca96ad48bd64d7297fda042db04c15117a730da3bbd590f139e89b150bd"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${ROOT}/tests/roms/mealybug"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

for command in curl tar unzip sha256sum; do
  command -v "${command}" >/dev/null || { echo "fetch-mealybug: missing ${command}" >&2; exit 2; }
done

curl --fail --location --silent --show-error \
  "https://codeload.github.com/mattcurrie/mealybug-tearoom-tests/tar.gz/${REVISION}" \
  -o "${TMP}/source.tar.gz"
mkdir -p "${TMP}/source" "${TMP}/roms"
tar -xzf "${TMP}/source.tar.gz" --strip-components=1 -C "${TMP}/source"

ARCHIVE="${TMP}/source/mealybug-tearoom-tests.zip"
echo "${ZIP_SHA256}  ${ARCHIVE}" | sha256sum --check --status || {
  echo "fetch-mealybug: pinned upstream ROM archive SHA256 mismatch" >&2
  exit 1
}
unzip -q "${ARCHIVE}" -d "${TMP}/roms"
COUNT="$(find "${TMP}/roms" -type f -name '*.gb' | wc -l | tr -d ' ')"
[[ "${COUNT}" == "31" ]] || { echo "fetch-mealybug: expected 31 ROMs; found ${COUNT}" >&2; exit 1; }

# The canonical hardware snapshots are kept separate. Never silently replace
# CGB C/D references with generic CGB or DMG-CPU B/DMG-blob snapshots.
for group in "DMG-blob" "CPU CGB C" "CPU CGB D"; do
  [[ -d "${TMP}/source/expected/${group}" ]] || {
    echo "fetch-mealybug: missing reference group ${group}" >&2
    exit 1
  }
done

rm -rf "${DEST}"
mkdir -p "${DEST}/expected"
cp "${TMP}/roms/"*.gb "${DEST}/"
for group in "DMG-blob" "CPU CGB C" "CPU CGB D"; do
  cp -R "${TMP}/source/expected/${group}" "${DEST}/expected/"
done

echo "fetch-mealybug: verified revision ${REVISION}, installed ${COUNT} ROMs and reference PNGs"
