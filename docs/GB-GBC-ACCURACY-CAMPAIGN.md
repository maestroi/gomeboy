# GB/GBC Accuracy Campaign

## Verified baseline

- **Branch:** `accuracy/gb-next-xfail`
- **Date:** 2026-11-28
- **Corpus:** **323/329** (6 XFAIL) after full sprite STAT XPASS
- **In-flight:** lcd-align / Mealybug / OAM

## Completed

### P1 — DS AGE window STAT WX=0 (#290)

- 765/768 → **768/768**
- PR: https://github.com/maestroi/gomeboy/pull/290

### P3a — Sprite ready-path fine-SCX Mode-3 bubble (#292)

- Ready-path sprite fetch inserts 1 Mode-3 idle when `SCX&7 != 0`
- AGE sprites: DMG **512→536**, CGB **533→553**, DS **802→867**
- Guards: mooneye sprites PASS, tellinglys CGB (flaky but not regressed), gate green
- PR: https://github.com/maestroi/gomeboy/pull/292

### P3h — NS sprite SCX≠0 ready-path stall — DONE (640/640)

- Widen ready-path bubble from `SCX&7!=0` to `SCX!=0` (AGE SCX=LY coarse-aligned lines)
- Combined with Mode-3 hold + enterHBlankDelay +2: NS **640/640**, DS **935/1024**
- XPASS: all 3 `stat-mode-sprites-dmgC-cgbBCE@*` (318→321); mooneye sprites PASS; tellinglys still flaky-ok
- Earlier “blanket SCX≠0 regresses tellinglys” was without Mode-3 hold — with hold, gate accepts

### P3i — DS sprite keep Mode 3 through even-phase HBlank — DONE (1024/1024)

- DS `cpuVisibleSTAT` early Mode-0 at EnterHBlank (`scx&1==0`) truncated sprite-extended Mode 3
- Gate out `spritesFetchedLine && scx!=0` (same predicate as Mode-3 hold); window WX=0 exception unchanged
- DS **935→1024/1024**; NS 640/640; window 512/768; mooneye sprites PASS
- XPASS: both `stat-mode-sprites-ds-cgbBCE@*` (321→323)

### P4 — STAT interrupt — DONE (40/40 / 80/80)

- Root cause: after Mode-0/1/2 STAT IRQ, handler clears STAT enables; DELAY samples Mode-0→Mode-2 edge without a visible Mode-0 hold → always `82`
- **Fix:** Mode-0 Bit3 IRQ arms hold at EnterHBlank; Mode-1 Bit4 / Mode-2 Bit5 rising edges latch `mode1IRQHoldPending` / `mode2IRQHoldPending` and arm the same hold on the following EnterHBlank (LY=0 for Mode-1). DS Mode-1/2 force 4-dot hold (Mode-0 DS keeps SCX-odd 2-dot)
- Raw matrix: DMG/NCM **40/40**; CGB native **80/80**
- Guards: `halt-m0`, AGE `stat-mode-*`, mooneye green; ungated LY=0 hold regressed stat-mode — not used
- XPASS: all 5 `stat-int-*` ROM@model (313→318)

## Blocked / failed experiments

### P3e — AGE SCX=LY coarse-aligned bubble alone — FAILED (superseded by P3h)

- Alone, `SCX!=0` ready-path +1 without Mode-3 hold flaked tellinglys; with hold (P3h) NS XPASS

### P3f — SameBoy always-on ready-path pre-OAM stall — FAILED

- Moved stall before object fetch (SameBoy display.c "TODO" sleep) and always-on: AGE **536→540 / 553→561 / 867→871** but **regresses mooneye intr_2_mode0_timing_sprites** — reverted
- Pre-fetch with `SCX&7!=0` gate only: same AGE scores as post-fetch stall (phase change, no sample gain)

### P3g — Sprite-line Mode-3 hold + enterHBlankDelay — DONE (enabler for P3h)

