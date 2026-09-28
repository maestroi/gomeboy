#!/usr/bin/env bash
set -euo pipefail

sha256="38569646cc84e7731fa461c5f23281cc1fe210df63deb3e3f17bcae8929e6198"
out="tests/gba/compatibility/roms/keypad-audio-sram.gba"

go run ./tests/gba/compatibility/multihw-fixture -output "$out"
echo "${sha256}  ${out}" | sha256sum --check --status

echo "built project keypad/audio/SRAM compatibility fixture"
