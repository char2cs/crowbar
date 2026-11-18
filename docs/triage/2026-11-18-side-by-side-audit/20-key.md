# 20. Keymap (KEY)

### Registry bindings (web/src/features/keymaps/registry.ts)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| KEY-001 | navigation.toggleConsole: toggle console panel — ⌘` / Ctrl+` (capture phase, wins over Monaco; on mac fired by Window > Toggle Console menu) | web/src/features/keymaps/registry.ts:199 | `rig key tauri ctrl+grave` twice; workspace main open, focus anywhere incl. editor | — | | | | | | | |
| KEY-002 | navigation.sidebarChats: sidebar mode Chats — ⌘1 / Ctrl+1 | web/src/features/keymaps/registry.ts:208 | `rig key tauri ctrl+1`; workspace main open, sidebar shown | — | | | | | | | |
| KEY-003 | navigation.sidebarFiles: sidebar mode Files — ⌘2 / Ctrl+2 | web/src/features/keymaps/registry.ts:215 | `rig key tauri ctrl+2`; workspace main open | — | | | | | | | |
| KEY-004 | navigation.sidebarGit: sidebar mode Git — ⌘3 / Ctrl+3 (ignored when workspace offers no Git) | web/src/features/keymaps/registry.ts:222 | `rig key tauri ctrl+3`; workspace main (repo checkout) | — | | | | | | | |
| KEY-005 | panes.navigateLeft: focus pane left — ⌘⌥← / Ctrl+Alt+← | web/src/features/keymaps/registry.ts:45 | Split pane first; `rig key tauri ctrl+alt+Left` | — | | | | | | | |
| KEY-006 | panes.navigateRight: focus pane right — ⌘⌥→ / Ctrl+Alt+→ | web/src/features/keymaps/registry.ts:52 | Split pane first; `rig key tauri ctrl+alt+Right` | — | | | | | | | |
| KEY-007 | panes.navigateUp: focus pane up — ⌘⌥↑ / Ctrl+Alt+↑ | web/src/features/keymaps/registry.ts:59 | Vertical split first; `rig key tauri ctrl+alt+Up` | — | | | | | | | |
| KEY-008 | panes.navigateDown: focus pane down — ⌘⌥↓ / Ctrl+Alt+↓ | web/src/features/keymaps/registry.ts:66 | Vertical split first; `rig key tauri ctrl+alt+Down` | — | | | | | | | |
| KEY-009 | tabs.new: New Tab page in active pane — ⌘T / Ctrl+T | web/src/features/keymaps/registry.ts:74 | `rig key tauri ctrl+t`; workspace main open | — | | | | | | | |
| KEY-010 | tabs.newTerminal: terminal tab in active pane (reuses workspace chat) — ⌘J / Ctrl+J | web/src/features/keymaps/registry.ts:82 | `rig key tauri ctrl+j`; workspace main with seeded chat | — | | | | | | | |
| KEY-011 | agent.newChat: new chat on first enabled provider, opened as its own view — ⌘N / Ctrl+N; spawn failure → toast | web/src/features/keymaps/registry.ts:92 | `rig key tauri ctrl+n`; focus outside editor. Seed: installed provider CLI for success | — | | | | | | | |
| KEY-012 | agent.newChat over an editor area: opens New Tab page in that pane instead | web/src/features/panes/hooks/use-pane-keyboard.ts:57 | Open a file, click into editor, `rig key tauri ctrl+n` | — | | | | | | | |
| KEY-013 | agent.newChatTerminal: new chat landing on Terminal — ⌘⌥N / Ctrl+Alt+N (mac: shadowed by Window > New Window menu accelerator) | web/src/features/keymaps/registry.ts:101 | `rig key tauri ctrl+alt+n`; workspace main. Seed: installed provider CLI | — | | | | | | | |
| KEY-014 | tabs.newFile: untitled document in active pane — ⌘⇧N / Ctrl+Shift+N | web/src/features/keymaps/registry.ts:108 | `rig key tauri ctrl+shift+n`; workspace main | — | | | | | | | |
| KEY-015 | tabs.openFile: native Open dialog → document in active pane — ⌘O / Ctrl+O (Tauri only) | web/src/features/keymaps/registry.ts:115 | `rig key tauri ctrl+o`; Escape the native dialog | — | | | | | | | |
| KEY-016 | agent.toggleViewMode: flip focused chat Chat ↔ Terminal — ⌘/ / Ctrl+/ (chat panes only) | web/src/features/keymaps/registry.ts:125 | Open seeded 30-turn chat, `rig key tauri ctrl+slash` twice | — | | | | | | | |
| KEY-017 | agent.cycleProvider: cycle open chat to next enabled provider — unbound by default | web/src/features/keymaps/registry.ts:134 | Bind in Settings > Keybindings (e.g. ctrl+shift+y), open seeded chat, press it. Seed: second enabled provider | — | | | | | | | |
| KEY-018 | tabs.reopenClosed: reopen last closed tab — ⌘⇧T / Ctrl+Shift+T | web/src/features/keymaps/registry.ts:141 | Close a tab (ctrl+w), `rig key tauri ctrl+shift+t` | — | | | | | | | |
| KEY-019 | tabs.reopenClosed under Compact preset — ⌘⇧R / Ctrl+Shift+R | web/src/features/keymaps/defaults/keybinding-presets.ts:18 | Settings > Keybindings > preset Compact; close a tab; `rig key tauri ctrl+shift+r` | — | | | | | | | |
| KEY-020 | tabs.closeActive: close active tab (neighbour activates; dirty buffer → unsaved prompt) — ⌘W / Ctrl+W, never closes the window | web/src/features/keymaps/registry.ts:148 | Open a file, `rig key tauri ctrl+w`; edit a file then ctrl+w for prompt | — | | | | | | | |
| KEY-021 | tabs.closeActive on an empty split closes that split; no-op in the last pane | web/src/features/panes/hooks/use-pane-keyboard.ts:158 | Split pane, focus empty one, `rig key tauri ctrl+w` | — | | | | | | | |
| KEY-022 | agent.zoomIn: chat zoom +10% (max 300%) — ⌘= / Ctrl+= (one step per press) | web/src/features/keymaps/registry.ts:155 | Open seeded chat, `rig key tauri ctrl+equal` ×3 | — | | | | | | | |
| KEY-023 | agent.zoomOut: chat zoom −10% (min 30%) — ⌘- / Ctrl+- | web/src/features/keymaps/registry.ts:162 | Open seeded chat, `rig key tauri ctrl+minus` ×3 | — | | | | | | | |
| KEY-024 | agent.zoomReset: chat (and editor/terminal) zoom to 100% — ⌘0 / Ctrl+0 | web/src/features/keymaps/registry.ts:169 | After KEY-022 `rig key tauri ctrl+0` | — | | | | | | | |
| KEY-025 | editor.save: save active buffer — ⌘S / Ctrl+S (display-only; hardcoded) | web/src/features/keymaps/registry.ts:177 | Edit a file, `rig key tauri ctrl+s`; dirty dot clears | — | | | | | | | |
| KEY-026 | editor.saveAll: save every dirty buffer — ⌘⇧S / Ctrl+Shift+S (display-only; hardcoded) | web/src/features/keymaps/registry.ts:184 | Edit two files, `rig key tauri ctrl+shift+s` | — | | | | | | | |
| KEY-027 | browser.togglePick: pick-element mode in focused browser tab with a page — ⌘⇧E / Ctrl+Shift+E | web/src/features/keymaps/registry.ts:191 | Browser tab on a page, focused pane, `rig key tauri ctrl+shift+e`. Seed: reachable local page URL | — | | | | | | | |

