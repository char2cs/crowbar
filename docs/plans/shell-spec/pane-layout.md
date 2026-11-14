# Shell spec: chat organisation within a tab (pane layout), part 1 of 2

Surface 4 of 4. Part 2 (`pane-layout-chrome.md`) covers pane headers, chat/editor halves, console dock, port list, persistence and live-capture checklist.

Scope: what is on screen below the tabs top bar, inside one tab. Source root is `app/panes/src` (abbreviated `src/`). Every line cites `file:symbol (~line)`. "VERIFY" marks a number that must be confirmed with `getComputedStyle` in the reference app; Tailwind classes are resolved at 1 rem = 16 px, `--spacing` = 4 px (`src/styles/theme-vars.css`), and `sm:` variants apply because the window is always wider than 640 px.

Colour tokens are named, not resolved here (the tokens file owns hex values). Values seen while reading, for orientation only: `--pane-background` #fcfcfa light / #1c1c1a dark (`styles/theme.css:11,171`), `--border` #dddcd6 / #333330 (`theme.css:51,210`), `--primary` = `--secondary` = #606c38 light / #98a86a dark (`theme.css:21-24,182-185`), `--radius` 0.625rem = 10 px (`theme.css:9`).

## 1. Vocabulary: what "tab" and "pane" mean in code

| Word | Code | Meaning |
|---|---|---|
| Tab (top bar) | `ViewRecord` (`features/panes/types/pane.ts:36`) | One entry of the tabs top bar. Holds a tiling tree of chat panes. Owned by another spec (tabs bar). |
| Stage | `ViewState.stage` (`lib/view-state.ts:22`) | The tree shown when no view is active: one chatless pane. Seeded id `root-pane` (`constants/pane.ts:5`). |
| Pane | `PaneGroup` (`types/pane.ts:1`) | A leaf. Holds at most one chat (`chatId`) plus an optional editor area (`editorTabIds`). |
| Split | `LayoutSplit` (`types/pane.ts:49`) | Binary node: `direction`, `sizes: [firstPct, secondPct]`, `first`, `second`. |
| Editor area | `PaneGroup.editorTabIds/activeEditorTabId/editorOpen` | The "second half" of a pane: files, terminals, browser, branch review, markdown/html preview, New Tab page. |

## 2. Data model

`LayoutNode = LayoutLeaf { type:'pane', id } | LayoutSplit { type:'split', id(nanoid), direction:'horizontal'|'vertical', sizes:[n,n], first, second }` (`types/pane.ts:44-58`).

- `horizontal` = children side by side (flex-row, first = left). `vertical` = stacked (flex-col, first = top). `pane-node-renderer.tsx` (~line 78: `flex-row` / `flex-col`).
- `sizes` are percentages summing to 100. Default `[50,50]` (`constants/pane.ts:3`). `normalizeSizes` (`utils/pane-layout.ts:~24`) rescales to 100 and clamps each side to at least `min(MIN_PANE_SIZE=10, 49)` percent.
- The tree is binary; there are no n-ary splits. Nested same-direction splits are real nodes (flattenForRender exists but is unused).
- `PaneGroup` fields (`types/pane.ts:1-26`): `id`, `chatId|null`, `runnerId|null`, `workspaceId?`, `draftParentId?`, `editorTabIds[]`, `activeEditorTabId|null`, `editorOpen:boolean`, `chatSelected?:boolean` (read as `!== false`), `locked?` (dead), `viewId|null`.
- `ViewState` (`lib/view-state.ts:15-27`): `panes{}`, `views{}`, `viewOrder[]`, `activeViewId|null`, `activeViewByProject{}`, `activeProjectId`, `stage`, `bottomLayout` (dead), `activePaneId`, `mostRecentActivePaneIds[]` (MRU), `fullscreenPaneId` (dead).
- `makePane` defaults: `chatId null, editorOpen false, chatSelected true, editorTabIds []` (`view-state.ts:35`).

