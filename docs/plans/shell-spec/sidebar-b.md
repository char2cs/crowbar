# Shell spec: sidebar, part B (behaviour, menus, drag and drop, ownership, port list, capture checklist)

Part A (anatomy, tokens, rows, panels): `sidebar.md`. Same conventions (sources cited as `file :: symbol`, paths under `app/panes/src/`).

## 1. State machines

### 1.1 Sidebar visibility (`components/layout/use-sidebar-panel.ts`, `sidebar-split-pane.tsx`, `sidebar-peek.tsx`, `components/ui/sidebar.tsx`)

| State | Meaning | Visual |
|---|---|---|
| `docked` | `open = true` | sidebar in grid column (width = preferred, clamped) |
| `closed` | `open = false`, no hover | column width 0; card parked off-screen, `inert` (not focusable) |
| `peeking` | `open = false` and pointer in trigger band or over the card | floating card slid in over the content; panel geometry unchanged |

| Event | From -> To | Notes |
|---|---|---|
| Mod+B (global keydown, `event.key==='b'` with meta or ctrl; `preventDefault`) | docked <-> closed | `components/ui/sidebar.tsx`; no `repeat` guard, no Shift/Alt check |
| Header toggle button, tabs-bar reopen toggle | docked <-> closed | |
| Double-click separator | docked -> closed, closed -> docked (at preferred width) | |
| Drag separator below 125px | docked -> closed live; back above 125 reopens | |
| Keyboard on separator | see part A 1.1 | |
| Pointer enters the trigger band while closed | closed -> peeking | no delay, no debounce |
| Pointer leaves card footprint (+/- 8px) / window / app blur | peeking -> closed | no close timer: the footprint counts as hover so there is no gap to cross |
| Open (dock) while peeking | -> docked | hover flag reset |
| Narrow window (< 640px split) | docked shows as overlay drawer; Esc or scrim click -> closed | |

Peek geometry (`sidebar-peek.tsx`): trigger = pointer `x <= 6` (dock-left) or `x >= innerWidth - 6` (dock-right), AND `y >= 42` (below chrome row, never behind traffic lights) AND `y <= innerHeight - 16` (clear of the bottom corner resize zone). Hit test on document `pointermove` (passive); it is NOT an element, so it never steals wheel/clicks from content. Card: `fixed inset-y-2 left-2 (right-2)` => 8px from top, bottom and the side edge, width = remembered preferred width (`max-w-[calc(100vw-1rem)]`), `z-40`, `overflow-hidden rounded-xl (14px) border bg-popover text-popover-foreground shadow-lg/5`, `not-dark:bg-clip-padding`; an inset `::before` overlay `rounded-[calc(14px-1px)] bg-muted` with `shadow-[0_1px black/4%]` (dark: `0 -1px white/6%`) lifts it. Because the card sits on the popover surface, it is NOT dark chrome: the card's inner `chrome-surface` still repaints the dark palette inside (the inner sidebar body is the same subtree; the visible card background is `bg-popover` behind a transparent subtree, hence the `before:bg-muted` wash). Verify live (capture B2).
Animation: `transition-transform duration-200 ease-[cubic-bezier(0.4,0,0.2,1)]`, parked at `translate-x-[calc(-100%-1rem)]` (left) or `[calc(100%+1rem)]` (right), peeking at `translate-x-0` with `will-change: transform` only while shown. Browser views and other native surfaces step aside while `occupied` is true (store `features/window/stores/sidebar-peek-store.ts`: true from summon until the slide-out `transitionend` on `translate`/`transform`; immediate false when the transition duration is 0, e.g. reduced motion). In GPUI the pane host must receive this "overlay occupied" signal so web views are hidden/clipped (pane-host design doc).

Under reduced motion every CSS animation/transition collapses to 0.01ms except `.animate-spin`, `[data-essential-motion]` (`index.css`).

### 1.2 Mode (chosen vs shown)
`activeTab` (persisted) in {chats, files, git}; shown = chosen if available (git needs a repo workspace in focus, files needs any workspace) else chats. Unavailable git tab is removed from the bar; files is dimmed. Switching never remounts panels.

### 1.3 Row editing (`features/workspace/tree/tree-ui-store.ts`)
`renaming` (row id, role tree|recents) and `branching` (row id) are mutually exclusive; starting one stops the other. Triggers: double-click on `[data-sidebar-row-label]` (refused for a locked branch), context menu Rename, Make-a-branch button/menu. `draft` (CreateDraft) = a naming input at the slot of a not-yet-created fork; `startDraft` unfolds the parent if folded.

### 1.4 Row removal hold (`tree-ui-store.ts :: hold, cancel, settle, release, askToDiscard`)
`REMOVAL_DRAIN_MS = 8000`.
| Kind | deadline | Confirm |
|---|---|---|
| workspace, folder, chat | now + 8000ms | none; row stays in place with countdown and Keep |
| repo, project | `null` (no clock) | `RemovalConfirmDialog` (below) |
Flow: `hold` hides the rows (`hiddenIds`) and records a tray entry; a held chat's panes are closed immediately (`trash-actions.ts :: leaveHeldChats`); one timer fires `commitRemoval` at the soonest deadline (`removal-tray.tsx`); the seconds numerals are updated by a shared 1-second clock aligned to second boundaries (`removal-clock.ts :: secondsLeft = max(0, ceil((deadline-now)/1000))`); `pagehide` flushes every draining entry with a keepalive request (clockless entries are dropped). Keep -> `cancel` (rows reappear). Commit sends the delete; on success rows release when the daemon's tombstones arrive; on refusal rows return with an error toast; a refusal over unsaved work re-holds the entry with `atRisk` and asks the confirm dialog ("Delete anyway"). If the viewed workspace is inside what is removed, the shell navigates to a fallback workspace resolved at hold time (`removal-commit.ts`). Refusals at hold: `handleTrash` false -> toast "Can't delete <label> yet" (X button) / "Can't remove <label>" (menu) / "Can't delete this space — it may already be gone" / "Can't delete <label> yet" (repo).