### Hardcoded shell-level keys

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| KEY-028 | Toggle sidebar — ⌘B / Ctrl+B (not in registry) | web/src/components/ui/sidebar.tsx:73 | `rig key tauri ctrl+b` twice; any focus | — | | | | | | | |
| KEY-029 | Switch project by position — ⌘⇧1…8 / Ctrl+Shift+1…8, 9 = last; past the end no-op (by `code`, capture) | web/src/features/keymaps/hooks/use-space-switcher-keyboard.ts:10 | `rig key tauri ctrl+shift+1`, `ctrl+shift+9`, `ctrl+shift+2`. Seed: second project | — | | | | | | | |
| KEY-030 | Esc dismisses the sidebar while it overlays content in a narrow window | web/src/components/layout/sidebar-split-pane.tsx:117 | `rig size tauri min`, ctrl+b to open overlay, `rig key tauri Escape` | — | | | | | | | |
| KEY-031 | Sidebar resize separator ("Resize sidebar"): ←/→ ±16px (Shift ×4), Home collapse, End max, Enter toggle | web/src/components/layout/sidebar-split-pane.tsx:262 | Tab to the separator; `rig key tauri Right shift+Right Home Enter End` | — | | | | | | | |
| KEY-032 | Esc closes the console panel when focus is inside it | web/src/features/console/components/console-panel.tsx:42 | ctrl+grave, click inside log, `rig key tauri Escape` | — | | | | | | | |
| KEY-033 | Tab strip keys on a focused tab: ←/→ previous/next, Home/End, Delete/Backspace close (pinned: refused), Enter/Space activate | web/src/features/tabs/hooks/use-tab-keyboard-nav.ts:31 | Open 3 files, click a tab, `rig key tauri Left Right End Home Delete` | — | | | | | | | |
| KEY-034 | Ctrl+F / ⌘F opens file-tree search when the tree is visible and nothing else took the key | web/src/features/file-explorer/file-explorer/hooks/use-tree-search.ts:75 | ctrl+2, click empty content area, `rig key tauri ctrl+f` | — | | | | | | | |

