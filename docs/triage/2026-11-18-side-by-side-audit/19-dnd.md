# 19. Drag and drop (DND)

### Sidebar tree rows

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-001 | Drag threshold: a sidebar row press arms a drag only after 8px of travel; less is a click that opens the row | web/src/components/sidebar/hooks/use-sidebar-drag.ts:41 | Press the "Perf fixture" row, move 5px, release (opens chat); then press, move 12px (drag starts) | — | | | | | | | |
| DND-002 | Chat row → before/after another chat row: reorders the chat in the tree (outer band, 30% of row height) | web/src/components/sidebar/lib/sidebar-drop-policy.ts:288 | Drag the "Perf fixture" row onto the top edge of the other chat row, release | — | | | | | | | |
| DND-003 | Chat row → middle of another chat row: nests it as a thread under that chat | web/src/components/sidebar/lib/sidebar-drop-policy.ts:125 | Drag the "Perf fixture" row onto the vertical middle of the other chat row, release | — | | | | | | | |
| DND-004 | Chat row → middle of a branch row: moves the chat into that workspace | web/src/components/sidebar/lib/sidebar-drop-policy.ts:137 | Drag a chat row under `main` onto the middle of the `feature/pricing-rounding` row, release | — | | | | | | | |
| DND-005 | Branch row → before/after sibling branch row: reorders the workspace (placeWorkspace) | web/src/components/sidebar/lib/drop-actions.ts:46 | Drag the `feature/pricing-rounding` row onto the top edge of the `main` row, release | — | | | | | | | |
| DND-006 | Branch row → middle of a branch row: re-parents the fork (workspace.reparent operation) | web/src/components/sidebar/lib/drop-actions.ts:29 | Drag the `feature/pricing-rounding` row onto the middle of the `main` row, release. Seed: a second fork workspace that is not locked | — | | | | | | | |
| DND-007 | Folder row → into/before/after: files or reorders a folder (20% edge band; only within its branch anchor) | web/src/components/sidebar/lib/sidebar-drop-policy.ts:240 | Drag a folder row onto another folder's middle, then onto its top edge. Seed: two sidebar folders in the repo and one in the project home | — | | | | | | | |
| DND-008 | Repo header row → before/after another repo header: reorders repos in the project home (no optimistic paint) | web/src/components/sidebar/lib/sidebar-drop-policy.ts:201 | Drag the "checkout" header row onto the top edge of another repo header. Seed: a second repo in "Crowbar Seed" | — | | | | | | | |
| DND-009 | Refused drop (policy allows no mode): release shows error toast "Can't move <label> there" | web/src/components/sidebar/lib/sidebar-drop-dispatch.ts:63 | Drag the "checkout" repo header row onto a chat row under `main`, release | — | | | | | | | |
| DND-010 | Release back on the dragged row itself: silent cancel, no toast | web/src/components/sidebar/lib/sidebar-drop-dispatch.ts:67 | Drag the "Perf fixture" row 20px down and back onto itself, release | — | | | | | | | |
| DND-011 | Row into its own descendant: refused by the cycle guard (no line/fill shown, toast on release) | web/src/components/sidebar/lib/sidebar-drag-hover.ts:50 | Nest a chat under "Perf fixture" (DND-003), then drag "Perf fixture" onto that child's middle | — | | | | | | | |
| DND-012 | Busy or working row: every mode refused (only a working chat may leave its workspace) | web/src/components/sidebar/lib/sidebar-drop-policy.ts:151 | Drag a working chat row onto a sibling chat. Seed: a chat mid-turn (running agent) | — | | | | | | | |
| DND-013 | Sidebar chat row → pane centre: opens that chat into the pane's view (adds, never replaces) | web/src/components/sidebar/lib/pane-drop-actions.ts:116 | Drag a sidebar chat row that is not open onto the middle of the chat pane, release | — | | | | | | | |
| DND-014 | Sidebar chat row → pane edge (left/right/top/bottom 25% band): splits the pane, chat lands on that side | web/src/features/panes/utils/pane-drop-zones.ts:10 | Drag a sidebar chat row onto the right edge of the chat pane, release; repeat for left, top, bottom | — | | | | | | | |
| DND-015 | Sidebar chat row → pane of another project: toast "That chat belongs to a different space", nothing opens | web/src/components/sidebar/lib/pane-drop-actions.ts:150 | Drag a chat row of project B onto a pane showing project A. Seed: a second project with a chat | — | | | | | | | |
| DND-016 | Sidebar chat row → view tab middle: merges the chat into that tab's view (split) | web/src/components/sidebar/lib/pane-drop-actions.ts:87 | Drag a sidebar chat row onto the middle of an open chat tab in the top strip, release | — | | | | | | | |
| DND-017 | Sidebar chat row → view tab left/right edge: reorders that chat's open tab beside the target | web/src/components/sidebar/lib/pane-drop-actions.ts:37 | Drag the sidebar row of one of the two open chats onto the left edge of the other chat's tab, release | — | | | | | | | |
| DND-018 | Folder/workflow row → pane or view tab: no chat to open, drop does nothing | web/src/components/sidebar/lib/pane-drop-actions.ts:103 | Drag a folder row onto the chat pane, release. Seed: a sidebar folder | — | | | | | | | |
| DND-019 | Drag ghost: cloned row on the lifted white surface follows the pointer at the grab offset, portalled above everything | web/src/components/layout/drag-ghost.tsx:46 | Drag any sidebar row; hold mid-pane; screenshot in light and dark (`rig theme`) | — | | | | | | | |
| DND-020 | Source row while dragging: stays in place at 40% opacity | web/src/components/sidebar/sidebar-row.tsx:164 | Drag the "Perf fixture" row away, hold | — | | | | | | | |
| DND-021 | Drop indicator line: 2px hairline with a 4px end-cap dot, inset 8px at the target's depth, slides between slots | web/src/components/layout/drop-indicator.tsx:9 | Drag a chat row slowly across the top/bottom edges of several rows | 60ms ease-out (top,left,width) | | | | | | | |
| DND-022 | Nest target fill: the row a drop would land inside fills with the drop-nest colour and edge; no line shown | web/src/components/layout/workspace-row-base.ts:27 | Drag a chat row onto the middle of another chat row, hold | — | | | | | | | |
| DND-023 | Edge auto-scroll: holding within 36px of the tree's top/bottom scrolls it, 1→14px per frame ramp; line re-tracks with transition off | web/src/components/layout/edge-scroll.ts:5 | Drag a row to the bottom edge of the sidebar list and hold still. Seed: enough sidebar rows to overflow the sidebar (≈40) | rAF ramp 1–14 px/frame | | | | | | | |
| DND-024 | Wheel locked during a row drag: only edge-scroll moves the list | web/src/components/sidebar/hooks/use-sidebar-drag.ts:240 | Drag a row and hold, `rig scroll` over the sidebar. Seed: sidebar overflowing (as DND-023) | — | | | | | | | |
| DND-025 | Click after drop is swallowed: the target row is not opened by the release | web/src/components/sidebar/hooks/use-sidebar-drag.ts:57 | Drop a chat row onto another chat row; check the target chat did not open | — | | | | | | | |
| DND-026 | No Esc cancel: Escape during a row drag does nothing; drag continues until release | web/src/components/sidebar/hooks/use-sidebar-drag.ts:369 | Drag a row, `rig key <app> Escape`, keep moving, release on a pane | — | | | | | | | |
| DND-027 | Window blur mid-drag ends the drag with no drop | web/src/components/sidebar/hooks/use-sidebar-drag.ts:361 | Drag a row, then focus another X window (e.g. `xdotool` activate root/other window), release | — | | | | | | | |
| DND-028 | Release outside the window: next move with no buttons ends the drag with no drop | web/src/components/sidebar/hooks/use-sidebar-drag.ts:264 | Drag a row off the window edge, release outside, move back in | — | | | | | | | |
| DND-029 | Double-click still renames: no pointer capture until the threshold passes | web/src/components/sidebar/hooks/use-sidebar-drag.ts:159 | Double-click the "Perf fixture" row; inline rename input appears | — | | | | | | | |

