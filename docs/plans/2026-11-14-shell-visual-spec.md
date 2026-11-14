# Shell visual and behavioural spec (index)

Status: reference for waves G, H, I (ui, sidebar/tab bar/layout, settings crates) and the parity pass. Scope: the four shell surfaces of the Tauri desktop app as they are today. Visuals are 1:1; behaviour records **intent**, never bugs (owner rule): flows with known defects carry separate "Intent" and "Observed bugs" subsections.

Reference: React app `app/panes` (was `web/`), debug Tauri build in container `inventory-app` (Linux, WebKitGTK, opaque chrome, `crowbar` theme, light, UI font size 15 px, scale 1). Source citations are `file:line` or `file :: symbol` per spec file. Every numeric value was read from source; the ones marked "live" below were also checked with `getComputedStyle` in the running app.

## Files

| Surface | Files in `docs/plans/shell-spec/` |
|---|---|
| Tokens, type, radii, shadows, motion, z-order, icons, UI primitives | `00-tokens.md`, `00-tokens-b.md` |
| 1. Sidebar | `sidebar.md` (anatomy), `sidebar-b.md` (behaviour, menus, DnD, ownership, port list, intent vs bugs) |
| 2. Settings modal | `settings.md` (chrome, rows, controls, intent vs bugs), `settings-pages.md` (every field), `settings-schema.md` (proposed schema, port list, ownership) |
| 3. Tabs top bar | `tabs-topbar.md` (window chrome, view-tab strip, drag engine, intent vs bugs), `tabs-pane-strip.md` (pane tab strip, New Tab page, shortcuts, port list) |
| 4. Chat organisation in a tab | `pane-layout.md` (model, operations, geometry, sash, DnD), `pane-layout-chrome.md` (headers, halves, console dock, animations, persistence, port list), `pane-layout-intent.md` (intent vs observed bugs) |
| Data-migration status (REPLACED) | `migration-banner.md` |
| Capture harness | `tools/scripts/parity/README.md` |

Golden captures (scratchpad, not committed): `.../scratchpad/golden/reference/<scenario>/` with index `.../scratchpad/golden/INDEX.md` (state to file) and `computed-styles-{shell,settings}.json`. About 300 PNGs: 1x client-area screenshots, native-menu captures (`kind=root`), 30 fps frame sequences.

## Reference environment (how it was run)

- Container `inventory-app` (image `inventory-dev:1`, noVNC on 127.0.0.1:6091, XFCE on Xvnc `:1`, 1500x950). Not touched: `crowbar-novnc`, `spike-*`.
- App: `/workspace/desktop/src-tauri/target/debug/crowbar-desktop` (debug Tauri with MCP bridge on :9223), Vite dev server on :5399 (`bun --bun x vite --port 5399`), launched by `/root/launch-ref.sh` with `CROWBAR_HOME=/root/data/home`, `CROWBAR_DEV_PORT=5399`. The `/workspace` volume is the pre-restructure copy (`web/`, `desktop/`); the old Tauri crate still builds, so no rebuild was needed.
- Data: `crowbar-seed` (project "Crowbar Seed": repo, review branch with diff, 2 review threads) built inside the container and run against the live daemon; the existing project "inv" (space "demo", `main`, chat `inv-rb`, terminals, review tab) comes from the inventory pass. `crowbar-seed-chat` needs the daemon stopped and no provider CLI exists, so there is no transcript (gap).
- State is frozen as `/root/snap-golden.tgz` (daemon home + webview storage). `snapshot.sh restore golden` returns to it.
- Desktop prepared once: one workspace and XFCE Ctrl+Alt+Arrow grabs removed (`prepare-desktop.sh`); `windowmove` to (5,40) and size 1200x800 at attach.
- Linux title bar is the OS one (XFCE); only macOS has the overlay title bar and traffic-light inset (spec in `tabs-topbar.md`). Captures are the client area, so decorations never enter a diff.
- Linux has no compositor: `--chrome-bg` and `--pane-chrome-bg` are opaque there. The macOS translucent values are specified (in `00-tokens.md`) but cannot be captured in this environment (gap).

