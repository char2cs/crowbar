# Shell parity matrix (GPUI shell vs Tauri+React shell)

Date: 2026-11-16. Branch `migration/rust-shell`. Read-only audit: no builds, no code edits. Evidence is read from Rust under `app/desktop/` (paths below are relative to it unless they start with `app/crates` or `docs`). Docs claims were not trusted; each row was checked by grep or reading the code. Screenshot comparison used the golden set in scratchpad `golden/reference/` (Tauri, light theme, 1200x800 client area) against the latest GPUI captures in scratchpad `e2e/` (`s0 s1 p3 p6 p7 p8`, `dnd/e-right-hover`, `ft/28-*`). The GPUI captures were taken in dark mode inside an X11 frame, the golden ones in light mode, so colour differences that follow from the theme are not listed as diffs.

Status vocabulary: done, partial, missing, hidden-by-decision (decision doc named in the evidence column). Items marked "pane-served" are handled by the React pane webviews, which still ship; the shell was only checked for forwarding them.

Sources of decisions: `docs/plans/2026-11-14-gpui-shell-migration-plan.md` ("Decisions on spec items flagged OWNER (lead's defaults)"), `docs/plans/2026-11-16-pane-tab-strips.md`, `docs/plans/2026-11-16-console-dock.md`, owner memory notes. The lead's defaults are not owner agreement; where an item was cut by them only, the row says so.

## 1. Commands, shortcuts, keymap

Web registry: `app/panes/src/features/keymaps/registry.ts` (24 commands plus hardcoded save/save-all). Rust: `crates/shell_keys/src/command_table.rs` (22 shell commands) and the Keybindings page catalog `crates/settings_ui/src/catalog/keybindings.rs` (24 rows).

| Item | Status | Evidence | Gap |
|---|---|---|---|
| ShellCommand enum: all 22 variants have a handler | done | `app/src/shell_view/shell_commands.rs:44-71` (exhaustive match) | none |
| tabs.new `mod+t` | done | `command_table.rs` NEW_TAB; `shell_commands.rs` NewTab | none |
| agent.newChat `mod+n` | partial | `shell_commands.rs:74` `new_chat` | needs an active project, otherwise only a log line (no toast). Spec 12.6: `mod+n` over an editor area opens a New Tab (intended, keep); not implemented |
| tabs.closeActive `mod+w` | partial | `shell_commands.rs` `close_active_tab` | closes the model tab, not the active strip tab; pane-tab-strips plan section 6 says `mod+w` closes the active strip tab. Pinned/dirty guard absent |
| tabs.reopenClosed `mod+shift+t` | done | `shell_commands.rs` `reopen_closed_tab`; `app/src/closed_panes.rs` | spec 12.3 intent (restore every tab kind) is met by pane history; not run |
| tabs.newTerminal `mod+j` | done | `shell_view/pane_commands.rs` `new_terminal` | none |
| tabs.newFile `mod+shift+n`, tabs.openFile `mod+o` | done | `pane_commands.rs`, `file_prompt.rs`, `file_reply.rs` | none seen |
| New browser, Review branch (no chord) | done | `pane_commands.rs` `new_browser`, `open_review:54` | none |
| navigation.toggleConsole `mod+\`` | partial | `pane_commands.rs:64` opens console as a stacked pane | no dock (see section 5) |
| panes.navigate left/right/up/down `mod+alt+arrow` | done | `shell_commands.rs` `focus_towards`; `app/src/focus_neighbour.rs` | none |
| navigation.sidebarChats/Files/Git `mod+1/2/3` | done | `shell_commands.rs` `set_sidebar_mode` | none |
| Toggle sidebar `mod+b` | done | `command_table.rs`; `ShellCommand::ToggleSidebar` | extra row, not on the Keybindings page (cannot be rebound in the UI) |
| Command palette `mod+shift+p` | partial | `crates/command_palette`, `shell_commands.rs` `show_palette` | web has no palette (inventory: "does not exist"). It is an addition, not parity; it is not on the Keybindings page. Needs an owner decision to keep |
| Open settings `mod+,` | done | `shell_commands.rs` OpenSettings | web has no such chord either (addition) |
| Open log, Export diagnostics | done | `shell_commands.rs` `open_log`, `export_diagnostics`; `app/src/diagnostics_export.rs` | additions, no menu entry except the palette |
| agent.newChatTerminal `mod+alt+n` | missing | only in catalog `keybindings.rs:31`; no `ShellCommand`, not in `PANE_SETTING_KEYS`, no consumer in `app/src` | row is editable but binds nothing in the shell; unverified in the React pane |
| agent.toggleViewMode `mod+/`, cycleProvider, zoomIn/Out/Reset, editor.save/saveAll, browser.togglePick | pane-served (unverified) | catalog rows only (`keybindings.rs:37-55`); `command_table.rs:214` lists them as not shell | the shell forwards only the 22 shell bindings (`Keymap::forwarded`). Whether the React panes read the user's overrides is not shown by any Rust code. `browser.togglePick` has no Rust handler and the browser bar has no pick button |
| Hardcoded project switch `mod+shift+<digit>` | missing | grep for digit/switch handlers in `crates/sidebar` and `app/src`: none | spec settings.md section 9 / sidebar-b keyboard summary |
| Keybinding preset (Default / Compact alternate) + "Active preset" row and IDE note | missing | no `keybindingPreset` anywhere in Rust; Keybindings page starts at "Navigation" (`keybindings.rs:60-72`) | golden `set-17-keybindings.png` shows the preset block. Not in any decision doc |
| Override conflicts, capture, reset-all | done | `settings_ui/src/chord_capture.rs`, `keymap.rs`, catalog Reset button | conflict icon/"preset"/"custom"/"display-only" chips not checked |
| Keymap overrides take effect live in the shell | done | `app/src/shell_view/keymap_settings.rs` | none |

