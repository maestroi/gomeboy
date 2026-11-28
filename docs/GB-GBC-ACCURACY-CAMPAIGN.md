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
- Gating on Mode-2 STAT Bit5 still regresses tellinglys; gating on `SCX` written in Mode 2 gave **no AGE gain** (Mode may not read as OAM at IRQ write) and still flaked tellinglys — reverted
- Telliglys is LY/joypad-entropy sensitive to even rare Mode-3 length changes

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE `oam-write.inc` marks that line `(*)` LCD-off history dependent (TODO even in AGE)
- Glitched-line early OAM WUnlock: no AGE gain — reverted

### P3b/c/d — X=0 absorb / spx0 stall / flat align — FAILED

- See prior notes; tellinglys or mooneye regressions

### P4 — STAT interrupt — PARTIAL (Mode-0/Mode-2 rows)

- Root cause: after Mode-0 IRQ, DELAY 34's `ldh a,[rSTAT]` samples exactly at our Mode-2 visible edge → always `82`
- **Fix (landed):** Mode-0-IRQ-gated CPU-visible Mode-0 hold into `StartOAMScan` (DMG/NCM 4 dots; CGB SS 8; CGB DS 4/2 by SCX parity). `modeToInt` / bus / Mode-2 IRQ unchanged
- Raw matrix: DMG/NCM **36/40** (Mode-0 + Mode-2 rows green); CGB native **72/80** (SS Mode-0/2 green). Mode-1 VBlank rows still `82/82`
- Guards: `halt-m0-interrupt` PASS, AGE `stat-mode-*` PASS (STAT enables clear → hold not armed), PPU Mode-2 entry unit tests PASS
- Ungated hold got 40/40 including Mode-1 but **regressed AGE stat-mode** — cannot use
- Early Mode-0 IRQ (`lx==166`/`164`): AGE gain but **regresses halt-m0** — cannot use
- Mode-1 residual: DELAY 1237/1238 at LY=0→1 edge; SCX=0 vs SCX=7 need opposite hold lengths; blocked for XPASS
- Still XFAIL all 5 stat-int ROM@model until Mode-1 lands

### P6 — Mealybug WX=6 — BLOCKED (instrumented)

Forensics (Mode-2 IRQ ROM `m3_wx_6_change`):

- Mode-2 writes `WX=6` at LX=168 (lx not yet reset); Mode-3 `WX=LY` lands at **LX=8 or 11**, `WX=80` at LX≈100–107
- Current poison at LX=7 (WX still 6) blocks **all** later opens → no staircase; LY=6 black matches via desync fill
- Dropping poison + exact-column opens: window opens at `WX=LY` (LY≥7) but **WLY is short** if glitch lines do not advance WLY; even with WLY++ on glitch suppress, LY=12 still lacks EXP whites (tile row / LCD desync still wrong)
- Disabling suppress entirely: **6945 → 13799** (worse)
- SameBoy model: equality open + early `WX == position+6` with `lcd_x--` + end-of-line last-color fill; `wx_just_changed` for 1 T-cycle on DMG WX writes. Not yet implemented end-to-end
- CGB expected PNGs remain **placeholders** — cannot XPASS

Baseline remains **6945** px DMG with poison/desync from #289. Experiments reverted.

### P5 — LCD-align-LY — BLOCKED (STOP/KEY1 freeze)

- Offset-0 normal-speed groups **PASS** (LY_VALUES match EXPECTED)
- All failures are after `SWITCH_SPEED`: offset-1+ DS/NS groups
- DS offset-1 actual LY ≈ `0A 0B 0C 0D …` vs expected `00 00 01 02 8F 98 …` — **~10 lines late**
- Fixed inverted `ChangeSpeed` reschedule (was halving PPU remaining on enter-DS; now keeps device-dot lengths). Did **not** move lcd-align LY
- SameBoy uses `speed_switch_countdown`/`speed_switch_freeze` during STOP; our KEY1 STOP flips speed immediately — next probe

## Remaining XFAIL (16)

| Family | Count | Notes |
|---|---|---|
| sprites | 5 | SCX=0 residual + incomplete matrix |
| STAT int | 5 | Mode-0/2 rows fixed; Mode-1 VBlank residual |
| Mealybug wx6 | 3 | DMG desync incomplete; CGB refs missing |
| LCD-align-LY | 2 | LCD-on LY phase |
| OAM write DMG | 1 | DELAY=2 history (*) |

## Next action

1. LCD-align-LY: 40/40 first-sample inverted — LCD-on LY phase
2. Sprite residual without blanket SCX≠0 / SCX=0 / SCX==LY bubbles (tellinglys-sensitive)
3. STAT-int Mode-1: needs LY=0 Mode-0 duration fix distinct from Mode-0-IRQ hold
4. Mealybug: SameBoy `lcd_x` + early WX+6 — needs suppress/poison rethink first
5. Skip inventing CGB Mealybug refs
