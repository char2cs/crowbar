# Shell spec: chat organisation within a tab, part 2 of 2

Part 1 (`pane-layout.md`) has the model, operations, geometry, sash, drag and drop, keyboard. Conventions are the same (paths relative to `app/panes/src`, VERIFY = confirm with `getComputedStyle`).

## 8. Inside one pane: chat half and editor half

### 8.1 Presentation state machine (`panes/hooks/use-pane-presentation.ts`, `agent/hooks/use-chat-presentation.ts:134-158`)

Inputs: `pane.chatId`, `pane.editorOpen`, `pane.editorTabIds.length`, `pane.chatSelected`, the measured size of the views container (`ResizeObserver`, `clientWidth/clientHeight`, only while `editorOpen`).

`presentation = editorOpen ? derive(w,h) : 'tabs'`, with `derive`:
- `w==0 && h==0` -> `side-by-side` (unmeasured guess, avoids a pop)
- `max(w,h) < 780` -> `tabs`
- else `w >= h` -> `side-by-side`, `w < h` -> `stacked`.

Constants (`use-chat-presentation.ts:10-13`): `SPLIT_SIDE_BY_SIDE_MIN_PX 780`, `SPLIT_MIN_HALF_PX 340`, `SPLIT_MIN_STACKED_PX 160`, `SPLIT_DEFAULT_SIZES [45,55]` = [chat%, editor%].

Derived booleans (hasChat = `chatId !== null`):

| Name | Formula |
|---|---|
| `chatFillsPane` | hasChat and no editor tabs and not editorOpen |
| `showChatTab` | hasChat and not chatFillsPane and presentation==='tabs' |
| `chatVisibleAlongsideEditor` | hasChat and not chatFillsPane and presentation !== 'tabs' |
| `showTopLevelHeader` | not hasChat, or chatFillsPane, or presentation==='tabs' |
| `editorViewHidden` | hasChat and (chatFillsPane or (tabs and (no chat tab or chatSelected!==false))) |
| `chatViewHidden` | showChatTab and chatSelected === false |
| `isStacked` | hasChat and stacked and not chatFillsPane |
| `chatIsFirst` | stacked, or sidebarPosition !== 'right' |
| `editorFacingChatEdge` | stacked: `top`; else chatIsFirst: `left`, otherwise `right` |

Resulting layouts (all with both views always mounted, `hidden` attribute toggled; a terminal or webview never remounts, `pane-container.tsx:148-150`):

| State | Condition | Screen |
|---|---|---|
| A. Chat only | chatFillsPane | One header row (chat header with "Open editor" button), chat fills pane |
| B. Side by side | editorOpen and w >= 780 and w >= h | Chat column (basis 45 percent) + 6 px sash + editor column (55 percent); each has its own 42 px row; chat first unless sidebar is on the right |
| C. Stacked | editorOpen and h > w and max >= 780 | Chat on top (always, even sidebar right), editor below |
| D. Tabs (collapsed) | editorOpen false with tabs, or container smaller than 780 on both axes | One `TabBar` row on top; the chat is the first tab ("chat tab"); content area shows the chat or the active editor tab. `pane.chatSelected` chooses (default chat) |
| E. Empty stage / chatless pane | no chat | `TabBar` row (split toggle hidden) + editor area showing the New Tab page |

State transitions: "Open editor"/split-toggle flips `editorOpen` (`pane-command-actions.ts:togglePaneEditor`); opening any tab into a chat-only pane sets `editorOpen=true` (`editor-tab-actions.ts:placeTab`, unless the user already toggled off a pane that had tabs); closing the last tab sets it false; opening any tab sets `chatSelected=false`; the chat tab or `activateChatInPane` sets it true. Resizing the container across 780 px flips side-by-side/stacked/tabs live with no animation.

### 8.2 Chat/editor sash and boxes