### Integrity rules (`lib/view-integrity.ts`, repaired on load by `lib/view-repair.ts`)

1. A pane is a leaf of exactly one tree (a view, the stage, or the bottom tree). 2. A view holds at least one chat (a view that loses its last chat is removed). 3. A chat appears in at most one pane. 4. Stage and bottom panes hold no chat. 5. `viewOrder` has no duplicates, pinned views lead. 6. `activePaneId` is a leaf on screen. 7. `activeViewByProject[p]` names a view of project `p`.

## 3. Operations on the tree (the only structural writers: `lib/view-ops.ts`)

Every write is followed by `settleFocus` (`view-ops.ts:~7`, `view-state.ts:213`): if `activePaneId` is not on screen it becomes the most recent MRU pane on screen, else the first leaf.

| Operation | Rule | Source |
|---|---|---|
| Open chat (`openChat`) | Chat already in a pane: focus that pane (reveals its view). Else if the stage is showing: fill the target stage pane (active pane if on stage, else first leaf) and promote the stage to a new view. Else mint a new view with one pane, appended to `viewOrder`. Another project's chat is only remembered, not shown. | `stores/slices/pane-actions/view-actions.ts:openChat` |
| Promote stage | Stage layout becomes a `ViewRecord` (id = first leaf id unless taken); a fresh chatless stage pane replaces it. | `view-ops.ts:promoteStage` |
| Drop chat on pane (`dropChatOnPane`) | Centre on chatless pane: fill it. Centre on occupied pane: split to the RIGHT (`side = zone==='center' ? 'right' : zone`). left/right: `horizontal`; top/bottom: `vertical`; left/top: new pane `before`, right/bottom: `after`. Chat already open elsewhere: that pane is MOVED, never duplicated. Target pane in bottom tree: refused. | `pane-actions/chat-drop-actions.ts` |
| Split insert | `insertLeaf` replaces the target leaf with `createSplit(direction, first, second)` with `sizes [50,50]`; `before` puts the new leaf first. | `utils/pane-layout.ts:~59-90` |
| Close pane (`closePane`) | `closeLayout` removes the leaf and replaces its parent split by the sibling (the sibling keeps its own sizes; ancestors keep theirs). `normalizeLayout` re-clamps every split. Survivor = first leaf of the remaining layout; if the closed pane was active, survivor becomes active. Editor tabs of the closed pane move to the survivor (de-duplicated, `editorOpen` forced true; the active tab carries over if the closed pane was active). If the view has no chat left it is removed and the next view of the same project and workspace is shown (else the stage). Unsaved editor tabs of a last-pane close go to the stage pane. | `view-ops.ts:removePane, transferTabs, keepUnsavedTabs`; `view-actions.ts:closePane` |
| Detach pane | Only when the view has two or more chats: the pane becomes its own view placed right after the source view; if the source was showing the new view is shown. | `view-actions.ts:detachPane`, `view-ops.ts:movePane` |
| Close view | Removes every pane of the view; releases chats. | `view-actions.ts:closeView` |
| Resize split | `resizePaneSplit(splitId, sizes)` finds the split in stage, bottom or any view and calls `updateSplitSizes` (normalised). | `pane-actions/layout-actions.ts:resizePaneSplit` |
| Focus pane | Focusing a pane of another view shows that view (if the active project may show it); stage pane while a view is showing: shows the stage. Updates MRU. | `view-state.ts:focusPane` |
| Navigate | See 7. | `layout-actions.ts:navigateToPane` |

Last tab closed in a chatless pane: pane is removed only when a sibling exists (`paneCanSafelyLoseLastTab`, `editor-tab-actions.ts:30`). Closing the last editor tab sets `editorOpen=false` (`editor-tab-actions.ts:126`).

## 4. Geometry and chrome of the pane tree

### 4.1 Containment (outside in)