Confirm dialog (desktop webview; the phone uses a native alert) `components/layout/removal-confirm-dialog.tsx`: Dialog primitive (below) `max-w-md`; title `Delete <noun> “<label>”?` (nouns: space, repository, workspace, chat, folder) or `... and lose work?` with `atRisk`; body bold lead + rest (copy constants `PROJECT_CASCADE`, `REPO_CASCADE`, `WORK_AT_RISK`); work-at-risk list (branch mono foreground, loss line muted "N uncommitted file(s) · N unmerged commit(s)"); footer: Cancel (ghost, autofocus) then destructive "Delete <noun>" / "Delete anyway".

Dialog primitive (`components/ui/dialog.tsx`): backdrop `fixed inset-0 z-50 bg-black/32 backdrop-blur-sm`, 200ms opacity; popup `max-w-lg rounded-2xl (18px) border bg-popover shadow-lg/5`, `scale-98 + opacity 0` -> 1 over 200ms ease-in-out; centred on a `grid-rows-[1fr_auto_3fr]` viewport with `p-4` (so the popup sits at the upper third); close button `icon ghost` at `end-2 top-2`; title `font-heading text-xl (20px) leading-none font-normal`; description `text-sm muted`. (Settings dialog styling is the settings spec's concern.)

### 1.5 Create-space (`components/layout/use-shell-route-state.ts`)
`creatingSpace` false <-> true: `+` in the switcher sets Chats mode and replaces the carousel with the panel; selecting a project or Cancel closes; Create closes after the operation settles (the new project already in the store, route follows).

## 2. Context menus

The desktop app shows these as OS-native menus (`components/ui/context-menu.tsx :: ContextMenuHost` -> `showNativeContextMenu`, Tauri/muda; items carry `shortcut` accelerators, no icons, no custom styling; separators allowed; destructive style is NOT shown natively). The rendered fallback (browser/dev/phone) is the reference for icons and styling, and is what a GPUI-drawn menu should copy if not using OS menus:

- Popup: `relative min-w-[180px] rounded-lg border bg-popover shadow-lg/5 not-dark:bg-clip-padding` with inset `before` highlight, `z-[10040]`, list `p-1 max-h-(--available-height) overflow-y-auto`. Item: `flex min-h-7 items-center gap-2 rounded-sm px-2 py-1 text-sm (14px) text-foreground`, highlight `bg-accent text-accent-foreground` (accent = oklch(0 0 0 / 8%) light, oklch(1 0 0 / 10%) dark), disabled 64% opacity; leading icon 16px at 80% opacity (`-mx-0.5`); shortcut `kbd` right-aligned 12px muted/72% `tracking-widest`; separator `mx-2 my-1 h-px bg-border`; submenu opens right, `align=start`, `sideOffset -4`, trigger shows `CaretRight` 16px at 60%. Destructive item: `text-destructive`, highlight `bg-destructive/10` (dark `/20`).
- Open: right click anywhere on a `[role=treeitem][data-sidebar-row-id]` (listener on `document.body`), at the pointer; the repo `...` button opens the same menu below the button (`x = rect.left, y = rect.bottom + 4`). Dismiss: click-away, Esc, choosing an item (keeps mounted until Base UI finishes so an inline input keeps focus).

### 2.1 Row menu (`components/sidebar/row-context-menu.tsx`, `lib/row-menu.ts :: rowMenuActions`) - order top to bottom

1. (view-tab rows only, tabs surface) tab verbs first.
2. Rename (`PencilSimpleLine`) - not for a locked branch.
3. Lock (`Lock`) / Unlock (`LockOpen`) - branch rows that are not the repo header and not placeholders.
4. Import branches (`DownloadSimple`) - repo header only; opens Import branches dialog.
5. New folder (`Folder`) - branch, folder, chat rows.
6. New thread in Terminal (`Terminal`) - same rows, only if the enabled provider can start on terminal.
7. Move to… (`ArrowsOutCardinal`, submenu: every legal drop-into container, each with `Folder` or `GitBranch` icon; label = branch name or label) - only if targets exist (`lib/move-targets.ts`).
8. Make a branch (`GitBranch`) - repo thread, no pending move.
9. Cancel move (`XCircle`) - pending move.
10. Kill-agent items (`useKillAgentMenuItems`, agent feature).
11. separator, Delete Repo (`Trash`, destructive) - repo header only; runs project/repo removal hold (confirm dialog).
Items 7-10 are spliced in ahead of the first separator (i.e. before the destructive group); with no destructive item they append.

### 2.2 Tree foot menu: Import a repo, Create a new folder, Delete space (destructive, no separator before it). See part A 6.

### 2.3 Hover button verbs not in the menu: Detach…, Retry setup, Remove, New thread, Fork branch (`rowVerbs`, used by the phone's single long-press menu: all verbs, destructive last).

### 2.4 Dialogs launched from the sidebar
- Add repository (`components/projects/add-repository-modal.tsx`): `DialogContent sm:max-w-md`; title "Add repository"; field "Path" (input placeholder `/absolute/path/to/repo`, optional native folder button), field "Name" (placeholder "Derived from folder name"); footer Cancel (ghost) and primary Add (disabled until the path looks absolute; shows loading); runs a `repo.create` operation, errors via toasts.
- Import branches (`components/layout/repo-import-dialog.tsx`): `DialogPopup flex h-[70vh] max-w-md flex-col p-0`; header `p-4 pb-2` title "Import branches" / "Bring remote branches into Crowbar as workspaces."; search input "Search branches…"; list with states "Fetching branches from the remote…" (loading), rows with checkboxes plus a lock option, footer buttons.
- Detach holder (`components/layout/detach-holder-modal.tsx`): title "Detach to manage <branch>"; body: "The checkout at <path> will move to a detached HEAD, releasing <branch> so Crowbar can manage it in its own worktree. Your files are safe — only the working directory's current branch changes; uncommitted changes and commits are preserved."; Cancel ghost / Detach primary; runs operation `workspace.detach_holder`.
- Settings: Settings spec. Remote host line and Add host: Settings > Hosts / `AddHostDialog` (shown in Create-space daemon row only under Tauri).

## 3. Space icon editor popover (`components/layout/icon-popover.tsx`)
Trigger = the mark; hover shows pencil overlay. Popover (`side=right align=start`, `w-64 p-0`): `p-3 gap-3` column: label "Icon" (`text-[10px] font-semibold uppercase tracking-wider text-muted-foreground`), preview Avatar `size-14 rounded-xl` (emoji 24px or image or fallback XL mark), button row `gap-1.5` of `xs` buttons "Upload" (UploadSimple 12px; png/jpeg/webp) / "Emoji" (Smiley; toggles input row: input `h-7` centred 16px placeholder "Type an emoji…" `maxLength 4`, "Set") / "GitHub" (Star; repos with a GitHub origin only), and "Reset to default" (Trash, destructive on hover, shown only when an emoji or image is set). Writes go through the project/repo icon REST routes (daemon stores the icon).

## 4. Create-space panel (`components/layout/create-space-panel.tsx`, `create-space-daemon-row.tsx`)
Column `flex min-h-0 flex-1 flex-col gap-5 overflow-hidden px-3 pt-4`:
1. Title "Create a Space" (`font-semibold text-base` 16px foreground), subtitle "Spaces are used to organize your tabs and sessions." (`mt-1 text-sm` 14px muted).
2. Row `gap-2.5`: dashed tile 36x36 (`size-9 rounded-lg border border-dashed border-border`) containing the icon popover (staged: nothing is sent until Create); `Input` (autofocus, placeholder "Space Name").
3. Daemon row (`rounded-lg border border-border bg-muted px-3 py-2.5 text-sm`, justify-between): `HardDrives` 16px muted + "Daemon" | Select trigger (borderless, muted; host status icon + name; remote names in tone colour) with options Local, separator, each paired host (status), separator, "+ Add a host…" (Tauri only). Selecting a host clears an incompatible folder.
4. Folder row (same box, `hover:bg-sidebar-element-hover`): `FolderOpen` 16px muted + "Folder" | value truncated muted: folder basename or "Choose…"; opens the daemon folder picker for that host.
5. Spacer `flex-1`; footer `shrink-0 gap-2 pb-4`: optional hint (center, 12px muted; remote only: "<name> needs pairing again." / "<name> is not answering." / "Choose a folder on <name> first."), full-width Create Space (primary; label "Creating…" while loading; disabled until a folder is chosen, or host unreachable), full-width Cancel (ghost).
Create: name = trimmed input or folder basename; `createProject(name, path, hostId)`; staged icon applied best-effort afterwards (failure toast "Failed to set icon"); then select the new project.

## 5. Drag and drop (rows)
Machinery: `components/sidebar/hooks/use-sidebar-drag.ts`, `components/tree-dnd/*`, `components/sidebar/lib/sidebar-drag-hover.ts`, `sidebar-drop-policy.ts`, `drop-plan*.ts`, `drop-actions.ts`, `pane-drop-actions.ts`, `components/layout/{drag-ghost,drop-indicator,edge-scroll}.ts*`. It is shared with the tabs bar (Recents tabs), which adds its own strip drag (`tab-strip-drag.ts`) - spec'd in the tabs document.

| Rule | Value |
|---|---|
| Arm | primary button pointerdown on a row (not on trailing buttons, not while renaming/branching); a second pointer is ignored; text selection blocked from the press (`selectstart` prevented) |
| Start threshold | **8 px** Euclidean from press point (`SIDEBAR_DRAG_THRESHOLD_PX`); no hold delay; pointer captured at start (not at press, so double-click rename still works) |
| Single-row drags | the tree has no multi-select; subjects = the pressed row |
| While dragging | `<html data-row-dragging>`: cursor `grabbing` everywhere; selection disabled; carousel pinned; wheel on the list is blocked (only the edge scroller moves it) |
| End / cancel | pointerup commits (swallows the click that follows); `pointercancel`, window blur, or a move with `buttons === 0` ends without commit |
| Source row | `opacity-40` |
| Ghost | portalled to body, `fixed top-0 left-0 z-50`, `pointer-events:none`, moved by `translate3d(pointer - grabOffset)` every move (grab offset = press point inside the row, clamped to its box); a deep clone of the row at its width; clone styling "lifted": ambient fills removed; `control-surface` (bg #fcfcfa, text #1b1b19), `outline 1px outline-border`, `shadow-md shadow-black/15`, `inset-shadow 0 1px elevated-highlight`; stacking: up to 3 clones with 4px x/y steps, back clones at opacity .25 (only relevant for multi-row, i.e. tabs); count badge `-right-1.5 -top-1.5 h-4 min-w-4 rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground` when count > 1 |
| Hit test | `elementsFromPoint` topmost row/zone wins (the ghost is skipped); a refusing row yields no drop (never falls through to an ancestor) |
| Drop mode by pointer height in the row | ratio = (y - top)/height. If nesting not allowed: < 0.5 before, else after. If only nesting allowed: into. Else: ratio < band -> before; > 1 - band -> after; else into. Band: **0.2 for folders, 0.3 for every other kind**. A mode the matrix refuses yields no drop. "after" an expanded parent with children resolves to its first child slot and counts as a re-parent. |
| Indicators | "between": a single 2px line (`h-0.5 rounded-full bg-sidebar-drop-line`; `--sidebar-drop-line` = secondary 55% mixed with foreground), `fixed z-[60]`, positioned `left = rowRect.left + 8`, `width = rowRect.width - 10`, `top = (before ? rect.top : rect.bottom) - 1`; one element that slides: `transition top,left,width 60ms ease-out` (disabled while the list edge-scrolls and for reduced motion); 4px circular end cap left of the line (`before:` `size-1 border-2` at `-left-1.5`, `-translate-x-0.5`); for horizontal strips a 2px vertical line `top+4`, height `rowHeight - 8`. "Into": no line; the target row fills with nest tokens (part A 5.1). Never both. |
| Pane drop | over a pane element carrying `data-pane-drop`: zone from pointer inside the pane rect with threshold **0.25** (left if nx < .25 and nearest left edge; right symmetric; else top if ny < .25, bottom if ny > .75; else centre). Centre opens the chat into that pane (merges into its view), edges split it (left/top before, right/bottom after). The pane gets attribute `data-pane-hit` (ring styled in the layout spec) and the split preview is published through the drag store. |
| Edge scroll | band **36px** at top/bottom of the scroll viewport; speed `max(1, round((1 - d/36) * 14))` px per animation frame (14 at the boundary, 1 at the band's inner edge), only while the pointer's x is within the scroller's column; re-runs the hit test after each scrolled frame |
| Commit | sequential placement calls (no parallel): reparent (operation `workspace.reparent`, awaited), workspace/folder/home-folder/repo-home/chat placement REST calls, each applying the daemon's answer plus shifted siblings to the entity store (`lib/store/applied-placement`). Failure -> `report` toast "Move"; owner-not-recorded -> toast `chatNotLoadedYet('move', name)`. |

Allowed-modes matrix (`sidebar-drop-policy.ts :: allowedModes`; `NO_MODES` = refuse; `REORDER` = before/after; `ALL`):
1. No subjects, any subject's workspace busy (operation in flight) -> refuse. Dropping onto itself -> refuse. Subjects of mixed kinds -> refuse (except on a Recents target).
2. A working chat may only be dropped where it leaves its workspace (the move waits for the turn end); no working non-chat may move at all; never onto Recents.
3. Chat subject: any chat/folder/branch target allowed (`ALL`), subject to rule 2; reordering within the same workspace is refused while working.
4. Home folder subject: only within the same project's home (to a header row only reorder).
5. Repo header (`repoIcon`) subject: onto another repo header of the same project -> reorder; onto home folders/chats of the same project -> reorder if the container is home root or a home folder; into only folders.
6. Otherwise targets must resolve to a repo in the same project; across repos only rows owning no worktree move.
7. Folder subject among branch anchors: before/after only when the nearest branch anchor of target container equals the subject's; into only when anchor of target equals.
8. Locked branch or fork with no owning chat recorded: may only reorder within its fork anchor; "after an expanded parent" refused; into only a folder at the same anchor.
9. A drop into a descendant is refused (cycle guard via `data-sidebar-path`).
Drop attributes the hit test reads from each row: `data-sidebar-{branch|folder|chat|workflow}-drop` (id), `data-sidebar-drop-parent`, `data-sidebar-path` (ancestor chain), flags `data-sidebar-expanded`, `data-sidebar-children`, `data-sidebar-recents-row`.

Native file drags into rows and files-tree rows to panes: `use-file-explorer-drag-drop.ts` (Files mode; same nest highlight on folders).

## 6. Keyboard and pointer summary

| Input | Effect | Source |
|---|---|---|
| Mod+B | toggle sidebar | `components/ui/sidebar.tsx` |
| Mod+1 / Mod+2 / Mod+3 | Chats / Files / Changes (remappable ids `navigation.sidebarChats|Files|Git`) | `features/keymaps/registry.ts` |
| Mod+Shift+1..9 | jump to project by position, 9 = last (hardcoded, `code`-based) | `features/keymaps/hooks/use-space-switcher-keyboard.ts` |
| Mod+` | toggle console dock (not sidebar) | registry |
| Esc | closes narrow-window sidebar overlay; closes inline editors | split pane, rename input |
| Enter / Space on focused row | open row | sidebar-row.tsx |
| Tab / Shift+Tab | rows are tabbable (`tabIndex 0`); no arrow navigation in the chats tree | |
| Enter/Esc/blur in inline input | confirm/cancel/confirm | inline-rename-input.tsx |
| Double click on label | rename | sidebar-tree-chrome.tsx |
| Right click row | row menu | row-context-menu.tsx |
| Wheel (horizontal dominant) / swipe | switch space | space-scroller.tsx |
| Click row | open (chat in its own pane/view; navigates to the workspace first when needed) | `components/layout/open-actions.ts` |
Mac-only: window traffic-light sync (`useMacTrafficLightSync`); not required on Linux.

## 7. Persistence

| What | Where | Key / shape |
|---|---|---|
| Sidebar width | localStorage | `sidebar-width` int px, clamped 250..640, default 294 (`use-sidebar-panel.ts`) |
| Sidebar open | localStorage | `sidebar-open` string; only `'false'` means closed (default open) |
| Active mode, folded rows, host filter, last project per host | IndexedDB store `sidebar-ui`, key `global` | `{collapsedChatRows: string[], activeTab, hostFilter, lastProjectByHost, updatedAt}`; one record, every write carries all fields (`lib/persistence/sidebar-ui.ts`, `tree-ui-store.ts :: persist`). Unknown tab -> chats. |
| Dock side, follow-focus | settings store | `sidebarPosition` ('left' default), `sidebarFollowsFocus` (false) |
| Changed-files folder collapse, files-tree expansion, search | in-memory (component/workspace stores) | not persisted across restarts (verify files tree: `features/file-explorer/stores/file-explorer-tree-store`) |
| Active project | route (`/ide/$projectId/...`) | the route is the source of truth; `lastProjectByHost` remembers per host |
GPUI shell: keep all of the above as shell-local view state in one preferences file; do not send width/open/fold to the daemon.

## 8. Daemon owns vs shell owns

Daemon (entity streams + REST; the shell reads and calls, never duplicates): projects (name, path, host, avatar/emoji), repos (name, default workspace, folders, chats, avatar), workspaces (branch, status, locked, parent, added/deleted counts, placeholder/needsProvisioning/heldByPath), chats (title, provisional flag, working, pending move, owned workspace), operations (create/reparent/detach/retry with items and problems), placement order of rows (every placement call returns the new order plus shifted siblings), unread/failed scope notices, active sessions, host list and status, git status and changed files, file tree, provider list (terminal capability), settings (persisted), removals' actual DELETEs and their cascades.
Shell (view state only): sidebar open, width, dock side, peek hover; chosen mode and shown-mode derivation; folded rows; host filter and per-host last project; row hover/focus; inline rename/branch-out/draft input state; removal hold list, countdown and hidden ids (the delete itself is daemon); drag state, ghost, indicators; carousel scroll position and gesture arming; create-space form; menu/dialog open state; toast list; files-tree selection/search; changed-files viewed checkboxes are persisted by the daemon through `useReviewViewedById` (not view state).
Rule from CLAUDE.md applies: do not copy daemon state into a second shell store; hold ids and render from the stream.

## 9. Port list (reachable = must port; unreachable = must not)

Method: `docs/triage/shell-surface-inventory.md` plus an import-graph scan (non-test importers per file) of `components/layout`, `components/sidebar`, `components/tree-dnd`, `features/window`.

MUST PORT (sidebar surface):
- Frame: `components/layout/ide-shell.tsx` (sidebar part), `sidebar-split-pane.tsx`, `sidebar-peek.tsx`, `use-sidebar-panel.ts`, `use-sidebar-focus-mode.ts`, `components/ui/sidebar.tsx` (Mod+B and open state only), `features/window/stores/sidebar-peek-store.ts`.
- Header and switcher: `sidebar-project-header.tsx`, `sidebar-host-picker.tsx`, `sidebar-project-switcher.tsx`, `project-icon-mark.tsx`, `icon-popover.tsx`, `features/hosts/components/host-status-icon.tsx`, `features/hosts/lib/{host-tone,host-filter}.ts`, `use-sidebar-host-filter.ts`, `components/ui/sidebar-toggle-icon.tsx`.
- Modes: `sidebar-body.tsx`, `sidebar-mode-bar.tsx`, `sidebar-panel-tabs.ts`, `components/ui/{tabs,selected-pill}`, keymaps `use-sidebar-tab-keyboard.ts`, `use-space-switcher-keyboard.ts`.
- Chats tree: `components/sidebar/{space-scroller,sidebar-tree,sidebar-row,sidebar-row-glyph,sidebar-row-actions,sidebar-row-removing,operation-sidebar-row,inline-rename-input,branch-out-input,row-context-menu,tree-foot,unread-notice,space-host-scope}.tsx`, `lib/{row-menu,row-actions,row-tree,row-metrics (desktop scale only; drop the touch scale),move-targets,branch-row-id,rendered-rows,chat-icon-index,drop-*,sidebar-drag-hover,sidebar-drop-dispatch,sidebar-drop-policy,pane-drop-actions,import-branches,workspace-owners}.ts`, `hooks/use-sidebar-drag.ts`, `components/tree-dnd/*`, `components/layout/{sidebar-tree-surface,sidebar-tree-chrome,workspace-row-base,workspace-branch-icon,repo-avatar,repo-icon-mark,drag-ghost,drop-indicator,edge-scroll,format-change-count,create-actions,thread-create,home-actions,open-actions,trash-actions,removal-*,draft-*,create-space-panel,create-space-daemon-row,repo-import-dialog,detach-holder-modal,icon-upload,sidebar-skeleton}.ts*`, `components/projects/add-repository-modal.tsx`, `features/workspace/tree/*` (row derivation), FlickerSpinner (`components/ui/{flicker-spinner,flicker-strip,spinners/*}`), Skeleton, ScrollArea.
- Files mode: `sidebar-files-panel.tsx`, `features/file-explorer/**` (tree, search header, dialogs, context menu, DnD), `sidebar-file-row.tsx`, `lib/sidebar-file-row-height.ts`, file-tree git status decoration.
- Changes mode: `features/git/components/{git-panel,changed-files-tree,review-merge-box,commit-changes-form}.tsx` and the hooks they use.
- Shell services: toasts (`sidebar-toast-overlay.tsx`, `toast-store`), `connection-indicator.tsx`, `operations-watcher.tsx`, `placeholder-toast-watcher.tsx`, `use-shell-route-state.ts`, `use-redirect-if-project-missing.ts`, `use-publish-focused-workspace-context.ts`, `use-ide-shell-workspace-retention.ts`, `shell-landing.tsx`, daemon halt/crash gates (`features/window/components/daemon-*`).

MUST NOT PORT (dead, mobile-only, dev-only, or owned by another surface):
- Dead: `components/layout/nav-stack.tsx` and `features/layout/stores/sidebar-nav.ts` (`push` has no caller; the pushed-screen layer, its 280ms slide and its Back header never appear). `NavStack` passes children through unchanged.
- Dev/stub: `build-readout.tsx`, `build-readout-lines.ts`, `use-readout-lines.ts`, `lib/frame-stats` and the `buildReadout` setting (inventory CUT); keep only the 40px foot spacer if layout needs the gap. `sidebar-row-drag.tsx` (phone).
- Mobile only (inventory REDESIGN / out of desktop scope): `components/layout/mobile/*`, `features/window/components/mobile-*-page.tsx`, `use-native-sidebar.ts`, `use-native-files.ts`, `use-mobile-native-chrome.ts`, `mobile-screen-store`, `row-touch.ts`, `TOUCH_ROW_METRICS` and every `useRowMetrics()` branch, `active-sessions-section.tsx` (touch only), `isHostPickerOpen` store field, native-confirm branches (`hasNativeChrome()`), `file-tree-touch` CSS.
- Dead UI-state fields: `useUIState` fields other than `isSettingsOpen`/`settingsInitialTab`/`openSettingsDialog`/`setIsSettingsDialogVisible` (sidebarWidth, bottomPane*, activeSidebarView, activeRightSidebarView, isRightSidebarVisible, quick open, agent launcher, global search, project picker flags) are not read by the sidebar; confirm with knip before porting. The sidebar width in that store (260) is a decoy; the real default is 294.
- Belongs to other surfaces: `components/sidebar/view-row.tsx`, `tab-strip-drag.ts`, `hooks/use-view-tab-menu-items.tsx`, `lib/recents-*.ts` (tabs bar); the `tab`/`activeGround`/`inlineRenameDisabled` props of `SidebarRow` exist only for the tabs bar; `console-dock` (layout spec); settings dialog (settings spec).
- Cut per inventory: commit-diff pane items, hunk staging, editor-group commands (none are in the sidebar; listed to avoid porting through shared utilities).

## 10. Edge cases to preserve
- A click during rename/branch-out must not open the row; a pointer click must blur the row/button so hover-only controls hide.
- Selecting the already-active project must not re-trigger scroll (no event fires; arming guard).
- Resizing the sidebar re-aligns the carousel without animation; collapsed (width 0) skips alignment.
- Drag over the sidebar edge must not scroll the carousel (pinned during drag).
- Tooltips on header buttons use `side=bottom`; row buttons use `title`/aria only.
- Remote project unreachable: rows grey (token re-point), still clickable; a `+`-less switcher when no add handler.
- With a single host, no host picker is shown and `shownHost` is undefined (all projects listed).
- A row whose id appears both in the tree and in Recents (tabs bar): rename editor appears on the tree copy if mounted else the Recents copy (`renamingRole`).
- Held removal rows keep their id so children/menus still resolve; hidden ids are derived with the primary row kept visible for workspace removals (`renderedHiddenIds`).
- Create bursts: one in-flight create per (kind, parent) key.
- `sidebar-open` defaults open; width over/under range is clamped on load.

## 11. States to capture live (checklist for the lead)

Name each capture `sidebar-<id>`; "reach" = how.

| Id | State | Reach |
|---|---|---|
| A1 | Docked, default 294px, light and dark app mode | fresh start; toggle Appearance mode |
| A2 | Width at 250 and 640 (clamps) | drag separator to extremes; window must be wide enough |
| A3 | Collapse/reopen: frames at 30fps around Mod+B (verify instant track change) | Mod+B |
| A4 | Separator hover/focus/drag, collapse while dragging at < 125px | mouse |
| A5 | Dock right | Settings > Appearance > Sidebar position |
| A6 | Narrow window < 640px drawer + scrim | resize window |
| B1 | Peek: closed -> peeking -> closed, frame sequence of slide (200ms) | collapse, move pointer to x<=6 |
| B2 | Peek card surface (popover bg vs chrome) | same |
| C1 | Header buttons hover and tooltips ("Settings", "Hide Sidebar") | mouse |
| C2 | Host picker closed, menu open, remote host dot states | needs second host (may not be capturable; document) |
| D1 | Switcher: 1 project vs several (+ reveal on hover, 150ms) | create second space |
| D2 | Project name rename, icon popover, host line (remote) | double-click, click icon |
| E1 | Mode bar: width < 280, 280-420, > 420; each active tab; Files disabled; Changes hidden | resize sidebar; focus no workspace vs repo workspace |
| E2 | Pill slide 200ms between modes | Mod+1/2/3 frame sequence |
| F1 | Chats tree: repo header, branch rows (1 and 2 line, with counts), folders open/closed, chat rows, locked, placeholder, working spinner, open-but-idle | seeded data |
| F2 | Row hover with cluster per row kind; focus ring | mouse/keyboard |
| F3 | Rename input, Make-a-branch input with error, draft fork input | double-click, hover button |
| F4 | Operation row running and failed; pending move sublabel | trigger create; drop working chat |
| F5 | Removing row: countdown numerals 8..1 and the drain bar (frame sequence) | click X on a chat |
| G1 | Row context menus (native GTK menu in reference): chat, folder, branch, locked, repo header; Move to submenu; tree-foot More… | right click |
| G2 | Confirm dialogs: delete repo, delete space, work-at-risk, detach | menus |
| H1 | Drag: ghost, line between rows, nest fill, edge scroll, drop onto pane zones (center/left/right/top/bottom) | mouse drag with xdotool |
| I1 | Carousel swipe between spaces (smooth scroll frames) | create 2 spaces, wheel horizontal |
| J1 | Create-space panel: empty, filled, loading, remote hint | switcher + |
| K1 | Skeleton (before projects sync), unread notice, unreachable host greyed | restart daemon quickly; needs failure injection (may be a gap) |
| L1 | Files mode: tree, hover/selected/ignored/cut rows, search header + filter menu, context menu, empty states, inline new file | Mod+2 |
| M1 | Changes mode: branch pill, merge box (each strategy; blocked reasons), commit form, changed tree with viewed checkboxes, no repo state | Mod+3 on a branch workspace with edits |
| N1 | Toasts in sidebar (open) and collapsed; pinned "Backend unavailable"; error shake not used | trigger errors; stop daemon |
| O1 | Computed styles for: row, header button, switcher tile, mode tab, pill, separator, toast, menu item (verify the numbers in part A) | getComputedStyle via bridge |

## 12. Flows: intent versus observed bugs

Owner rule: the Rust shell implements INTENT; "Observed" items are what the React code does wrong or works around and must NOT be copied. Visuals above stay 1:1.

### 12.1 Row drag and drop (reorder, nest, open in pane)
Intent
- Pressing and moving a row more than a small distance picks it up; a ghost follows the pointer; the target under the pointer shows exactly one signal (a line between rows, or a filled row to nest into); releasing commits that placement; Esc/leaving the window cancels with no change.
- Dropping on a pane opens the chat there (centre) or splits (edges). Dropping on a row never also "clicks" it.
- Illegal drops show no indicator (never a misleading one) and do nothing.
- The list scrolls when the pointer holds near its top/bottom edge.
- Order shown after the drop is the daemon's answer, never a guess.
Observed bugs and workarounds
- Release outside the webview is invisible to the page; recovery relies on "next pointermove has `buttons===0`", window blur, and pointercancel (three guards for one event).
- Selection is blocked by a document `selectstart` listener added at press and removed on every exit path; leaking it breaks text selection app-wide.
- The click following a drop is swallowed with a one-shot capture listener removed by `setTimeout(0)`.
- The ghost is a DOM deep clone with classes stripped by name list (`AMBIENT`), fragile to any new row styling.
- The carousel is pinned (`overflow-x:hidden; will-change`) during a drag only to stop WebKit snapping a panel; hence drags can never cross spaces.
- Edge scroller needs the drop line's transition disabled while running (`setDropLineTracking`) or it trails.
- The allowed-modes matrix reads store state (`getState`) inside pointer code on every move; the rules (section 5) are accreted special cases (home folders, repo headers, locked/ownerless branches) and several comments cite spec section numbers that no longer exist in the repo.
- Tree drag is single-row only but the ghost stack/count badge path exists for tabs only.

### 12.2 Move chats between spaces/projects and workspaces
Intent
- A chat can be moved to any folder, repo or branch of the SAME space via drag or a "Move to…" menu (keyboard twin of the drag). A working chat's move to another workspace waits for its turn to end and the row says "Moving to <x> after this turn" with a "Cancel move".
- Moving between spaces is not offered in the tree.
Observed bugs
- The matrix says "only a chat crosses a project" but no UI path exists: only the active space's panel is mounted/hit-testable, `moveTargets` is computed from one project's rows, and the carousel is pinned during drags. Treat cross-space chat move as dead; if wanted it needs an explicit product decision and UI.
- "Move to…" duplicates the drop plan by calling `performSidebarDrop([row], target, 'into')`, and filters targets by running the real planner and checking for an empty plan (an empty plan means "no-op" is used as a signal).
- A fork with no recorded owning chat cannot be reparented: refused with "still loading" toast `chatNotLoadedYet` (race between entities arriving).

### 12.3 Close, restore, removal tray and undo
Intent
- Removing a chat, folder or workspace hides it at once, shows a countdown (8 s) with Keep, and only then deletes; Keep restores it exactly. Deleting a repository or space asks first, spells out the cascade, and names unsaved work at risk ("Delete anyway").
- Panes showing a removed chat close immediately, and the window leaves a removed workspace to a sensible fallback.
- The delete must still happen if the app quits during the countdown.
Observed bugs and workarounds
- Three clocks for one concept: a one-shot timer aimed at the soonest deadline, a once-per-second DOM text writer (`removal-clock.ts`) aligned to second boundaries, and a CSS 8 s drain animation hard-coded to 8 s independent of the actual `deadlineAt` (so a custom `drainMs` or a resumed entry draws a wrong bar). The text is written into the DOM outside React.
- Persistence on quit relies on `pagehide` + keepalive fetch (64 KB cap); clockless repo/project holds are dropped on unload.
- "Hidden ids" kept separate from the tray entries, plus `renderedHiddenIds`/`primaryRowId` to keep the owning chat row visible during a workspace hold so the row does not re-key.
- The repo home row resolves to the default branch, so trashing it would delete one branch; a separate `handleTrashRepo` exists and `draftFor` returns null as a "safe no-op" for the home row.
- The daemon's folder list leaks a home folder into every repo, so home rows are resolved first (`trash-actions.ts` comment).
- A refused delete over unsaved work re-queues the entry as a clockless confirm entry (`askToDiscard`) with a fresh id.
- Restore of closed TABS (Ctrl+Shift+T) belongs to the tabs surface; there is no restore of a committed deletion.

### 12.4 Rename, create, branch-out
Intent: double-click or menu starts an inline edit that confirms on Enter/blur and cancels on Esc; a refused name keeps the input open with the reason; creating a fork asks for a branch name first.
Observed: rename role (`tree` vs `recents`) exists only because the same chat id can render in two places; `blur()` after every button click to cancel stuck `group-focus-within` actions; one in-flight create per (kind,parent) key to avoid duplicate chats from click bursts (daemon race with the runner's startup).

## 13. Workarounds and suspected bugs (consolidated)

Timers, polls, resync, flags (all to be replaced by explicit owned state in Rust):
1. `SpaceScroller` gesture arming (`isUserGesture`, `settlingToIndex`, ResizeObserver re-align, bounce-back on non-user scroll): emulates "active project is the source of truth" on top of a scroll container. Native: model carousel offset as animation toward the active index; user swipe sets active on settle.
2. `SidebarPeek` hit-test on every pointermove, `inert`, and a `transitionend` listener to learn when the card has left (`occupied` store); reduced-motion special case.
3. `use-sidebar-focus-mode`: `interacted` ref, `lastKind` ref, `focusin` listener to follow focus; ignores startup focus.
4. Sidebar width: `data-pane-resizing` attribute on `<html>` that strips shadows/radii/backdrop blur of other panes while dragging (perf workaround), `pane-resize-end` custom event for virtualisers to flush.
5. Row-level: `contain-layout`, `hidden` (not `invisible`) hover buttons that reflow the row on every hover to let the label measure; `touch-action` pan axes; `e.currentTarget.blur()` after clicks.
6. `ConnectionIndicator`: 500 ms debounce for "Backend unavailable"; shared toast key to avoid stacking; "Reconnected" only when the warning was shown.
7. `ShellLanding`: `PERSISTENT = 2` failed snapshots to distinguish restart from failure.
8. Space switcher keyboard: stale comment in `ide-shell.tsx` says Cmd/Ctrl+1-9 selects a space; the real binding is Mod+Shift+1..9 (Mod+1..3 are sidebar modes).
9. `Mod+B` handler: global, no input-focus or repeat guard, hard-coded (not in the remappable keymap registry) so it cannot be remapped.
10. `useUIState` carries many dead fields (right sidebar, quick open, global search, project picker, bottom pane, `sidebarWidth: 260` decoy); `NavStack` and `useSidebarNavStore.push` are dead; `SidebarFoot` is an empty 40px spacer in release.
11. Dead drop branches: cross-project chat drop (12.2), `data-sidebar-workflow-drop` kind (`workflow` row kind has no producer in `deriveRows`; `kind` union still lists it) - confirm before porting.
12. Row glyph colours for PR states are hard-coded Tailwind hues (amber/green/red/violet-500), outside the token system.
13. Flicker spinner picks a random asset per instance (non-deterministic visuals; parity captures must pin it) and bakes SMIL into a sprite strip as a WebKit perf workaround.
14. Folded rows and mode live in IndexedDB while width/open live in localStorage (two stores for one concern) and the sidebar-ui record rewrites all fields on every change.
15. Context menus: native OS menus cannot show icons or destructive styling, so two menu implementations exist (native and rendered fallback) with a documented divergence.
