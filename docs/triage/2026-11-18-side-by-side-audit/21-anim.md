# 21. Spinners and animations (ANIM)

### CSS keyframes

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-001 | flicker-strip: FlickerSpinner sprite strip steps through frames (one of 31 random flip-dot assets per instance) | web/src/index.css:191 | Any FlickerSpinner site below; open the "Perf fixture" chat right after `rig reset` to catch "Loading messages…" | 0.36–1.62s steps(4–18) infinite (90ms per frame, all assets); paused off-screen | | | | | | | |
| ANIM-002 | skeleton: Skeleton bar diagonal highlight sweep | web/src/styles/theme-vars.css:98 | `rig reset`, watch the sidebar tree before its first snapshot lands (sidebar skeleton) | 2s linear infinite, delay -1s | | | | | | | |
| ANIM-003 | tray-drain: removal-countdown bar under a removed sidebar row shrinks to 0 | web/src/styles/theme-vars.css:184 | Right-click a chat row in the sidebar → Remove; watch the red line under the held row | 8s linear forwards (essential motion: runs under reduced motion) | | | | | | | |
| ANIM-004 | oobe-drift: first-run backdrop blobs drift/rotate/scale (teal, orange, lavender) | web/src/components/oobe/oobe-gradient.css:51 | First-run screen. Seed: daemon with no projects (OOBE landing) | teal 26s ease-in-out infinite alternate; orange 32s alternate-reverse; lavender 21s; none under reduced motion | | | | | | | |
| ANIM-005 | attach-bar: attach-file modal indeterminate progress stripe slides | web/src/features/agent/styles/composer-pickers.css:471 | Chat composer "+" → attach a file by host path; watch the bar while the daemon reads it. Seed: a large host file so the read is visible | 1.1s ease-in-out infinite alternate | | | | | | | |
| ANIM-006 | rain-fall: compaction dot-matrix rain behind the composer pill (two layers) | web/src/features/agent/styles/composer-pill.css:383 | Compact a chat (/compact). Seed: running agent that compacts | layer A 2.8s, layer B 4.2s steps(14) infinite; paused (not removed) under reduced motion | | | | | | | |
| ANIM-007 | chat-token-fade: streamed words fade in, per-word stagger | web/src/features/agent/styles/transcript-stream.css:128 | Send a prompt and watch the reply stream. Seed: streaming agent reply | 260ms ease-out both; per-word delay 30ms, cascade capped at 320ms; none under reduced motion | | | | | | | |
| ANIM-008 | tool-verb-shimmer: live tool-call verb text gradient sweep | web/src/features/agent/styles/transcript-tools.css:207 | Watch a running tool row in a chat. Seed: running agent with a live tool call | 1.6s linear infinite | | | | | | | |