1. `IDEShell` wrapped by `ConsoleDock` (`components/layout/ide-shell.tsx:35`). Content column `relative z-[1] flex h-full min-w-0 flex-col bg-transparent` holds `ViewTabsRow` (tabs bar, other spec) then the workspace host (`components/layout/ide-shell-workspace-content.tsx:23`).
2. `WorkspaceLayoutRoot` = `div.min-h-0.flex-1` taking the rest below the tab strip (`features/workspace/components/workspace-layout-root.tsx:10`).
3. `SplitViewRoot` renders one `div.h-full.w-full` per mounted view; parked views `display:none` + `inert` (`panes/components/split-view-root.tsx:71-87`).
4. `PaneNodeRenderer` recursion. Split container `flex h-full w-full flex-row|flex-col`; each child wrapper `min-h-0 min-w-0 grow-0 shrink` with inline `flex-basis: <pct>%` (`pane-node-renderer.tsx:~80-95`). The sash is a flex sibling between them, so the two basis percentages overshoot by the sash size and `shrink:1` takes the 6 px out proportionally.
5. `PaneContainer` (`panes/components/pane-container.tsx`): outer `group/pane relative flex h-full w-full flex-col overflow-hidden` (`[data-pane-container]` forced `overflow: visible !important` by `index.css`), containing, in z order: file-drop tint (z-40), `SplitDropOverlay` (z-50), the bordered `data-pane-content` box (z-1), `PaneAccentRing` (z-2).

### 4.2 Pane content box (`utils/pane-border.ts:buildPaneContentStyle`)

Fill `bg-pane-chrome-bg`, `transition-colors duration-150` (`pane-container.tsx:~230`). Not a shadow: no box-shadow on a pane in the normal tree.

Per-edge values. `we(edge)` = "this edge touches the window/shield boundary":
- `we(top)` is always false. `we(left)` = `position.atLeft && !(sidebarOpen && sidebarPosition==='left')`. `we(right)` = `position.atRight && !(sidebarOpen && sidebarPosition==='right')`. `we(bottom)` = `position.atBottom`.
- Border: `1px solid var(--pane-border)` on each side where `!we`, else `none`. Top is always bordered.
- Radius `R = var(--radius-lg)` = 10 px (VERIFY): TL = we(left)?0:R; TR = we(right)?0:R; BL = (we(left)||we(bottom))?0:R; BR = (we(right)||we(bottom))?0:R.
- Margins: left = atLeft?0:1px; right = atRight?0:1px; bottom = we(bottom)?0:1px; top = `atTop ? (CHROME_ROW_HEIGHT 42 - 28)/2 = 7px : 1px`.
- `position` propagates down: horizontal split: first keeps atLeft, second keeps atRight, both keep atTop/atBottom; vertical split: first keeps atTop, second keeps atBottom (`pane-node-renderer.tsx:binaryPosition`). Root position: all true, except `atTop = !hasViewStrip` (strip exists whenever at least one tab is listed) and `atBottom = !isBottomPaneVisible` (always true, bottom pane is dead).
- Consequence in the default config (sidebar left open, strip present): a lone pane has a 1 px top and left border, TL radius 10 px, no right/bottom border, other corners square, margins top 1 px, others 0. Two panes side by side (sidebar left): first = left+top border, TL and TR radius 10 px, margin-right 1 px; second = margin-left 1 px, left+top border, TL radius 10 px, TR 0 (window right edge), no right border. Bottom corners are 0 on every bottom-row pane because `we(bottom)` is true. Inner gap between neighbours = 1 px margin + 1 px border + 6 px sash + 1 px border + 1 px margin = 10 px (VERIFY).
- Sidebar collapsed (sidebarOpen=false): `we(left)` true for the left-edge pane, so no left border and TL radius 0.
- Sidebar position `right`: mirrors (`sidebarPosition` setting, default `left`, `features/settings/config/default-settings.ts:18`).
- Border colour token `--pane-border`: default theme `var(--border)`; theme `crowbar` non-dark = `oklch(0.55 0 0)` (`styles/crowbar.css:22`). On Linux with theme crowbar, `--pane-chrome-bg` = `--pane-background` (opaque, `crowbar.css:40-42`); on macOS it is 95 % translucent. VERIFY which theme the reference runs.