## 2. Settings modal

Rust: `crates/settings_ui` (schema, views), `crates/shell_data/src/{settings_core,settings_adapter,setting_file}.rs` (store), `app/src/shell_view/{appearance_settings,pane_settings_watch,sidebar_position,focus_follow}.rs` (consumers). Pane-bound keys are pushed to webviews through `PANE_SETTING_KEYS` (45 keys, `app/crates/protocol/src/pane_setting_keys.rs:5`).

### Chrome and navigation

| Item | Status | Evidence | Gap |
|---|---|---|---|
| Modal frame, close X, title "Settings", search field, rail with 9 pages | done | `settings_ui/src/{frame,rail,header}.rs`; `e2e/p7.png` | backdrop: golden dims and blurs the app behind the modal; GPUI capture shows no dim/blur (`p7`, `p8`) |
| Rail pages Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active sessions, Keybindings, Hosts | done | `settings_ui/src/catalog.rs` + `catalog/*.rs` | Hosts shown on desktop only (matches `isTauri`) |
| Developer tab | hidden-by-decision | migration plan: "dropped only if nothing user-facing remains"; build readout row kept | none |
| Search, first-visible-tab, section hiding, Escape clears | done | `settings_ui/src/{visibility,navigation}.rs` | not driven live |
| Deep link to a tab (`Command::OpenHostSettings`) | missing | `app/src/sidebar_action.rs:25` maps `OpenHostSettings(_)` to plain `OpenSettings`; the host id is dropped, Hosts page is not selected | spec settings-schema section 3.9 |
| Opens at Appearance always | done | `data_events.rs:136` | none |
| Title dropdown below 720 px | missing | no narrow layout in `settings_ui` (grep) | spec settings.md section 3. Minor: default window is 1200 px |

### Settings keys (every catalog key, consumer checked)