- NS/DS: keep Mode 3 until EnterHBlank when `spritesFetchedLine && scx!=0`; delay +=2
- Peak alone: NS 638/640, DS 931/1024; with P3h stall → NS 640/640, DS 935/1024

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE `oam-write.inc` marks that line `(*)` LCD-off history dependent (TODO even in AGE)
- Glitched-line early OAM WUnlock: no AGE gain — reverted
- Re-diagnosed (write-port trace: PPU line dot at each `ld [hl+],a`; probes are +4 dots apart, so every sample is a 1-in-4-dot boundary test). Required DMG-C DELAY=2 verdicts (A=accepted, B=blocked), SCX 4-7: `ABAB BABA ABAB`; SCX 0-3 differ only in the Mode-3-end samples (1,3,5,7). I.e. samples 0-3 behave like DELAY 1 (earlier phase), samples 4-7 like DELAY 3 (later phase), samples 8-11 like DELAY 1 again
- Current model fails 40 samples = 8 SCX x ~5: s0 (line-0 glitched Mode-3 start, raw dot 79: we block, DMG-C accepts), s1/s3 (Mode-3 end, dot 255: we accept at SCX 0/1, DMG-C blocks), s4/s6 (Mode-2 lock at 142→143 f0 and 153→0 f1 edge, dot 455: we accept, DMG-C blocks), s5/s7 (SCX 0 and SCX 4-7 flips in opposite directions)
- **Why no principled fix**: the same event type needs opposite shifts — Mode-2 write lock at 142→143 is "late" (>455) for f1 s10 and for 0→1 s2/s8, but "early" (≤455) for f0 s4 and 153→0 s6. No uniform per-event offset, per-model constant, or SCX rule can satisfy both; only a frame-count/LCD-off-history phase variable could, which is exactly AGE's TODO (a (*) line that changes when more than 2 frames are measured; not seen on CGB-B/E). Fitting it would be a per-(frame,line,SCX) table — overfit, not shipped
- No code change; `cpuOAMWriteOpen` trace (env-gated) was temporary and reverted. Baseline stays **440/480**

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

### P5 — LCD-align-LY — PARTIAL (SameBoy KEY1 landed, still XFAIL)

- Offset-0 normal-speed groups **PASS**; raw matrix ~35 byte mismatches (mostly enter-DS groups one LY behind + missing `LY&(LY+1)` glitch rows)
- Landed SameBoy-faithful KEY1 STOP: `countdown=6`, `freeze=5` via `TickFrozen`, halt `0x20008`, leave-DS flip-before-freeze, device-stamp rebase
- Compact probes baseline **11/12**; freeze/delay sweep best **10/9** at freeze=5 delay=7 — still XFAIL
- Post-enter-DS `Tick` fudge has **zero** effect (halt absorbs relative phase); `TickFrozen(10)` (NS×2 hypothesis) worsens to ~28
- Flip-before-preflip enter-DS order worsens probes; leave-DS path mostly OK (g=2 NS passes on CGBBC)
- Still XFAIL lcd-align-ly CGBBC+CGBDE

### P5b — LY bus AND-glitch — PARTIAL / insufficient

- Lazy LY reader returning `bus&p.ly` during `HBlankUpdateOAM` does **not** regress `^TestAge$/^ly-` (earlier note used a filter that also matched lcd-align)
- Alone does **not** fix lcd-align: edge samples still miss the 2-dot glitch window (phase), so AND never observed on failing delays
- Mealybug clear-poison-on-WX-rewrite: 6945→7478 (worse) — reverted

## Remaining XFAIL (6)

| Family | Count | Notes |
|---|---|---|
| Mealybug wx6 | 3 | DMG desync incomplete; CGB refs missing |
| LCD-align-LY | 2 | KEY1 residual LY phase |
| OAM write DMG | 1 | DELAY=2 history (*) |

## Next action

1. LCD-align-LY residual after KEY1
2. Mealybug: SameBoy `lcd_x` + early WX+6 — needs suppress/poison rethink
3. Skip inventing CGB Mealybug refs
4. OAM DELAY=2 last if still blocked
