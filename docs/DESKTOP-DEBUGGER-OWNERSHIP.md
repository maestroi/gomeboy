# Desktop debugger/tool ownership

This document defines the migration boundary for the Fyne-only desktop tooling so GomeBoy can converge on GLFW without recreating every Fyne panel.

The rule is intentionally narrow: GomeBoy keeps generic emulator-development primitives and lightweight standalone controls; PokéPilot owns agent/operator workflow UI; obsolete or low-value Fyne-specific presentation is removed rather than ported.

## Decisions

| Fyne-only area | Decision | Destination / rationale |
| --- | --- | --- |
| CPU/register inspection | **Keep primitive, drop Fyne view** | Keep the public `pkg/gomeboy` CPU/debug inspection API. Do not recreate the current Fyne register window in GLFW. Generic emulator debugging can consume the API from tests, CLI tooling, or a future focused debugger. |
| Memory viewer | **Keep primitive, drop Fyne view** | Keep `Peek8`, `Peek16`, `PeekInto`, snapshots, and memory tracking in `pkg/gomeboy`. PokéPilot should build Pokémon/operator-specific memory UX on top of those primitives. No GLFW hex-editor panel is required for the Fyne removal. |
| OAM / sprite inspection | **Keep primitive when useful, drop Fyne view** | Preserve generic PPU/OAM inspection capability where already exposed or needed by emulator tests. Do not port the Fyne sprite browser to GLFW as a prerequisite. |
| Palette viewer | **Drop Fyne view** | Palette rendering is primarily emulator-development inspection. Keep underlying PPU state/data access where useful; no standalone GLFW palette window is required. |
| Tiles / tilemaps | **Drop Fyne views** | These are specialist emulator-debugger visualizations. Do not block the desktop cutover on recreating them. Future debugger tooling may consume generic PPU inspection APIs. |
| Audio visualizer | **Drop** | The visualization is not required for normal standalone use or PokéPilot. Audio mute/unmute belongs in the lightweight GLFW control work tracked by #11. |
| Camera tooling | **Keep emulator capability, drop Fyne tooling** | Preserve Game Boy Camera emulation and generic camera device support. Remove the Fyne-specific camera control/preview UI unless a concrete standalone requirement emerges. |
| Printer tooling | **Keep emulator capability, drop Fyne tooling** | Preserve printer emulation/output support. Do not port the Fyne printer window merely for parity. A future export-oriented utility can be added independently if users need it. |
| Cheats | **Keep core capability, defer UI** | Keep generic cheat support in the emulator core. A GLFW cheat editor is not required to remove Fyne; add a focused UI later only if there is demonstrated standalone demand. |
| Clipboard / integration utilities | **Remove with Fyne unless still used elsewhere** | Fyne-specific clipboard helpers are presentation-layer glue. Generic integrations should be implemented at the consuming frontend rather than retained solely for Fyne parity. |
| Video layer controls | **Keep only everyday controls** | Lightweight user-facing controls that materially affect normal play belong in GLFW. Deep layer-by-layer PPU debugging does not. Any required everyday control should be tracked under #11. |
| Settings/debug windows | **Split by audience** | Everyday standalone settings go to GLFW under #11. Emulator-development inspection remains available through generic APIs/tests; agent/operator settings belong in PokéPilot; Fyne-only window chrome is removed. |

## Consequences for the Fyne removal

Fyne can be deleted once the following blockers are satisfied:

1. #10 provides a usable GLFW open-ROM/no-ROM flow.
2. #11 provides the remaining everyday standalone controls such as screenshot and mute.
3. Generic emulator capabilities currently hidden behind Fyne remain available through core/public APIs where they are still valuable.
4. No PokéPilot integration imports or depends on the Fyne packages.

The following are explicitly **not blockers** for #13: recreating CPU, memory, OAM, palette, tile, tilemap, visualizer, camera, printer, or cheat windows in GLFW.

## Ownership boundary

### GomeBoy

GomeBoy owns game-agnostic emulator behavior and inspection primitives: CPU/PPU state, side-effect-free memory inspection, cartridge/ROM metadata, save-state mechanics, device emulation, and lightweight controls needed to use the emulator as a standalone desktop application.

### PokéPilot

PokéPilot owns agent/operator workflow and Pokémon-specific interpretation: semantic memory labels, route/planner diagnostics, game-state dashboards, reverse-engineering workflows, and any UI whose primary purpose is supervising or debugging an automated Pokémon agent.

### Removed with Fyne

Fyne-specific presentation code with no standalone requirement should be removed instead of mechanically ported. If a future concrete use case appears, it should be implemented against the generic emulator APIs rather than restoring a second desktop framework.

## Follow-up tracking

Existing issues already cover the blocking implementation work:

- #10 — GLFW standalone open-ROM flow.
- #11 — remaining everyday GLFW controls.
- #13 — make GLFW the default desktop frontend and remove Fyne.
- #15 — generic introspection API for external automation and reverse engineering.

Additional Fyne-only debugger views do not require migration issues before #13 unless a missing generic emulator capability is discovered while deleting Fyne.