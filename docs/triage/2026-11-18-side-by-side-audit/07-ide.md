# 7. IDE shell inside a pane (IDE)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| IDE-001 | Chat-only pane header (chat fills pane, editor closed, no tabs): ChatBranchHeader + "Open editor" button + chat header slot, 42px | web/src/features/tabs/components/chat-only-pane-header.tsx:18 | rig reset | — | | | | | | | |
| IDE-002 | Branch header lead: repo/project icon, chat title, route line `repo / parent / branch` with `+added -deleted` | web/src/features/tabs/components/chat-branch-header-view.tsx:27 | rig reset; show feature-workspace chat | — | | | | | | | |
| IDE-003 | Branch header kind glyph: spinner while working, ChatsCircle for thread, GitBranch for workspace | web/src/features/tabs/components/chat-branch-header-view.tsx:100 | rig reset; compare the two chats. Seed: running agent for spinner | flicker-strip steps(N) infinite | | | | | | | |
| IDE-004 | Double-click header title: inline rename of the chat title; Enter commits, Escape cancels | web/src/features/tabs/components/chat-branch-header.tsx:21 | double-click title in pane header | — | | | | | | | |
| IDE-005 | Header blur variant over chat: EdgeDissolve backdrop extends 56px below row, transcript scrolls under it | web/src/features/tabs/components/pane-top-row.tsx:35 | rig reset; scroll Perf fixture transcript | — | | | | | | | |
| IDE-006 | "Open editor" (Code icon) click: editor opens beside chat, New Tab page fills editor side | web/src/features/tabs/components/open-editor-button.tsx:7 | click Open editor | — | | | | | | | |
| IDE-007 | Tooltip on Open editor | web/src/features/tabs/components/open-editor-button.tsx:15 | hover Open editor | fade/zoom-in-95 150ms after 150ms delay | | | | | | | |
| IDE-008 | Side-by-side presentation (editor open, views box w≥h, max side ≥780): chat column left with its own header, editor right with tab strip | web/src/features/agent/hooks/use-chat-presentation.ts:181 | rig size standard; Open editor | — | | | | | | | |
| IDE-009 | Stacked presentation (h>w, max side ≥780): chat top, editor below, horizontal sash | web/src/features/agent/hooks/use-chat-presentation.ts:195 | rig size 800x1000; Open editor | — | | | | | | | |
| IDE-010 | Tabs presentation (max side <780): chat is a tab in the strip, one surface at a time | web/src/features/agent/hooks/use-chat-presentation.ts:190 | rig size 640x480; Open editor | — | | | | | | | |
| IDE-011 | Live presentation switch on resize between side-by-side / stacked / tabs | web/src/features/panes/hooks/use-pane-presentation.ts:8 | Open editor; rig size standard → 800x1000 → 640x480 | — (instant) | | | | | | | |
| IDE-012 | Chat/editor sash (side-by-side): 1.5 wide, col-resize, hover bg-border/60; default 45/55 | web/src/features/panes/components/pane-container.tsx:113 | Open editor; hover the seam | transition-colors 150ms | | | | | | | |
| IDE-013 | Drag chat/editor sash: resizes; each side clamps at 340px | web/src/features/panes/lib/split-sizing.ts:6 | Open editor; drag seam far left / far right | — | | | | | | | |
| IDE-014 | Stacked sash: row-resize, each side clamps at 160px | web/src/features/panes/lib/split-sizing.ts:6 | rig size 800x1000; Open editor; drag seam | — | | | | | | | |
| IDE-015 | Chat/editor ratio is not persisted: restart returns to 45/55 | web/src/features/panes/components/pane-container.tsx:79 | drag seam; rig restart tauri | — | | | | | | | |
| IDE-016 | Split toggle button (aria-pressed) in editor strip: "Show chat only" when alongside | web/src/features/tabs/components/split-toggle-button.tsx:13 | Open editor; hover toggle | tooltip 150ms | | | | | | | |
| IDE-017 | Split toggle click: editor closes, chat fills pane, header back to IDE-001; tabs kept | web/src/features/panes/utils/pane-command-actions.ts:237 | Open editor; click split toggle | — | | | | | | | |
| IDE-018 | Split toggle "Show editor alongside chat" re-opens editor with previous active tab | web/src/features/tabs/components/split-toggle-button.tsx:21 | open a file, close editor via toggle, toggle again | — | | | | | | | |
| IDE-019 | Editor tab strip: role=tablist "Open files", horizontal scroller, tabs h-full | web/src/features/tabs/components/tab-bar.tsx:379 | open README.md from Files sidebar | — | | | | | | | |
| IDE-020 | Chat tab in strip (tabs presentation only): AgentChatGlyph + title max-w-160, not closable | web/src/features/tabs/components/chat-tab-item.tsx:15 | rig size 640x480; open a file | — | | | | | | | |
| IDE-021 | Chat tab working spinner | web/src/features/tabs/components/chat-tab-item.tsx:37 | rig size 640x480. Seed: running agent | flicker-strip steps(N) infinite | | | | | | | |
| IDE-022 | Click chat tab: chat shown, editor hidden (chatSelected) | web/src/features/tabs/components/tab-bar.tsx:392 | rig size 640x480; open file; click chat tab | — | | | | | | | |
| IDE-023 | Active editor tab: foreground label, active surface | web/src/features/tabs/components/tab-bar-item.tsx:134 | open README.md | — | | | | | | | |
| IDE-024 | Inactive editor tab: muted label; hover shows close × at 60% | web/src/features/tabs/components/tab-bar-item.tsx:172 | open two files; hover inactive tab | opacity 150ms ease | | | | | | | |
| IDE-025 | Tab icon per kind: file icon by name, terminal TerminalWindow, browser Globe, branch review GitPullRequest, commit diff GitBranch, New Tab Plus, marketplace Package | web/src/features/tabs/components/tab-buffer-icon.tsx:13 | open file, terminal, browser, review, New Tab | — | | | | | | | |
| IDE-026 | Tab tooltip: name, file path (editor), "Read-only: this branch is locked" when locked | web/src/features/tabs/components/tab-bar-item.tsx:93 | hover README.md tab. Seed: locked workspace for read-only line | tooltip 150ms | | | | | | | |
| IDE-027 | Preview tab: italic label; opened by single-click in file tree; next preview replaces it | web/src/features/tabs/components/tab-bar-item.tsx:140 | single-click README.md then src/pricing.ts in Files | — | | | | | | | |
| IDE-028 | Double-click preview tab: promoted to permanent (label upright) | web/src/features/tabs/components/tab-bar.tsx:258 | single-click a file, double-click its tab | — | | | | | | | |
| IDE-029 | Dirty inactive tab: size-2 primary dot "Unsaved changes" replaces × | web/src/features/tabs/components/tab-bar-item.tsx:144 | type in README.md, switch to another tab | — | | | | | | | |
| IDE-030 | Dirty active tab: primary border, rounded-sm, shadow | web/src/features/tabs/components/tab-bar-item.tsx:113 | type in README.md | — | | | | | | | |
| IDE-031 | Read-only tab: Lock icon after label, aria-label suffix | web/src/features/tabs/components/tab-bar-item.tsx:143 | Seed: locked branch with file open | — | | | | | | | |
| IDE-032 | Pinned tab: Pin icon at 60% in × slot, pinned tabs sorted first; click pin unpins ("Unpin tab") | web/src/features/tabs/components/tab-bar-item.tsx:158 | right-click tab > Pin (see MENU); click pin | — | | | | | | | |
| IDE-033 | Click tab ×: closes tab; neighbour becomes active; last tab closes editor (chat fills pane) | web/src/features/panes/stores/slices/pane-actions/editor-tab-actions.ts:110 | open file; click × | — | | | | | | | |
| IDE-034 | Close × tooltip "Close" with shortcut ctrl+w | web/src/features/tabs/components/tab-bar-item.tsx:174 | hover × | tooltip 150ms | | | | | | | |
| IDE-035 | Middle-click tab closes it | web/src/features/tabs/components/tab-bar-item.tsx:66 | open file; middle-click its tab | — | | | | | | | |
| IDE-036 | ctrl+w closes active editor tab | web/src/features/panes/hooks/use-pane-keyboard.ts:146 | open file; ctrl+w | — | | | | | | | |
| IDE-037 | Close dirty tab (× or ctrl+w): unsaved-changes dialog (see DLG) | web/src/features/tabs/components/tab-bar.tsx:117 | type in README.md; ctrl+w | — | | | | | | | |
| IDE-038 | ctrl+shift+t reopens the last closed file tab | web/src/features/panes/stores/slices/buffer-slice.ts:266 | open README.md, close it, ctrl+shift+t | — | | | | | | | |
| IDE-039 | "+" New tab button after tabs: opens/focuses the pane's New Tab page (one per pane) | web/src/features/tabs/components/tab-add-button.tsx:12 | open file; click + | — | | | | | | | |
| IDE-040 | ctrl+t opens New Tab page in focused pane | web/src/features/panes/hooks/use-pane-keyboard.ts:56 | ctrl+t | — | | | | | | | |
| IDE-041 | ctrl+n with focus in editor area opens New Tab page (not a chat) | web/src/features/panes/hooks/use-pane-keyboard.ts:60 | focus editor; ctrl+n | — | | | | | | | |
| IDE-042 | ctrl+j opens a new terminal tab ("Terminal N") | web/src/features/panes/hooks/use-pane-keyboard.ts:67 | ctrl+j | — | | | | | | | |
| IDE-043 | ctrl+shift+n opens a new untitled file tab | web/src/features/panes/hooks/use-pane-keyboard.ts:84 | ctrl+shift+n | — | | | | | | | |
| IDE-044 | ctrl+o opens native file dialog; picked file opens as tab | web/src/features/panes/hooks/use-pane-keyboard.ts:97 | ctrl+o | — | | | | | | | |
| IDE-045 | Tab overflow: "More tabs" caret appears when strip overflows; dropdown lists tabs, active one semibold | web/src/features/tabs/components/tab-overflow-menu.tsx:19 | rig size 640x480; open ~8 files/terminals | dropdown fade/zoom-in-95 + slide-in 2, 100ms | | | | | | | |
| IDE-046 | Pick tab from More tabs menu: activates and scrolls it into view | web/src/features/tabs/components/tab-overflow-menu.tsx:56 | open overflow menu; pick a hidden tab | smooth scroll | | | | | | | |
| IDE-047 | Active tab auto-scrolls into view, centred | web/src/features/tabs/components/tab-bar.tsx:237 | overflow strip; activate an off-screen tab | smooth scroll | | | | | | | |
| IDE-048 | Wheel over editor strip scrolls horizontally (vertical wheel mapped to horizontal) | web/src/features/tabs/hooks/use-tab-bar-scroll.ts:40 | overflow strip; rig scroll down on strip | — | | | | | | | |
| IDE-049 | Tab cap: 10 open tabs per workspace, opening an 11th evicts the oldest clean unpinned tab | web/src/features/panes/stores/slices/buffer-slice.ts:99 | open 11 files/terminals in one workspace | — | | | | | | | |
| IDE-050 | Keyboard on focused tab: ArrowLeft/Right move, Home/End jump, Enter/Space activate | web/src/features/tabs/hooks/use-tab-keyboard-nav.ts:31 | focus a tab; arrows | — | | | | | | | |
| IDE-051 | Delete/Backspace on focused tab closes it; pinned refused | web/src/features/tabs/hooks/use-tab-keyboard-nav.ts:77 | focus tab; Delete | — | | | | | | | |
| IDE-052 | Tab focus ring | web/src/features/tabs/components/tab-bar-item.tsx:88 | Tab key into strip | — | | | | | | | |
| IDE-053 | Right-click tab: context menu at tab bottom-centre +4px; items see MENU | web/src/features/tabs/components/tab-bar.tsx:267 | right-click README.md tab | — (native menu) | | | | | | | |
| IDE-054 | Close All skips pinned and uncloseable tabs | web/src/features/tabs/components/tab-bar-context-menu.tsx:50 | pin one tab; right-click > Close All | — | | | | | | | |
| IDE-055 | Drag tab to reorder: 6px distance arms, source opacity-40, neighbours shift; lifted pill overlay with name (+"•" if dirty) | web/src/features/tabs/components/sortable-editor-tab.tsx:11 | open 3 files; drag first past third | transform 250ms ease; overlay drop animation none | | | | | | | |
| IDE-056 | Tab drop reorder commits; pinned tabs stay first | web/src/features/panes/stores/slices/pane-actions/editor-tab-actions.ts:207 | drag and release | — | | | | | | | |
| IDE-057 | Sidebar toggle buttons in editor strip when sidebar collapsed: Show/Hide Sidebar, mirrored on right | web/src/features/tabs/components/tab-navigation-buttons.tsx:14 | collapse sidebar; Open editor | tooltip 150ms | | | | | | | |
| IDE-058 | "Review this branch" shortcut button at strip end when workspace can be reviewed | web/src/features/tabs/components/branch-review-shortcut-button.tsx:11 | feature workspace chat; Open editor | tooltip 150ms | | | | | | | |
| IDE-059 | Click Review this branch: opens branch review tab (deduped) | web/src/features/panes/utils/pane-command-actions.ts:53 | click Review this branch | — | | | | | | | |
| IDE-060 | Status announcements (sr-only) on tab open/close | web/src/features/tabs/components/tab-bar.tsx:474 | screen reader / a11y tree; close a tab (see A11Y) | — | | | | | | | |
| IDE-061 | New Tab page: muted Crowbar wordmark clamp(96px,14cqmin,148px) centred, buttons New Terminal / New Browser / New File / Review Branch | web/src/features/panes/components/new-tab-view.tsx:27 | ctrl+t in feature workspace | — | | | | | | | |
| IDE-062 | New Tab page without Review Branch (main workspace cannot review) | web/src/features/panes/components/new-tab-view.tsx:71 | ctrl+t in main workspace chat | — | | | | | | | |
| IDE-063 | New Tab page buttons replace the New Tab tab with terminal / browser / untitled file / review | web/src/features/tabs/hooks/use-pane-new-tab-actions.ts:15 | ctrl+t; click each button | — | | | | | | | |
| IDE-064 | Pane with no chat (stage / New Tab-only): no chat column, strip shows editor tabs only; no split toggle | web/src/features/tabs/components/tab-bar.tsx:134 | close all view tabs | — | | | | | | | |
| IDE-065 | Empty stage pane with no workspace: wordmark only, no buttons | web/src/features/panes/components/new-tab-view.tsx:52 | close all view tabs; select project home | — | | | | | | | |
| IDE-066 | Editor area with no active buffer shows New Tab view | web/src/features/panes/components/pane-editor-view-content.tsx:91 | Open editor | — | | | | | | | |
| IDE-067 | Terminals and browsers stay mounted when switching tabs (scrollback, page state kept) | web/src/features/panes/components/pane-editor-view-content.tsx:96 | ctrl+j, run `ls`; switch tab; switch back | — | | | | | | | |
| IDE-068 | Terminal exit closes its tab | web/src/features/terminal/components/terminal-tab.tsx:40 | ctrl+j; type `exit` Return | — | | | | | | | |
| IDE-069 | Reopen an existing terminal elsewhere jumps to its pane | web/src/features/panes/stores/slices/buffer-slice.ts:120 | split; open the same terminal from sidebar in other pane (see SB) | — | | | | | | | |
| IDE-070 | Mousedown anywhere in a pane activates it (accent ring moves in a split) | web/src/features/panes/hooks/use-pane-activation.ts:6 | split; click inside inactive pane | ring opacity 150ms | | | | | | | |
| IDE-071 | Stage after last view closed: single borderless-header pane, New Tab view | web/src/features/panes/components/split-view-root.tsx:58 | close both view tabs | — | | | | | | | |
| IDE-072 | Sidebar follows focus: focusing a file tab switches sidebar to Files, review/commit to Git, terminal/browser/chat to Chats | web/src/components/layout/use-sidebar-focus-mode.ts:17 | enable setting (see SET); click into README.md then terminal | — | | | | | | | |
| IDE-073 | Open tabs and active tab persist across restart (debounced 300ms) | web/src/features/panes/stores/window-pane-store.ts:84 | open two files; rig restart tauri | — | | | | | | | |
| IDE-074 | Light / dark theme of header and editor strip | web/src/features/tabs/components/pane-top-row.tsx:35 | rig theme light; rig theme dark | — | | | | | | | |

Skipped:
- Editor-group commands (split/close/close others/reset sizes/move to adjacent/lock group) in web/src/features/panes/utils/pane-command-actions.ts:39-234: no callers on develop.
- `switchToNext/PreviousEditorTab`: no callers, no chord.
- Bottom pane (BOTTOM_PANE_ID; "+" hidden there, tab-add-button.tsx:12): `setIsBottomPaneVisible` has no caller.
- Mobile shell / mobile-chat-stage (web/src/components/layout/mobile/*, web/src/features/panes/components/mobile-chat-stage.tsx): mobile only.
- Extensions marketplace tab internals: belong to extensions; icon row covered by IDE-025.