| Key(s) | Status | Evidence (consumer) | Gap |
|---|---|---|---|
| theme, themeMode | done | `shell_view/appearance_settings.rs`, `app/src/pane_theme.rs` | live OS-follow not driven |
| iconTheme | done | `crates/sidebar/src/icon_theme.rs`, `shell_data/src/file_tree_reads.rs`; in pane keys | none |
| uiFontFamily, headingFontFamily, uiFontSize | done | `appearance_settings.rs`, `settings_ui/src/fields.rs` | none |
| sidebarPosition | done | `shell_view/sidebar_position.rs`, `pane_settings_watch.rs` | none |
| tabListScope | done | `shell_view/tab_strip.rs:21` | none |
| sidebarFollowsFocus | done | `shell_view/focus_follow.rs` | none |
| buildReadout | done | `shell_view/readout.rs:17` | row kept per lead default; inventory CUT list overridden. Owner has not signed off |
| fontFamily, fontSize, editorLineHeight, tabSize, wordWrap, lineNumbers, renderWhitespace, renderIndentGuides, semanticHighlighting, highlightOccurrences, showMinimap, autoSave, formatOnSave, inlineBlame, autoCompletion, parameterHints, markdownFontSize | done (pane-served) | all 17 are in `PANE_SETTING_KEYS`; pushed by `app/src/pane_settings.rs` | no Rust consumer by design: the editor stays a React pane. If the React editor ignores a key the row is dead; not verifiable from Rust |
| maxOpenTabs | missing | in `PANE_SETTING_KEYS` but no shell code enforces a tab cap or eviction | spec tabs-pane-strip 12.5 (cap, LRU eviction with a toast). Shell owns the strip now, so the React cap no longer applies |
| fileTreeIndentSize, fileTreeDensity, compactFoldersInFileTree, showHiddenFilesInFileTree, showGitignoredFilesInFileTree (inverted), hiddenFilePatterns, hiddenDirectoryPatterns, compactGitStatusBadges, showGitStatusInFileTree | done | `shell_data/src/file_tree_reads.rs:9-121`, `sidebar/src/file_tree_flag.rs`, `app/src/shell_view/file_tree_flag.rs` | none |
| terminalFontFamily/Size/LineHeight/LetterSpacing/Scrollback/CursorStyle/CursorBlink/CursorWidth | done | `app/src/terminal_preferences.rs` | none |
| browserHomepage, browserSearchEngine, browserCustomSearchUrl | done | `app/src/{browser_homepage,search_engine_setting}.rs` | none |
| browserZoom | partial | `app/src/pane_settings.rs` (pushed) | no native consumer found for the browser webview zoom |
| browserClearData button ("Clear data") | missing | `catalog/browser.rs:16,72`; `SettingsCore::invoke` (`shell_data/src/settings_core.rs:320-333`) serves only `agents.upload` and `hosts.add`, everything else returns `Error::Unavailable`; no `capability("browser.clearData")` is ever granted (grep `capabilit` in `app/src`, `shell_data`: none) | the row is permanently disabled with the "Not available on this system" note. Flag: UI wired to an action that is unavailable. No owner decision recorded |
| chatIsDefaultPresentation | done | `shell_data/src/landing.rs:9` | none |
| defaultPermissionLevel | done | `shell_data/src/settings_core.rs` (daemon, optimistic) | none |
| Agents list: reorder, Installed, Tools, Enabled | partial | `catalog/agents.rs`, `shell_data/src/agent_records.rs`; `e2e/p8.png` | up/down arrows instead of drag handles; Installed drawn as a switch (golden: a small status dot); no provider icons; "Upload agent descriptor" button sits in the row beside the permission select (golden: button below it with the explanation under it) |
| Descriptor checks (shipped / override badges) | done | `catalog/agents.rs` DESCRIPTORS, `descriptor_records.rs` | not driven with a live daemon |
| Upload agent descriptor | done | `settings_core.rs:324` -> `Outcome::PickDescriptor` -> `shell_view/data_events.rs:67` | none |
| Active sessions list + Kill | done | `settings_core.rs:361,429` | none |
| Hosts list, Add host, revoke, remove anyway | done | `settings_core.rs:362-369`, `crates/pairing_view` | revoke confirm branches not checked |
| Keybindings page | partial | section 1 | preset block missing |
| keybindingPreset | missing | see section 1 | |
| windowTransparency, sidebarWidth, coreFeatures, diffViewMode | done (no row) | `app/src/window_vibrancy.rs`, `sidebar` preferences, `diffViewMode` in pane keys | none |
| syncSystemTheme / autoThemeLight / autoThemeDark migration | missing | grep: no migration of the deprecated keys | spec says migrate then drop; only matters for old installs |
| Normalisation (uiFontSize snap, clamps, legacy font map) | done | `settings_ui/src/number_range.rs`, `validator.rs`, `shell_data/src/setting_file.rs` | clamp list not diffed key by key |
| 300 ms debounced write; only stored keys rewritten | not verified | `setting_file.rs` | read-only audit |
| Web importing of the old localStorage settings | missing | no importer for `crowbar:settings:*` values from the webview store | users upgrading lose their settings; `docs/plans/2026-11-08-migration-evidence.md` should say whether this is intended |

## 3. Sidebar

Crate `crates/sidebar` (~230 files), data in `crates/shell_data`. Every `sidebar::Command` variant has a handler: planner (`shell_data/src/planner.rs`), file commands (`file_commands.rs`) or shell actions (`app/src/sidebar_action.rs`, `shell_view/file_actions.rs`). Exceptions below.

