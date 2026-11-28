# GB/GBC Accuracy Campaign

## Verified baseline

- **Main SHA:** `850e2860` (#290 merged) — corpus **313/329**, 16 XFAIL, gate green
- **Date:** 2026-11-28
- **Branch:** `accuracy/gb-sprite-stat-m3` (docs only; no PPU change yet)

## Completed

### P1 — DS AGE window STAT WX=0 (#290)

- 765/768 → **768/768**; both DS window ROMs required PASS
- Rule: under DS, even-SCX early Mode-0 STAT expose at lx==168/EnterHBlank is suppressed for WX=0 when `SCX&7 != 0`
- PR: https://github.com/maestroi/gomeboy/pull/290

## Blocked / failed experiments

### P2 — DMG OAM-write DELAY=2 (440/480) — BLOCKED

- All 40 misses are probe DELAY=2; AGE notes LCD-off history dependence
- Glitched-line early OAM WUnlock: unit OK, AGE unchanged — reverted

### P3b — X=0 absorb SCX discard — FAILED

- On X=0 fetch complete, consume `min(remaining SCX discard, 5)` from `scxDiscarded`
- Broke `cgb-acid-hell` (and ran XFAIL Mealybug as fail in Test_All) — reverted

### P3 — Sprite X=0 SCX stall — FAILED (partial AGE gain, regression)

- Tried post-fetch `objX0Stall = min(SCX&7,5)` after X=0 sprite (AGE `spx0_delay`)
- AGE sprites: DMG 512→517, CGB 533→537, DS 802→808
- **Regressed** `little-things-gb/tellinglys (CGB)` — reverted entire change
- Do not re-apply naive post-fetch stall; need SameBoy-accurate x=0 render stall that preserves tellinglys

## P3 — Sprite Mode-3 STAT (ACTIVE)

Raw scores (current main):

| Profile | Score |
|---|---|
| DMG NS | 512/640 |
| CGBBC/CGBDE NS | 533/640 |
| CGBBC/CGBDE DS | 802/1024 |

AGE setup: 1 sprite/line X=0..15, SCX=LY, OBJ on.

Next slices (distinct from reverted stall):

1. SameBoy: “Rendering (including scrolling) does not occur while object at x=0 is pending” — stall SCX discard / lx advance until X=0 fetch completes, not a post-fetch idle
2. BG-fetcher wait / extra advance before OBJ OAM sample (SameBoy display.c ~1956–972)
3. Remeasure sprites + tellinglys + full gate after every change

Repro: `go test -tags test ./tests -run '^TestAgeSpriteSTATRawDiagnostics$' -count=1 -v`

## Remaining XFAIL (16)

OAM(1) + sprites(5) + STAT int(5) + LCD LY(2) + Mealybug wx6(3)

## Next action

Implement SameBoy-style X=0 pending render stall (block pushPixel/SCX discard while X=0 sprite queued), verify tellinglys CGB still PASS, then remeasure sprite matrix.