### 4.3 Active-pane ring (`panes/components/pane-accent-ring.tsx`)

Separate childless absolutely-positioned overlay with the SAME margins, borders, radii as 4.2 but `border: var(--pane-accent-width) solid var(--pane-accent)`; `z-[2]`, `pointer-events-none`, `transition-opacity duration-150` (Tailwind default easing `cubic-bezier(0.4,0,0.2,1)`), opacity 1 when `showActiveBorder`. Width: 1 px default theme, 2 px crowbar light (`crowbar.css:21`); accent colour `--secondary` / `--brand`. Visible only when `isActivePane && visiblePaneCount > 1` (`pane-container.tsx:65`, `workspace/stores/hooks/use-pane-store.ts:useVisiblePaneCount`). A single pane never shows a ring. A pane of a parked view is never active.

### 4.4 Sash (`panes/components/pane-sash.tsx`)

| Property | Value |
|---|---|
| Size | `w-1.5` (6 px) wide for horizontal splits, `h-1.5` (6 px) tall for vertical; hit area equals the visible bar (no extension) |
| Element | `div role="separator"`, `relative z-10 flex shrink-0 items-center justify-center`, `data-slot="resizable-handle"` |
| Idle | transparent (no paint) |
| Hover | `bg-border/60`, `transition-colors` (150 ms default ease) |
| Cursor | `col-resize` / `row-resize` |
| Pointer | primary button only (`e.button !== 0` ignored); `preventDefault` on down; window-level `pointermove`/`pointerup`/`pointercancel` |
| Drag | first real move sets `<html data-pane-resizing="1">`; each move sets BOTH siblings' `flex-basis` in px: `pointerPx = clientX - containerLeft`; `first = clamp(pointerPx, min, containerPx - min)`; second = container - first. `containerPx` = container size minus the sash's own size |
| Min size | grid sash default `minPx = MIN_PANE_SIZE = 10` px (the same constant is also a 10 percent floor, see below); chat/editor sash 340 px side-by-side, 160 px stacked |
| Too small | if `containerPx < 2*min` the divider centres |
| Release | converts to percentages (`pxToPercents`) and calls `onResizeCommit`; for the grid this writes `sizes` to the store, which normalises and clamps to at least 10 percent per side (so a drag allowed down to 10 px snaps to 10 percent on release); inline basis restored to React's value |
| Bare click | total no-op (no attribute, no event, no commit) |
| During drag | `index.css` removes pane radius and shadow, `backdrop-filter` of the edge dissolves and masks (perf only; Rust may ignore) |
| Not present | double-click reset (`distributePaneSplit` exists but nothing calls it), keyboard resize, snap points, ARIA value, tab stop |
| Source | `pane-sash.tsx`, `lib/split-sizing.ts`, `constants/pane.ts` |

Persistence of grid ratios: saved with the layout (section 9 of part 2). Ratios of the chat/editor sash are NOT persisted (section 5 of part 2).

## 5. Drag and drop

### 5.1 Zones (`utils/pane-drop-zones.ts:getPaneDropZoneFromRect`)

Normalised point `nx, ny` in the pane container rect (the whole `[data-pane-id]` element including header). Threshold 0.25. Test order:
1. `left` if `nx < .25 && nx < ny && nx < 1-ny`
2. `right` if `nx > .75 && 1-nx < ny && 1-nx < 1-ny`
3. `top` if `ny < .25`
4. `bottom` if `ny > .75`
5. else `center`.
Left/right win the corners inside their triangular regions; top/bottom own the rest of the top and bottom bands. `getPaneSplitDropOptions`: left/right -> `horizontal`; top/bottom -> `vertical`; left/top -> `before`; right/bottom -> `after`; centre -> null.