| Item | Status | Evidence | Gap |
|---|---|---|---|
| Frame: default width 294, min 250, collapse below 125, resize handle, peek on hover, toggle button, dock left/right | done | `sidebar/src/{metrics,resize_ops,peek,peek_exit,dock_side}.rs` | peek/collapse animation frames not compared |
| Header: gear, toggle, host picker (multi-host), project switcher, `+` create panel, icon editor popover | done | `host_picker.rs`, `switcher.rs`, `create_space_panel.rs`, `space_icon_editor.rs` | golden header shows library glyphs next to the project name; GPUI shows a tinted letter tile (`s0.png` vs `sb-01`). Check the glyph fallback |
| Mode bar Chats / Files / Changes + carousel | done | `mode_bar.rs`, `panel.rs` | `p3.png`: Changes panel mid-slide is horizontally clipped and misaligned (carousel not aligned to the sidebar width at capture time) |
| Mode keyboard (`use-sidebar-tab-keyboard`), space switcher keyboard (`mod+shift+digit`) | missing | grep: no key handling beyond the three mode chords | spec sidebar-b section 6 |
| Tree rows: glyphs, working spinner, rename inline, branch-out input, hover verbs, trailing cluster, unread notice | done | `tree_row.rs`, `row_glyph.rs`, `inline_edit.rs`, `row_cluster.rs`, `notice_row.rs` | none seen |
| Row context menu (Rename, Import branches, New folder, New thread in Terminal, Delete) | done | `row_menu.rs`, `menu_*.rs` | "Reveal in Finder" wording: check that Linux says "Show in Files" |
| Tree foot menu (Import a repo, Create a new folder, Delete space) | done | `foot_menu.rs`, `tree_foot.rs` | none |
| Add repository dialog (typed path), Import branches dialog, remote folder picker, pairing dialog | done | `app/src/shell_view/remote_folder.rs`, `crates/{branch_import_view,remote_folder_picker,pairing_view}` | none |
| Removal hold / tray / undo, detach holder modal, discard question | done | `removals.rs`, `removal_tray.rs`, `question_view.rs`, `confirm_view.rs` | none |
| Drag and drop of rows (reorder, nest, open in pane), edge scroll, drop line, ghost | done | `drag*.rs`, `drop_*.rs`, `edge_scroll.rs`, `row_ghost.rs`; `e2e/dnd/*` | `dnd/e-right-hover.png` shows the ghost and a sidebar drop line but no right-half preview over the pane; golden `pan-22-drop-right.png` shows a tinted half. Possibly hidden under the webview hole |
| Files panel: tree, search, filter menu, density, git status, context menu | done | `files_panel.rs`, `file_menu.rs`, `filter_menu.rs`, `e2e/ft/*` | |
| File menu: Generate Image, Remove Folder from Workspace | partial | `shell_data/src/files_reads.rs:37` `shown()` hides both; no handler for either (`file_actions.rs` falls through to `warn_unserved`) | spec: "Generate Image (if handler)", so by design hidden; "Add Folder to Workspace" is served |
| File menu: Upload File, Add Folder, Open in Terminal, Open all files, env template | done | `shell_view/file_actions.rs:28-36` | |
| OS file drag-drop into panes / file tree | missing | grep `ExternalPaths` / file drop: none | spec 13.1 `use-pane-drop-handlers` file-drop part |
| Changes panel: branch card, merge box, commit form, changed files | partial | `changes_panel.rs`, `merge_box.rs` | `ft/28-changes-badges-comfortable.png`: the white merge card holds dark-styled select, textarea and checkboxes; the "Delete this branch and its chats after merging" label is clipped at the right edge; golden has light controls and a wrapped label |
| Foot / build readout | partial | `shell_view/readout.rs`, `readout_view.rs` | GPUI centres the three text lines; golden left-aligns them at x=12 |
| Toasts in the sidebar (toast layer) | done | `sidebar/src/toast_layer.rs`, `ui/src/toast_*.rs`, `app/src/notices.rs` | swipe-to-dismiss ported; placement not diffed |
| Connection indicator, operations watcher, placeholder toast | done | `app/src/{daemon_notice,link_state,notices}.rs`, `sidebar/src/operation_row.rs` | |
| Drag handle `sidebarWidth` clamp on load | done | `sidebar/src/preferences.rs` | |

## 4. Tabs top bar (view tabs) and window chrome

