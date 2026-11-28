# GB/GBC Accuracy Campaign

## Verified baseline

- **Main SHA:** `850e2860` (#290 merged) — corpus **313/329**, 16 XFAIL, gate green
- **Date:** 2026-11-28
- **Branch:** `accuracy/gb-sprite-stat-m3`

## Completed

### P1 — DS AGE window STAT WX=0 (#290)

- 765/768 → **768/768**; both DS window ROMs required PASS
- Rule: under DS, even-SCX early Mode-0 STAT expose at lx==168/EnterHBlank is suppressed for WX=0 when `SCX&7 != 0`
- PR: https://github.com/maestroi/gomeboy/pull/290

### P3a — Sprite ready-path fine-SCX Mode-3 bubble (this branch)

- Ready-path sprite fetch inserts 1 Mode-3 idle when `SCX&7 != 0`
- AGE sprites: DMG **512→536**, CGB **533→553**, DS **802→867**
- Guards: mooneye `intr_2_mode0_timing_sprites` PASS (SCX=0), tellinglys CGB PASS, `Test_Regressions` green
- Residual: NOP=0 SPX 4–7 / 12–15 still Mode0 on fine SCX **0** only (1 sample each)

## Blocked / failed experiments

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE notes LCD-off history dependence
- Glitched-line early OAM WUnlock: unit OK, AGE unchanged — reverted

### P3b — X=0 absorb SCX discard — FAILED

- On X=0 fetch complete, consume `min(remaining SCX discard, 5)` from `scxDiscarded`
- Broke `cgb-acid-hell` — reverted

### P3c — Sprite X=0 SCX stall (`min(SCX&7,5)`) — FAILED

- AGE +5/+4/+6 but **regressed tellinglys CGB** — reverted

### P3d — Flat `objFetchAlign=1` every sprite — FAILED

- AGE 512→540 / 533→561 / 802→871 but **mooneye sprites + tellinglys CGB** fail — reverted

## P3 — Sprite Mode-3 STAT (ACTIVE, residual)

| Profile | Score |
|---|---|
| DMG NS | 536/640 |
| CGBBC/CGBDE NS | 553/640 |
| CGBBC/CGBDE DS | 867/1024 |

Remaining: fine SCX=0 samples for SPX≥4 (Mode3 still 1 dot short). Cannot flat-add on SCX=0 (breaks mooneye). Next: SCX=0-safe edge (STAT visibility / prelude timing), then other XFAIL families if blocked.

Repro: `go test -tags test ./tests -run '^TestAgeSpriteSTATRawDiagnostics$' -count=1 -v`

## Remaining XFAIL (16)

OAM(1) + sprites(5) + STAT int(5) + LCD LY(2) + Mealybug wx6(3)

## P4 — STAT interrupt (BLOCKED probe)

- Matrix always reads Mode 2 (`82`) where expected Mode 0 (`80`) after Mode-0 IRQ + delay
- Raising Mode-0 IRQ at lx==168 (instead of EnterHBlank) helped AGE slightly (26→28 / 52→57) but **regressed** `halt-m0-interrupt` (3) + `hblank_ly_scx_timing-GS` — reverted
- Need a fix that lengthens post-IRQ Mode-0 without moving the IRQ edge earlier than enterHBlankDelay

## Next action

P3a PR #292 CI green — merge when ready. Next: SCX=0 sprite residual (STAT visibility, not IRQ edge), or Mealybug WX=6 / LCD-align-LY.
