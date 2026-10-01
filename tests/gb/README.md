# GB/GBC compatibility smoke matrix

This directory contains a small project-owned ROM corpus for end-to-end GB/GBC
compatibility checks. It is deliberately separate from the hardware conformance
percentage in `tests/regression-results.json`.

The matrix reports staged outcomes:

1. ROM loaded;
2. non-uniform rendered output appeared;
3. an emulator-visible checkpoint was reached;
4. deterministic input caused progress where required;
5. battery-backed data survived a close/reopen cycle where required;
6. the requested bounded stability window completed.

`fixturegen` is the source of all ROMs in this matrix. `build-fixtures.sh`
rebuilds them and verifies pinned SHA-256 values before the runner executes.

The render/input fixtures exercise BG scrolling, a window tile, OBJ rendering,
a running timer, an internal-clock serial transfer, and joypad input on both
DMG and native CGB models. Mapper fixtures cover MBC1 and MBC5 ROM/RAM banking.
The MBC3 case writes battery-backed RAM, closes the emulator, reopens the same
ROM with the same save directory, and requires the ROM itself to observe the
persisted byte before publishing its success signature.

Run locally:

```sh
bash tests/gb/compatibility/build-fixtures.sh
go run ./cmd/gb-compat \
  -manifest tests/gb/compatibility/smoke.json \
  -output gb-gbc-compatibility.json \
  -markdown gb-gbc-compatibility.md
```

Compatibility stages are user-facing smoke milestones, not hardware-accuracy
percentages. The per-test conformance gate remains the authority for accuracy.