Skipped:
- `web/src/features/window/components/sidebar-row-drag.tsx` (dnd-kit DraggableRow/RowDragScope): only imported by `mobile-sidebar-page.tsx`, mobile UI only.
- `web/src/components/layout/nav-stack.tsx` pushed-screen layer: `useSidebarNavStore.push` has no caller, so it never hosts a drag.

### View tab strip

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-030 | Chat tab → another slot in the strip: reorders views; neighbours slide aside past each tab's midpoint | web/src/components/sidebar/lib/tab-strip-drag.ts:32 | After `rig reset`, drag the first chat tab right past the second tab's midpoint, release | 150ms ease-out (transform) | | | | | | | |
| DND-031 | In-strip ghost: the whole tab is cloned, clamped to the strip horizontally; source slot is an empty gap | web/src/components/sidebar/lib/tab-strip-drag.ts:295 | Drag a chat tab sideways inside the strip, hold | — | | | | | | | |
| DND-032 | Drop settle: released tab glides from the ghost position into its slot | web/src/components/sidebar/lib/tab-strip-drag.ts:312 | Drag a chat tab one slot over and release mid-motion; record | 150ms ease-out (transform) | | | | | | | |
| DND-033 | Slide hysteresis: 6px of extra travel before a slide changes hands at a boundary | web/src/components/sidebar/lib/tab-strip-drag.ts:78 | Drag a tab to sit exactly at a neighbour's midpoint and jiggle ±3px | — | | | | | | | |
| DND-034 | Pinned tabs move only among pinned tabs; unpinned only among unpinned | web/src/components/sidebar/lib/tab-strip-drag.ts:151 | Right-click a chat tab → Pin; drag the unpinned tab left past the pinned one, release | 150ms ease-out | | | | | | | |
| DND-035 | Tab dragged down out of the strip band (≥1 tab height): ghost leaves the strip, source tab shows faded | web/src/components/sidebar/lib/tab-strip-drag.ts:203 | Drag a chat tab straight down into the pane, hold | — | | | | | | | |
| DND-036 | Tab → pane edge: splits the view, the dragged chat joins the pane on that side | web/src/components/sidebar/lib/pane-drop-actions.ts:139 | Drag the second chat tab down onto the right edge of the first chat's pane, release | — | | | | | | | |
| DND-037 | Tab → pane centre: opens the dragged chat into that pane's view | web/src/components/sidebar/lib/sidebar-drop-dispatch.ts:37 | Drag a chat tab down onto the middle of the other chat's pane, release | — | | | | | | | |
| DND-038 | Grabbing any member of a split tab carries every chat of that view | web/src/features/tabs/components/view-tabs-row.tsx:45 | Make a split (DND-036), drag its tab to the other slot, release | 150ms ease-out | | | | | | | |
| DND-039 | Strip edge auto-scroll: holding a tab within 36px of the strip's end scrolls it sideways | web/src/components/sidebar/lib/tab-strip-drag.ts:188 | Drag a chat tab to the strip's right end and hold. Seed: ≥10 open chat tabs (strip overflows at 112px min) | rAF ramp 1–14 px/frame | | | | | | | |
| DND-040 | Close/unpin buttons on a tab are not drag handles | web/src/components/sidebar/view-row.tsx:108 | Press the × of a chat tab and drag 20px; no drag starts | — | | | | | | | |
| DND-041 | Strip vertical drop line: 2px vertical hairline on the target tab's near edge (sidebar-row drags over the strip) | web/src/components/layout/drop-indicator.tsx:60 | Drag a sidebar chat row onto the left edge of a chat tab, hold | 60ms ease-out | | | | | | | |