## Live verification of source numbers (getComputedStyle, 1200x800)

All matched the specs unless noted. Raw data: `computed-styles-shell.json`, `computed-styles-settings.json`.

| Element | Live | Spec |
|---|---|---|
| Sidebar track | 294 px grid column, 1 px divider, handle 8 px wide at x=290 | default 294, handle (`sidebar.md`) ok |
| Sidebar resize | min 250 (drag below clamps), collapse when dragged below 125, stored width kept (e.g. 419) | `sidebar.md` ok (min 250, collapse < 125, max 640) |
| Mode bar | y=92, h=36, radius 10, tab 90x32 radius 8, transition .15 s, indicator .2 s `#fcfcfa` radius 10 | ok |
| Project switcher row | y=42, h=44; project name 19 px | ok |
| Space scroller | y=134, h=626 | ok |
| Tree foot divider | 1 px, 15 % alpha | ok |
| View-tab strip | row y=1 h=40 inside the 42 px top row; tab y=5 h=32, 13 px text, radius 10, w=234 for one tab | ok (`tabs-topbar.md`) |
| Pane tab | y=49 h=32 radius 6 13 px, transition .15 s | ok |
| Pane top row | h=42 | ok |
| Pane content | radius 10, fill `#fcfcfa`, transition .15 s | ok |
| Segmented control (review tabs) | h=34, items h=30, radius 10/8 | ok |
| Settings dialog | x=60 y=60 w=1080 h=592, radius 14, fill white, 0.2 s, backdrop black 32 % | ok (74vh capped 820, 90vw capped 1120) |
| Settings rail tab | w=192 **h=32**, radius 14, 0.15 s | `settings.md` estimated about 42.5 px (VERIFY): live is 32 px. Use 32. |
| Select trigger | 176x28, radius 10, 14 px | ok |
| Switch | 30x18 | ok |
| Search input | 254x26 in the header | ok |

## Port list (summary; per-surface lists with every file are in the spec files)

Must port (reachable): sidebar frame, resize handle, collapse and peek, header with project switcher and host picker, mode bar (Chats, Files, Changes), space carousel and tree rows with all states, row actions and menus, create-space panel, drag and drop with ghost and drop indicators, Files and Changes panels, toasts, removal tray; view-tab strip, drag engine, pane tab strip and its menus, New Tab page, pane handle, review shortcut button, split and detach behaviours; pane tree with sashes, active ring, drop zones and overlay, chat/terminal switch, console dock; settings modal (Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active sessions, Keybindings, Hosts), keymap registry (25 commands) and resolver.

Must not port (dead, stub, dev-only, mobile-only, or not shell): `commitDiff` pane, nav-stack layer, build readout and the Appearance "Build readout" row, Developer settings tab, extension toolbar and registry stubs, editor-group commands, `ui-state-store` dead fields (incl. the 260 px `sidebarWidth` decoy), bottom pane, pane fullscreen, `distributePaneSplit`, `setPaneLocked`, `splitPane`/`createPaneBeside` centre-zone path, unused `editor-pane.tsx`, dev-only chat `split` presentation, every `components/layout/mobile/*` and `use-native-*`/`mobile-*` file, settings keys with no consumer (`windowTransparency`, `coreFeatures`, `sidebarWidth`, `buildBadgeOverride`, theme-sync keys), Editor page unless the editor stays a web pane.

Web-view panes stay React (chat, Plate, files editor, review, browser content); the shell hosts them. The terminal gets a native crate later (wave 3).

## Non-visual behaviour to keep (full lists per surface)

Keyboard (Ctrl/Cmd 1/2/3 sidebar modes, T, J, N, Shift+N, Shift+T, W, O, grave, slash, zoom, pane navigation), sidebar width/open persistence and clamps, drag thresholds (view tabs 8/6 px, 150 ms settle, 36 px edge band, 14 px/frame; pane tabs 6 px dnd-kit), drop rules (reorder at edges, merge at centre, pinned groups isolated, working chats refused), pane drop zone threshold 0.25 and 100 ms overlay animation, 300 ms debounced layout persistence, 10-tab cap with eviction, dirty-close dialog, removal hold and undo clock, settings search and recorder, optimistic agent table writes. Details and thresholds: `sidebar-b.md`, `tabs-topbar.md`, `tabs-pane-strip.md`, `pane-layout*.md`, `settings-schema.md`.