### Spinners and loaders

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-009 | FlickerSpinner: off-screen pause via IntersectionObserver | web/src/components/ui/flicker-spinner.tsx:37 | Scroll a list holding a spinner out of view and back. Seed: running agent row in sidebar | play-state paused/running | | | | | | | |
| ANIM-010 | FlickerSpinner: sidebar row working/busy glyph, operation row, workspace branch icon | web/src/components/sidebar/sidebar-row-glyph.tsx:27; web/src/components/sidebar/operation-sidebar-row.tsx:66; web/src/components/layout/workspace-branch-icon.tsx:122 | Sidebar Chats mode. Seed: running agent or workspace setup in progress | as ANIM-001 (size-3.5) | | | | | | | |
| ANIM-011 | FlickerSpinner: chat working line, send button busy, reviving signpost | web/src/features/agent/activity/working-line.tsx:69; web/src/features/agent/composer/composer-action-button.tsx:64; web/src/features/agent/composer/composer-signpost.tsx:31 | Chat pane composer area. Seed: running agent turn | as ANIM-001 (size-4) | | | | | | | |
| ANIM-012 | FlickerSpinner: running tool row, tool run group, subagent card | web/src/features/agent/transcript/rows/tool-row.tsx:84; web/src/features/agent/transcript/rows/tool-run-row.tsx:30; web/src/features/agent/transcript/rows/subagent-card.tsx:95 | Chat transcript. Seed: running agent with tool calls/subagents | as ANIM-001 (size-3) | | | | | | | |
| ANIM-013 | FlickerSpinner: plan tray and todo tray running item | web/src/features/agent/trays/plan-tray.tsx:44; web/src/features/agent/trays/tray-row.tsx:29 | Dock trays above composer. Seed: running agent with a plan/todos | as ANIM-001 (size-4, primary) | | | | | | | |
| ANIM-014 | FlickerSpinner: chat tab glyph and chat branch header while working | web/src/features/agent/shared/agent-chat-glyph.tsx:16; web/src/features/tabs/components/chat-branch-header-view.tsx:114 | Chat tab strip / chat header. Seed: running agent | as ANIM-001 | | | | | | | |
| ANIM-015 | FlickerSpinner: terminal-view connecting spinner | web/src/features/agent/terminal/agent-terminal-surface.tsx:168 | Chat → toggle chat/terminal view (`rig key <app> ctrl+slash`). Seed: provider CLI so the terminal view can attach | as ANIM-001 (size-6) | | | | | | | |
| ANIM-016 | FlickerSpinner: "Loading messages…" transcript placeholder | web/src/features/agent/transcript/agent-transcript.tsx:431 | `rig reset`, click the "Perf fixture" chat tab immediately | as ANIM-001 (size-4); brief | | | | | | | |
| ANIM-017 | Spinner (CircleNotch animate-spin): button loading indicator | web/src/components/ui/spinner.tsx:12; web/src/components/ui/button.tsx:57; web/src/features/panes/components/editor-pane.tsx:149 | Editor pane load-failure Retry, or daemon crash panel Restart. Seed: file that fails to load / crashed daemon | 1s linear infinite (exempt from reduced motion) | | | | | | | |
| ANIM-018 | LoadingSpinner: labelled spinner (review diff, migrating daemon, remote folders) | web/src/components/ui/loading-spinner.tsx:20; web/src/features/git/components/review-diff-tab.tsx:115; web/src/components/layout/shell-landing.tsx:66 | Git mode (`rig key <app> ctrl+3`) → open the branch diff of feature/pricing-rounding; catch "Loading branch diff" | 1s linear infinite | | | | | | | |
| ANIM-019 | Toast loading icon spin (stacked sidebar toast) | web/src/components/layout/sidebar-toast-overlay.tsx:81 | Stop a queued prompt ("Taking the prompt back…") or daemon migration toast. Seed: running agent or migrating daemon | 1s linear infinite (data-essential-motion) | | | | | | | |
| ANIM-020 | Sidebar skeleton rows (tree not yet synced / files panel Suspense) | web/src/components/layout/sidebar-tree-surface.tsx:145; web/src/components/layout/sidebar-skeleton.tsx:9; web/src/components/layout/sidebar-files-panel.tsx:49 | `rig reset` and watch sidebar; Files mode first open (`rig key <app> ctrl+2`) | skeleton 2s linear infinite | | | | | | | |
| ANIM-021 | Diagram skeleton then height grow to rendered mermaid (WAAPI) | web/src/features/agent/composer/plate/attachments/diagram-block.tsx:51 | Open a chat whose reply has a mermaid fence. Seed: chat with a mermaid code block | skeleton 2s linear; grow 160px→content 220ms ease-out; skipped under reduced motion | | | | | | | |
| ANIM-022 | OOBE prerequisite check row spinner | web/src/components/oobe/oobe-screen.tsx:53 | First-run screen → Get started. Seed: daemon with no projects | 1s linear infinite | | | | | | | |
| ANIM-023 | Working-line verb rotation (text swap) | web/src/features/agent/activity/working-line.tsx:60 | Chat working line with no specific heading. Seed: running agent turn | interval 3400ms, instant swap | | | | | | | |
| ANIM-024 | Elapsed clock ticking beside working line / removal seconds | web/src/features/agent/activity/lib/text-clock.ts:31 | Chat working line. Seed: running agent turn | interval 1000ms, text only | | | | | | | |