### Pane handle (split chat)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-042 | Pane grip ("Drag this chat") appears on hovering a split pane's top edge | web/src/features/panes/components/pane-handle.tsx:60 | Make a split (DND-036), hover the right pane | — | | | | | | | |
| DND-043 | Pane grip → tab strip: a tab-shaped preview opens a gap in the strip; release adds the chat as its own tab | web/src/components/sidebar/lib/tab-strip-drag.ts:68 | Drag the right pane's grip up into the tab strip between tabs, release | 150ms ease-out (gap slide) | | | | | | | |
| DND-044 | New tab settle: tab created by a pane drop glides from the ghost into its slot | web/src/components/sidebar/lib/tab-strip-drag.ts:263 | As DND-043; record the release | 150ms ease-out | | | | | | | |
| DND-045 | Pane grip → other pane's edge/centre: moves the chat into that pane on that side | web/src/features/panes/components/pane-handle.tsx:52 | In a split, drag the right pane's grip onto the left pane's bottom edge, release | — | | | | | | | |
| DND-046 | Pane grip ghost outside the strip: lifted ViewTabPreview (160px) of that chat | web/src/features/panes/components/pane-handle.tsx:96 | Drag the grip into the middle of a pane, hold | — | | | | | | | |

### Panes and split zones

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-047 | Zone preview: secondary-tinted rounded box over the half (edge) or whole pane (centre) the drop would take | web/src/features/panes/components/split-drop-overlay.tsx:70 | Drag a sidebar chat row over a pane, sweep centre → left → top → right → bottom | transition-all 100ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| DND-048 | Zone geometry: edges are the outer 25% (left/right win on diagonals), rest is centre | web/src/features/panes/utils/pane-drop-zones.ts:18 | Drag a row to the pane's top-left corner region and along the diagonal | — | | | | | | | |
| DND-049 | Pane-hit ring: 2px secondary ring on the hovered pane when no zone preview is drawn | web/src/features/panes/components/pane-container.tsx:208 | Drag a sidebar chat row over a pane, hold | — | | | | | | | |
| DND-050 | Rounded/shadowed pane chrome during drags stays as at rest (only sash resize flattens) | web/src/index.css:142 | Drag a sidebar row across panes; compare corners/shadows with rest | — | | | | | | | |
| DND-051 | Browser pane native webview hides while any drag ghost/line is on screen, returns on release | web/src/features/browser/hooks/use-native-overlay-active.ts:41 | Open a browser pane, drag a sidebar row over it, hold, release. Seed: a browser tab open on a local page | — | | | | | | | |

