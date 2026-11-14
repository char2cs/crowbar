# Shell spec: pane tab strip, New Tab page, shortcuts, port list (tabs part 2)

Part 2 of the tabs surface. Part 1 (`tabs-topbar.md`) covers window chrome and the view-tab strip. Paths are relative to `app/panes/src/`. Hex values live in `00-tokens.md`; values quoted here come from `styles/theme.css`, `styles/crowbar.css`. **VERIFY LIVE** marks static conclusions to confirm in the running app.

## 1. Where the pane strip appears (presentation modes)

A pane (`PaneGroup`, `features/panes/types/pane.ts:1-26`) holds at most one chat plus editor tabs (`editorTabIds`: files, terminals, browser, branch review, New Tab page, md/html preview). How the top row looks depends on `usePanePresentation` (`features/panes/hooks/use-pane-presentation.ts:8-50`) and `usePaneViewPresentation` (width/height of the pane's view container):

| Mode | Condition | Top of pane |
|---|---|---|
| chat only | has chat, no editor tabs, `editorOpen=false` (`chatFillsPane`) | `ChatOnlyPaneHeader`: chat identity header + "Open editor" button + chat header slot, row is an **overlay** with chat-blur glass (`features/tabs/components/chat-only-pane-header.tsx`) |
| side by side / stacked | chat + editor, container >= 780 px on its larger axis (`SPLIT_SIDE_BY_SIDE_MIN_PX`, `features/agent/hooks/use-chat-presentation.ts:11`) | chat column has `ChatColumnHeader` (overlay glass); the editor box has its **own** `TabBar` above the editor content (`pane-container.tsx:131-134`). Default split 45/55, min half 340, min stacked 160 (`use-chat-presentation.ts:10-13`) |
| collapsed ("tabs") | chat + editor, container < 780 | one `TabBar` spans the pane, the chat is the **first tab** (`ChatTabItem`) (`pane-container.tsx:201-213`, `tab-bar.tsx:392-398`) |
| no chat (empty pane, or bottom tray) | `chatId === null` | `TabBar` only; the empty last pane keeps its row for the drag region / inset / reopen toggle (`tab-bar.tsx:131-134`) |

Chat header contents (title/route) are specified with the pane layout; here only the shared row shell matters.

## 2. Row shell (`PaneTopRow`), shared by TabBar and both chat headers

| Property | Value | Source |
|---|---|---|
| Height | 42 px | `features/tabs/components/pane-top-row.tsx:60`, `lib/chrome-metrics.ts` |
| Box | `flex shrink-0 items-center gap-1.5 px-2 py-1` (padding 8/4, gap 6) | `pane-top-row.tsx:53` |
| Fill | `bg-pane-background` (opaque): light `#fcfcfa`, dark `#1c1c1a` (`styles/theme.css:11,171`). With `variant='chat-blur'`: no fill + `EdgeDissolve edge=top height=42+56=98` | `pane-top-row.tsx:55,63-65` |
| Position | normal flow with `relative overflow-hidden`, or `overlay` = `absolute inset-x-0 top-0 z-10` | `pane-top-row.tsx:54` |
| TabBar variant | `chat-blur` only while the chat tab is the selected entry in collapsed mode, else opaque; **TabBar never overlays** (it would resize the split) | `tab-bar.tsx:347-348`, comment `pane-top-row.tsx:23-26` |
| macOS inset | `pl-[88px]` when top-left corner and not bottom pane (see part 1 s.1) | `pane-top-row.tsx:57` |
| `data-tab-bar-pane-id` | hit-test marker so tab drags find which pane a drop lands on | `pane-top-row.tsx:51` |
| EdgeDissolve | 7 stacked `backdrop-filter` layers, blurs 1,2,4,8,16,32,64 px, each masked to a band; blur disabled while a pane sash is dragged (`html[data-pane-resizing]`) | `components/ui/edge-dissolve.tsx:6-30`, `index.css` |

GPUI note: the chat-blur glass is a progressive blur of the scrolled chat text behind the 42 px header; replicate as a gradient scrim or a real blur pass (parity agent decides tolerance).

## 3. Pane strip contents, left to right (`tab-bar.tsx:341-459`)

1. **Split toggle** (`SplitToggleButton`): shown when the pane has an id and is not empty. `icon-sm` ghost, 28x28 (desktop), `rounded-sm`, icon Phosphor `SquareSplitHorizontal` 16 px, muted; `text-foreground` when `editorOpen`. Tooltip/aria: "Show chat only" when active, else "Show editor alongside chat". Toggles `PaneGroup.editorOpen` (`features/tabs/components/split-toggle-button.tsx`, `features/panes/utils/pane-command-actions.ts:163-169`).
2. **Reopen-sidebar toggle (left)** when sidebar closed, left side, flush left AND top (the view strip normally owns it) (`tab-bar.tsx:212-213,356-363`).
3. **Scroller** (only when the pane has editor tabs; a chat-only pane draws no scroller): `role=tablist aria-label="Open files"`, `tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden overscroll-behavior-x:contain`, `data-tauri-drag-region` (`tab-bar.tsx:379-387`).
   - collapsed mode: `ChatTabItem` first (not sortable, not closable, not draggable),
   - editor tabs (sorted pinned-first, stable otherwise) in `SortableEditorTab` wrappers,
   - **"+" button last inside the scroller** (scrolls with tabs, not sortable) (`tab-bar.tsx:426`).
4. **"More tabs" caret** (`TabOverflowMenu`) outside the scroller, only while `scrollWidth > clientWidth + 1` (re-evaluated on tab list change and ResizeObserver) (`features/tabs/components/tab-overflow-menu.tsx`).
5. **Branch review shortcut** (`BranchReviewShortcutButton`): pane not empty and `canReview`. Phosphor `GitPullRequest`, `icon-sm` ghost 28x28, tooltip/aria "Review this branch" (`branch-review-shortcut-button.tsx`, `use-pane-can-review-branch.ts`).
6. **Reopen-sidebar toggle (right)** when sidebar on the right.

Chat-only header (`ChatOnlyPaneHeader`): chat identity header (flex-1) + `OpenEditorButton` (Phosphor `CodeIcon`, 28x28, tooltip/aria "Open editor", only with a workspace; toggles `editorOpen`) + `ChatHeaderSlot` (pane-owned actions from the chat). No close control: closing a chat is a sidebar/view-strip operation only (`chat-only-pane-header.tsx:254-257`).

## 4. Editor tab anatomy (`tab-bar-item.tsx`)

Wrapper `div.group/tab.relative.max-w-[200px]` (`tab-bar-item.tsx:85-89`); the tab is a `<button role=tab>`, the close/pin button is a **sibling** (nested buttons invalid; dnd drag-start was eating the close click).

| Property | Value | Source |
|---|---|---|
| Height | `h-8` = **32** | `tab-bar-item.tsx:114` |
| Width | content-sized up to **max 200 px** (wrapper); no min width; shrinks only through ellipsis (`max-w-full`); scroller scrolls when too many | `tab-bar-item.tsx:88,114` |
| Padding | `pl-2.5 pr-8` = **10 left, 32 right** (room for the 20 px button + 6 px inset) | `:115` |
| Gap icon-label | 6 (`gap-1.5`) | `:115` |
| Radius | `rounded-sm` = `radius*0.6` = **6 px** (`--radius 10 px`) | `components/ui/tabs.tsx:392`, `styles/theme-vars.css:80`, `theme.css:9` |
| Border | 1 px transparent (`border-transparent`) | `tabs.tsx:383,392` |
| Typography | `ui-font text-[13px]`, weight 500 (`font-medium` from Tab base), one line, `text-ellipsis whitespace-nowrap`; font `CalSansUI` | `tab-bar-item.tsx:136`, `tabs.tsx:383` |
| Icon box | `grid size-3.5 place-content-center` = **14x14**; kind icon colour muted. Icons: file -> `FileExplorerIcon` (14 px), terminal -> `TerminalWindow`, browser -> `Globe`, branch review -> `GitPullRequest`, new tab -> `Plus`, `extensions://marketplace` path -> `Package` (dead path, see port list) | `tab-bar-item.tsx:131-133`, `features/tabs/components/tab-buffer-icon.tsx:10-30` |
| Label source | `useBufferDisplayName` (below) | `features/tabs/hooks/use-buffer-display-name.ts` |
| Tooltip | side bottom, 150 ms: line 1 display name; line 2 (editor) file path muted; line 3 "Read-only: this branch is locked" muted when read-only | `tab-bar-item.tsx:93-103` |
| ARIA | `aria-selected`, `tabIndex = active ? 0 : -1`, label = name + " (unsaved)" / " (pinned)" / " (preview)" / " (read-only)" | `:107-109` |

**Colours/states** (ghost variant, `components/ui/tabs.tsx:391-397`):

| State | Background | Label colour | Notes |
|---|---|---|---|
| inactive | transparent | `--muted-foreground` (light `oklch(0.4 0 0)`, dark `oklch(0.834 0.008 278.6)`) | |
| hover (inactive) | `--sidebar-element-hover` (light `oklch(0 0 0 / 8%)`, dark `oklch(1 0 0 / 10%)`) | `--foreground` | |
| active | `--sidebar-element-hover` (stays filled) | `--foreground` | |
| active + dirty editor | **same fill**, border becomes `--primary` (light `#606c38`, dark `#98a86a`), `rounded-sm`, `shadow-xs shadow-black/10`, `inset 0 1px 0 var(--elevated-highlight)`; pressed: inset black 8%, no shadow | foreground | `tab-bar-item.tsx:116-122` |
| inactive + dirty editor | no border change; **8 px dot** `size-2 rounded-full bg-primary` at the label's right, title/aria "Unsaved changes" | muted | `:144-151` |
| preview | label **italic**; promoted on double-click | | `:138`, `tab-bar.tsx:258-265` |
| read-only | `LockIcon size-3` muted after the label | | `:143` |
| focus-visible | `ring-2 ring-ring ring-offset-1 ring-offset-background` (no ring on mouse click: `onMouseDown` preventDefault and blur after click) | | `tabs.tsx:384,372-380` |
| dragging (source) | wrapper `opacity-40 z-10` (while the overlay copy is carried) | | `sortable-editor-tab.tsx:261` |
| transition | `transition-colors` 150 ms `cubic-bezier(0.4,0,0.2,1)` | | `tabs.tsx:383` |

**Close / pin button** (`tab-bar-item.tsx:158-186`): present unless `isUncloseable`. `absolute inset-y-0 my-auto right-1.5`, **20x20** (`!size-5`), `rounded-md` (8 px), `text-muted-foreground`, `transition-opacity` 150 ms. Opacity: **0.6 when the tab is active or pinned, else 0 -> 0.6 on tab-group hover**. Glyph: unpinned `X` `size-3` (12); pinned: `PushPin` `size-3.5 fill-current text-secondary`. Click: pinned => unpin; else close (stop propagation). Tooltip "Unpin tab" / "Close" with shortcut hint `mod+w` (unpinned only). `data-no-dnd` so it never starts a drag.

**Display names** (`use-buffer-display-name.ts:94-121`): terminal with user-set name -> that name; else terminal title when "useful" (non-empty, not "Default Terminal", <= 28 chars, no `@`, no `/` or `\`, no control chars); else first word of `initialCommand` basename; else cwd's last segment; else buffer name. Other tabs: `calculateDisplayNames` (disambiguates equal file names by shortest distinguishing parent path, and by workspace name; `features/tabs/utils/path-shortener.ts`). Tab default names: "Terminal N", "New Tab", "Branch Review", browser `browserTabName(title,url)` (`features/panes/stores/slices/new-buffer.ts`).

## 5. Pane strip behaviour

### 5.1 Selection, close, pin (`tab-bar.tsx`)

- Click: `activateEditorTabInPane` + `setActivePane`; also `updateActivePath` and a screen-reader live announcement "Switched to X[, unsaved changes]" (`:94-103,176-189`; `<div role=status aria-live=polite class=sr-only>` at `:474`).
- Close (`×`, middle click, `mod+w`, menu): refused if `isUncloseable` (a pane's **sole** editor tab when the pane has no chat and is the last split); dirty editor => `pendingClose` and the **Unsaved Changes dialog**: title "Unsaved Changes" (Warning icon, size sm), body "<file> has unsaved changes. Do you want to save before closing?", buttons Cancel (ghost) / Discard (destructive) / Save (default); a failed save keeps the tab open (`:111-125`, `tab-close-confirm.tsx`, `features/window/components/unsaved-changes-dialog.tsx`). Else remove from pane then release buffer; the neighbour at the same index (else last) becomes active; if the pane has no tabs `editorOpen` becomes false; an emptied chatless pane is removed (`features/panes/stores/slices/pane-actions/editor-tab-actions.ts:98-118`).
- Middle click (`onAuxClick button 1`) closes (`tab-bar-item.tsx:66-74`).
- Double-click: only promotes a **preview** tab to permanent (`tab-bar.tsx:258-265`). No rename on double-click. Terminal rename is via the context menu prompt.
- Pinned tabs sort to the front of the strip (stable sort; relative order kept) (`:156-162`).
- Tab cap: `min(EDITOR_CONSTANTS.MAX_OPEN_TABS = 10, settings.maxOpenTabs (default 100))` = **10 per workspace**; opening past the cap closes the first non-pinned, non-terminal, non-dirty tab of that workspace (`features/panes/stores/slices/buffer-slice.ts:135-156`, `features/editor/config/constants.ts:86`). Re-opening an already open document focuses it (terminals jump to the pane that holds them, never duplicated).
- Reopen last closed (`mod+shift+t`) pops `closedBuffersHistory[0]` and reopens that document (`buffer-slice.ts:266-276`).

### 5.2 Keyboard on a focused tab (`use-tab-keyboard-nav.ts`)

`ArrowLeft/Right` select and focus previous/next (no wrap), `Home`/`End` first/last, `Delete`/`Backspace` close (pinned: announces "Cannot close pinned tab X", does nothing), `Enter`/`Space` activate. Each announces to the live region.

### 5.3 Scrolling (`use-tab-bar-scroll.ts:21-62`)

Wheel over the scroller maps to horizontal scroll: horizontal delta wins; shift+vertical or pure vertical delta also scroll; ignored while a tab drag is active, when ctrl/meta is held, or when the strip does not overflow; clamps to [0, max]; default prevented only when it actually scrolls. The active tab is scrolled into view with `scrollIntoView({behavior:'smooth', block:'nearest', inline:'center'})` only if it is partially outside the scroller (`tab-bar.tsx:237-250`). Overflow caret menu: `DropdownMenu` `align=end`, `w-auto max-w-80`, one item per tab (sorted order, display names, truncated), active one `font-semibold` (none when the chat tab is selected); trigger `size-6` (24) ghost, caret `size-3.5`, `title/aria "More tabs"`, hover `--sidebar-element-hover` (`tab-overflow-menu.tsx`).

### 5.4 Drag reorder (dnd-kit, per pane)

| Item | Value | Source |
|---|---|---|
| Library | `@dnd-kit/core` 6.x + `@dnd-kit/sortable` 10.0.0, `horizontalListSortingStrategy`, `closestCenter` | `tab-bar.tsx:1-2,334-336` |
| Sensor | custom `PointerSensor`: primary button only, **activation distance 6 px**, ignores `[data-no-dnd]` (close/pin button) | `tab-bar.tsx:150-154`, `lib/no-dnd-pointer-sensor.ts` |
| Own `DndContext` per TabBar | a tab can only reorder inside its pane; cross-pane tab moves are **not** possible by drag | `use-tab-drag.ts:14-19` |
| Neighbour shift | dnd-kit sortable default transition: **200 ms `ease`** on `transform` | `@dnd-kit/sortable` `defaultTransition`, `sortable-editor-tab.tsx:257` |
| Source | `relative z-10 opacity-40` | `sortable-editor-tab.tsx:261` |
| Overlay | `DragOverlay dropAnimation={null}` portalled to `<body>`; `LIFTED` surface (white control surface, 1 px border-colour outline, `shadow-md shadow-black/15`, inset highlight) `rounded-lg px-2 py-1 ui-text-xs` (11 px x app scale), label `max-w-[200px] truncate text-foreground`, dirty adds a muted bullet `•` after the name; just the name (no icon, no X) | `tab-drag-preview.tsx` |
| On start | the dragged tab becomes the active tab | `use-tab-drag.ts:35-43` |
| On end | if over a different tab: `reorderEditorTabs(pane, tabId, newIndex)` and activate; drop on itself/outside: no change; indices are over the **pinned-first sorted list** | `use-tab-drag.ts:47-62`, `tab-bar.tsx:168-174` |
| Drop line | `showDropIndicatorBefore` exists in the item but **nothing passes it** (dead); no hairline in this strip | `tab-bar-item.tsx:20,90-92` |
| Wheel | ignored while dragging | `use-tab-bar-scroll.ts:411` |

### 5.5 Context menu (pane tab), native menu in desktop app

Order and enabling (`tab-context-menu.tsx:201-326`, `tab-bar-context-menu.tsx`). Anchor: opened at **tab's horizontal centre, 4 px below its bottom** (not the pointer) (`tab-bar.tsx:267-276`).

| # | Item | Icon (fallback popup only) | Shown when | Action |
|---|---|---|---|---|
| 1 | Pin Tab / Unpin Tab | PushPin / PushPinSlash | always | toggle `isPinned` |
| 2 | Rename | PencilSimpleLine | terminal tabs only | prompt dialog "Rename Terminal" (label "Enter a terminal name:", placeholder "Terminal name", confirm "Rename"); control chars stripped; sets session `customName` |
| - | separator | | | |
| 3 | Copy Path | Copy | always | clipboard |
| 4 | Copy Relative Path | Copy | always | path relative to the pane's workspace |
| 5 | Reveal in Finder | FolderOpen | only when the buffer's workspace is on the local host | opens file manager (label is macOS wording on every OS; inventory flags this) |
| 6 | Open in Terminal | TerminalWindow | real file path (not virtual, no `://`) | opens a terminal tab with cwd = file's directory |
| 7 | Reload | ArrowCounterClockwise | editor tabs that are not virtual | `window.confirm("Discard unsaved changes to <name> and reload it from disk?")` then reload |
| - | separator | | | |
| 8 | Close (hint `Ctrl/Cmd W`) | X | always | same as x |
| 9 | Close Others | | always | closes all but this in the pane |
| 10 | Close to Right | | always | tabs after this one in strip order |
| 11 | Close All | | always | all |

"Close *" skip pinned tabs and `isUncloseable` ones; dirty tabs are closed without a prompt in these bulk actions (code path `closeAll` does not call `setPendingClose`) - VERIFY LIVE and reproduce as is (`tab-bar-context-menu.tsx:100-111`).

### 5.6 "+" button and sub-menus

"+" (`TabAddButton`): `icon-sm` ghost 28x28 `rounded-sm`, Phosphor `Plus` 16 px, muted, hover `--sidebar-element-hover`, tooltip/aria "New tab" (bottom). Click -> `openNewTabPage(wsId, paneId)`: opens a **New Tab page** tab in that pane (after ensuring the pane has a chat: a chatless pane takes the workspace's owning chat; none resolved => nothing happens) (`tab-add-button.tsx`, `pane-command-actions.ts:121-160`). Hidden in the bottom tray pane. No dropdown on the button.

## 6. New Tab page (`features/panes/components/new-tab-view.tsx`)

Also shown as the **empty editor area** of a pane whose `editorOpen` is true with no tabs.

| Property | Value |
|---|---|
| Container | `relative flex h-full w-full flex-col items-center justify-center gap-6 overflow-hidden`, focusable (`tabIndex -1`) so a click focuses the editor area (then `mod+n` opens New Tab there) |
| Logo | `CrowbarWordmark`, `pointer-events-none h-auto w-[clamp(96px, 14cqmin, 148px)]` (container-query min-dimension; `.pane-cq` is `container-type:size`), colour `--muted-foreground`, `aria-hidden` |
| Action list | `flex flex-col items-stretch gap-0.5` (2 px between rows), only when the pane's workspace id resolved |
| Action row | `Button variant=ghost size=sm` (`h-8 sm:h-7` = **28 px**, `gap-1.5`, `px-2.5-1`), `justify-start text-muted-foreground`, icon 16 px (`size-4`), then label; hover bg `--accent` (light `oklch(0 0 0 / 8%)`, dark `oklch(1 0 0 / 10%)`) |
| Actions (in order) | `TerminalWindow` "New Terminal"; `Globe` "New Browser"; `FilePlus` "New File"; `GitPullRequest` "Review Branch" (only when `canReview`: a branch workspace) |
| Hidden below 380 px container height | `.nt-history` (history list) is `display:none` under `@container pane (max-height: 380px)`; **nothing in `new-tab-view.tsx` renders `.nt-history` anymore** (stale CSS; do not port) |
| Replace semantics | choosing an action opens the new tab **in the New Tab page's slot** (`replaceTabId`) and releases the New Tab buffer |
| Phone variant | `IS_MOBILE` only (not ported) |

Action implementations (`features/tabs/hooks/use-pane-new-tab-actions.ts`): Terminal = `openContent({type:'terminal'})`; Browser = `{type:'browser', url: browserHomepage(settings)}`; File = `openUntitledDocument` (a virtual untitled buffer); Review = `openBranchReviewForWorkspace` (buffer path `branch-review://<wsId>`).

## 7. Keyboard shortcuts (tab-related), `mod` = Cmd on macOS, Ctrl elsewhere

All resolved from the keymap registry (`features/keymaps/registry.ts`) through `useEffectiveChordMap`, so presets/rebinds apply live; handler `features/panes/hooks/use-pane-keyboard.ts:45-199` (window `keydown`, capture default prevented per match).

| Command id | Default | Behaviour | Source |
|---|---|---|---|
| `tabs.new` | `mod+t` | New Tab page in the active pane | registry 70-76; keyboard 58-65 |
| `tabs.newTerminal` | `mod+j` | terminal in active pane (chat-first rule) | 77-84; 67-82 |
| `agent.newChat` | `mod+n` | new chat as its **own view** on the first enabled provider; **but** when focus is inside an editor area (`[data-editor-view]`) it acts as New Tab there | 86-94; 56-65, 103-138 |
| `agent.newChatTerminal` | `mod+alt+n` | same, landing on the terminal surface when the provider supports it | 95-103 |
| `tabs.newFile` | `mod+shift+n` | untitled file | 105-110; 84-94 |
| `tabs.openFile` | `mod+o` | native open dialog (desktop app only) | 112-117; 97-101 |
| `tabs.reopenClosed` | `mod+shift+t` | reopen last closed document tab | 138-143; 140-144 |
| `tabs.closeActive` | `mod+w` | close active tab of active pane (dirty => dialog); with no tabs: closes the pane/split if it is not the last in its tree; **always claimed so it never closes the OS window** | 144-150; 146-180 |
| `panes.navigateLeft/Right/Up/Down` | `mod+alt+arrow*` | focus neighbouring pane | 41-68; 182-194 |
| `agent.toggleViewMode` | `mod+/` | chat/terminal toggle of the focused chat | 119-127 |
| `agent.cycleProvider` | unbound | | 128-136 |
| `agent.zoomIn/Out/Reset` | `mod+=`, `mod+-`, `mod+0` | chat zoom (part 1 s.1) | 151-171 |
| `browser.togglePick` | `mod+shift+e` | element pick in browser tab | 187-193 |
| `editor.save` / `editor.saveAll` | `mod+s` / `mod+shift+s` | display-only in settings (hardcoded) | 172-186 |
| `navigation.toggleConsole` | ``mod+` `` | console dock | 195-201 |
| `navigation.sidebarChats/Files/Git` | `mod+1/2/3` | sidebar mode (sidebar spec) | 202-224 |

macOS note: Ctrl+Alt+Arrow is swallowed by XFCE in the test container; verify pane navigation outside it.

## 8. Console dock toggle

No button in any tab row. Toggle is `mod+backtick` only (`use-console-keyboard.ts`); the dock has its own header (`features/console/components/console-header.tsx`, layout spec).

## 9. Persistence (pane strip)

Everything in the panes and buffers is in the **window layout row** (IndexedDB `workspace-layout`/`"window"`, version 4, debounced 300 ms): per pane `editorTabIds`, `activeEditorTabId`, `editorOpen`, `chatSelected`, `locked`; buffers via `toPersistedBuffers` (transient `loading`/`unavailable` fields stripped; unknown content types dropped on load; unsaved text lives in `editor-drafts` store; tabs unlisted by any pane and not dirty are dropped) (`features/panes/utils/persisted-layout.ts:14-60`, `lib/persistence/workspace-layout.ts:8-11`). Scroll position of the strip, drag state, context menu state, tooltips are not persisted. Terminal PTY sessions are daemon-owned; the tab only stores `sessionId`.

## 10. Port list

### 10.1 Must be ported (reachable)

| Component | Role |
|---|---|
| `features/tabs/components/view-tabs-row.tsx` (+ `ViewTabSlot`) | the top bar |
| `components/sidebar/view-row.tsx` (`RecentsViewRow`, `RecentsMemberRow`, `ViewTabPreview`, `useMemberRow`) | the tab itself |
| `components/sidebar/sidebar-row.tsx` (tab branch only: `tab`/`tab-member`, `activeGround` not used by tabs) | label + glyph |
| `components/sidebar/sidebar-row-glyph.tsx` (RowGlyph, working spinner) | glyphs |
| `components/ui/flicker-spinner.tsx` + `flicker-strip.ts` + `spinners/*.svg` | working indicator |
| `features/tabs/components/pane-top-row.tsx`, `lib/chrome-metrics.ts`, `use-pane-top-row-edges.ts`, `use-mac-traffic-light-sync.ts` | chrome row, inset, lights |
| `features/tabs/components/tab-navigation-buttons.tsx`, `components/ui/sidebar-toggle-icon.tsx` | reopen toggle |
| `components/sidebar/hooks/use-sidebar-drag.ts`, `lib/tab-strip-drag.ts`, `components/layout/{drag-ghost,drop-indicator,edge-scroll}.tsx`, `sidebar-drop-dispatch.ts`, `sidebar-drop-policy.ts`, `pane-drop-actions.ts` (Recents + pane drops), `recents-actions.ts` | drag engine |
| `features/panes/lib/{view-state,view-ops,view-pins,view-selectors,view-repair}.ts`, `stores/slices/pane-actions/view-actions.ts` | view model |
| `features/tabs/components/tab-bar.tsx` + `tab-bar-item.tsx`, `sortable-editor-tab.tsx`, `tab-drag-preview.tsx`, `tab-add-button.tsx`, `tab-overflow-menu.tsx`, `split-toggle-button.tsx`, `branch-review-shortcut-button.tsx`, `tab-buffer-icon.tsx`, `tab-context-menu.tsx`, `tab-bar-context-menu.tsx`, `tab-close-confirm.tsx` (+ `unsaved-changes-dialog.tsx`), `chat-tab-item.tsx`, `open-editor-button.tsx`, `chat-only-pane-header.tsx`, `chat-column-header.tsx`, `chat-branch-header*.tsx`, hooks `use-tab-bar-scroll`, `use-tab-drag`, `use-tab-keyboard-nav`, `use-buffer-display-name`, `use-pane-new-tab-actions`, `use-rendered-pane-buffers`, `utils/path-shortener.ts` | pane strip |
| `features/panes/components/new-tab-view.tsx`, `ui/crowbar-wordmark` | New Tab page |
| `features/panes/components/pane-handle.tsx` | the tab hanging from a split pane's top edge (drag out / detach / close): bg `--primary`, `rounded-b-md`, `px-[7.5px] pb-1 pt-[5px]`, `gap-[9px]`, `shadow 0 0 5px 1px rgb(0 0 0 / 10%)`, fades in on pane hover/focus-within with `transition-opacity`; three 12 px icon buttons: grip (DotsSixVertical rotated 90, 10 px, "Drag this chat"), detach (ArrowsOutSimple 10 px, "Open in its own tab"), close (X 8 px, "Close this chat"); centred at the pane's top (layout spec) |
| `features/keymaps/*` (registry, chord matching, effective map, presets), `use-pane-keyboard`, `use-zoom-keyboard`, `features/window/stores/zoom-store.ts`, `use-document-title.ts` | shortcuts, title |

### 10.2 Do not port (unreachable or dev/stub)

| Item | Evidence |
|---|---|
| `commitDiff` pane content, `CommitDiffPane`, its case in `tab-buffer-icon`/`use-native-tabs` | no UI creates it (inventory) |
| `features/window/hooks/use-native-tabs.ts`, `use-mobile-native-chrome.ts`, `use-native-sidebar.ts`, `use-native-files.ts`, `mobile-*-page.tsx`, `use-surface-theme-color.ts` (theme-color meta) | iOS/phone only (`mobile-ide-screen.tsx:14`) |
| `BuildReadout` + `build-readout*.ts*`, `use-readout-lines.ts` anchor in both strips | debug build only |
| `showDropIndicatorBefore` branch in `tab-bar-item.tsx` | never passed |
| `extensions://marketplace` icon branch in `tab-buffer-icon.tsx`; extension toolbar stub | no producer / stub |
| `features/window/stores/ui-state-store.ts` fields: `sidebarWidth`, bottom pane, activity items, quick-open, agent launcher, global search, project picker, right sidebar, `isSettingsDialogVisible`, `activeView` | no readers outside tests (verify with knip before deleting) |
| `.nt-history` CSS | no element renders it |
| Any VS-Code editor-group command (`splitActiveEditorGroup`, `lock`, `moveActiveEditorToAdjacentGroup`, ...) | inventory CUT list |
| Settings Developer tab, build badge | dev only |

### 10.3 Non-visual behaviour the Rust side must keep

1. View model laws: pinned-first order; one view per chat pane; showing view per project remembered (`activeViewByProject`); closing the showing view picks next by recency within project/workspace; focus is derived (`settleFocus`); `mayShow` gates cross-project focus (`view-state.ts:460-540`).
2. Close view never kills agents; per-chat close from the pane handle closes just that pane (`recents-actions.ts:29-39`).
3. Drop semantics in the strip: before/after reorder, middle merges as split, pinned groups isolated, working chats refused, toast text.
4. Drag engine rules of section 5.4 of part 1; thresholds 8/6/150ms/36px/14px.
5. Editor-tab store rules: single owner buffer list, `placeTab` seating (active, `editorOpen=true` if first, `chatSelected=false`, preview replacement), uncloseable sole tab, release on last pane (C2), 10-tab cap with eviction rule, terminals never duplicated, closed-tab history for reopen, dirty-close dialog.
6. Shortcuts of section 7, including the `mod+n` over-editor rule and always-claimed `mod+w`.
7. Layout persistence (debounce 300 ms) + load repair (`view-repair.ts`, `validateLoadedBuffers`).
8. Accessibility: tablist/tab roles, live announcements, roving tabindex (the Rust UI needs equivalents or an owner-approved cut).
9. Window title updates; macOS traffic-light follow (CI-verified only).

### 10.4 Daemon owns vs shell owns

| Daemon | Shell |
|---|---|
| chats, titles, working state, runner ids, workspaces, branch names, change counts, repo icons, providers, settings (incl. `tabListScope`, `sidebarPosition`, `maxOpenTabs`), terminal sessions (PTY, titles), documents (content, save, reload), review data | views, view order, pinned flags, panes tree and sizes, editor-tab lists, active tab/pane/view, chatSelected, editorOpen, closed-tab history, drag/hover/focus state, window geometry, strip scroll, context menus and dialogs, keymap resolution |

Open question for the lead: layout/view state is webview-IndexedDB today; the GPUI shell must pick its store (and whether to migrate old rows).

## 11. States to capture live (golden checklist)

Reach each in the debug app (noVNC 6091; seed data first). Capture light and dark (Settings > Appearance), sidebar open and closed.

| # | State | How to reach |
|---|---|---|
| T1 | Strip with 1 view (width 240) | open one chat |
| T2 | 2, 4, 8, 12 views (shrink to 112, then scroll) | Ctrl+N repeatedly / resize window |
| T3 | showing vs idle vs hover vs pressed | mouse move/down on tab |
| T4 | working tab (spinner) | needs a provider (allowed per memory) or fake via daemon |
| T5 | provisional ("Loading..."), draft ("New thread"), untitled chat | new chat before title |
| T6 | set tab (2 chats in one shell, separator hairline) | drag a chat onto a tab centre |
| T7 | pinned tab (pin icon, front position) | use the pin path if reachable; else daemon-less store call via bridge `windowPaneStore.getState().paneActions.setViewPinned` |
| T8 | focus ring (keyboard) | Tab key |
| T9 | drag: pending, reorder with slide, ghost out of band, external chat from sidebar (gap), drop settle glide | xdotool mousedown/move/up, record 60 fps |
| T10 | edge-scroll while dragging at strip end | 12+ views |
| T11 | reopen toggle, sidebar closed, left and right sidebar positions | Settings > Appearance sidebar position; Ctrl+B / toggle |
| T12 | right-click on view tab (expect none/native) | xdotool click 3 |
| T13 | Pane strip: chat + 1 file tab (collapsed and side-by-side widths 700 / 1000) | open file from Files; resize |
| T14 | pane strip tab states: active, inactive, hover (close fade-in), dirty active (border), dirty inactive (dot), preview (italic), pinned (pin icon), read-only (lock), terminal, browser, branch review, New Tab page | each open action |
| T15 | overflow: caret menu open, wheel scroll, active scrolled into view | open > 10? (cap is 10 per workspace; use 2 workspaces or narrow window) |
| T16 | tab drag reorder (dnd-kit, 200 ms neighbour shift, overlay) | xdotool |
| T17 | tab context menu (native), terminal Rename prompt, Unsaved Changes dialog | right-click; edit file then Ctrl+W |
| T18 | New Tab page (desktop, with/without Review Branch), empty editor area | `+`, Ctrl+T |
| T19 | chat-only header (overlay glass) and chat column header; scrolled chat behind the glass | long chat |
| T20 | window title text | `xdotool getwindowname` |
| T21 | tooltips (toggle, +, close, split) | hover 150 ms+ |
| T22 | pane handle on split hover | two chats in a view |

Computed-style dumps to take (`getComputedStyle`) for the verification of section numbers: `[data-testid=pane-top-row]` (height 42, paddings, gap), `[data-view-tab-strip]` children (slot widths, min/max), tab shell (`[data-view-showing]`) (height, margin, border-radius, box-shadow, background), inner `[role=tab]` row (height 32, padding 4, font), close button (24x24, right 6), `[data-testid=split-toggle]` (28x28), editor tab button (height 32, padding 10/32, radius 6), tab close button (20x20), tooltip bubble.

## 12. Intent vs observed bugs (owner rule: Rust implements the INTENT; visuals stay 1:1 for every state)

### 12.1 Tab drag reorder (pane strip)
**Intent.** Reorder tabs inside a pane; the dragged tab becomes active; pinned stay in front.
**Observed bugs.** Tabs cannot be moved between panes by dragging; `moveEditorTabToPane` exists in the store (`features/panes/stores/slices/pane-actions/editor-tab-actions.ts:~130`) but no UI calls it, and a code comment forbids resolving drops across panes (`features/tabs/hooks/use-tab-drag.ts:14-19`). Intent (owner): decide whether cross-pane tab drag is wanted; today it is not. Reorder indices are computed on the pinned-first sorted list but written to `editorTabIds` (`tab-bar.tsx:168-174`), so reordering around pinned tabs can leave the stored order inconsistent with the display (suspected bug, verify with 1 pinned + 3 tabs).

### 12.2 Move chats between tabs/panes
Covered by the view strip (tabs-topbar 9.2/9.3); the pane strip has no chat moves.

### 12.3 Close, restore, reopen (editor tabs)
**Intent.** Closing never loses unsaved work without asking; `mod+shift+t` brings back the last closed tab of any kind.
**Observed bugs.**
- "Close Others / Close to Right / Close All" close **dirty** tabs without the Unsaved Changes dialog (`features/tabs/components/tab-bar-context-menu.tsx:100-111`, bypasses `setPendingClose`). Intent: prompt (or save/discard per dirty tab) before closing any dirty tab.
- Reopen restores documents only (`bufferActions.reopenLastClosedBuffer` calls `openDocument`, `features/panes/stores/slices/buffer-slice.ts:266-276`); terminals, browser, review, New Tab pages are not restored. Intent: restore the tab kind (terminal tab would start a new shell; browser its URL) (owner).
- Middle-click and `mod+w` close a **pinned** tab, while Delete/Backspace refuses and the button unpins (`tab-bar-item.tsx:66-74`, `tab-bar.tsx:111-125`, `use-tab-keyboard-nav.ts:77-86`). Intent: a pinned tab is not closed by any accidental gesture; unpin first (the keyboard rule is the intended one).
- `mod+w` on a pane with no tabs closes the pane/split; on the last pane it does nothing and is still claimed (`use-pane-keyboard.ts:146-169`): intended (never closes the OS window).

### 12.4 Pin (editor tabs)
**Intent.** Pinned tab sits first, keeps its place, is protected from eviction and bulk close, close button becomes unpin.
**Observed bugs.** Protection incomplete (see 12.3 middle-click / `mod+w`). Pin is per tab buffer, so a document open in two panes pins as one buffer.

### 12.5 Eviction at the tab cap
**Intent.** Opening a tab beyond the user's limit closes the least valuable tab, never silently one with unsaved work, never one the user is looking at.
**Observed bugs.** The cap is `min(EDITOR_CONSTANTS.MAX_OPEN_TABS = 10, settings.maxOpenTabs)` (`buffer-slice.ts:138-140`, `features/editor/config/constants.ts:86`) while the setting defaults to 100 (`features/settings/config/default-settings.ts:80`): the setting can only lower the cap, the effective default is **10**, and the eviction is silent (no toast). Evictee = first tab in the workspace's buffer list that is unpinned, non-terminal and non-dirty (`:142-147`): oldest by creation, not least recently used, and it may be a visible active tab of another pane. If every tab is pinned/terminal/dirty, no eviction happens and the cap is exceeded. Intent (owner): honour the setting (default 100 or documented limit), evict least-recently-used non-visible tab, tell the user.

### 12.6 New Tab page / "+"
**Intent.** "+" and `mod+t` open a New Tab page in the pane the user means.
**Observed bugs.** With a chatless pane and no resolvable owning chat the action silently does nothing (`features/panes/utils/pane-command-actions.ts:121-149`). `mod+n` has two meanings (new chat, or New Tab when focus is in an editor area, `use-pane-keyboard.ts:56-65`): intended per code comment, keep.

### 12.7 Context menu (pane tab)
**Intent.** Same items, positioned where the user clicked.
**Observed bugs.** Anchored at the tab's centre, 4 px below (`tab-bar.tsx:267-276`), not at the pointer. "Reveal in Finder" label on every OS (`tab-context-menu.tsx:268`; use the platform name: Finder / Show in Files). Labels "Close Others" etc. have no accelerators. Reload uses `window.confirm` (web dialog): native confirm dialog in Rust.

### 12.8 Workarounds and suspected bugs (consolidated, both files)
| # | Item | Source | Verdict |
|---|---|---|---|
| 1 | view-tab context menu unreachable (role mismatch) | `components/sidebar/row-context-menu.tsx:76` | bug; implement intended menu |
| 2 | bulk close skips dirty prompt | `tab-bar-context-menu.tsx:100-111` | bug (data-loss risk) |
| 3 | pinned tab closable by middle-click / `mod+w` | `tab-bar-item.tsx:66`, `tab-bar.tsx:111` | inconsistent; intent: protected |
| 4 | tab cap 10 hard-wired, setting default 100 ignored upward, silent eviction | `buffer-slice.ts:138-156` | bug/owner |
| 5 | reopen-closed: documents only, views never | `buffer-slice.ts:266` | gap |
| 6 | working chat refused on Recents merge/reorder | `sidebar-drop-policy.ts:153` | suspected bug (owner) |
| 7 | pane-strip reorder on sorted list vs stored list | `tab-bar.tsx:168-174` | suspected bug |
| 8 | view strip does not scroll showing tab into view | `view-tabs-row.tsx` | bug |
| 9 | pinned view "compact" never implemented | `pane.ts:240` | mismatch |
| 10 | `showDropIndicatorBefore` dead branch | `tab-bar-item.tsx:20,90` | delete |
| 11 | `.nt-history` CSS with no element | `styles/theme.css` (@container pane) | delete |
| 12 | `Reveal in Finder` wording on Linux | `tab-context-menu.tsx:268` | fix label |
| 13 | focus ring suppression via blur / mousedown preventDefault | `components/ui/tabs.tsx:372-380` | web-view workaround, not needed natively |
| 14 | 20 px edge heuristic, two-frame wait, MutationObserver for traffic lights | `use-pane-top-row-edges.ts`, `use-mac-traffic-light-sync.ts` | workaround; native knows geometry |
| 15 | pointer-capture delay, click swallow, selectstart block | `use-sidebar-drag.ts` | web workaround |
| 16 | `moveEditorTabToPane` store action with no UI | `editor-tab-actions.ts` | dead unless owner wants cross-pane drag |
| 17 | `ui-state-store` unused flags | `features/window/stores/ui-state-store.ts` | dead |
| 18 | `window.confirm` for Reload | `tab-bar-context-menu.tsx:126-130` | replace with native dialog |