### Overlays: menus, popovers, dialogs, toasts, tooltips

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-025 | Dropdown (framer m.div): menu open/close fade + scale 0.98 + 4px rise toward anchor | web/src/components/ui/menu-popover.tsx:80 | Chat composer permission switcher, model/agent picker, context gauge, editor status menus, file search header menu | 120ms easeOut (cubic-bezier(0,0,0.58,1)); transforms dropped under reduced motion | | | | | | | |
| ANIM-026 | DropdownMenu (base-ui + tw-animate): popup fade + zoom 95% + slide 8px from anchor side, open and close | web/src/components/ui/dropdown-menu.tsx:40 | Chat composer "+" menu; tab overflow menu (open many tabs); host picker in sidebar | 100ms ease in/out | | | | | | | |
| ANIM-027 | DropdownMenu submenu: same fade/zoom/slide | web/src/components/ui/dropdown-menu.tsx:137 | Hover a submenu item in the composer "+" menu or settings menu | 100ms ease | | | | | | | |
| ANIM-028 | Tooltip: fade + zoom 95% + slide 4px from side, open and close | web/src/components/ui/tooltip.tsx:9 | Hover any icon button with a tooltip (e.g. sidebar "Add space", breadcrumb Preview) | 150ms ease; open delay 150ms | | | | | | | |
| ANIM-029 | Popover popup: scale 0.98 + fade on open; size morph; no exit style | web/src/components/ui/popover.tsx:54 | Click a project/repo icon in the sidebar to open the icon picker popover | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-030 | Popover positioner: position glide when anchor moves; viewport cross-fade between pages | web/src/components/ui/popover.tsx:47; web/src/components/ui/popover.tsx:64 | Icon picker popover open, resize window / switch content | 150ms cubic-bezier(0.4,0,0.2,1); none when data-instant | | | | | | | |
| ANIM-031 | Dialog backdrop dim + blur fade in/out | web/src/components/ui/dialog.tsx:23; web/src/components/ui/alert-dialog.tsx:18 | Open any modal (e.g. Settings, Add repository); Esc to close | 200ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-032 | Dialog popup: scale 0.98 + fade in/out; nested dialog scales parent down 10% | web/src/components/ui/dialog.tsx:72; web/src/components/ui/alert-dialog.tsx:62 | Open Add repository / import project modal; open a nested confirm from inside it | 200ms ease-in-out (cubic-bezier(0.4,0,0.2,1)) | | | | | | | |
| ANIM-033 | AppDialog popup: fade in/out (no scale) | web/src/components/ui/dialog.tsx:227 | Edit README.md, close its tab unsaved → unsaved-changes dialog | 200ms ease-in-out | | | | | | | |
| ANIM-034 | Chat image lightbox: scale 0.98 + fade | web/src/features/agent/composer/plate/attachments/chat-image-lightbox.tsx:35 | Click an image in a chat message. Seed: chat with an image | 200ms ease-in-out | | | | | | | |
| ANIM-035 | Stacked toast (sidebar column / fixed): fade in/out, swipe to dismiss | web/src/components/layout/sidebar-toast-overlay.tsx:69 | Hover a reply in "Perf fixture" → Copy → "Copied to clipboard" | 200ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-036 | Toast close button opacity on hover | web/src/components/layout/sidebar-toast-overlay.tsx:104 | Hover the × of a visible toast | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-037 | OOBE: presentation fade, tagline/button rise, card cross-fade, step slide 8px, check rows slide-in staggered | web/src/components/oobe/oobe-screen.tsx:272; web/src/components/oobe/oobe-screen.tsx:327; web/src/components/oobe/oobe-screen.tsx:42 | First-run screen → Get started → Continue. Seed: daemon with no projects | fade 0.3s cubic-bezier(0.25,0.1,0.35,1) (exit 0.25s); rise y spring k500 d25, delay 0.15/0.25s; card 0.3s; step 0.2s; rows 0.3s easeOut delay order×0.35s | | | | | | | |
| ANIM-038 | OOBE blocked-reason line fade | web/src/components/oobe/oobe-screen.tsx:153 | First-run prerequisites with git missing. Seed: no git on PATH + no projects | 0.3s cubic-bezier(0.25,0.1,0.35,1) | | | | | | | |