| Item | Status | Evidence | Gap |
|---|---|---|---|
| View-tab strip: pill, close X, working spinner, set members, reopen toggle | done | `crates/tab_bar/src/{tab_shell,member_view,tab_view,sidebar_glyph}.rs` | `e2e/s1.png`: three members merged into one pill and each label is cut to 5 letters ("New c", "Termir"); golden elides with an ellipsis at a wider limit |
| Reorder, merge into split, drop from sidebar, ghost, thresholds | done | `strip_drag.rs`, `tab_drop.rs`, `incoming_drop.rs`, `drag_session.rs` | |
| Working-chat refusal toast | missing | `member_view.rs` has the `working` flag; no refusal message grep'd | spec 9.6 asks owner for the intent; ambiguous |
| Pin / unpin views | missing | grep `pin` in `model`, `tab_bar`, `layout_view`: only a comment | spec tabs-topbar 9.5 and `pane` pinned-first rule; no decision doc cuts it |
| View-tab context menu | partial | `tab_bar/src/tab_bar_menu.rs` has Close and Close Others only | golden: the menu was a leaking native WebKit one (bug); intended menu in spec 9.8 is richer (Pin etc.) |
| Strip scrolling and scroll-into-view of the active tab | missing | no scroll handling in `tab_bar_*.rs` | spec 9.7 (bug fix required) |
| Tab list scope (project vs all) | done | `app/src/tab_scope.rs`, `shell_view/tab_strip.rs` | |
| Window chrome: native frame on Linux, default 1200x800, size memory | done | `app/src/window_memory.rs`, `x11_session.rs` | GPUI captures show the WM frame like Tauri on Linux |
| Window title `"<chat title> - Crowbar"` | missing | `shell_view.rs:395` sets the constant "Crowbar" once; `titles.rs` holds pane titles but nothing writes the window title | spec tabs-topbar section 1; web `use-document-title.ts` |
| Drag region (move window from empty strip) | not verified | no `window_control`/`start_window_move` in `tab_bar` | native frame covers it on Linux; macOS overlay needs a check |
| macOS traffic-light inset (88 px) and follow | not verified | spec says verified by CI only | |
| Tab animations (hover, select pill, close) | not verified | frames not compared | |

## 5. Layout, panes, pane strip, New Tab page

| Item | Status | Evidence | Gap |
|---|---|---|---|
| Split tree, sash (6 px, min 160, double click reset), accent ring, active-pane focus | done | `crates/layout_view`, `app/crates/model` (Group, `ResizeSplit`, `ResetSplit`) | |
| Pane handle (grip, detach, close), drop overlay, five drop zones | done | `layout_view/src/{pane_handle,drop_target,drop_hover,pane_zone}.rs` | preview not visible in `dnd/e-right-hover.png` |
| Chat is split-only, others stack in groups | done | `app/src/stack_policy.rs`, model groups (plan 2026-11-16-pane-tab-strips) | |
| Chat/editor halves with `editorOpen`, `chatSelected`, leading split toggle, "open editor" button, review shortcut button | hidden-by-decision | `docs/plans/2026-11-16-pane-tab-strips.md` sections 1-3 (lead-level "DECIDED", not an explicit owner answer) | visual change: golden `pan-01..04` show one pane with chat + editor strip and the toggle/review buttons; the owner gate says 1:1 visuals, so this needs an explicit sign-off |
| Pane strip (42 px row, tabs, active highlight, close X, drop to reorder, "+", wheel scroll and scroll-into-view, "More tabs" menu, pin, context menu, middle-click close, group close) | partial | `layout_view/src/{pane_strip,strip_tab,strip_actions,strip_menu,strip_key,strip_state,close_button,tab_end}.rs`; model `Group` pins, `ClosePanes`, `PinPane`, `UnpinPane` | done and checked live on Xvfb: "+" opens a New Tab page in the group, overflow caret and menu appear when tabs outgrow the row, context menu (Pin/Unpin, Move to Group, Close, Close Others, Close to the Right, Close All; bulk closes skip pinned tabs), pinned tabs sit in front and show a solid pin that unpins, the handle close closes the whole group in one undo step. Still missing: arrow/Home/End/Delete work only while the strip holds keyboard focus, and a click hands focus to the pane content (`PaneHost` focuses the activated pane), so the keyboard cannot reach the strip yet; Rename, Copy Path, Reveal, Open in Terminal, Reload menu rows; menu opens at the pointer, not 4 px under the tab centre; dirty dot, italic preview tabs, tab tooltips, "Cannot close pinned tab" announcement; tab cap |
| Unsaved changes dialog on close (tab, group, bulk) | missing | grep "unsaved": no dialog in Rust; plan says "the app asks first", not implemented | data-loss risk once the editor pane has dirty buffers |
| Tab cap and LRU eviction with toast | missing | see `maxOpenTabs` | |
| New Tab page | partial | `app/src/{new_tab_action,new_tab_view}.rs` | "Review Branch" is always listed; spec: only for a branch workspace (`canReview`). Opening in the New Tab page's own slot: done by plan |
| Closed-tab history / reopen | done | `app/src/closed_panes.rs` | |
| Layout persistence (versioned, atomic) and load repair | done | `app/src/{layout_store,layout_writer,loaded_layout}.rs`, model integrity | one-time import of the old IndexedDB layout is not present (same as settings import) |
| Console dock (overlay/push, 4 sides, resize, Escape, 200 ms animation) | missing | `pane_commands.rs:64` toggles a stacked console pane; no dock state in `window_memory.rs`; `docs/plans/2026-11-16-console-dock.md` is "tracked follow-up, gate item" | recorded deviation, still open |
| Chat pane header row (title, branch, chat/terminal switch, provider picker) | partial | lives in the React chat pane, not Rust | `e2e/p6.png`/`s1.png` show the chat pane without any header or view switch (no agent installed in that run); unverified |
| Pane fullscreen, bottom pane, split chat presentation, editor-group commands | hidden-by-decision | inventory CUT list; spec 13.2 | |
| Edge cases F1-F9 of `pane-layout-intent.md` | not verified | model has exhaustive tests per plan; not re-run | |

