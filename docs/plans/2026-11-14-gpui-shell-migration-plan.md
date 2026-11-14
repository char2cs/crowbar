# GPUI shell migration: execution plan

Status: ACTIVE. Branch `migration/rust-shell`. Design lives in `2026-11-14-gpui-shell-pane-host.md` (pane host, transport, spike results) and `2026-11-14-repo-layout.md` (directories). This file is the order of work and the gate.

## Goal

Replace the Tauri desktop shell with a native GPUI shell that looks and behaves the same. "Shell" means:

1. Sidebar behaviour.
2. Settings modal.
3. Tabs top bar.
4. Chat organisation within a tab (the pane layout inside a tab).

Chat, Plate, files, review and browser content stay React panes loaded as web views. The old Tauri desktop code is removed on this branch. Tauri stays only for iOS.

## Gate (all must hold before returning to the owner)

- App works as today. The settings modal is native and schema-driven, prepared for extensions but without them.
- Desktop looks 1:1 with the Tauri app, animations included. The settings modal keeps the same visual identity.
- Nothing from the inventory (`docs/triage/shell-surface-inventory.md`) is dropped unless the owner agreed to cut it.
- Only React components that are in use are ported.
- Mobile (Swift and Tauri) builds under the new paths and its files follow the guidelines. Owner checks mobile later.
- Code is reviewed by a separate agent and by the global-rules checker; no best-effort code.
- Stable for daily use: no crashes in a long soak, bounded memory, clean shutdown, daemon recovery works.
- Rust CI green on Linux and macOS runners.

## Rules for all code

- Rabbyte manifest global rules apply (naming, no dead code, small files under 500 lines, one concept per file, tests beside code, no inline narrative comments). Go-specific rules do not; Rust equivalents come from the Rust API Guidelines and common Clean Code guidance (researched by the rules agent).
- Shared crates never depend on gpui or tauri. Logic and view are separate crates.
- Every pane message follows the extension contract and host protocol docs of the extensions branch (pane lifecycle `mount/activate/deactivate/park/destroy`, `state` capped at 64 KB, per-webview identity, `extension.event` and `extension.status` frames, hide is not deactivate).

## Verification method

Two Docker desktops with noVNC: one runs the Tauri app (reference), one runs the GPUI app. Drive both with the same scripted steps and compare screenshots per surface and per state; animations are compared frame by frame from screen recordings. The parity agent keeps a checklist and a diff report.

## Waves

**Wave 0 (me, sequential):** commit dead-code removal; commit plans; move-only restructure (`web/` to `app/panes/`, `desktop/` to `app/mobile/`, `docker/` and `scripts/` to `tools/`, license files to `legal/`); fix every path; verify web checks.

**Wave 1 (parallel):**
- A. Workspace skeleton: `app/Cargo.toml`, `crates/transport`, `crates/protocol`, `crates/model`, `desktop/app`, `crates/webview` (with the vendored gpui patch), `crates/pane_host`. First vertical slice: window, one chat pane over the daemon.
- B. Visual spec: extract exact tokens, fonts, spacing, icons and animations of the four shell surfaces from `app/panes` and from live Tauri screenshots; golden screenshots and recordings.
- C. Daemon supervisor and host management: port `sidecar/*`, `hosts`, `host_tokens`, `tls_pin`, `api_proxy`, navigation policy into Rust crates.
- D. Rules and review: rules agent compiles the Rust guideline checklist and a lint config; reviewer agent starts reviewing as code lands.
- E. CI: Rust workflows for Linux and macOS runners, workspace caching, path-fixes for existing workflows.
- F. Mobile: adapt Swift package, plugin and Tauri crate to the new paths and shared crates; bring them in line with the guidelines.

**Wave 2 (after A and B):**
- G. `ui` crate: theme tokens, fonts, icons, animation primitives, base components.
- H. Sidebar crate, tab bar crate, layout crate (pane arrangement within a tab), command palette.
- I. Settings modal crate (native, from the daemon schema, same visual identity).

**Wave 3:**
- J. Native terminal crates (`terminal`, `terminal_view`) from the spike.
- K. Browser pane and window chrome, menus, context menus, file manager, keymaps.
- L. Pane host hardening: parking, pooling, overlays hiding, extension isolation, focus and shortcut routing.

**Wave 4:** parity pass (checklist green), soak and stability, performance pass, final review, mobile build check, docs.

## Risks tracked

- macOS is verified by CI runner only; any macOS-specific issue is found there.
- IME and drag-and-drop through web views are unproven; tested during wave 3.
- GPUI is pinned to 0.2.2 with a vendored patch; upstream the patch.
