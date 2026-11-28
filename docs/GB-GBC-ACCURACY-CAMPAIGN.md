# GB/GBC Accuracy Campaign

Persistent engineering memory for the continuous GB/GBC hardware-conformance campaign.

## Verified baseline

- **Main SHA (pre-session):** `f3b3ef052c10ece01fbefdba952597f0ae8b8c0d` (#289)
- **Pending P1 branch:** `accuracy/gb-window-ds-wx0`
- **Date:** 2026-11-28
- **Corpus after P1 (local gate):** **313/329 PASS**, 16 XFAIL, 0 regressions, 0 XPASS
- **AGE window STAT:** all 7 profiles 512/512 or 768/768

## Completed this session

### P1 — Double-speed AGE window STAT (WX=0 residual)

- **Before:** DS CGBBC/CGBDE 765/768 (3 samples each, all WX=0)
- **After:** 768/768 both; ROMs promoted to required PASS
- **Rule:** Under DS, even-SCX early Mode-0 STAT expose at `lx==168`/`EnterHBlank` is suppressed for WX=0 window lines when fine SCX (`SCX&7`) is nonzero. Fine SCX 0 still exposes Mode 0. Ordinary WX unchanged.
- **Evidence:** AGE samples are SCX=LY across lines; WX=0 expected Mode 3 one fine-scroll step longer than WX≥1. WinActivating already adds the Mode-3 dot; residual was CPU-visible STAT handover under DS.
- **Failed experiments:**
  - Suppress early Mode 0 for all WX=0 even SCX → 767/768 (broke fine=0 at nop+1)
  - Extra `windowMode3Stall` under DS for WX=0 → 763/768 (Mode 3 too long)
- **Files:** `internal/ppu/ppu.go`, `internal/ppu/cgb_double_speed_stat_hblank_read_test.go`
- **Repro:** `go test -tags test ./tests -run '^TestAgeWindowSTATRawDiagnostics$' -count=1 -v`

## Remaining XFAIL queue (16)

### Priority 2 — DMG OAM write (NEXT)

- `oam-write-dmgC@DMG` — floor 440/480; probe-2 residual
- Stash `stash@{0}` on old `fix/age-oam-write-dmg-probe2` has experimental glitched-line0 OAM reopen (unverified; do not apply blindly)
- Repro: `go test -tags test ./tests -run '^TestAgeOAMWriteDiagnostics$' -count=1 -v`

### Priority 3 — Sprite Mode-3 STAT (5 ROMs)

### Priority 4 — STAT interrupt (5 ROMs)

### Priority 5 — LCD align LY CGB (2 ROMs)

### Priority 6 — Mealybug `m3_wx_6_change` (DMG 6945 px, CGB 19819 px)

## Next action

1. Land/push P1 PR for DS WX=0 STAT.
2. Start P2: measure OAM-write DMG mismatches; investigate probe-2.
