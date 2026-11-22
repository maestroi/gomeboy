# Mealybug Tearoom conformance (initial PPU subset)

Source: [mattcurrie/mealybug-tearoom-tests](https://github.com/mattcurrie/mealybug-tearoom-tests), MIT license, revision `70e88fb90b59d19dfbb9c3ac36c64105202bb1f4` (as pinned by `c-sp/game-boy-test-roms` v7.0).

GomeBoy currently measures **six source ROMs on three physical hardware profiles** (18 test/model pairs). This is a partial integration: the upstream ZIP contains 31 ROMs, and only the tests below are included in the structured accuracy percentages. The remaining upstream tests are **not run** (not implicit PASS/FAIL). Source ROMs and reference screenshots are downloaded at CI time, not committed to GomeBoy.

| ROM | Behavior |
| --- | --- |
| `m2_win_en_toggle` | Mode 2 / window enable |
| `m3_bgp_change` | Mid-Mode-3 DMG palette register writes |
| `m3_lcdc_win_en_change_multiple` | Mid-scanline window enable changes |
| `m3_scx_low_3_bits` | Fine SCX scroll changes |
| `m3_window_timing` | Window fetch and timing |
| `m3_wx_6_change` | Mid-scanline WX=6 changes |

## Hardware reference mapping

- `DMG` → upstream `expected/DMG-blob/`
- `CGBBC` → `expected/CPU CGB C/` (CGB B/C revision-family approximation)
- `CGBDE` → `expected/CPU CGB D/` (CGB D/E revision-family approximation)

The comparison uses exact **160×144 RGB pixels** at the ROM's `LD B,B` (`0x40`) software breakpoint, *not* a fixed number of emulated frames. This matches Mealybug's documented automation protocol. DMG references use 00/55/AA/FF shades and CGB references use 5-to-8-bit channel expansion `(v << 3) | (v >> 2)`. No tolerance or automatic palette normalization is applied.

Test identities include ROM name and hardware profile, e.g. `mealybug/ppu/m3_window_timing@CGBDE`. Existing regression statuses distinguish PASS, XFAIL, REGRESSION, XPASS and new/missing tests. Failing cases generate `tests/results/mealybug/*_{actual,diff}.png` images, uploaded by the GB/GBC Actions job; magenta pixels in `diff` are mismatches. A missing/corrupted fixture is a hard setup failure, not a tolerated XFAIL.

## Reproduce locally

From the repository root:

```bash
# Unpack the ordinary bundled ROM corpus, then install the pinned upstream files.
go test -count=1 -tags test ./tests -run '^$'
bash tests/fetch-mealybug.sh

# Run self-contained comparator checks.
go test -count=1 ./tests -run '^TestMealybugPixelDifferences$'

# Run the full checked-in per-test regression gate (expected failures tolerated).
go test -count=1 -tags test -v ./tests -run '^Test_Regressions$'
```

For single-test debugging, `go test -count=1 -v ./tests -run '^TestMealybug/ppu/m3_window_timing@DMG$'` runs the raw test; known XFAILs intentionally return failure in this direct mode. The gate above classifies them against `tests/regression-baseline.json`.

## Updating this subset

1. Confirm the upstream ROM has a matching reference image for the **specific model** and uses the documented breakpoint.
2. Add its ROM name to `mealybugPPUTests` (or introduce a per-ROM model list for images absent on some revisions); do not silently fall back across hardware revisions.
3. Run the full suite and record measured outcomes **before** setting PASS/XFAIL in `tests/regression-baseline.json`.
4. Review artifacts and any new failures under #172 / #173; update generated `tests/README.md`, root `README.md` and corpus coverage.
5. Keep original upstream revision, ZIP checksum, and licensing/credits in sync when bumping the corpus.

The Mealybug expected images represent upstream references and selected hardware revisions, not a guarantee of every DMG/CGB hardware revision's exact behavior.