### Shell transitions: sidebar, tabs, panes, console

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-039 | Sidebar peek card slides in/out over the editor | web/src/components/layout/sidebar-peek.tsx:140 | Collapse sidebar (`rig key <app> ctrl+b`), move pointer to the window edge on the sidebar side, then away | translate 200ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-040 | Sidebar collapse/expand (docked) | web/src/components/layout/ide-shell.tsx:84 | `rig key <app> ctrl+b` twice | — (instant) | | | | | | | |
| ANIM-041 | Sidebar mode bar (Chats/Files/Git) active pill slides | web/src/components/ui/tabs.tsx:35 | `rig key <app> ctrl+1`, `ctrl+2`, `ctrl+3` | width+translate 200ms ease-in-out | | | | | | | |
| ANIM-042 | Sidebar mode tab text/bg colour on hover/select | web/src/components/ui/tabs.tsx:48 | Hover each sidebar mode tab | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-043 | Sidebar row indent eases when depth changes (nest/un-nest) | web/src/components/layout/workspace-row-base.ts:38 | Drag a chat row to nest it under another / drag it out | margin-inline-start 0.1s ease-in-out (motion-safe only) | | | | | | | |
| ANIM-044 | Sidebar row disclosure caret rotates 90° | web/src/components/sidebar/sidebar-row-actions.tsx:246 | Expand/collapse a repo or workspace row in Chats mode | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-045 | Project switcher "+" (Add space) reveals width/margin/opacity on hover | web/src/components/layout/sidebar-project-switcher.tsx:219 | Hover the project switcher row. Seed: 2+ projects | 150ms cubic-bezier(0.4,0,0.2,1); none under reduced motion | | | | | | | |
| ANIM-046 | Project carousel smooth scroll to the active project panel | web/src/components/sidebar/space-scroller.tsx:196 | Switch project with ctrl+shift+2 / switcher. Seed: 2+ projects | scroll-behavior smooth (UA timing) | | | | | | | |
| ANIM-047 | ScrollArea scrollbar fades in on hover/scroll, out after delay | web/src/components/ui/scroll-area.tsx:45 | Scroll the sidebar tree or Git panel list (`rig scroll`) | in 100ms, out 150ms after 300ms delay | | | | | | | |
| ANIM-048 | Sidebar split sash hairline colour | web/src/components/layout/sidebar-split-pane.tsx:336 | Hover the sidebar/content divider | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-049 | Sidebar drop line glides between slots | web/src/components/layout/drop-indicator.tsx:22 | Drag a chat row up/down the sidebar tree | top/left/width 60ms ease-out; none under reduced motion | | | | | | | |
| ANIM-050 | Drop line transition suspended while list edge-scrolls | web/src/components/sidebar/lib/sidebar-drag-hover.ts:127 | Drag a row to the top/bottom edge of an overflowing tree. Seed: enough rows to scroll | transition none while tracking | | | | | | | |
| ANIM-051 | Edge auto-scroll during drag (rAF ramp) | web/src/components/layout/edge-scroll.ts:78 | Hold a dragged row within 36px of the tree's top/bottom edge. Seed: enough rows to scroll | rAF, 1→14px/frame ramp | | | | | | | |
| ANIM-052 | Pane tab strip: tabs slide apart to open a slot for a dragged sidebar row/tab | web/src/components/sidebar/lib/tab-strip-drag.ts:198 | Drag a sidebar chat row over a pane's tab strip | transform 150ms ease-out | | | | | | | |
| ANIM-053 | Pane tab strip: dropped tab glides from ghost to its slot | web/src/components/sidebar/lib/tab-strip-drag.ts:112; web/src/components/sidebar/lib/tab-strip-drag.ts:274 | Drop a sidebar chat row into a pane's tab strip | transform 150ms ease-out | | | | | | | |
| ANIM-054 | Editor tab bar sortable reorder shift (dnd-kit) | web/src/features/tabs/components/sortable-editor-tab.tsx:24 | Drag one tab along the tab bar | transform 250ms ease (dnd-kit default); dragged tab 40% opacity | | | | | | | |
| ANIM-055 | Tab drag overlay drop | web/src/features/tabs/components/tab-drag-preview.tsx:12 | Release a dragged tab | — (dropAnimation null) | | | | | | | |
| ANIM-056 | Tab bar smooth-scrolls active tab into view | web/src/features/tabs/components/tab-bar.tsx:246 | Open many files so tabs overflow, then activate an off-screen tab (overflow menu) | scrollIntoView smooth, inline center | | | | | | | |
| ANIM-057 | Tab close button opacity on hover | web/src/features/tabs/components/tab-bar-item.tsx:171 | Hover an inactive tab | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-058 | Focused-pane accent ring fade | web/src/features/panes/components/pane-accent-ring.tsx:44 | Split the pane (drag a tab onto the right edge), then click between panes / ctrl+alt+Left/Right | opacity 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-059 | Pane chrome background colour on focus change | web/src/features/panes/components/pane-container.tsx:230 | Two panes; switch focus | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-060 | Pane handle (top-centre grip) fades in on pane hover | web/src/features/panes/components/pane-handle.tsx:58 | Two panes; hover a pane | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-061 | Pane sash hover colour | web/src/features/panes/components/pane-sash.tsx:200 | Two panes; hover the divider | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-062 | Split drop overlay zone box morphs between zones | web/src/features/panes/components/split-drop-overlay.tsx:70 | Drag a tab over a pane's edges/centre | all 100ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-063 | Console panel slide (overlay) / size (docked) open-close | web/src/features/console/components/console-panel.tsx:84; web/src/features/console/components/console-panel.tsx:85 | `rig key <app> ctrl+grave` twice; switch dock mode in console header | 200ms ease-out; none while resizing | | | | | | | |
| ANIM-064 | Files tree folder caret rotates | web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx:76 | Files mode, expand/collapse src/ | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-065 | Files tree rename input smooth-scrolls into view | web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx:153 | Files mode → right-click a file → Rename / New file | scrollIntoView smooth, block center | | | | | | | |
| ANIM-066 | Markdown preview link underline + anchor smooth scroll | web/src/features/editor/markdown/styles.css:143; web/src/features/editor/markdown/markdown-preview.tsx:97 | Open README.md → breadcrumb Preview; hover a link / click a #anchor link. Seed: README with an anchor link | all 0.2s ease; smooth scroll | | | | | | | |
| ANIM-067 | Chat view switch (Chat/Terminal/Split) segmented pill slides | web/src/components/ui/segmented-control.tsx:33 | Chat header view switch, or `rig key <app> ctrl+slash` | width+translate 200ms ease-in-out; none under reduced motion | | | | | | | |
| ANIM-068 | Segmented control item text/outline colour | web/src/components/ui/segmented-control.tsx:62 | Hover/focus the chat view switch items; branch review pane switch | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |

