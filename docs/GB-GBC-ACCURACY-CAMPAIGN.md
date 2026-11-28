# GB/GBC Accuracy Campaign

## Verified baseline

- **Main SHA:** `039583f7` (#292 merged) — corpus still **313/329** XFAIL (sprite scores up but not full PASS), gate green
- **Date:** 2026-11-28
- **Branch:** `accuracy/gb-next-xfail`

## Completed

### P1 — DS AGE window STAT WX=0 (#290)

- 765/768 → **768/768**
- PR: https://github.com/maestroi/gomeboy/pull/290

### P3a — Sprite ready-path fine-SCX Mode-3 bubble (#292)

- Ready-path sprite fetch inserts 1 Mode-3 idle when `SCX&7 != 0`
- AGE sprites: DMG **512→536**, CGB **533→553**, DS **802→867**
- Guards: mooneye sprites PASS, tellinglys CGB (flaky but not regressed), gate green
- PR: https://github.com/maestroi/gomeboy/pull/292
- Residual: NOP=0 SPX≥4 still Mode0 on fine SCX **0** only (cannot +1 at SCX=0 — breaks mooneye)

## Blocked / failed experiments

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE `oam-write.inc` marks that line `(*)` LCD-off history dependent (TODO even in AGE)
- Glitched-line early OAM WUnlock: no AGE gain — reverted

### P3b/c/d — X=0 absorb / spx0 stall / flat align — FAILED

- See prior notes; tellinglys or mooneye regressions

### P4 — STAT interrupt IRQ at lx==168 — FAILED

- AGE stat-int: always Mode2 (`82`) vs expected alternating Mode0/2 (`80`/`82`)
- Moving Mode-0 IRQ to lx==168: +2/+5 samples but **regressed** `halt-m0-interrupt`×3 + `hblank_ly_scx_timing-GS` — reverted
- Need longer post-IRQ Mode-0 without earlier IRQ edge

### P6 — Mealybug WX=6

- DMG: 6945 px diff — actual is solid BG “B”s; expected staircase + black desync bands (partial desync fill from #289)
- CGB expected PNGs are **placeholders** (“Expected result not currently available”) — cannot XPASS until real refs exist

### P5 — LCD-align-LY

- 40/40 probes mismatch (`value=00 inverted`); LY edge after LCD-on phase alignment

## Remaining XFAIL (16)

| Family | Count | Notes |
|---|---|---|
| sprites | 5 | SCX=0 residual + incomplete matrix |
| STAT int | 5 | Mode0/2 edge after IRQ |
| Mealybug wx6 | 3 | DMG desync incomplete; CGB refs missing |
| LCD-align-LY | 2 | LCD-on LY phase |
| OAM write DMG | 1 | DELAY=2 history (*) |

## Next action

1. DMG Mealybug WX=6: finish LCD/PPU desync so black bands + window staircase match reference
2. Sprite SCX=0 residual only with mooneye-safe mechanism
3. Skip inventing CGB Mealybug refs; treat as blocked until upstream images exist
