# GBA conformance runner

`cmd/gba-conformance` runs GBA test ROMs directly against the integrated
`internal/gba/system.Machine` without the desktop frontend. Manifests pin the
suite revision and ROM SHA-256, declare deterministic execution budgets, and
identify pass/fail through emulator-visible conditions.

Run the checked-in smoke suite with:

```sh
go run ./cmd/gba-conformance \
  -manifest tests/gba/conformance/smoke.json \
  -output gba-conformance.json
```

The command exits non-zero when a test fails or times out. Its JSON contains
suite/test names, pass/fail/timeout status, emulated steps/cycles/frames,
GomeBoy commit metadata, suite revision, fixture SHA-256, and failure detail.
There is deliberately no wall-clock timestamp, so repeated runs of the same
ROM/configuration/build metadata can be byte-for-byte identical.

## Manifest format

Each test declares:

- `rom` and `rom_sha256` so fixture changes are explicit.
- `boot.mode`: `direct` for BIOS-independent ROMs or `reset` to start at the
  architectural reset vector with the supplied `bios` file.
- one or more `limits` (`steps`, `cycles`, `frames`) so a broken ROM cannot hang
  CI.
- `pass_all`: memory/register/PC conditions that must all match.
- optional `fail_any`: conditions that immediately classify the run as failed.

Addresses and values accept JSON numbers or quoted values such as
`"0x02000000"`. Memory widths are 1, 2, or 4 bytes. Register checks currently
cover r0-r14; use the `pc` condition for the execution address.

Suite-specific parsing belongs in conformance tooling/adapters, not the GBA
hardware core. The ARM/Thumb, mGBA-suite, and compatibility-report work can
therefore add manifests and result adapters without introducing game-specific
behavior into emulation.

## Smoke fixture

`conformance/roms/arm-memory-signature.gba` is a tiny project-authored ARM ROM
fixture. It loads a fixed signature and stores it to EWRAM, then loops. It is
covered by the repository MIT license and exists only to prove the complete
ROM -> CPU -> bus -> result path in normal CI; independent external suites are
tracked separately under issues #63 and #64.
