#!/usr/bin/env bash
set -euo pipefail

out="tests/gb/compatibility/roms"
go run ./tests/gb/compatibility/fixturegen -out "$out"

sha256sum --check --status <<'EOF'
44500b4ba7cd6a17e66d6dddbcd309d2ab1eb7e51ecb6f12a1857373d2172c9d  tests/gb/compatibility/roms/dmg-input-render.gb
20b3b7cb9966a15fbd658bc696f96acd6e626b0fe664c8028cfea0029512a4f3  tests/gb/compatibility/roms/cgb-input-render.gbc
8eac69a5531a3c7a1c8905e8571e34553cbae7a6df6fe1d281189be2a446c375  tests/gb/compatibility/roms/mbc1-banking.gb
1583b72a7fb2564e316bfa05bbd2d4a2ebec5d65a824218f2e20167ef037aebf  tests/gb/compatibility/roms/mbc3-persistence.gb
e44e15e256918971716ae6cbbf98dc98e2ed7c06b94f9acfaa69ba710c6b1a0b  tests/gb/compatibility/roms/mbc5-banking.gb
EOF