## 6. Dialogs, menus, toasts, startup

| Item | Status | Evidence | Gap |
|---|---|---|---|
| Daemon halt / crash / outage view, Quit | done | `crates/startup_view/src/{halt_panel,outage_panel}.rs`, `app/src/{daemon_notice,phase_input}.rs` | web gate text not diffed |
| Startup / migration full-screen view, determinate `i / n`, step line, hand-over | done | `startup_view/src/{waiting_panel,migration_step,handover,startup_machine}.rs`; `startup_input.rs` | polling is explicit; no screenshot of it in the scratchpad set (`e2e` has none) |
| OOBE (`#/oobe`): tagline, System check (git ok, gh/glab optional), Add first space | missing | grep oobe / system check / glab in `app` and `crates`: none. First launch with no project shows the sidebar create-space panel only | inventory KEEP list (route wired and LIVE). Not cut by any decision doc |
| `/add-host` route (brand frame) | partial | host adding exists in Settings > Hosts and create-space picker (`pairing_view`) | the standalone route is not ported |
| Native app menu | partial | `app/src/app_menu.rs`: Crowbar (Services, Quit) and Edit (Cut/Copy/Paste) only | no File/View/Window items; on macOS there are no Close Window, Zoom, Minimise entries. The web app had Tauri default menus; not diffed |
| Context menus (files, rows, tabs, space) | done | `crates/ui/src/{menu,context_menu}.rs`, sidebar menu files | |
| Confirm dialogs (revoke, clear data, kill, delete space, replace file) | done | `settings_ui/src/confirm.rs`, `sidebar/src/confirm_view.rs`, `shell_view/file_actions.rs` (Overwrite::Ask) | |
| Toast queue (kinds, lifetime, swipe, selection) | done | `crates/ui/src/toast_*.rs`, `app/src/notices.rs` | |
| Notices: backend unavailable (500 ms debounce), Reconnected | done | `app/src/{daemon_notice,link_state,reconnect_count}.rs` | |

## 7. Command palette, browser chrome, terminal

| Item | Status | Evidence | Gap |
|---|---|---|---|
| Command palette | partial | `crates/command_palette`; entries from `palette_entries` | addition to the web (no palette existed). Lists shell commands only |
| Browser bar: address, back, forward, reload, search-engine routing | done | `crates/browser_chrome/src/browser_bar*.rs`; `app/src/{browser_bars,browser_homepage}.rs` | |
| Browser: pick element (`mod+shift+e`), stop/loading state | missing | `BrowserBarEvent` has Go, Back, Forward, Reload only (`browser_bar_event.rs:4`); no pick handler in Rust | inventory: LIVE chrome incl. pick element |
| Browser clear data | missing | see section 2 | |
| Terminal pane: PTY, selection, scrollback, cursor styles/blink, IME preedit, zoom | done | `crates/{terminal,terminal_session,terminal_view}`; `terminal_preferences.rs` | `e2e/s1.png`: the terminal pane renders a white background in a dark shell (golden terminal is a light surface in light mode). Theme token for the terminal surface does not follow dark mode |
| Terminal ANSI palette per theme | not verified | `ui/src/palette/` | |
| Chat terminal view (agent terminal) | pane-served | React | |