### 5.2 Preview overlay (`panes/components/split-drop-overlay.tsx`)

Rendered while a sidebar row or file is dragged over a pane (`internalHoverZone`). Container `absolute inset-0 z-50`, `pointer-events-none` when driven by the drag store. The highlighted rectangle:

| Zone | Class | Rect inside pane (px offsets) |
|---|---|---|
| left | `right-1/2 inset-y-1 left-1` | x 4 .. 50 percent, y 4 .. h-4 |
| right | `left-1/2 inset-y-1 right-1` | x 50 percent .. w-4 |
| top | `bottom-1/2 inset-x-1 top-1` | y 4 .. 50 percent |
| bottom | `top-1/2 inset-x-1 bottom-1` | y 50 percent .. h-4 |
| center | `inset-1` | whole pane inset 4 px |

Common: `rounded-lg` (10 px), `border-2 border-secondary`, `bg-secondary/14` (14 percent), `shadow-[0_0_0_1px color-mix(in oklch, var(--color-secondary) 25%, transparent)]`, `transition-all duration-100` (default easing). Because the rectangle changes position between zones the 100 ms transition animates it moving/resizing from zone to zone. First appearance has no fade-in (the element mounts).

Extra pane feedback: when a sidebar-row drag hovers a pane without an overlay zone, the pane element gets `data-pane-hit` and `ring-2 ring-secondary` (`pane-container.tsx:~208`, `components/sidebar/lib/sidebar-drag-hover.ts:paintPaneHit`). An OS or explorer file drag shows `ring-2 ring-secondary` plus a `bg-secondary/10 z-40` tint (`pane-container.tsx:~202-219`).

### 5.3 What each drag does

| Drag | Hit test | Effect on drop | Source |
|---|---|---|---|
| Sidebar chat/branch row onto a pane | `elementsFromPoint`, nearest `[data-pane-drop]`, zone as 5.1 | `dropChatOnPane(chat, pane, zone)` per section 3. A chat from another project is refused with toast "That chat belongs to a different space". | `components/sidebar/lib/pane-drop-actions.ts:performSidebarPaneDrop`, `sidebar-drag-hover.ts:paneZone` |
| Sidebar row onto a tab in the top bar | tabs spec | merge into view / reorder | `pane-drop-actions.ts:performRecentsDrop` |
| Pane handle grip dragged out | same sidebar drag engine, strip target | detach/reorder (`performSidebarDrop`) | `panes/components/pane-handle.tsx` |
| Explorer file | pointer drag, `resolveDropTarget` (tab bar -> centre, pane -> zone) | opens the file in that pane; zone ignored | `features/file-explorer/.../use-file-explorer-drag-drop.ts:217` |
| OS file | Tauri file drop on pane | `openDocumentsAtPaths(paths, paneId)` | `hooks/use-pane-drop-handlers.ts:91`, `features/file-system/lib/tauri-file-drop` |
| Editor tab within its own pane | HTML5 `application/tab-data` | Only re-activates/re-adds in the SAME pane. A tab never crosses panes or creates a split. Overlay still draws during the drag. | `use-pane-drop-handlers.ts:62-81` |

Do not port: tab-drag-to-edge splitting (`getOrCreatePaneDropTarget`, `createPaneBeside`, `splitPane` are only reached with zone centre, so they never split).

### 5.4 Pane handle (`panes/components/pane-handle.tsx`)