### Controls

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-069 | Button focus ring / shadow | web/src/components/ui/button-variants.ts:4 | Tab onto any button | box-shadow 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-070 | Text fields focus ring (input, textarea, select) | web/src/components/ui/input.tsx:45; web/src/components/ui/textarea.tsx:26; web/src/components/ui/select.tsx:11 | Settings → click a text/select field; Git commit message box | box-shadow 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-071 | Checkbox / radio / OTP field focus shadow | web/src/components/ui/checkbox.tsx:29; web/src/components/ui/radio-group.tsx:22; web/src/components/ui/otp-field.tsx:35 | Git changed-files checkboxes; add-host dialog code field | box-shadow 150ms | | | | | | | |
| ANIM-072 | Switch track colour + focus ring | web/src/components/ui/switch.tsx:29 | Settings → Appearance/Editor → toggle a switch | background-color, box-shadow 200ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-073 | Switch thumb slide, press-stretch | web/src/components/ui/switch.tsx:39 | Settings → toggle a switch; hold mouse down on it | translate .15s, border-radius .15s, scale .1s delay .1s, transform-origin .15s (ease) | | | | | | | |
| ANIM-074 | Badge focus shadow | web/src/components/ui/badge.tsx:10 | Git / review panes badges with focus | box-shadow 150ms | | | | | | | |
| ANIM-075 | Menu item hover/highlight colour (dropdown items) | web/src/components/ui/dropdown-styles.ts:29 | Open composer permission switcher; arrow through items | colors 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-076 | Search field / clear button colour | web/src/components/ui/search.tsx:47; web/src/components/ui/search.tsx:62 | Files mode search header; type then hover clear | colors 150ms | | | | | | | |
| ANIM-077 | Toggle group item colours (review diff unified/split) | web/src/components/ui/toggle-group.tsx:25 | Review diff tab toggle | color, bg, box-shadow 150ms | | | | | | | |
| ANIM-078 | Effort slider fill and knob move | web/src/features/agent/controls/agent-selection-picker.tsx:226; web/src/features/agent/controls/agent-selection-picker.tsx:230 | Chat composer model picker → change effort | width / left 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-079 | Agent picker row hover colour | web/src/features/agent/controls/agent-selection-picker.tsx:108 | Open model picker, hover rows | colors 150ms | | | | | | | |
| ANIM-080 | Remote folder tree caret rotate + row hover | web/src/components/ui/tree.tsx:174; web/src/components/ui/tree-row.tsx:33 | Remote file picker. Seed: paired remote host | 150ms | | | | | | | |
| ANIM-081 | Review message overflow (…) button fades in on message hover | web/src/features/git/components/review-message-row.tsx:90 | Open a seeded review thread, hover a message | 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| ANIM-082 | Review author / settings nav / settings rows hover colour | web/src/features/git/components/review-message-author.tsx:154; web/src/features/settings/components/settings-layout.tsx:43; web/src/features/settings/components/settings-section.tsx:256 | Hover in review thread; Settings sidebar items | colors 150ms | | | | | | | |
| ANIM-083 | Settings provider rows: reorder shift (dnd-kit) and hover colour | web/src/features/settings/components/tabs/sortable-provider-row.tsx:36; web/src/features/settings/components/tabs/sortable-provider-row.tsx:39 | Settings → Providers, drag a row's handle | transform 250ms ease; colors 150ms | | | | | | | |
| ANIM-084 | Settings tabs (sessions, hosts, terminal) row hover colour | web/src/features/settings/components/tabs/active-sessions-settings.tsx:21; web/src/features/settings/components/tabs/hosts-settings.tsx:50; web/src/features/settings/components/tabs/terminal-settings.tsx:83 | Settings → each tab, hover rows | colors 150ms | | | | | | | |
| ANIM-085 | Editor status bar actions hover colour | web/src/features/editor/components/toolbar/editor-status-actions.tsx:28; web/src/features/editor/components/toolbar/editor-status-actions.tsx:33 | Open src/pricing.ts, hover status actions | colors 150ms | | | | | | | |