- Sash: part 1 section 4.4 with `minPx` 340 (side-by-side) / 160 (stacked); `sizes` = `splitSizes` kept in React `useState` of `PaneContainer` (NOT persisted; resets to [45,55] on remount or restart). Commit swaps order when chat is second (`pane-container.tsx:134-146`).
- Chat view: `data-chat-view`, no fill of its own (the pane box paints it), `relative flex min-h-0 min-w-0 flex-col overflow-hidden`; `flex-basis` = chat percent (`pane-chat-view.tsx`).
- Editor view: `data-editor-view`, `relative flex min-h-0 flex-col overflow-hidden bg-pane-background` (opaque, painted over the translucent box), `flex-basis` = editor percent; border only on the seam facing the chat: `1px solid var(--border)`, no radius (`pane-border.ts:buildInnerViewStyle`).
- Beside the chat, the editor view gets its own `TabBar` row (42 px); the chat gets `ChatColumnHeader`.

### 8.3 Row anatomy (the 42 px chrome row)

`CHROME_ROW_HEIGHT_PX = TAB_HEIGHT 32 + 2 * TAB_GAP 5 = 42` (`lib/chrome-metrics.ts`). Shared by the tabs bar, pane rows and the sidebar header.

`PaneTopRow` (`features/tabs/components/pane-top-row.tsx`): `flex shrink-0 items-center gap-1.5 px-2 py-1` -> gap 6 px, horizontal padding 8 px, vertical 4 px, height 42 px inline; `data-tauri-drag-region` (drag the window by empty area). macOS only: `pl-[88px]` when the row is at the window's top-left (traffic lights). Linux: no inset.

| Variant | Fill | Position |
|---|---|---|
| `opaque` | `bg-pane-background`, `overflow-hidden`, in flow | editor-half row, tabs-mode row when an editor tab is selected |
| `chat-blur` + `overlay` | none; `absolute inset-x-0 top-0 z-10`; contains `EdgeDissolve edge="top" height=98 (42+56) -z-10` | chat header (state A and B/C chat half); chat content scrolls under it |

`EdgeDissolve` (`components/ui/edge-dissolve.tsx`): 7 stacked layers, each `backdrop-filter: blur(Npx)` with a vertical linear-gradient mask. Table (blur px: mask stops as fraction of 98 px zone): 1:[0,.1,.3,.4]; 2:[.1,.2,.4,.5]; 4:[.15,.3,.5,.6]; 8:[.2,.4,.6,.7]; 16:[.4,.6,.8,.9]; 32:[.6,.8]; 64:[.7,1]. 4-stop layers are transparent-black-black-transparent, 2-stop are transparent-black; the whole stack is flipped (`scaleY(-1)`) so it blurs from the top edge. Layer `top = max(0, h*firstStop - blur)`. `contain: paint`, no pointer events. Native equivalent: a progressive blur or an approximating gradient veil; flag for visual diff.

Row children by state:

| Row | Children, left to right |
|---|---|
| Chat-only header (`ChatOnlyPaneHeader`) | `ChatBranchHeader` (flex-1) , `OpenEditorButton` (only if the pane's workspace resolved), `ChatHeaderSlot` |
| Chat column header (`ChatColumnHeader`) | `ChatBranchHeader`, `ChatHeaderSlot` |
| Pane tab row (`TabBar`) | `SplitToggleButton` (hidden for an empty pane), sidebar-reopen toggle (only when sidebar collapsed and this row is the window's top corner on the sidebar side AND there is no tab strip; with a tab strip the tabs bar owns it), scroller [chat tab (tabs state only), editor tabs, `+` button], `TabOverflowMenu`, `BranchReviewShortcutButton` (if branch review possible) |

Scroller (`tab-bar.tsx:379-387`): `tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden`, hidden scrollbar, wheel converted to horizontal scroll, active tab scrolled into view with `scrollIntoView({behavior:'smooth', inline:'center'})`. Tab sortable with dnd-kit `distance: 6`, `closestCenter`, `horizontalListSortingStrategy`; pinned tabs sort first. A pane with only a chat draws no scroller at all. Tab visuals (`TabBarItem`, 32 px height, close button, context menu) belong to the tabs spec but the pane-level tab strip reuses them.

Buttons: `Button variant=ghost size=icon-sm` = `size-8 sm:size-7` = 28 px, `rounded-sm` (`--radius-sm` = 6 px), `text-muted-foreground`, hover `bg-sidebar-element-hover` (`oklch(0 0 0 / 8%)` light, `oklch(1 0 0 / 10%)` dark, `theme.css:68,227`), tooltips on bottom. Icons: `CodeIcon` (Open editor, tooltip "Open editor"), `SquareSplitHorizontal` (split toggle; tooltip "Show chat only" / "Show editor alongside chat"; `aria-pressed`, active colour `text-foreground`), `PlusIcon` ("New tab"), `GitPullRequest` ("Review this branch"). Default icon size inside buttons is 16 px (`[&_svg]:size-4`, VERIFY for icon-sm).

`ChatBranchHeader` (`tabs/components/chat-branch-header-view.tsx`): `flex items-center gap-2 text-[13px]`; lead mark (repo or project icon `size xl`, else a 20 px (`size-5`) glyph with 18 px icon: working spinner `FlickerSpinner` / `ChatsCircle` for threads / `GitBranch`); two-line column: title `CHAT_TITLE_TEXT` = `font-heading text-[17px] leading-5 font-normal` truncated; second line `ROW_SUBLABEL` with 13 px glyph and `repo / parent / branch` (leading segments shrink first, `shrink-[1000]`) plus `+added -deleted` counts. Double-click title: inline rename (`InlineRenameInput`). The lead/route model comes from `use-chat-branch-header-model.ts`.

`ChatHeaderSlot`: `flex shrink-0 items-center gap-2`; the chat surface injects its controls here by portal: the `ViewSwitch` (chat/terminal) and `ProviderSwitchDropdown` (`agent/hooks/chat-header-slot.tsx`, `agent/terminal/agent-terminal-surface.tsx:94-110`).

### 8.4 Chat / terminal switch (view mode)

Per-chat surface: `'chat' | 'terminal'` (`'split'` exists only in dev builds, do not port). Switch component `ViewSwitch` (`agent/controls/view-switch.tsx`): Base UI Tabs with a `SegmentedControl`:
- container `relative z-0 flex w-fit items-center gap-0.5 rounded-lg(10px, overridden `rounded-full`) bg-muted p-0.5` (2 px padding)
- item `h-7 w-10 rounded-full px-0` = 28 x 40 px, icons `ChatCircleIcon` / `TerminalWindowIcon` size 18; inactive `text-muted-foreground/72`, hover `text-muted-foreground`, active `text-foreground`
- sliding indicator: `rounded-md bg-background shadow-sm/5 (dark: bg-input) transition-[width,translate] duration-200 ease-in-out` (no motion with reduced motion), placed from Base UI CSS vars `--active-tab-left/width/height/bottom` (`components/ui/segmented-control.tsx:33`; indicator radius VERIFY, container is `rounded-full` but indicator class is `rounded-md`).
- Arrow keys move between segments and activate (`activateOnFocus`); disabled while a provider without hot-swap is mid-turn (`handoverBlocked`, opacity 64 percent).
- Chord `mod+/`. Persistence: choice remembered per chat id only in memory for the session (`lastChosenByChatId`); the initial value comes from the daemon's recorded surface for that chat, else the setting `chatIsDefaultPresentation` ('chat' unless false) (`use-chat-presentation.ts:46-90`, `settings/lib/chat-presentation.ts:13`). On the terminal surface the chat DOM stays mounted but `hidden`.
- Chat surface layout in the pane: column `mx-auto flex w-full flex-1 flex-col`; chat: `max-w-none`, left padding 16 px (`pl-4`); terminal: `max-w-4xl` (896 px) and `px-4 pt-4` (`agent-chat-pane.tsx:~317-325`).

### 8.5 Editor-area content types (`types/pane-content.ts:86-105`, `panes/components/pane-editor-view-content.tsx`)

| Type | Reachable | How | Notes |
|---|---|---|---|
| `newTabPage` | yes | `+`, `mod+t`, empty editor area | at most one per pane; replaced by choice (`replaceTabId`) |
| `terminal` | yes | New tab page, `mod+j`, "New thread in Terminal" | one PTY per tab; never mounted in two panes; kept mounted when not active (`visibility:hidden`) |
| `browser` | yes | New tab page | native webview, hidden when not active, when parked, or while a drop overlay is up (`dropTargeted`) |
| `editor` | yes | New file, open file, explorer click | Monaco; portaled from `EditorHostRegistry` (REDESIGN) |
| `branchReview` | yes | New tab page "Review Branch" (branch workspace only), git panel, pane toolbar | has `conversation|files` view |
| `markdownPreview`/`htmlPreview` | by code (editor toolbar for .md/.html) | not driven in the inventory | |
| `commitDiff` | no | removed from the type; inventory lists it as dead | do not port |

New tab page (`panes/components/new-tab-view.tsx`): centred column `gap-6` (24 px): `CrowbarWordmark` width `clamp(96px, 14cqmin, 148px)` muted, then a column `gap-0.5` (2 px) of left-aligned ghost `size=sm` (28 px high) buttons: "New Terminal" (`TerminalWindowIcon`), "New Browser" (`GlobeIcon`), "New File" (`FilePlusIcon`), "Review Branch" (`GitPullRequest`, only when `usePaneCanReviewBranch`). Buttons appear only once the pane's workspace id resolves; until then only the logo. Focusable (`tabIndex=-1`) so `mod+n` there means New Tab.

Limits: open-tab cap `maxOpenTabs` (setting, engine max `EDITOR_CONSTANTS.MAX_OPEN_TABS`), counted per workspace; evicts the first non-pinned, non-dirty, non-protected buffer (`buffer-slice.ts:~137-160`).

### 8.6 Error and loading states

- Pane crash: `PaneBoundary` shows centred column `p-8 gap-3`: red 14 px title "This pane encountered an error", optional message in `<pre>` (`bg-muted text-xs rounded-md px-3 py-2 max-w-sm`), "Retry" button (`bg-secondary text-secondary-foreground rounded-md px-3 py-1.5 text-xs`) (`pane-boundary.tsx`). Resets when `paneId` changes.
- Chat surface loading: lazy chunk, `Suspense fallback={null}` (blank). Pending surfaces gate mounting of parked views.
- Parked-view mounting order: showing view at once; the others one at a time on idle, most recently used first, only after pending chat surfaces settle; once mounted a view stays mounted (`hooks/use-mounted-views.ts`). The native shell may choose a different parking strategy (see pane host plan) but must preserve terminal scrollback, editor undo and browser page across tab switches.

## 9. Console dock (the one non-pane chrome that wraps the whole shell)

Component tree: `ConsoleDock` wraps the entire IDE shell (sidebar included) so the console can cover or shrink the window (`features/console/components/console-dock.tsx`, mounted `components/layout/ide-shell.tsx:35`). Not part of the pane tree, not in the layout persistence.

| Item | Value | Source |
|---|---|---|
| Open/close chord | ``mod+` `` (capture phase, repeat ignored; macOS menu item) | `keymaps/hooks/use-console-keyboard.ts` |
| Close | Escape while focus is inside; overlay mode: any pointerdown outside (capture; the click still reaches its target); push mode only Escape or the version badge toggle (`aria-controls="console-panel"`) | `console-panel.tsx:36-65` |
| Docks | `top` (default), `right`, `bottom`, `left`; header button cycles top -> right -> bottom -> left | `lib/dock-prefs.ts:18` |
| Mode | `overlay` (default) or `push` | `dock-prefs.ts:33` |
| Size | default 420 px, min 160, max 80 percent of the window axis (`min(var(--console-size), 80dvh|80dvw)`), measured along the axis facing the app (height for top/bottom, width for left/right) | `dock-prefs.ts:13-26`, `console-panel.tsx:87-88` |
| Panel | `section#console-panel`, `z-[45]`, `bg-code text-code-foreground border-border`, border only on the inner edge (`border-b` for top, `border-t` bottom, `border-r` left, `border-l` right), `flex flex-col overflow-hidden`; `inert` and `invisible` when closed | `console-panel.tsx:70-93` |
| Overlay animation | `absolute shadow-lg`, `transition-[transform,visibility] duration-200 ease-out`; closed = translated out by 100 percent of its own size toward its dock edge (`-translate-y-full` top, `translate-y-full` bottom, `-translate-x-full` left, `translate-x-full` right); open = `translate-0` | `console-panel.tsx:9-21,84,91` |
| Push animation | `relative shrink-0`, `transition-[width,height] duration-200 ease-out`; closed = `h-0|w-0 border-0` (the app column is `flex-1`, so the window content animates its size) | `console-panel.tsx:85,92` |
| Resize | `data-resizing` removes the transition during drag; only the CSS variable moves while dragging, store and storage see the value on release | `console-resize-handle.tsx` |
| Resize handle | 6 px (`h-1.5` / `w-1.5`) strip centred on the inner edge (`translate-y-1/2` etc.), `z-10`, hover and focus-visible `bg-ring/40`, cursors `row-resize|col-resize`, `touch-none`, `tabIndex=0`, `role=separator`; arrow keys step 16 px (outward grows); double-click resets 420 | same |
| Header | `header h-9` (36 px) `border-b px-2 gap-1`, buttons right-aligned: dock position (icons `AlignTopSimple|AlignRightSimple|AlignBottomSimple|AlignLeftSimple`), mode (`StackIcon` overlay / `SquareHalfIcon` push; `aria-pressed` = push); both `icon-xs` (24 px, icon 14 px); macOS adds `pl-20` when top or left; `data-tauri-drag-region` | `console-header.tsx` |
| Log pane | `role=log`, `flex-1 overflow-y-auto py-2`, focus ring `ring-2 ring-ring inset`; focuses on open; virtualised (estimate 20 px, overscan 15); follows newest line until a user scroll (wheel, touch, key, scrollbar press within 250 ms) moves it more than 32 px above bottom; ResizeObserver keeps it pinned | `console-log.tsx` |
| Log row | button grid `88px 44px 1fr`, `gap-x-2.5` (10 px), `px-4` (16 px), `py-[3px]`, `font-mono text-xs leading-[18px]` (12/18); time `text-muted-foreground`; level uppercase `font-semibold tracking-[0.04em]` coloured debug muted / info `text-info` / warn `text-warning` / error `text-destructive`; message tinted only for warn and error; fields `key=value` with `ml-2.5` (10 px) gaps, value colour by kind (text, number and duration `text-info`, bool `text-success`, path underlined, error destructive); hover and expanded `bg-muted`; click expands a JSON block `bg-muted py-1.5 pr-4 pl-[168px]` with 16 px indented keys | `log-row.tsx` |
| Notes | raw text and "Daemon restarted": `text-muted-foreground px-4 pl-[114px] font-mono text-xs leading-[18px]` | `entry-row.tsx` |
| Data | WebSocket log stream from the daemon, replay held until `ready`, frames batched every 50 ms, buffer cap 5000 lines, cursor resume, `reset` clears and inserts a restart note | `lib/controller.ts`, `stores/console-store.ts:15` |
| Persisted (localStorage) | keys `console-dock`, `console-mode`, `console-size` | `dock-prefs.ts:19-21` |

## 10. Animation table (everything in the layout surface that animates)

| Element | Property | Duration | Easing | Trigger | Source |
|---|---|---|---|---|---|
| Accent ring | opacity 0<->1 | 150 ms | cubic-bezier(0.4,0,0.2,1) | pane gains/loses focus with 2 or more panes | `pane-accent-ring.tsx` |
| Pane content box | background colour | 150 ms | same | theme/focus fill change | `pane-container.tsx:230` |
| Sash hover | background colour | 150 ms | same | pointer enter/leave | `pane-sash.tsx` |
| Pane handle | opacity | 150 ms | same | pane hover/focus-within | `pane-handle.tsx` |
| Drop overlay rectangle | all (position, size) | 100 ms | same | zone changes while dragging | `split-drop-overlay.tsx` |
| View switch indicator | width, translate | 200 ms | ease-in-out | selection change | `segmented-control.tsx:33` |
| View switch text | colour, outline colour | 150 ms | same | hover/active/focus | `segmented-control.tsx:62` |
| Console (overlay) | transform, visibility | 200 ms | ease-out (cubic-bezier(0,0,0.2,1)) | toggle | `console-panel.tsx` |
| Console (push) | width/height | 200 ms | ease-out | toggle | `console-panel.tsx` |
| Console resize | none (transition removed) | n/a | n/a | dragging | `console-panel.tsx:82` |
| Tab scroll into view | scroll | browser default smooth | n/a | active tab change | `tab-bar.tsx:246` |
| Chat/editor split, sash drag, presentation switch | none | n/a | n/a | n/a | n/a |
| Reduced motion | all transitions set to 0.01 ms | | | `prefers-reduced-motion` | `index.css` |

Pane open, close, split and view switch do not animate at all (parked view toggles `display:none`).

## 11. Edge cases to reproduce

1. `normalizeSizes` clamps to a 10 percent minimum even though the live drag floor is 10 px (snap on release).
2. Closing the active pane makes the FIRST leaf of the remaining tree active, not the neighbour.
3. A chat dropped centre on an occupied pane splits RIGHT, never replaces.
4. A chat cannot be in two panes; dropping an open chat moves its pane.
5. A drop across projects is refused with a toast.
6. Pane navigation uses first-leaf jumps from the outermost matching split.
7. `mod+w` in the last pane of a tree with no tabs does nothing; with a dirty editor tab shows the unsaved dialog.
8. A terminal is never opened in a second pane: reopening focuses the existing pane.
9. `activePaneId` is always on screen after any write (`settleFocus`).
10. Hidden views keep sizes; ratio changes made while hidden persist.
11. Legacy saved layouts may carry a `newTab` placeholder (not `newTabPage`) and unknown content types: dropped on load (`persisted-layout.ts:validateLoadedBuffers`). `chatSelected` missing means true.
12. A workspace slot hidden by retention is `display:none` + `inert`; the single window pane tree is a sibling of the slots (`workspace-host.tsx:229`).
13. Restore keeps the largest valid part of a saved layout and drops violating panes, then reseeds stage if empty (`view-repair.ts`).

## 12. Persistence and ownership

### 12.1 What is persisted, where, today

| State | Store | Key / format | Write |
|---|---|---|---|
| Layout: `panes`, `views`, `viewOrder`, `activeViewId`, `activeViewByProject`, `stage`, `bottomLayout`, `activePaneId`, `mostRecentActivePaneIds`, `buffers` (transient fields stripped) | IndexedDB (webview-local) | object store `workspace-layout`, single row id `window`, `version: 4` (`lib/persistence/workspace-layout.ts`) | debounced 300 ms after any change of those fields (`stores/window-pane-store.ts:~205-240`) |
| Editor drafts (unsaved text) | IndexedDB `editor-drafts` | per document uri | on edit |
| Console dock/mode/size | localStorage | `console-dock`, `console-mode`, `console-size` | on change |
| Sidebar width etc. | other specs | | |
| Chat/editor split ratio, view-mode pick (chat vs terminal) | memory only | `splitSizes` state, `lastChosenByChatId` | lost on restart |
| Open flags | `editorOpen`, `chatSelected` | inside each pane in the layout row | with layout |

### 12.2 Daemon owns vs shell owns (target split for the native shell)

| Daemon (source of truth, reachable through the API) | Shell (view state, persist locally) |
|---|---|
| Projects, repos, workspaces, chats and titles, working state, provider and model, per-chat recorded surface (chat or terminal), terminal PTY sessions and their screen models, documents on disk, git state and review threads, daemon log stream | The layout tree (`panes`, `views`, `viewOrder`, `stage`), split ratios, active view/pane, MRU, `editorOpen`/`chatSelected`, tab order and pins of editor tabs, open buffer descriptors, console dock prefs, window geometry, chat zoom, per-session view-mode pick, drag state, hover/focus |
| `workspaceId` ownership of a chat can change (branch-out and moves); panes only store a hint and resolve through the chat entity (`lib/pane-chat-workspace-id.ts`) | Must re-resolve workspace ids after restart (`placeRestoredMembers`: chats gone from the daemon leave the layout, their unsaved buffers stay) |

Existing integrity and repair logic (`view-integrity.ts`, `view-repair.ts`) is non-visual behaviour to keep.

## 13. Port list

### 13.1 Reachable, must be ported (shell chrome)

`panes/components/`: `split-view-root` (parked views only; not fullscreen), `pane-node-renderer`, `pane-container`, `pane-sash`, `pane-accent-ring`, `pane-handle`, `split-drop-overlay`, `pane-boundary`, `new-tab-view`, `pane-editor-view-content` (host of tabs; content types become panes), `pane-chat-view` (host of the chat webview), `pane-content-registry` (host of web content), `terminal-pane`.
`panes/utils`: `pane-layout` (all but `flattenForRender`), `pane-border`, `pane-drop-zones`, `pane-routing`, `pane-command-actions` (`openBranchReviewForWorkspace`, `ensurePaneChatThenOpen`, `openNewTabPage`, `togglePaneEditor`), `persisted-layout`.
`panes/lib`: `view-state`, `view-ops`, `view-repair`, `view-integrity`, `view-pins`, `view-selectors`, `split-sizing`, `pane-chat-workspace(-id)`, `buffer-release`, `release-closed-chat`.
`panes/stores`: `window-pane-store`, `drag-store`, `pane-slice`, `buffer-slice`, pane actions (`view-actions`, `layout-actions` minus fullscreen/distribute, `chat-drop-actions`, `runner-actions`, `editor-tab-actions` minus `setPaneLocked`/`moveEditorTabToPane`).
`panes/hooks`: `use-pane-keyboard`, `use-pane-activation`, `use-pane-drop-handlers` (file-drop part only), `use-pane-presentation`, `use-tab-view-ids`, `use-mounted-views` (policy), `use-pane-workspace`, `use-pane-can-review-branch`.
`tabs/components` used inside a pane: `pane-top-row`, `tab-bar` (+ item, sortable, add, overflow, context menu, close confirm, drag preview), `chat-tab-item`, `chat-column-header`, `chat-only-pane-header`, `chat-branch-header(-view)`, `open-editor-button`, `split-toggle-button`, `branch-review-shortcut-button`, `tab-navigation-buttons`.
`ui`: `edge-dissolve`, `segmented-control`, `button` (ghost icon-sm), `crowbar-wordmark`; `agent/controls/view-switch`.
`workspace/`: `workspace-host` and `keep-alive-policy` (retention logic; rendering replaced by the pane host), `workspace-slot-style`.
`console/`: every file (all reachable).
Keymap commands in section 7.

### 13.2 Do NOT port (dead or out of scope)

| Item | Evidence |
|---|---|
| Bottom pane: `bottomLayout`, `BOTTOM_PANE_ID`, `isBottomPaneVisible`, `data-bottom-pane-drop-target`, `getPaneScopeForPaneId` bottom branch, all `isBottomPane` props | no caller of `setIsBottomPaneVisible`; `bottomLayout` never rendered |
| Pane fullscreen overlay, `fullscreenPaneId`, `togglePaneFullscreen`, `exitPaneFullscreen`, `useBrowserOccluded` fullscreen branch | `togglePaneFullscreen` has no caller (`split-view-root.tsx:88-99` renders it) |
| `distributePaneSplit`, `flattenForRender`, `setPaneLocked`, `moveEditorTabToPane`, `setEditorTabPreview` | no non-test caller |
| `splitPane`, `createPaneBeside`, `getOrCreatePaneDropTarget` split branch, tab-drag edge overlay | only reached with zone `center` |
| `pane-command-actions` editor-group commands listed in the inventory | not present in the file now |
| `panes/components/editor-pane.tsx` | zero importers |
| `panes/components/comment-composer.tsx` | belongs to review (git feature), not shell |
| `mobile-chat-stage.tsx`, `IS_MOBILE` branches, phone variants of new-tab view | iOS only |
| Chat `split` presentation (`SPLIT_PRESENTATION_AVAILABLE`, `splitEnabled`, `ColumnsIcon`) | dev builds only |
| `BuildReadout` slots in pane rows | dev only |
| `commitDiff`, Developer settings | inventory CUT |
| Editor-portal registry (`editor-host-registry`, `editor-portal-registry`) | Monaco is REDESIGN; the host decides |

## 14. States to capture live (golden checklist, 1x)

How to reach each state in the reference app (inventory container `inventory-app`; sidebar rows are draggable with xdotool `mousedown`, moves, `mouseup`).

| # | State | How |
|---|---|---|
| L1 | Empty stage with New Tab page logo and 3 buttons | fresh space with no chat open |
| L2 | One chat pane, chat only (state A), sidebar open, then collapsed | click a chat; `ctrl+1` / sidebar toggle |
| L3 | Chat only, light and dark theme | settings Appearance |
| L4 | Open editor: side by side (B) | header "Open editor", wide window (at least 1000 px) |
| L5 | Stacked (C) | resize the window to a tall narrow shape (height over width, max over 780) |
| L6 | Tabs/collapsed (D) with chat tab selected, then a terminal tab selected | window under 780 px or pane under 780 px after a split |
| L7 | Chat/terminal switch, both segments, hover, focus-visible | `ctrl+/`, click |
| L8 | Two chats side by side with active ring on each | drag a second chat row onto the right edge of a pane (zone right) |
| L9 | Vertical split (top/bottom) and a 3-pane nest | drag onto top/bottom edge, then onto one child's edge |
| L10 | Drop overlays for left, right, top, bottom, centre, with pane ring | hold the drag mid-air and screenshot per zone; capture 100 ms transition as a frame sequence by moving between zones |
| L11 | Pane handle visible on hover; the three tooltips | hover top centre of a pane in a split |
| L12 | Sash hover, and mid-drag at min size (10 px drag floor) | drag the sash to extremes |
| L13 | Release at extreme: snap to 10 percent | release and screenshot |
| L14 | Chat/editor sash drag to 340 px minimum | drag in side-by-side |
| L15 | Close a pane via handle X, close view, `ctrl+w` on empty split pane | observe survivor focus |
| L16 | Pane navigate with `ctrl+alt+arrow` (the XFCE swap shortcut must be disabled: xfconf unbind or use xdotool key after remapping) | GAP if not possible |
| L17 | Console overlay open, each dock (top/right/bottom/left), push mode, resize drag, expanded log row | the toggle chord, then the header buttons; capture open/close as a 60 fps frame sequence (200 ms) |
| L18 | Pane error boundary | GAP unless an error can be forced via the bridge |
| L19 | Terminal tab in the editor half; browser tab; branch review tab | New tab page buttons |
| L20 | Sidebar position right (chat first order flips) | settings Appearance |
| L21 | Pane tab strip with overflow menu and pinned tab | open many files |

Computed styles to dump (CSS selectors in the DOM): `[data-pane-content]` (border, radius, margin, background), `[data-pane-accent]`, `[data-slot="resizable-handle"]`, `[data-testid="pane-handle"]`, `[data-testid="pane-top-row"]`, `[data-testid="edge-dissolve"] > div` (blur, mask), `[data-split-drop-overlay] > div`, `#console-panel`, `[data-slot="console-resize"]`, `.viewswitch [role=tab]` and its indicator, `[data-chat-view]`, `[data-editor-view]`.
