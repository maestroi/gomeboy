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

# GBA compatibility smoke runner

`cmd/gba-compat` measures software progress separately from conformance. Each
manifest entry selects a target stage:

1. `loaded`
2. `executed`
3. `first_frame`
4. `checkpoint`
5. `bounded_stability`

The checked-in first compatibility case reuses the project-authored ARM fixture
but keeps executing beyond its early EWRAM signature until the PPU completes a
frame. The checkpoint is only credited after that first frame, which makes this
a ROM -> CPU -> bus -> PPU compatibility smoke rather than another instruction
test.

Run it with:

```sh
go run ./cmd/gba-compat \
  -manifest tests/gba/compatibility/smoke.json \
  -output gba-compatibility.json \
  -markdown gba-compatibility.md
```

Compatibility manifests must pin `rom_sha256` and declare a `steps` limit so
HALT/STOP or other non-advancing states cannot hang automation. `direct` boot
may also declare an initial `cpsr` and r0-r14 values for software that expects
post-BIOS CPU state.

For local-only commercial ROM checks, keep the ROM, BIOS, and manifest outside
the repository and use absolute paths in the manifest. The loader accepts those
paths directly, so commercial ROMs and Nintendo BIOS images never need to
become repository dependencies.

The JSON and generated Markdown matrix report the highest stage reached, target
stage, ROM hash, cycles/frames/steps, and deterministic failure detail. These
compatibility stages are not hardware-accuracy percentages.


## @rkanoid compatibility fixture

The first real-game compatibility case uses the MIT-licensed modern @rkanoid
rebuild from `benoror/gbadev`, pinned to upstream revision
`b61cb1e8d6646789c833522a7d3d64adda27581a`.

The ROM is not committed to this repository. Fetch it deterministically with:

```sh
bash tests/gba/compatibility/fetch-rkanoid.sh
```

The fetch script verifies SHA-256
`99ae825ffb1b7e8b48b33cd2715ceb8ca524858f99fee4b22938489460083d94`.
The generated compatibility case requires @rkanoid to direct-boot with the same
post-BIOS stack state as the desktop frontend, render a frame, and program the
expected display control value `DISPCNT=0x1140`.

Manual bring-up has additionally reached the title menu and playable level-one
gameplay with working input/audio. Those deeper interactive observations are
useful compatibility evidence, but are not yet asserted by CI because the game
intentionally spends roughly 1,900 emulated frames in its splash/fade path
before the menu. Realtime host performance is tracked separately from
compatibility; a correct compatibility result does not imply the emulator is
already sustaining the GBA's ~59.7 Hz realtime rate.


# mGBA hardware conformance

`cmd/gba-mgba-suite` adapts the upstream MIT-licensed
`mgba-emu/suite` ROM into deterministic headless category reports. The pinned
target is **v0-r76**, corresponding to upstream source commit
`7320640f9aad4e48418324d1243cb4402a03cfaa`.

The upstream ROM is not committed. Fetch the immutable release with:

```sh
bash tests/gba/mgba-suite/fetch-suite.sh
```

Then run all enabled categories with:

```sh
go run ./cmd/gba-mgba-suite \
  -config tests/gba/mgba-suite/config.json \
  -rom tests/gba/mgba-suite/roms/suite.gba \
  -output gba-mgba-suite.json \
  -markdown gba-mgba-suite.md
```

The adapter drives the stock suite menu through KEYINPUT and consumes the
suite's SRAM result stream. No test expectations are embedded in GomeBoy's
hardware implementation. BIOS-less CI supplies the BIOS services required by
the enabled suite categories (`IntrWait`, `VBlankIntrWait`, `Div`,
`DivArm`, `Sqrt`, `ArcTan`, `CpuSet`, and `CpuFastSet`) and forwards
hardware IRQs through the suite's real libgba user IRQ vector. This support is
test-harness-only and is not part of the emulator core.

Every planned Lane B category is enabled and tracked independently. The pinned
v0-r76 baseline is:

| Category | Passed | Failed | Total |
| --- | ---: | ---: | ---: |
| Memory | 1304 | 248 | 1552 |
| I/O reads | 29 | 94 | 123 |
| Timing | 312 | 1608 | 1920 |
| Timer count-up | 285 | 651 | 936 |
| Timer IRQ | 18 | 72 | 90 |
| DMA | 996 | 260 | 1256 |

The checked-in baseline has xfail-style behavior:

- fewer passes / more failures => regression and CI failure;
- more passes / fewer failures => XPASS and CI failure until the baseline is
  intentionally reviewed and updated;
- identical counts => expected.

Generated JSON and Markdown reports keep these hardware-accuracy results
separate from the compatibility matrix. The baseline records current hardware
accuracy; individual failures are follow-up correctness work, not silently
accepted emulator behavior.