## 8. Hygiene findings (CLAUDE.md "no stubs that ship")

| Finding | Evidence |
|---|---|
| UI wired to an action that always fails: Clear Browsing Data | `settings_core.rs:332` returns `Error::Unavailable`; capability never granted |
| `OpenHostSettings` drops its argument | `app/src/sidebar_action.rs:25` |
| Keybinding rows with no consumer in the shell: `agent.newChatTerminal`, `browser.togglePick` | `catalog/keybindings.rs:31,37` |
| `warn_unserved` fallthrough for commands the file menu hides | `shell_view/file_actions.rs:36`, `files_reads.rs:37` (acceptable: hidden, not shown) |
| No method returning `[]`/`false` as a stub was found in the shell crates; grep for `todo!`, `unimplemented!`, `stub`, `placeholder` returned only text-field placeholders | grep over `app/src`, `crates` |

## 9. Golden vs GPUI screenshot diffs per surface

- **Sidebar chats (`sb-01` vs `s0`, `s1`)**: GPUI matches the row/icon/typography closely. Diffs: project header icon (glyph vs tinted tile), foot readout alignment (centred vs left), GPUI shows only two mode tabs when no repo workspace is focused (spec allows this), "Untitled chat" label not italic in GPUI (golden italic for unnamed chats), working-chat children layout.
- **Changes mode (`sb-22` vs `p3`, `ft/28`)**: merge card controls styled for the dark surface inside a light card; long label clipped; carousel misaligned during the slide.
- **Settings (`set-01/15/17` vs `p7`, `p8`)**: layout, rail, typography and row anatomy match well. Diffs: no backdrop dim/blur; Agents page uses arrows, a switch for Installed and no icons or drag grips; button placement of "Upload agent descriptor"; the "Descriptor checks" block is an error line only (daemon unreachable in the run, so unverified); Keybindings lacks the preset block.
- **Top bar and pane strip (`tab-01` vs `s1`)**: view pill labels truncated; pane strip lacks "+" and overflow chevron and the leading split toggle; strip tabs lack the close X on non-hovered tabs (matches golden) but also lack the italic preview style.
- **Panes (`pan-01`, `pan-22` vs `s1`, `p6`, `dnd/e-right-hover`)**: chat pane shows dark-on-dark placeholder text and a light composer (the pane's theme/surface mismatch in dark mode); terminal pane is white; no chat header row or chat/terminal switch visible; drop preview over the pane not visible; console dock absent.
- **Startup / New Tab**: New Tab page wordmark matches the spec; the startup view has no capture in the scratchpad.

## 10. Prioritised gap list (top 25)

1. Unsaved-changes dialog and dirty guard on close (tab, group, bulk) are missing; data loss once the editor pane has dirty buffers.
2. Console dock not built (toggle opens a stacked pane); gate item with a plan but no code.
3. Pane strip lacks "+" menu, overflow menu, tab context menu, pin, scroll and keyboard nav (`layout_view` `TabActions` has select/close/drop only).
4. OOBE (tagline, system check, add first space) and `/add-host` route missing; first launch has no guided path. Inventory KEEP item.
5. Chat/editor-half replacement and the removed split toggle / review button change the visuals; recorded only as a lead decision, needs the owner's explicit yes.
6. Pane theming in dark mode: terminal renders white, chat pane text is dark on dark; pushed theme tokens do not cover these surfaces.
7. Clear Browsing Data is permanently disabled (`invoke` returns Unavailable, capability never granted): ship it or remove the row with an owner decision.
8. Window title is the constant "Crowbar", not "<chat title> - Crowbar".
9. View pin/unpin missing, and the pinned-first/protected-close rules with it.
10. Tab cap (`maxOpenTabs`) and LRU eviction with a toast are not enforced by the shell.
11. Keybindings page lacks the preset block; `keybindingPreset` unsupported.
12. `agent.newChatTerminal` has a rebindable row but no shell handler; same risk for pane-served commands (toggle view, zoom, save, pick, cycle provider) whose forwarding is unverified.
13. Browser "pick element" (button and `mod+shift+e`) missing from the browser bar.
14. `OpenHostSettings` ignores its host and does not open the Hosts page.
15. Project switch `mod+shift+<digit>` and sidebar mode keyboard handling missing.
16. OS file drag-drop onto panes and file tree missing.
17. Settings import from the old webview store (`crowbar:settings:*`, layout IndexedDB) not present; upgrading users lose settings and layout unless the migration plan says otherwise.
18. New Tab page always lists Review Branch; spec limits it to branch workspaces.
19. View strip scroll-into-view and overflow handling missing (spec bug fix 9.7); labels truncate to a few characters.
20. Window title, drag-region and macOS traffic-light follow are unverified (CI-only); the macOS app menu has only Crowbar and Edit menus.
21. Agents page differs from the golden (no drag handles, switch for Installed, no provider icons, button placement).
22. Settings modal has no backdrop dim/blur and no sub-720 px title dropdown.
23. Merge box styling and clipped label in the Changes panel; carousel misalignment during mode slide.
24. Command palette and `mod+b`/`mod+,` chords are additions without owner agreement and are not on the Keybindings page.
25. `browserZoom` has no native consumer for the browser pane; working-chat refusal toast on strip merge/reorder absent.

Not covered by this audit (needs a live run): animation timing frame by frame, macOS-only behaviour, behaviour of the React panes under the pushed settings, and the effective result of `mod+n` over an editor area.

## Update 2026-11-17 (rows re-checked in code and live on Xvfb)

Live run: debug build in the shared builder, isolated `CROWBAR_HOME`, built pane bundle, seeded daemon. The rows below supersede the earlier ones with the same subject.

| Item | Status now | Evidence |
|---|---|---|
| Keybinding preset block (Active preset select, Default and Compact, IDE note) | done | `settings_ui/src/catalog/keybindings.rs`, `app/src/shell_view/keymap_settings.rs`; seen on the Keybindings page |
| `agent.newChatTerminal` | done | `ShellCommand::NewChatTerminal` handled in `shell_commands.rs` |
| Project switch `mod+shift+<digit>` | done | `shell_keys::slot`, `shell_commands.rs` `switch_project_on` |
| `OpenHostSettings` | done for the page, not the host | maps to `SidebarAction::OpenHosts` and opens the Hosts page; the host id is still dropped, so no host is pre-selected |
| Clear Browsing Data (Linux) | done | button enabled, confirm dialog, `PaneHost::clear_browsing_data` to the browser store, toast "Browsing data cleared"; seen live |
| Clear Browsing Data (macOS) | implemented, unverified | `webview/src/macos_browsing_data.rs` clears the shared browser data store through any open browser view, else through a throwaway view in the last window; `ViewCapture` was also missing for `MacosPlatform`, so the macOS build did not satisfy `Platform`. Unverified until the macOS CI runner |
| Settings backdrop | dim done, blur not possible | scrim at 32% black on every `Dialog`; GPUI has no backdrop blur primitive, so the 8 px blur of the web dialog is a recorded deviation |
| Agents list | done | grips, provider icons, Installed as a status dot, upload button below the permission row; seen live |
| View-tab label truncation | fixed | cause: members of a set kept the resting glyph, which the spec drops for set members (spinner stays while working); each label lost 22 px. Test `tab_shell::tests::the_members_of_a_set_leave_their_labels_room_instead_of_a_resting_glyph`. Not reproduced live: merging two tabs by drag is not reachable with scripted pointer events |
| Chat header and chat/terminal switch | fixed | the shell chat pane mounted only the chat surface; it now mounts the identity row (title, branch) and the header slot the surface draws its switch into (`app/panes/src/entries/chat-pane-header.tsx`). Seen live: title, branch path and the chat/terminal switch |
| Changes panel mid-slide | not reproducible | the mode pill glides while the panel swaps; the panel is aligned at every captured frame (`d0`, `d1`); the space carousel only runs in Chats mode |
| Pin view tabs | open, blocked | the model has `PinPane` (pane strips, in progress by the strips owner) but no tab-level pin; adding one touches `model/command.rs` and `layout_execute.rs` that are being edited. The web had no reachable pin UI (spec tabs-topbar 9.5), so this needs an owner decision on scope |
| OOBE (tagline, System check, first space) | done | seen live on first launch |