### Markdown editor (Plate)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-086 | Block selection highlight fade | web/src/components/ui/block-selection.tsx:8 | Open README.md (rich editor), drag-select across blocks | opacity 150ms | | | | | | | |
| ANIM-087 | Table row controls fade on row hover | web/src/components/ui/table-node.tsx:257 | README.md with a table, hover a row. Seed: README containing a table | opacity 100ms | | | | | | | |
| ANIM-088 | Table cell colour swatches hover | web/src/components/ui/table-cell-color-menu.tsx:43 | Table floating toolbar → cell colour menu. Seed: table | all 150ms | | | | | | | |
| ANIM-089 | Slash menu item highlight colour | web/src/components/ui/inline-combobox.tsx:278 | README.md editor, type `/` | colors 150ms | | | | | | | |
| ANIM-090 | Mark floating toolbar buttons colour/shadow | web/src/components/ui/toolbar.tsx:33 | README.md editor, select text | color, box-shadow 150ms | | | | | | | |

### Chat

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-091 | Transcript fade-in on chat mount (@starting-style) | web/src/features/agent/styles/transcript-stream.css:140; web/src/features/agent/styles/transcript-layout.css:39 | Click between the two chat tabs open after `rig reset` | opacity 0→1 320ms cubic-bezier(0,0,0.2,1); none under reduced motion | | | | | | | |
| ANIM-092 | Composer dock arrival slide (empty document → dock on first send) | web/src/features/agent/chat/lib/arrival-animation.ts:28 | New chat (`rig key <app> ctrl+n`), type, Enter. Seed: provider so the send is accepted | transform 320ms cubic-bezier(0,0,0.2,1) | | | | | | | |
| ANIM-093 | Dock underbar fade/rise after arrival | web/src/features/agent/styles/composer-dock.css:79 | As ANIM-092 | opacity, transform 200ms ease-out | | | | | | | |
| ANIM-094 | Released pin room collapses under reply | web/src/features/agent/styles/transcript-layout.css:114 | Send a prompt, let the reply finish. Seed: streaming agent reply | min-height 560ms cubic-bezier(0.4,0,0.2,1); none under reduced motion | | | | | | | |
| ANIM-095 | Eased follow-scroll to bottom while streaming (rAF) | web/src/features/agent/hooks/lib/follow-scroll.ts:46 | Watch a streaming reply grow. Seed: streaming agent reply | exponential ease τ=100ms, frame dt cap 32ms | | | | | | | |
| ANIM-096 | Composer pill focus ring | web/src/features/agent/styles/composer-pill.css:18 | Click into the chat composer; Tab out | box-shadow 0.18s ease | | | | | | | |
| ANIM-097 | Send button press scale + state colour change | web/src/features/agent/styles/composer-pill.css:209 | Type in composer (off→on), mouse-down on send | transform, bg, border 0.12s ease; :active scale 0.94 | | | | | | | |
| ANIM-098 | Reply turnbar (copy/time) fades in on hover/focus | web/src/features/agent/styles/transcript-tools.css:9 | Hover an assistant reply in "Perf fixture" | opacity 0.15s ease | | | | | | | |
| ANIM-099 | Tool row disclosure chevron rotates | web/src/features/agent/styles/transcript-tools.css:86 | Expand a tool row in "Perf fixture". Seed: chat with tool calls if the fixture has none | transform 120ms ease | | | | | | | |
| ANIM-100 | Attachment controls fade in on attachment hover | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:218 | Composer with an attachment, hover it. Seed: file to attach | opacity 100ms | | | | | | | |
| ANIM-101 | Determinate attach progress bar width | web/src/features/agent/styles/composer-pickers.css:469 | Attach a file sent as bytes. Seed: file large enough to show progress | width 0.15s linear | | | | | | | |
| ANIM-102 | Markdown image in chat hover colour | web/src/features/agent/composer/plate/attachments/chat-markdown-image-node.tsx:53 | Hover an image in a chat message. Seed: chat with an image | colors 150ms | | | | | | | |
| ANIM-103 | Terminal cursor blink (xterm) | web/src/features/terminal/hooks/use-xterm-instance.ts:108 | Open a terminal tab (`rig key <app> ctrl+j`), focus it | xterm blink (default on, block cursor) | | | | | | | |
| ANIM-104 | Monaco cursor blink | web/src/features/editor/lib/monaco-adapters.ts:36 | Open src/pricing.ts, click in the editor | Monaco 'blink' | | | | | | | |