Shown only when the pane has a chat AND its view has 2 or more chats; visible only on pane hover (`group-hover/pane`) or focus-within.
- Position: `absolute left-1/2 top-0 z-30 -translate-x-1/2`, inside the pane content box.
- Box: `flex items-center gap-[9px] rounded-b-md px-[7.5px] pb-[4px] pt-[5px] bg-primary text-primary-foreground shadow-[0_0_5px_1px_rgba(0,0,0,0.1)]`. Computed 69 x 21 px (3 buttons of 12 px, two 9 px gaps, 15 px horizontal padding). `rounded-b-md` = `--radius-md` 8 px (VERIFY).
- Buttons `size-3` (12 px), ghost, no border: (1) grip `DotsSixVerticalIcon` size 10 rotated 90, `cursor-grab`, tooltip "Drag this chat" (starts the sidebar drag engine with a drag ghost `ViewTabPreview`); (2) `ArrowsOutSimpleIcon` 10, tooltip "Open in its own tab" -> `detachPane`; (3) `XIcon` 8, tooltip "Close this chat" -> `closeRecentChat` -> `closePane`.
- Animation: `transition-opacity` (150 ms default), opacity 0 -> 1, `pointer-events-none` until revealed.

## 6. Focus and activation

- `activePaneId` single value. Any `mousedown` (capture, native listener) inside a pane activates it unless the target ITSELF (not an ancestor) matches `button, input, textarea, [role=button], [role=menu]`; Monaco `.inputarea` and xterm `.xterm-helper-textarea` do activate (`hooks/use-pane-activation.ts`). Keyboard-triggered click also activates (`pane-container.tsx:handlePaneClick`).
- MRU list `mostRecentActivePaneIds` drives fallbacks (view re-entry, focus after close).
- Active visual = accent ring (4.3) only when more than one pane is visible.
- Unfocused panes are not dimmed.

## 7. Keyboard (all resolved from the keymap registry, `features/keymaps/registry.ts`; `mod` = Cmd on macOS, Ctrl on Linux; handler `features/panes/hooks/use-pane-keyboard.ts`, window `keydown`, bubble phase)

| Command id | Default chord | Behaviour |
|---|---|---|
| `panes.navigateLeft/Right/Up/Down` | `mod+alt+arrow*` | `navigateToPane(dir)`: `getAdjacentLeafId` on the showing layout, else the bottom tree. Quirk: it scans from the ROOT split outward; at the first split whose direction matches and that contains the active pane on the right side, it jumps to the FIRST LEAF of the other child (not the geometrically nearest). No wrap. No match: nothing. MRU touched. (`utils/pane-layout.ts:~147-175`; XFCE steals Ctrl+Alt+Arrow in the reference desktop) |
| `tabs.new` | `mod+t` | New Tab page in active pane (`openNewTabPage`); needs the pane's workspace; chatless pane first receives the workspace's owning chat |
| `agent.newChat` | `mod+n` | creates chat on first enabled provider, opens it as its own view; over a focused editor area it means New Tab instead |
| `agent.newChatTerminal` | `mod+alt+n` | same, landing on Terminal surface |
| `tabs.newTerminal` | `mod+j` | terminal tab in active pane's editor area |
| `tabs.newFile` | `mod+shift+n` | untitled file |
| `tabs.openFile` | `mod+o` | native file dialog (Tauri only) |
| `tabs.reopenClosed` | `mod+shift+t` | reopen last closed buffer |
| `tabs.closeActive` | `mod+w` | no editor tabs: close pane if its tree has 2 or more panes (else no-op); dirty editor tab: confirm dialog; else close tab (neighbour at the same index, else last, becomes active) |
| `agent.toggleViewMode` | `mod+/` | flips chat/terminal surface of the focused chat (`agent-chat-pane.tsx`) |
| `agent.zoomIn/Out/Reset` | `mod+=`, `mod+-`, `mod+0` | chat zoom |
| `navigation.toggleConsole` | ``mod+` `` | console dock; capture phase, `e.repeat` ignored; macOS menu item owns it |
| `browser.togglePick` | `mod+shift+e` | element picker |
| `editor.save/saveAll` | `mod+s`, `mod+shift+s` | display-only in the keybinding list |

Escape closes the console only (`console-panel.tsx`).
