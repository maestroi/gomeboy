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
- Residual: NOP=0 on AGE SCX=LY coarse-aligned lines (LY=8,16,…); cannot +1 at SCX=0 (mooneye) or blanket SCX≠0 (tellinglys)

## Blocked / failed experiments

### P3e — AGE SCX=LY coarse-aligned bubble — FAILED

- `SCX==LY && SCX!=0` ready-path +1: AGE **+4/+8/+4** samples but **regresses tellinglys** (DMG+CGB) — reverted
- Blanket `SCX!=0` also regresses tellinglys; always-on ready bubble breaks mooneye

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE `oam-write.inc` marks that line `(*)` LCD-off history dependent (TODO even in AGE)
- Glitched-line early OAM WUnlock: no AGE gain — reverted

### P3b/c/d — X=0 absorb / spx0 stall / flat align — FAILED

- See prior notes; tellinglys or mooneye regressions

### P4 — STAT interrupt IRQ at lx==168 — FAILED

- AGE stat-int: always Mode2 (`82`) vs expected alternating Mode0/2 (`80`/`82`)
- Moving Mode-0 IRQ to lx==168 (set mode+IRQ there): +2/+5 samples but **regressed** `halt-m0-interrupt`×3 + `hblank_ly_scx_timing-GS` — reverted
- Calling `statUpdate()` at existing lx==168 HBlank arm (without moving bus unlock): same +2/+5 and **same regressions** — reverted
- Need longer post-IRQ Mode-0 without earlier IRQ edge / without breaking halt-m0

### P6 — Mealybug WX=6 — BLOCKED (instrumented)

Forensics (Mode-2 IRQ ROM `m3_wx_6_change`):

- Mode-2 writes `WX=6` at LX=168 (lx not yet reset); Mode-3 `WX=LY` lands at **LX=8 or 11**, `WX=80` at LX≈100–107
- Current poison at LX=7 (WX still 6) blocks **all** later opens → no staircase; LY=6 black matches via desync fill
- Dropping poison + exact-column opens: window opens at `WX=LY` (LY≥7) but **WLY is short** if glitch lines do not advance WLY; even with WLY++ on glitch suppress, LY=12 still lacks EXP whites (tile row / LCD desync still wrong)
- Disabling suppress entirely: **6945 → 13799** (worse)
- SameBoy model: equality open + early `WX == position+6` with `lcd_x--` + end-of-line last-color fill; `wx_just_changed` for 1 T-cycle on DMG WX writes. Not yet implemented end-to-end
- CGB expected PNGs remain **placeholders** — cannot XPASS

Baseline remains **6945** px DMG with poison/desync from #289. Experiments reverted.

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

1. Continue sprite residual without blanket SCX≠0 / SCX=0 / SCX==LY bubbles (tellinglys-sensitive)
2. STAT-int: longer Mode-0 *after* IRQ without moving the edge (halt-m0 sensitive)
3. Mealybug: SameBoy `lcd_x` + early WX+6 — additive probe left score at 6945 and needs suppress/poison rethink first
4. LCD-align-LY: 40/40 first-sample inverted — LCD-on LY phase (not a one-dot tweak)
5. Skip inventing CGB Mealybug refs