### Reduced motion

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ANIM-105 | Global kill-switch: all CSS animations/transitions forced to 0.01ms ×1, smooth scroll off; `.animate-spin` and `[data-essential-motion]` exempt | web/src/index.css:31 | Run ANIM-031, ANIM-039, ANIM-041 with reduced motion on. Seed/rig: prefers-reduced-motion=reduce (GTK enable-animations=false / app setting) | 0.01ms | | | | | | | |
| ANIM-106 | framer-motion follows OS reduced-motion (transforms dropped, opacity kept) | web/src/app-main.tsx:115 | Open a Dropdown (ANIM-025) with reduced motion on. Seed/rig: prefers-reduced-motion=reduce | reducedMotion="user" | | | | | | | |
| ANIM-107 | Essential spinners keep running under reduced motion (FlickerSpinner, removal drain, toast spinner) | web/src/components/ui/flicker-spinner.tsx:56; web/src/components/sidebar/sidebar-row-removing.tsx:98; web/src/components/ui/toast.tsx:92 | ANIM-003 with reduced motion on. Seed/rig: prefers-reduced-motion=reduce | unchanged durations | | | | | | | |

Skipped:
- web/src/styles/theme-vars.css:103 `caret-blink` keyframes: `animate-caret-blink` utility never used, so not emitted.
- web/src/styles/theme-vars.css:176 `row-action-in` keyframes: `animate-row-action-in` never used.
- web/src/styles/theme-vars.css:192 `label-in` keyframes: `animate-label-in` never used.
- web/src/styles/theme-vars.css:114 and web/src/index.css:220 `toast-success/error-odd/even` keyframes: only applied by AnchoredToasts (web/src/components/ui/toast.tsx:34), and nothing ever adds to `anchoredToastManager`.
- web/src/components/ui/toast.tsx:69 anchored toast scale/fade transition: same, no producer.
- web/src/components/layout/nav-stack.tsx:25 and :39 push/pop slide 280ms: `useSidebarNavStore.push` has no caller, stack always empty.
- web/src/components/ui/sidebar.tsx:161 and :172 width/offset 200ms linear: the `<Sidebar>` component is never rendered (only provider/subparts are used).
- web/src/features/hosts/components/add-host-screen.tsx:42, :47, :49 wordmark/tagline 500ms ease-out: `/add-host` route lands only without a local daemon (phone app); desktop always has one.
- web/src/features/window/components/sidebar-row-drag.tsx:165 DragOverlay: mounted only by mobile-sidebar-page.tsx.
- web/src/components/ui/scroll-area.tsx:21 `transition-shadows`: not a Tailwind utility (typo), produces no transition.
- web/src/components/ui/input.tsx:29 `background-color 5000000s` transition: autofill-tint suppression, no visible motion.
- web/src/styles/mobile.css: no animations; mobile shell files (components/layout/mobile/*) have no motion utilities.
- web/src/components/ui/spinners/*.svg SMIL fallback path (flicker-spinner.tsx:90): only taken for an asset the baker rejects; all 31 bundled assets bake.
