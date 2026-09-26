# jsmolka/gba-tests ARM and Thumb conformance

This adapter runs the prebuilt ARM and Thumb ROMs from
[`jsmolka/gba-tests`](https://github.com/jsmolka/gba-tests), pinned to commit
`a7113b67e63f83a9b321696ddd7042ccfad6c881`. Upstream is MIT-licensed
(Copyright 2019 Julian Smolka).

The ROMs are fetched rather than copied into this repository. `fetch.sh`
verifies the exact Git blob before a ROM is accepted:

| ROM | Git blob | Checks |
| --- | --- | ---: |
| `arm/arm.gba` | `dd0c023da55cc9a62afc90bc6917ceca689d1e29` | 183 |
| `thumb/thumb.gba` | `47e50db6920d5a996051d0902365e5cdf7b166b6` | 109 |

Run locally from the repository root:

```sh
bash tests/gba/conformance/jsmolka/fetch.sh
go run ./cmd/gba-conformance \
  -manifest tests/gba/conformance/jsmolka/suite.json \
  -output gba-conformance-jsmolka.json
```

The upstream ROMs normally enter a final ARM `b .` loop after all tests pass.
On the failure path they execute `swi 0x60000` while `r0` contains the first
failed test number. The conformance adapter recognizes both states *before*
executing them, so the suite does not require a Nintendo BIOS image merely to
format its result text. The manifest explicitly supplies the minimal
post-BIOS CPU assumptions the ROMs rely on: System mode and a valid IWRAM
stack pointer.

The test-number ranges in `suite.json` are taken from the pinned upstream
sources. They let JSON results report the failing group and exact passed/total
check count. ARM and Thumb totals remain separate; they are not combined into
an overall accuracy percentage.

## Current baseline

`baseline.json` records the current known failures so CI can distinguish a
regression from an already-known gap without hiding improvements:

- ARM: XFAIL in `data_processing` test 224 — R15/PC as the shifted source in a
  register-specified shift is currently unsupported. Tracked by #92.
- Thumb: XFAIL in `memory` test 211 after 86/109 checks — odd-address `LDRH`
  rotation semantics. Tracked by #93.

An expected pass becoming a failure, a known failure moving to a different
first failure, or a timeout fails CI. A known failure becoming a pass is
reported as XPASS and also fails CI until `baseline.json` is intentionally
updated. This keeps improvements visible instead of silently moving the
compatibility line.