## Daemon owns vs shell owns

Daemon (shell reads and calls, never copies): projects, repos, workspaces, chats (titles, working state, runner), operations, row placement order, unread and failed notices, active sessions, hosts and status, git status and diffs, file tree and documents, providers and descriptors, terminal sessions, review threads, persisted settings (including `tabListScope`, `sidebarPosition`, `maxOpenTabs`), migration/health state (`GET /v0/health`).

Shell (view state): sidebar open/width/peek/dock side, selected mode, folded rows, host filter, inline edit state, removal hold list, drag state, toasts; views (order, pinned, active), pane tree and ratios, editor-tab lists and active tab, recent-pane order, closed-tab history, chat/editor ratio (not persisted today, bug), console dock prefs, window geometry; settings modal open/page/search state and keybinding capture. Today this view state lives in webview IndexedDB (`workspace-layout`, version 4, row `window`) and localStorage; the GPUI shell must choose its own store and decide whether to import old rows (open question).

## Data migration (REPLACED)

No banner component exists: a pinned toast at the sidebar foot ("Crowbar is upgrading its data", "Step i of n: name"), a landing spinner and a halt card render one daemon state read by polling `GET /v0/health` (`status` migrating|halted|ok, `detail` with step, index, total). The native replacement is a full-screen Rust view that hides the layout, shows state and progress, then hands over. Full anatomy, state source, fields and requirements: `migration-banner.md`. Visual capture of the old toast: not obtained (gap below).

## Intent vs observed bugs

Present in `tabs-topbar.md` (section 9), `tabs-pane-strip.md` (section 12), `pane-layout-intent.md`, `settings.md` (section 11), `sidebar-b.md`, `migration-banner.md` (section 5), `00-tokens-b.md` (workarounds). Highest-impact items for the Rust side: view-tab context menu is unreachable today (handler looks for `role=treeitem`; tabs are `role=tab`; a native WebKit menu leaks instead, captured in `30-tabs/tab-22-view-tab-menu`), bulk-close skips the dirty prompt, focus after close goes to the first leaf, a closed pane's editor tabs attach to an unrelated chat, reopen restores only file tabs, split-drop previews that do not match the drop result, the 10 px drag floor vs 10 % commit clamp, `mod+w` closing a chat pane, settings search matchers that disagree, the Browser tab capability race, no pin UI reachable. Owner decisions pending are marked OWNER in the files.

## Gaps (not captured or not verified)

- Migration toast, landing spinner and halt card visuals: needs a daemon in migrating state (a schema-older home) or health injection; not done.
- Real agent states (transcript, composer with provider, permission prompt, Agents page with installed providers, Active sessions with rows, working spinner): no provider CLI in the container. `crowbar-seed-chat` cannot run while the daemon owns the home. Allowed by policy, not done here.
- Hosts page with remote hosts, host picker with 2+ hosts, unreachable-host styling, Add-host dialog: need a second daemon.
- macOS-only: overlay title bar, traffic-light inset, translucent chrome/vibrancy, native menu bar. CI-verified only.
- Dark theme and the `frosted` theme: specified from source with resolved hex in `00-tokens.md`; screenshots were captured in the light `crowbar` theme only (switching theme through Settings was not scripted).
- Keyboard chords swallowed by the desktop (Ctrl+Alt+Arrow pane navigation) were freed by `prepare-desktop.sh` but pane navigation was not captured.
- Native OS menus (context menus, select popups) are captured as whole-desktop frames; the styled React fallback menu is specified from source only.
- `ctrl+N` and several chords did nothing while focus was in the terminal; the sidebar-row and view-tab flows were driven by clicks instead.
- Animations captured as 30 fps frame sequences (sidebar collapse/expand, mode switch, hover, settings open/close, pane tab switch, console, tab hover); drag-follow and edge-scroll animations not recorded.
- Computed styles cover the elements listed in `tools/scripts/parity/styles/*.tsv`, not every row and control.
