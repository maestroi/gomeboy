# GB/GBC Accuracy Campaign

## Verified baseline

- **Main SHA:** `039583f7` (#292 merged) — corpus still **313/329** XFAIL (sprite scores up but not full PASS), gate green
- **Date:** 2026-11-28
- **Branch:** `accuracy/gb-next-xfail`
- **In-flight:** STAT-int Mode-1/Mode-2 IRQ hold → **318/329** (5 XPASS), pending commit

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

### P4 — STAT interrupt — DONE (40/40 / 80/80)

- Root cause: after Mode-0/1/2 STAT IRQ, handler clears STAT enables; DELAY samples Mode-0→Mode-2 edge without a visible Mode-0 hold → always `82`
- **Fix:** Mode-0 Bit3 IRQ arms hold at EnterHBlank; Mode-1 Bit4 / Mode-2 Bit5 rising edges latch `mode1IRQHoldPending` / `mode2IRQHoldPending` and arm the same hold on the following EnterHBlank (LY=0 for Mode-1). DS Mode-1/2 force 4-dot hold (Mode-0 DS keeps SCX-odd 2-dot)
- Raw matrix: DMG/NCM **40/40**; CGB native **80/80**
- Guards: `halt-m0`, AGE `stat-mode-*`, mooneye green; ungated LY=0 hold regressed stat-mode — not used
- XPASS: all 5 `stat-int-*` ROM@model (313→318)

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
- Landed KEY1 enter-DS halt of **0x20008 device dots** (Tick(2)×N in DS; clear hasFrame/hasInt across frames). Moves LY ~20→~0x8F; still XFAIL. Next: SameBoy `countdown=6` + `freeze=5`. Note: `+456*10+7` can match off1-DS d0 LY row but all five DELAY variants collapse to the same plateau and off1-NS stays ~0x14 — not an XPASS

## Remaining XFAIL (11)

| Family | Count | Notes |
|---|---|---|
| sprites | 5 | SCX=0 residual + incomplete matrix |
| Mealybug wx6 | 3 | DMG desync incomplete; CGB refs missing |
| LCD-align-LY | 2 | KEY1 STOP freeze / LY phase |
| OAM write DMG | 1 | DELAY=2 history (*) |

## Next action

1. Sprite residual without blanket SCX≠0 / SCX=0 / SCX==LY bubbles (tellinglys-sensitive)
2. LCD-align-LY: integrated KEY1 STOP freeze+halt (SameBoy `speed_switch_freeze` + `halt_countdown`)
3. Mealybug: SameBoy `lcd_x` + early WX+6 — needs suppress/poison rethink first
4. Skip inventing CGB Mealybug refs
5. OAM DELAY=2 last if still blocked