Skipped:
- `web/src/features/panes/hooks/use-pane-drop-handlers.ts:62` `handleSplitDrop` for `application/tab-data`: nothing in develop sets that type, so the HTML5 tab-split path is unreachable.

### Editor tabs

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-052 | Editor tab → another editor tab slot (same pane only): reorders; drag start selects the tab | web/src/features/tabs/hooks/use-tab-drag.ts:30 | Open README.md and src/pricing.ts from the Files sidebar; drag the README.md tab past pricing.ts, release | dnd-kit sortable 250ms ease (transform) | | | | | | | |
| DND-053 | Editor tab drag activation: 6px distance; not from the close/pin button (`data-no-dnd`) | web/src/features/tabs/components/tab-bar.tsx:150 | Press an editor tab's × and drag 20px (no drag); press the label and drag 4px (no drag), 10px (drag) | — | | | | | | | |
| DND-054 | Editor tab overlay: lifted pill with the file name (+ • if dirty); no drop animation | web/src/features/tabs/components/tab-drag-preview.tsx:12 | Drag an editor tab, hold; release and record (snaps, no fly-back) | none (dropAnimation null) | | | | | | | |
| DND-055 | Editor tab source while dragging: 40% opacity, raised z | web/src/features/tabs/components/sortable-editor-tab.tsx:26 | Drag an editor tab, hold | — | | | | | | | |
| DND-056 | Editor tab → another pane or the sidebar: no effect (each tab bar has its own DndContext) | web/src/features/tabs/hooks/use-tab-drag.ts:46 | In a split, drag an editor tab onto the other pane's tab bar, release | — | | | | | | | |
| DND-057 | Esc during an editor tab drag cancels and restores order | web/src/features/tabs/components/tab-bar.tsx:339 | Drag an editor tab past its neighbour, `rig key <app> Escape`, release | dnd-kit sortable 250ms ease | | | | | | | |
| DND-058 | Chat tab and + button in the editor strip are not draggable | web/src/features/tabs/components/tab-bar.tsx:392 | With editor tabs open, drag the chat entry and the + button | — | | | | | | | |

### File explorer

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-059 | File drag starts after 5px of travel from a tree row | web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts:175 | Switch the sidebar to Files; press `src/pricing.ts`, move 3px (no drag), then 10px (drag) | — | | | | | | | |
| DND-060 | File drag ghost: floating name label at pointer +10px/−10px, lifted surface, shadow | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:87 | Drag `README.md` in the Files tree, hold | — | | | | | | | |
| DND-061 | File → folder row: moves the file into that folder; folder row shows nest fill | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:229 | Drag `README.md` onto the `src` folder row, release | — | | | | | | | |
| DND-062 | File → file row: moves into that file's folder (target file row highlighted) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:242 | Drag `README.md` onto `src/inventory.ts`, release | — | | | | | | | |
| DND-063 | File → empty tree area: moves to the root; whole tree gets a dashed secondary border | web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx:261 | Drag `src/pricing.ts` to blank space below the last row, hold, release | — | | | | | | | |
| DND-064 | Hovering a collapsed folder 550ms during a drag auto-expands it | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:76 | Collapse `src`; drag `README.md` onto `src` and hold still | 550ms delay | | | | | | | |
| DND-065 | Folder into its own subtree: no target highlighted, nothing moves | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:148 | Drag `src` onto one of its own child rows, release. Seed: a nested folder under src | — | | | | | | | |
| DND-066 | File → pane: opens the file in that pane (zone preview shows but zone is ignored) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:218 | Drag `src/pricing.ts` onto the chat pane's right edge, release | transition-all 100ms (zone preview) | | | | | | | |
| DND-067 | Folder → pane: no pane highlight and no open | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:189 | Drag `src` onto the chat pane, release | — | | | | | | | |
| DND-068 | Move failure: error message "Failed to move <name>: …" | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:253 | Drag `README.md` into a folder already holding a README.md. Seed: a folder containing a README.md | — | | | | | | | |
| DND-069 | Leaving the window mid-drag ends it as a release over the last hovered target | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:265 | Drag `README.md` over `src`, continue out of the window edge | — | | | | | | | |
| DND-070 | No Esc cancel for file drags | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:263 | Drag `README.md`, `rig key <app> Escape`, release on `src` | — | | | | | | | |
| DND-071 | Dragged row shows move cursor | web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx:213 | Drag `README.md`, hold | — | | | | | | | |
| DND-072 | Git panel changed-file row: native HTML drag image follows pointer; no target accepts it | web/src/features/git/components/changed-files-tree.tsx:273 | Open workspace `feature/pricing-rounding`, Git sidebar mode; drag `src/pricing.test.ts` onto a pane, release | — | | | | | | | |

