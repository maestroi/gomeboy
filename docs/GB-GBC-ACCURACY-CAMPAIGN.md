# GB/GBC Accuracy Campaign

## Verified baseline

- **Main SHA:** `f3b3ef052c10ece01fbefdba952597f0ae8b8c0d` (#289)
- **Date:** 2026-11-28
- **P1 PR:** https://github.com/maestroi/gomeboy/pull/290 — CI green; local gate **313/329**

## Completed

### P1 — DS AGE window STAT WX=0 (PR #290)

- 765/768 → **768/768** CGBBC+CGBDE; ROMs promoted PASS
- Rule: DS even-SCX early Mode-0 STAT expose suppressed for WX=0 when `SCX&7 != 0`

## Blocked

### P2 — DMG OAM write DELAY=2

- 440/480; AGE notes LCD-off history dependence; glitched-line WUnlock experiment reverted (no AGE gain)

## P3 — Sprite Mode-3 STAT (next)

| Profile | Score |
|---|---|
| DMG NS | 512/640 |
| CGBBC/CGBDE NS | 533/640 |
| CGBBC/CGBDE DS | 802/1024 |

NOP=0: higher SPX fail first (DMG ≥4, CGB ≥5). Compact agreement full.

Repro: `go test -tags test ./tests -run '^TestAgeSpriteSTATRawDiagnostics$' -count=1 -v`

## Next

Sprite Mode-3 length vs SPX; SameBoy/AGE sprite penalty; smallest fetcher fix.
