# 9. Pane chrome (PANE)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| PANE-001 | Pane box: 1px var(--pane-border), radius-lg corners, pane-chrome-bg fill | web/src/features/panes/utils/pane-border.ts:40 | rig reset | — | | | | | | | |
| PANE-002 | Pane at window top: top gutter 7px ((42-28)/2); 1px gutter on inner sides | web/src/features/panes/utils/pane-border.ts:54 | rig reset; close both tabs (strip gone) | — | | | | | | | |
| PANE-003 | Edge against window (sidebar collapsed / far side): border and radius drop on that edge, flush | web/src/features/panes/utils/pane-border.ts:16 | collapse sidebar (see SB) | — | | | | | | | |
| PANE-004 | Sidebar on right: pane flush on left window edge, bordered toward sidebar | web/src/features/panes/utils/pane-border.ts:21 | rig sidebar right | — | | | | | | | |
| PANE-005 | Bottom edge: pane always flush to window bottom (no border/radius) | web/src/features/panes/utils/pane-border.ts:25 | rig reset | — | | | | | | | |
| PANE-006 | Inner seam border (1px var(--border)) only on editor side facing the chat; never rounded | web/src/features/panes/utils/pane-border.ts:75 | Open editor (side-by-side), then rig size 800x1000 (stacked) | — | | | | | | | |
| PANE-007 | Pane fill colour change on theme | web/src/features/panes/components/pane-container.tsx:230 | rig theme light ↔ dark | transition-colors 150ms | | | | | | | |
| PANE-008 | Chat pane: transcript + composer surface (contents see CHAT); header see IDE-001 | web/src/features/panes/components/pane-chat-view.tsx:57 | rig reset | — | | | | | | | |
| PANE-009 | Chat surface still loading: nothing painted inside the pane box (Suspense fallback null) | web/src/features/panes/components/pane-chat-view.tsx:77 | rig restart tauri, capture first frames | — | | | | | | | |
| PANE-010 | Editor pane loading: nothing painted while Monaco loads | web/src/features/panes/components/editor-pane.tsx:103 | open README.md just after rig restart tauri | — | | | | | | | |
| PANE-011 | Editor pane ready: breadcrumb toolbar over Monaco (contents see ED) | web/src/features/editor/components/editor-surface.tsx:274 | open src/pricing.ts | — | | | | | | | |
| PANE-012 | Editor failed: "Editor failed to load. Try closing and reopening this file." centred | web/src/features/panes/components/editor-pane.tsx:88 | Seed: Monaco load failure | — | | | | | | | |
| PANE-013 | Unavailable document: title, muted message, outline "Try again" button (spinner while retrying) | web/src/features/panes/components/editor-pane.tsx:146 | Seed: open file tab whose file was deleted on disk / host offline | button spinner | | | | | | | |
| PANE-014 | Terminal pane: xterm surface, kept mounted across tab switches (contents see TERM) | web/src/features/panes/components/pane-editor-view-content.tsx:96 | ctrl+j | — | | | | | | | |
| PANE-015 | Terminal pane without session: renders nothing | web/src/features/panes/components/terminal-pane.tsx:33 | rig restart daemon with a terminal tab open | — | | | | | | | |
| PANE-016 | Browser pane toolbar: Back, Forward, Reload, Pick element (crosshair), Address input "Search or enter address", border-b | web/src/features/browser/components/browser-pane.tsx:91 | New Tab page > New Browser | — | | | | | | | |
| PANE-017 | Browser buttons disabled state with no URL / no history (Pick element disabled without URL) | web/src/features/browser/components/browser-pane.tsx:129 | New Browser, empty address | — | | | | | | | |
| PANE-018 | Browser "Agent" info badge while an agent drives the page | web/src/features/browser/components/browser-pane.tsx:138 | Seed: agent driving browser | — | | | | | | | |
| PANE-019 | Browser with page loaded (contents see BRW) | web/src/features/browser/components/browser-pane.tsx:149 | type a URL, Return. Seed: network | — | | | | | | | |
| PANE-020 | Branch review pane header: segmented Conversation (thread count badge) / Files changed (file count badge) | web/src/features/git/components/branch-review-pane.tsx:129 | feature workspace; Review this branch | — | | | | | | | |
| PANE-021 | Review header badges hidden when pane narrower than 420px | web/src/features/git/components/branch-review-pane.tsx:130 | open review, rig size 640x480 in tabs presentation | — | | | | | | | |
| PANE-022 | Review diff loading: spinner + "Loading branch diff" / "Loading commit diff" | web/src/features/git/components/review-diff-tab.tsx:115 | open review, Files changed, capture first frame | spinner | | | | | | | |
| PANE-023 | Review empty: "No changes between this branch and its parent." + Refresh | web/src/features/git/components/review-diff-tab.tsx:125 | Review Branch from New Tab in main workspace. Seed: reviewable branch with no diff | — | | | | | | | |
| PANE-024 | Commit diff pane: title "Commit <sha7>", empty "This commit changed no files." | web/src/features/panes/components/commit-diff-pane.tsx:29 | Git sidebar > click a commit (see SB) | — | | | | | | | |
| PANE-025 | Markdown / HTML preview pane (contents see ED) | web/src/features/panes/components/pane-content-registry.tsx:55 | open README.md, breadcrumb Preview | — | | | | | | | |
| PANE-026 | New Tab page pane (see IDE-061) | web/src/features/panes/components/new-tab-view.tsx:46 | ctrl+t | — | | | | | | | |
| PANE-027 | Pane error state: "This pane encountered an error" (destructive), error message pre, Retry | web/src/features/panes/components/pane-boundary.tsx:42 | Seed: content that throws during render | — | | | | | | | |
| PANE-028 | Pane error Retry: re-renders the pane | web/src/features/panes/components/pane-boundary.tsx:31 | Seed: transient render error; click Retry | — | | | | | | | |
| PANE-029 | Sidebar-surface focus: clicking pane blank area focuses its root (no outline) | web/src/features/panes/lib/sidebar-surface.ts:23 | click empty transcript area | — | | | | | | | |
| PANE-030 | Console closed by default (dock top, overlay, 420px) | web/src/features/console/lib/dock-prefs.ts:33 | rig reset | — | | | | | | | |
| PANE-031 | ctrl+` toggles console; overlay slides in from dock edge over the app, shadow-lg, bg-code | web/src/features/console/components/console-panel.tsx:84 | ctrl+grave | transform 200ms ease-out | | | | | | | |
| PANE-032 | macOS: Window > Toggle Console menu item (accelerator mirrors keymap) | desktop/src-tauri/src/console_menu.rs:74 | macOS menu bar Window > Toggle Console | — (native) | | | | | | | |
| PANE-033 | Console header 36px, right-aligned Dock and Mode buttons, header is window drag region; macOS pl-20 docked top/left | web/src/features/console/components/console-header.tsx:32 | open console | — | | | | | | | |
| PANE-034 | Dock button cycles top → right → bottom → left; tooltip names current and next | web/src/features/console/components/console-header.tsx:40 | open console; click dock button 4× | panel re-docks; slide 200ms ease-out | | | | | | | |
| PANE-035 | Mode button: Overlay (Stack icon) ↔ Push (SquareHalf icon, aria-pressed); push shrinks the app beside it | web/src/features/console/components/console-header.tsx:50 | open console; click mode button | width/height 200ms ease-out | | | | | | | |
| PANE-036 | Push mode open/close animates panel size, app reflows | web/src/features/console/components/console-panel.tsx:85 | push mode; ctrl+grave twice | width/height 200ms ease-out | | | | | | | |
| PANE-037 | Overlay: pointer-down outside closes (press still reaches its target); push mode does not | web/src/features/console/components/console-panel.tsx:54 | overlay open; click the chat | transform 200ms ease-out | | | | | | | |
| PANE-038 | Escape inside console closes it | web/src/features/console/components/console-panel.tsx:38 | open console; click a log line; Escape | transform 200ms ease-out | | | | | | | |
| PANE-039 | Resize handle: drag edge, clamps 160px…80% of window; hover/focus bg-ring/40; resize skips transition | web/src/features/console/components/console-resize-handle.tsx:81 | open console; drag its inner edge | — | | | | | | | |
| PANE-040 | Resize handle keyboard: arrows step 16px; double-click resets to 420px | web/src/features/console/components/console-resize-handle.tsx:92 | focus handle (Tab); ArrowDown ×3; double-click | — | | | | | | | |
| PANE-041 | Daemon log lines: mono xs grid time(HH:MM:SS.mmm) / LEVEL / message / key=value; level colours debug muted, info, warn, error | web/src/features/console/components/log-row.tsx:52 | open console | — | | | | | | | |
| PANE-042 | Click a log line: expands JSON view (bg-muted, aria-expanded); click again collapses | web/src/features/console/components/log-row.tsx:49 | open console; click a line | — | | | | | | | |
| PANE-043 | Raw (non-JSON) line and "Daemon restarted" note rows | web/src/features/console/components/entry-row.tsx:18 | console open; rig restart daemon | — | | | | | | | |
| PANE-044 | Log follows newest line while scrolled within 32px of bottom; stays put when scrolled up | web/src/features/console/components/console-log.tsx:69 | console open; scroll up; generate daemon activity | — | | | | | | | |
| PANE-045 | Log virtualized, capped at 5000 entries; stream only while open | web/src/features/console/stores/console-store.ts:15 | console open long session | — | | | | | | | |
| PANE-046 | Dock / mode / size persist across restart (localStorage) | web/src/features/console/lib/dock-prefs.ts:19 | change dock + mode + size; rig restart tauri | — | | | | | | | |
| PANE-047 | Console in light / dark theme | web/src/features/console/components/console-panel.tsx:81 | rig theme light; rig theme dark | — | | | | | | | |

Skipped:
- Pane fullscreen overlay (web/src/features/panes/components/split-view-root.tsx:88-99): `togglePaneFullscreen` has no caller.
- Bottom pane chrome (BOTTOM_PANE_ID): `setIsBottomPaneVisible` has no caller.
- Mobile chat stage and phone New Tab variant (web/src/features/panes/components/mobile-chat-stage.tsx, new-tab-view.tsx:42): mobile only.
- Console version-badge trigger: no `aria-controls="console-panel"` element exists on develop; only the chord and the macOS menu open it.