### OS file drops

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-073 | OS files → chat composer pill: each path uploads and inserts as an attachment | web/src/features/agent/composer/agent-composer.tsx:147 | Drag a file from an X file manager onto the composer, release. Seed/rig: an X drag source (file manager or dragon) and a file on disk | — | | | | | | | |
| DND-074 | Composer pill drop-target ring (border + 3px ring) while an OS file hovers | web/src/features/agent/styles/composer-pill.css:50 | As DND-073, hold over the composer. Seed/rig: X drag source | box-shadow 180ms ease | | | | | | | |
| DND-075 | OS files → new-chat empty document: dashed inset ring while hovering; drop attaches | web/src/features/agent/styles/composer-document.css:19 | Start a new chat, drag a file over the empty document, release. Seed/rig: X drag source | — | | | | | | | |
| DND-076 | OS files → Attach file modal dropzone: border/background highlight; drop stages the files | web/src/features/agent/composer/attach-file-modal.tsx:251 | Composer + menu → Attach file; drag a file onto the dashed zone, release. Seed/rig: X drag source | — | | | | | | | |
| DND-077 | Most specific consumer wins: a drop over the modal goes to the modal, not the composer/pane under it | web/src/features/file-system/lib/tauri-file-drop.ts:64 | With the Attach modal open over the composer, drop a file on the modal. Seed/rig: X drag source | — | | | | | | | |
| DND-078 | OS files → editor/chat pane: opens each path as an editor tab in that pane, in drop order | web/src/features/panes/hooks/use-pane-drop-handlers.ts:91 | Drag two files onto the chat pane outside the composer, release. Seed/rig: X drag source | — | | | | | | | |
| DND-079 | Pane file-drag ring and 10% tint while an OS file hovers a pane | web/src/features/panes/components/pane-container.tsx:217 | As DND-078, hold over the pane. Seed/rig: X drag source | transition-colors 150ms | | | | | | | |
| DND-080 | OS files → terminal: shell-quoted absolute paths typed at the prompt, terminal focused | web/src/features/terminal/hooks/use-terminal-file-drop.ts:11 | Open a terminal, drag a file whose name has a space onto it, release. Seed/rig: X drag source | — | | | | | | | |
| DND-081 | OS files → Files tree or sidebar: no consumer, nothing happens | web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx:53 | Drag a file onto the Files sidebar, release. Seed/rig: X drag source | — | | | | | | | |
| DND-082 | OS file drop onto the composer's text field: file's text not inserted as raw text | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:228 | Drag a .txt file onto the composer text, release; only an attachment appears. Seed/rig: X drag source | — | | | | | | | |