### macOS application menu accelerators (desktop/src-tauri/src/app_menu.rs, built only on macOS — lib.rs:89)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| KEY-035 | Crowbar > Hide Crowbar — ⌘H (mac) / none on Linux | desktop/src-tauri/src/app_menu.rs:23 | macOS only; Linux: `rig key tauri ctrl+h` expect no app action | — | | | | | | | |
| KEY-036 | Crowbar > Hide Others — ⌥⌘H (mac) / none on Linux | desktop/src-tauri/src/app_menu.rs:24 | macOS only | — | | | | | | | |
| KEY-037 | Crowbar > Quit Crowbar — ⌘Q (mac) / none on Linux | desktop/src-tauri/src/app_menu.rs:27 | macOS only; Linux: `rig key tauri ctrl+q` expect no quit | — | | | | | | | |
| KEY-038 | Edit > Cut — ⌘X (mac menu) / Ctrl+X native webview on Linux | desktop/src-tauri/src/app_menu.rs:32 | Select text in an input, `rig key tauri ctrl+x` | — | | | | | | | |
| KEY-039 | Edit > Copy — ⌘C (mac menu) / Ctrl+C native webview on Linux | desktop/src-tauri/src/app_menu.rs:33 | Select text in an input, `rig key tauri ctrl+c` | — | | | | | | | |
| KEY-040 | Edit > Paste — ⌘V (mac menu) / Ctrl+V native webview on Linux | desktop/src-tauri/src/app_menu.rs:34 | Focus an input, `rig key tauri ctrl+v` | — | | | | | | | |
| KEY-041 | Window > New Window — ⌥⌘N (mac) / not bound on Linux (Ctrl+Alt+N = KEY-013) | desktop/src-tauri/src/app_menu.rs:41 | macOS only; Linux: see KEY-013 | — | | | | | | | |
| KEY-042 | Window > Toggle Console — accelerator mirrors navigation.toggleConsole at runtime (default ⌘`), cleared if inexpressible | desktop/src-tauri/src/console_menu.rs:108 | macOS only; rebind Toggle console in Settings, check menu | — | | | | | | | |
| KEY-043 | Window > Close Window — ⇧⌘W (mac) / none on Linux | desktop/src-tauri/src/app_menu.rs:45 | macOS only; Linux: `rig key tauri ctrl+shift+w` expect no close | — | | | | | | | |
| KEY-044 | Window > Minimize — ⌘M (mac) / none on Linux | desktop/src-tauri/src/app_menu.rs:54 | macOS only | — | | | | | | | |

Skipped:
- app_menu.rs:19-25 About / Services / Show All — no accelerator; its items are MENU rows.
- Composer keys (composer-keys.ts:81 Ctrl+R, plus-menu.ts:12 mod+U, prompt-stash-persistence.ts:204 mod+S), terminal find/zoom (use-terminal-shortcuts.ts:28), review diff find (review-diff-tab.tsx:172), tree clipboard (use-tree-container-events.ts:189), Plate markdown marks/blocks (basic-marks-kit.tsx, basic-blocks-kit.tsx) — pane-internal, not in the registry; rows are in CHAT (CHAT-265, CHAT-271), TERM, REV, SB (SB-187, SB-188) and ED (ED-122).
- Settings rail arrows and keybinding capture Esc — listed in SET-013 / SET-112.
- Dialog / dropdown Escape (dropdown.tsx:172, AppDialog) — MENU/DLG rows.
- use-command-shortcut.ts:2 — stub, returns undefined; no shortcut hints rendered from it.
- `tab-bar-item.tsx:176` shortcut hint "mod+w" — context-menu item label; see MENU.
- Mobile shell keyboard/native chrome — phone only.