### Composer attachments

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-083 | Attachment controls (grip + trash) fade in on hovering an attachment in the draft | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:218 | Hover an attachment in the composer. Seed: a composer draft with two attachments (file card + image) | opacity 100ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| DND-084 | Attachment grip → above/below a sibling block: moves the attachment there (4px activation) | web/src/features/agent/chat/dnd-scope.tsx:25 | Drag the first attachment's grip below the second, release. Seed: draft with two attachments and a paragraph | — | | | | | | | |
| DND-085 | Attachment drop line: 2px brand/50 line on the top or bottom half of the hovered block | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:226 | Drag an attachment grip over a paragraph's top half, then bottom half. Seed: as DND-084 | — | | | | | | | |
| DND-086 | Dragged attachment stays in place at 50% opacity; no overlay ghost | web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx:119 | Drag a file-card attachment's grip, hold. Seed: as DND-084 | — | | | | | | | |
| DND-087 | Attachment into a list item/table cell (different parent): no line, no move | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:45 | Drag an attachment grip onto a bullet list item in the draft. Seed: draft with an attachment and a bullet list | — | | | | | | | |
| DND-088 | Image attachment and diagram/code attachment use the same grip and line | web/src/features/agent/composer/plate/attachments/chat-markdown-image-node.tsx:70 | Drag an image attachment's grip, then a diagram attachment's grip. Seed: draft with image and excalidraw/mermaid attachment | — | | | | | | | |
| DND-089 | Esc during an attachment drag cancels and clears the line | web/src/features/agent/chat/dnd-scope.tsx:53 | Drag an attachment grip, `rig key <app> Escape`. Seed: as DND-084 | — | | | | | | | |

### Settings

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-090 | Provider row grip → another slot: reorders which agent a new chat opens first | web/src/features/settings/components/tabs/providers-settings-view.tsx:142 | Settings → Agents; drag the first row's grip below the second, release. Seed: daemon reports ≥2 providers (installed or not) | dnd-kit sortable 250ms ease | | | | | | | |
| DND-091 | Provider row while dragging: 40% opacity, raised; switches stay clickable (listeners on grip only) | web/src/features/settings/components/tabs/sortable-provider-row.tsx:40 | Drag a provider grip, hold; separately click a row's switch. Seed: as DND-090 | — | | | | | | | |
| DND-092 | Provider reorder by keyboard: Tab to grip, Space, arrows, Space to drop; Esc cancels | web/src/features/settings/components/tabs/providers-settings-view.tsx:60 | Tab to "Reorder <provider>", `rig key <app> space Down space`; repeat with Escape. Seed: as DND-090 | dnd-kit sortable 250ms ease | | | | | | | |

### Text and window drags

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DND-093 | Selected text in Monaco → elsewhere in the same editor: moves the text (Monaco default dragAndDrop) | web/src/features/editor/lib/monaco-adapters.ts:13 | Open src/pricing.ts, select a word, drag it to another line, release | — | | | | | | | |
| DND-094 | Selected transcript text → composer: inserts the plain text at the drop caret | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:233 | In "Perf fixture", select a sentence, drag it into the composer, release | — | | | | | | | |
| DND-095 | Selected composer text → another spot in the composer: moves it | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:228 | Type two words in the composer, select the first, drag it after the second | — | | | | | | | |
| DND-096 | Dragged text → terminal: text typed at the prompt (insertFromDrop) | web/src/features/terminal/lib/mount-xterm.ts:81 | Open a terminal, select text in the editor, drag it onto the terminal, release | — | | | | | | | |
| DND-097 | Sidebar/tab row selection blocked during row drags; global grabbing cursor | web/src/index.css:167 | Drag a sidebar row across the transcript; no text gets selected, cursor is grabbing everywhere | — | | | | | | | |
| DND-098 | Repo avatars and tab pin icons are not natively draggable images | web/src/components/layout/repo-avatar.tsx:30 | Drag the "checkout" repo avatar; a row drag (not an image drag) starts | — | | | | | | | |
| DND-099 | Empty strip / chrome area drag moves the window (`data-tauri-drag-region`) | web/src/features/tabs/components/pane-top-row.tsx:61 | Drag from the empty area right of the chat tabs down 100px, release; window moves | — | | | | | | | |
| DND-100 | Sidebar project carousel pinned while a row drag nears its side edge (no project switch) | web/src/index.css:183 | Drag a sidebar row horizontally past the sidebar's edge and back. Seed: a second project | — | | | | | | | |

Skipped:
- `web/src/components/ui/table-node.tsx:247` row handle: select-only, the editor never registers a Plate dnd plugin.
- `web/src/features/console/components/console-resize-handle.tsx:32`, `components/layout/sidebar-split-pane.tsx`, `features/panes/components/pane-sash.tsx`: resizes via pointer capture, not content drags (owned by panes/sidebar areas).
- `web/src/features/agent/controls/agent-selection-picker.tsx:168` effort track: a slider scrub, not a drag of content.
- `web/src/features/agent/composer/agent-composer.tsx:170` and `attach-file-modal.tsx:59` DOM `drop` with `DataTransfer.files`: plain-browser fallback; Tauri's native file-drop takes OS drags first.
