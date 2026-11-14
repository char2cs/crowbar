# Shell spec: tabs top bar (window chrome and the view-tab strip)

Surface 3 of 4. Part 1 of 2 (part 2: `tabs-pane-strip.md`: per-pane editor tab strip, New Tab page, shortcuts, persistence, port list, live checklist).

All paths are relative to `app/panes/src/` unless they start with `app/`. `file:line` cites the source the fact came from. Colour hex values and the full token table belong to `00-tokens.md` (fork 1); this file names the token and gives the value only where it is needed to read the spec (values quoted from `styles/theme.css`, `styles/crowbar.css`). Items marked **VERIFY LIVE** are static-analysis conclusions the lead must confirm in the running app.

## 0. Vocabulary: two strips, do not confuse them

| Name | What it is | Where drawn | Source |
|---|---|---|---|
| **View-tab strip** ("the tabs top bar") | One tab per *view* (a view = one chat, or a split of 2+ chats). Spans the full width of the content column, above all panes. Drawn only while the active project has at least one view. | top of content column | `features/tabs/components/view-tabs-row.tsx:101-183`, mounted at `components/layout/ide-shell-workspace-content.tsx:24` |
| **Pane tab strip** | Editor tabs (files, terminals, browser, branch review, New Tab page) of one pane. Lives inside each pane (above the editor view; or the pane's top row in collapsed mode). | inside pane | `features/tabs/components/tab-bar.tsx` (part 2) |
| **Chat header row** | Title/branch header of a chat that fills its pane, same row shell. | inside pane | `features/tabs/components/chat-only-pane-header.tsx` (part 2) |

The migration plan's "tabs top bar" is the **view-tab strip** plus the window chrome around it (title-bar inset, sidebar toggle, drag region). The pane tab strip is part of "chat organisation within a tab" but is specified here because it shares the row shell (`PaneTopRow`).

## 1. Window chrome

| Item | Value | Source |
|---|---|---|
| Window default size | 1200 x 800, resizable | `app/mobile/src-tauri/tauri.conf.json:26-28` |
| macOS | `titleBarStyle: Overlay`, `hiddenTitle: true`, `transparent: true`, traffic lights at x=12, y=23 | `tauri.conf.json:29-35` |
| Linux | no overlay, no transparency, **native OS title bar and frame** (Tauri default decorations); the web content starts below it | `app/mobile/src-tauri/tauri.linux.conf.json:3-11` |
| Background throttling | disabled (all platforms) | `tauri.conf.json:36`, `tauri.linux.conf.json:10` |
| Window title text | `"<chat title> — Crowbar"`, or `"Crowbar"` when the active pane has no titled chat | `features/window/hooks/use-document-title.ts:6-15` |
| Custom window buttons (min/max/close) | **None.** No component draws them. Linux uses the OS frame; macOS uses the native traffic lights | grep of `src/` for window-controls: none |
| Drag region | `data-tauri-drag-region` on the pane-top row element and on the empty scroller of each strip; Tauri drags only on the exact element carrying the attribute, so tabs/buttons stay interactive | `features/tabs/components/pane-top-row.tsx:61`; `view-tabs-row.tsx:152`; `tab-bar.tsx:384` |
| Row height | `CHROME_ROW_HEIGHT_PX = TAB_HEIGHT 32 + 2 * TAB_GAP 5 = 42` | `lib/chrome-metrics.ts:1-6` |
| Traffic-light Y | `round(42/2)+2 = 23` | `lib/chrome-metrics.ts:9`; matches `tauri.conf.json:34` |
| Traffic-light inset on the row | `pl-[88px]` (88 px) **only on macOS**, only on a row that is flush to window left (rect.left < 20) AND top (rect.top < 20) and not the bottom pane | `pane-top-row.tsx:57`; `EDGE_THRESHOLD = 20` at `features/tabs/hooks/use-pane-top-row-edges.ts:6` |
| Traffic-light follow (macOS, sidebar on the right) | lights moved to `(row.left + 12, row.top + 23)` of the top-left row, re-applied on window resize, theme change, OS colour-scheme change; left sidebar: fixed (12, 23) | `features/tabs/hooks/use-mac-traffic-light-sync.ts:94-178` |
| Zero-size rows | a `display:none` (parked) row has no box and is ignored for the corner test; once a real box was measured that answer is kept | `use-pane-top-row-edges.ts:25-31`, `use-mac-traffic-light-sync.ts:103-111` |

GPUI port note (Linux/XWayland per memory): there is no inset to port on Linux; the macOS inset and light-follow are macOS-only and are only verified by CI per the plan. Keep the 88 px constant and the corner test in the model so the macOS build is correct.

Window zoom / UI scale: there is no window-level zoom in the tab bar. `mod+=`, `mod+-`, `mod+0` (commands `agent.zoomIn/Out/Reset`, labelled "Zoom chat in/out", "Reset chat zoom") drive `useZoomStore` (`features/window/stores/zoom-store.ts:23-39`): step 0.1, clamp 0.3..3, rounded to one decimal, reset sets zoom, editor zoom and terminal zoom all to 1; one step per press (`e.repeat` ignored, `features/keymaps/hooks/use-zoom-keyboard.ts:14-18`). The chrome rows do not scale with it; `--ui-text-*` scale with `--app-ui-scale` (a setting, `styles/theme.css:105-109`).

## 2. The view-tab strip: row anatomy

```
x=0                                                                              x=W
|8| [reopen toggle 28x28?] |6| [ tab slot ][ tab slot ] ... [ tab slot ] |8|
 ^pl                                                                      ^pr
y: 0..42   row bg transparent (chrome shows through), tab shells at y=5..37
```

Row = `PaneTopRow` with `className="chrome-surface bg-transparent py-0 pr-2"` (`view-tabs-row.tsx:137-143`).

| Property | Value | Source |
|---|---|---|
| Height | 42 px (inline style) | `pane-top-row.tsx:60` |
| Layout | `flex shrink-0 items-center gap-1.5 px-2 py-1`, then `py-0 pr-2` override: padding-left 8, padding-right 8, vertical 0, child gap 6 | `pane-top-row.tsx:53`, `view-tabs-row.tsx:142` |
| Overflow | `relative overflow-hidden` | `pane-top-row.tsx:54` |
| Fill | transparent. The window's translucent chrome wash (`--chrome-bg`) shows through | `view-tabs-row.tsx:142` |
| Palette | `.chrome-surface` re-paints text tokens with the **dark palette in light mode too** (Crowbar theme paints chrome dark): foreground `#f2f0ea`, muted-foreground `oklch(0.834 0.008 278.6)`, border `#333330`, `--sidebar-element-hover oklch(1 0 0 / 10%)`, `--elevated-highlight = 16% foreground` | `styles/crowbar.css:25-31`, `styles/theme.css:166-250` |
| Render condition | draws nothing when the view list is empty (the pane's own top row then carries the chrome) | `view-tabs-row.tsx:124` |
| Z-order | `relative z-[1]` column; ghost `z-50`, drop line `z-[60]` (both portalled to `<body>`) | `ide-shell-workspace-content.tsx:23`, `components/layout/drag-ghost.tsx:54`, `drop-indicator.tsx:193` |

### 2.1 Which views are listed, in what order

- Scope setting `tabListScope`: `'project'` (default) lists the active project's views in band order, `'all'` lists every view open on this client (`features/tabs/hooks/use-tab-view-ids.ts:7-14`, default at `features/settings/config/default-settings.ts:20`).
- Order = `viewOrder`, always with **pinned views first** (each group in its own order). `settlePinnedOrder` is re-applied after every write that can break it (`features/panes/lib/view-pins.ts:5-11`, `view-actions.ts:135`).
- Pin: view joins the **end of the pinned group**; unpin: joins the **start of the unpinned group** (`view-pins.ts:13-23`).
- A view with zero chat members renders nothing (`view-tabs-row.tsx:186-187`, `view-row.tsx:111`).

### 2.2 The sidebar-reopen toggle (lives in the strip)

| Condition | Shown | Source |
|---|---|---|
| sidebar closed AND `sidebarPosition === 'left'` AND the row is flush left | at the **left** end, before the scroller | `view-tabs-row.tsx:144` |
| sidebar closed AND `sidebarPosition === 'right'` AND row flush right | at the **right** end, after the scroller, icon mirrored (`scale-x-[-1]`) | `view-tabs-row.tsx:174`, `tab-navigation-buttons.tsx:30` |
| sidebar open | not shown (the sidebar header owns the toggle) | same |

Toggle: `Button variant=ghost size=icon-sm`, wrapper `h-8` (32), button `size-8 sm:size-7` = **28x28 on desktop** (viewport >= 640), `rounded-sm` (6 px), icon `SidebarToggleIcon` `size-4` (16, stroke 2, rect 3,4 18x16 rx 2.5, divider x=9, two rail lines), colour `text-muted-foreground`, hover bg `--sidebar-element-hover`. Tooltip "Show Sidebar"/"Hide Sidebar" (aria "Show sidebar"/"Hide sidebar"), side bottom, delay 150 ms. Sources: `view-tabs-row.tsx:126-135`, `tab-navigation-buttons.tsx`, `components/ui/button-variants.ts` (size map: `icon-sm: size-8 sm:size-7`), `components/ui/sidebar-toggle-icon.tsx`, `components/ui/tooltip.tsx:15`.

The dev-only `BuildReadout` anchor (zero-width element at `view-tabs-row.tsx:169-173`) is **cut** (not in release; inventory).

## 3. View tab: anatomy

### 3.1 Slot (flex item in the scroller)

| Property | Value | Source |
|---|---|---|
| `flex` | `1 1 240px` | `view-tabs-row.tsx:193` |
| `min-width` / `max-width` | **112 px / 240 px** | `view-tabs-row.tsx:94-95,194-195` |
| Behaviour | all tabs share the strip equally between 112 and 240; once every tab is at 112 the strip scrolls horizontally. A lone tab is 240 wide | comment `view-tabs-row.tsx:93-94` |
| `data-pinned` | attribute on the slot when the view is pinned (used by drag logic) | `view-tabs-row.tsx:191` |

Scroller: `tab-scrollbar -my-1 flex min-w-0 flex-1 items-center overflow-x-auto overflow-y-hidden py-1`, `role=tablist`, `aria-label="Open views"`, `data-drop-axis="x"` (`view-tabs-row.tsx:145-155`). Scrollbar hidden (`scrollbar-width:none`, webkit scrollbar `display:none`, `index.css` `.tab-scrollbar`). The `py-1` / `-my-1` pair gives the lifted tab 4 px of clip room above/below with zero layout change: in flow the scroller is exactly 32 high, vertically centred in the 42 row, so the **tab shell occupies y = 5..37**. There are **no overflow arrows, no fade masks and no overflow menu** on the view strip (the "More tabs" caret exists only on the pane strip, part 2).

Scroll input: native wheel/trackpad scrolling of the overflow container; no custom wheel handler on this strip. While a row drag runs, the strip scrolls itself by edge-scroll (section 5.4) (`components/sidebar/hooks/use-sidebar-drag.ts:223`).

### 3.2 Shell (the painted pill)

`TAB_SHELL = 'group/tab relative mx-0.5 flex h-8 items-center rounded-full border'` (`components/sidebar/view-row.tsx:172`).

| Property | Value |
|---|---|
| Height | 32 |
| Horizontal margin | 2 px each side (slot width = shell width + 4, so adjacent shells are 4 px apart) |
| Radius | `rounded-full` (pill) |
| Border | 1 px (colour per state) |
| Shell width | slot width - 4 (range 108..236) |

Shell states (all on the **shell**, not the inner row; inner row hover is nulled by `[&_[role=tab]]:hover:bg-transparent`, `view-row.tsx:243`):

| State | Fill / border / shadow | Text | Source |
|---|---|---|---|
| Idle (`TAB_IDLE`) | border transparent, no fill | `--foreground` (chrome palette `#f2f0ea`), muted for non-showing rows (`text-muted-foreground`, see 3.3) | `components/layout/workspace-row-base.ts:16-18` |
| Hover (idle) | bg `--sidebar-element-hover` (`oklch(1 0 0 / 10%)` in chrome palette), `shadow-xs shadow-black/10` = `0 1px 2px 0 rgb(0 0 0 / 10%)`, `inset-shadow 0 1px 0 var(--elevated-highlight)` | unchanged | `workspace-row-base.ts:16-18` |
| Active / showing (`TAB_ACTIVE`) | class `control-surface`: bg = border-color = outline-color = `--control-surface` **`#fcfcfa` in both themes**; `outline 1px`; `shadow-xs shadow-black/10`; `inset 0 1px 0 var(--control-highlight)`. `--foreground` re-pointed to `--control-foreground #1b1b19`, `--muted-foreground` to `oklch(0.4 0 0)`, `--elevated-highlight` to `--control-highlight = 16% of #1b1b19` | dark ink on near-white | `view-row.tsx:196-198`, `index.css` `@utility control-surface`, `styles/crowbar.css:10-14` |
| Active, pressed (`:active` on the shell) | inset becomes `0 1px 0 rgb(0 0 0 / 8%)`, outer shadow removed | | `view-row.tsx:198` |
| Focus (keyboard) | inner row: `focus-visible:ring-2 ring-ring ring-offset-1 ring-offset-background`; no ring on mouse click (the row blurs itself on click) | | `workspace-row-base.ts:5-8`, `components/sidebar/sidebar-row.tsx:294-302` |
| Dragged source | inner row `opacity-40` while the ghost is out of the strip band; `visibility:hidden` (empty gap) while the pointer is in the band | | `sidebar-row.tsx:287`, `components/sidebar/lib/tab-strip-drag.ts:317` |
| Nest target (a drop would merge into it) | inner row `border-sidebar-drop-nest-edge bg-sidebar-drop-nest text-foreground` (`--sidebar-drop-nest = 30% secondary`, edge `65% secondary`; `--secondary = --primary`, light `#606c38`, dark `#98a86a`) | | `workspace-row-base.ts:27`, `sidebar-row.tsx:283` |

Colour cross-check to capture live: active pill is `#fcfcfa` in light AND dark; idle text on the dark chrome wash.

### 3.3 Inner row (single-chat tab, `tab='tab'`) and set member (`tab='tab-member'`)

Row = `SidebarRow` with `ROW_BASE` overridden for tabs (`sidebar-row.tsx:281-293`, `workspace-row-base.ts:4-9`):

| Property | Value |
|---|---|
| Box | `flex items-center gap-1.5 rounded-lg border border-transparent px-1.5 text-[13px] font-medium cursor-pointer select-none contain-layout`, tab overrides: `mx-0 my-0 h-8 px-1`. So height 32, **padding 4 px left/right**, gap 6 |
| Row flex | `relative min-w-0 flex-1` inside the shell (`view-row.tsx:242-247`) |
| Font | UI face `CalSansUI` (variable, 300-700; `styles/theme-fonts.css`), 13 px, weight 500 (`font-medium`) |
| Glyph box | `ROW_GLYPH_BOX = inline-flex size-4` (16x16), `shrink-0`, centred (`workspace-row-base.ts:71`) |
| Glyph (idle) | Phosphor `ChatsCircle` `size-4` weight regular; a chat owning a worktree uses `GitBranch` weight fill; a branch row with workspace status shows the workspace status icon; locked: `Lock` fill (`components/sidebar/sidebar-row-glyph.tsx:67-70,100-116`) |
| Glyph (working) | `FlickerSpinner` `size-3.5` (14 px), stepped sprite-strip animation (`flicker-strip`, `index.css`), random one of `components/ui/spinners/*.svg` per instance, paused when off-screen via IntersectionObserver; exempt from reduced-motion (`data-essential-motion`) (`sidebar-row-glyph.tsx:500`, `components/ui/flicker-spinner.tsx`) |
| Set member | `tab='tab-member'` drops the resting glyph (spinner still shows while working) (`sidebar-row.tsx:315`) |
| Label | `min-w-0 flex-1 truncate` (ellipsis); italic when title is provisional; **muted** (`text-muted-foreground`) when the row's view is not showing (`hasView = !isShowing`) (`sidebar-row.tsx:384-392`, `view-row.tsx:132`) |
| Label fallback strings | provisional/unknown chat: `"Loading…"`; draft chat: `"New thread"`; known but untitled: `"Untitled chat"` (`view-row.tsx:60-61,78`, `features/agent/lib/chat-label.ts:2`) |
| Branch second line | **never** in a tab (`showBranchSecondLine` requires `!tab`) |
| Trailing actions | none rendered (no `onClose`/`onTrash`/`onCreate` passed); the pin/close button is the shell's (3.4) |
| Pending move | text replaced by nothing special in tab mode (`pendingMove ... && !tab`) |
| ARIA | `role=tab`, `aria-selected = !row.hasView`, `aria-label = label`, `tabIndex=0`, Enter/Space opens (`sidebar-row.tsx:273-310`) |
| Inline rename | **disabled** in tabs (`inlineRenameDisabled`): a tab cannot be renamed in place; rename is a sidebar-tree action (`view-row.tsx:259`) |
| Click | blur + `focusRecent(viewId)`: pick the member pane most recently active (else first), `setActivePane`, then route to the chat's workspace (`components/sidebar/lib/recents-actions.ts:14-26`) |

Set (2+ members): one shell, members side by side, each `flex-1` sharing the shell equally; between a non-last member and the next a **hairline 1 px wide x 16 px high `bg-foreground/15`**, vertically centred at the member's right edge, drawn **only while the set is not the showing one** (`view-row.tsx:119-141,262-267`). Whole set is one drag unit and one close target (`view-tabs-row.tsx:110-117`).

### 3.4 Close / unpin button (the "×")

| Property | Value | Source |
|---|---|---|
| Position | `absolute right-1.5 top-1/2 z-10 size-6 -translate-y-1/2` = **24x24, 6 px from the shell's right edge, vertically centred** | `view-row.tsx:173-174` |
| Shape | `rounded-full`, text `--muted-foreground` | same |
| Glyph | Phosphor `XIcon` size 12 | `view-row.tsx:165` |
| Visibility | **always visible** (not hover-revealed), so no label ever moves | comment `view-row.tsx:119-120` |
| Hover | bg `--sidebar-element-hover`, text `--foreground` | `view-row.tsx:163` |
| Action | `closeRecent(viewId)` = `paneActions.closeView`: removes the view record and all its panes; **chats are not killed** (agents keep running), the next view of the same project/workspace takes focus, else the stage (`recents-actions.ts:29-31`, `features/panes/lib/view-ops.ts:41-66`) |
| Aria | `aria-label="Close view"` (no tooltip) | `view-row.tsx:160` |
| Label fade | the **last** member's right end is masked: `mask-image: linear-gradient(to right, black calc(100% - 34px), transparent calc(100% - 24px))`, so the label fades out under the button | `view-row.tsx:244-245` |
| Pinned view | the × is replaced by a ghost Button (same box, `opacity-60`, `!min-h-0 !min-w-0 !p-0`) holding Phosphor `PushPinIcon` `size-3.5` `fill-current text-secondary`; tooltip/aria "Unpin tab"; click unpins. Not closable by click while pinned | `view-row.tsx:143-157` |
| Keyboard hint | none on the strip button | |

**Pinned tabs are NOT narrower** in code: the slot style has no pinned branch (`view-tabs-row.tsx:190-198`) even though the type comment says "compact" (`features/panes/types/pane.ts:240`). VERIFY LIVE and replicate what is rendered.

### 3.5 Tab context menu (view tab): **VERIFY LIVE, probably unreachable**

Intended items (prepended before the standard row menu): `Pin Tab`/`Unpin Tab` (Phosphor `PushPin`/`PushPinSlash`), `Close` (`X`), separator; then the sidebar row verbs (Rename, Move to..., Make a branch, Cancel move, Kill agent, Delete...) (`components/sidebar/hooks/use-view-tab-menu-items.tsx:8-22`, `components/sidebar/row-context-menu.tsx:120-150`).

But the listener resolves the row with `e.target.closest('[role="treeitem"]')` (`row-context-menu.tsx:76`) and tab rows carry `role="tab"` (`sidebar-row.tsx:273`), so **statically no custom menu opens on a view tab**. The inventory saw the native WebKit menu (Back/Reload/Inspect) leak over the sidebar. Capture right-click on a view tab; if no menu appears, the Rust shell must not invent one without owner approval; flag as an issue (do not fix in the port silently).

When a context menu does open it is native (muda) in the desktop app: `isDesktopApp()` -> `showNativeContextMenu`, rendered popup only as fallback (`components/ui/context-menu.tsx:196-232`). The popup fallback (min-width 180, `rounded-lg border bg-popover`, `p-1`, item `min-h-7 px-2 py-1 text-sm gap-2 rounded-sm`, highlight `bg-accent`, separator `mx-2 my-1 h-px bg-border`, z 10040) is the web look (`context-menu.tsx:43-47`). Native menus ignore icons and `keybinding` nodes (`context-menu.tsx:16-24`), so on Linux/macOS the menu has **text only + accelerator**; GPUI should draw its own menu in the same text-only form unless the owner wants icons.

### 3.6 Tooltips on the strip

None on tabs. The sidebar-toggle and pin button have tooltips (150 ms hover delay, side bottom, `sideOffset 6`, `collisionPadding 8`, bubble `rounded-lg border-border/70 bg-card/95 px-2.5 py-1.5 ui-text-sm shadow-lg backdrop-blur-sm`, enter `fade-in-0 zoom-in-95 slide-in-from-top-1`) (`components/ui/tooltip.tsx:9-60`).

## 4. State machine: a view tab

| State | Entered by | Visual | Exit |
|---|---|---|---|
| idle (not showing) | default | no fill, muted label | hover, press, activate |
| hover | pointer enters | `TAB_IDLE` hover fill + faint shadow | pointer leaves |
| showing | `activeViewId === viewId` | raised white pill, dark text | another view activated |
| pressed | mousedown on shell (`:active`) | inset highlight flips to black 8%, drop shadow off | mouseup |
| pending drag | pointerdown on row | nothing yet; text selection blocked document-wide | move > 8 px => dragging; release => click |
| dragging | threshold crossed | source hidden/dimmed, ghost follows, tabs slide | drop / cancel (section 5) |
| working | agent turn or workspace operation on a member | glyph swapped for the spinner (14 px) | turn ends |
| pinned | `views[id].pinned` | pin icon in place of ×, tab moves to the front | unpin |
| set | 2+ members | members share the shell, no resting glyph | a member closes/detaches |
| keyboard-focused | Tab key | focus ring on inner row | blur |

No renaming state, no preview state, no dirty/unsaved dot on view tabs. There is no unread indicator on the view strip (an unread notice exists in the sidebar only; not part of the tab).

## 5. Drag and drop in the strip (custom pointer engine, not dnd-kit)

The view strip uses the **sidebar drag engine** (`components/sidebar/hooks/use-sidebar-drag.ts`) with `createStripDrag` (`components/sidebar/lib/tab-strip-drag.ts`). dnd-kit is used only for the pane strip (part 2).

### 5.1 Constants

| Constant | Value | Source |
|---|---|---|
| Press-to-drag threshold | **8 px** Euclidean from press point (button 0 only; ignored if already dragging) | `use-sidebar-drag.ts:41,152,272` |
| Slide duration/easing | `transform 150ms ease-out` | `tab-strip-drag.ts:87,134,220` |
| Hysteresis | 6 px of pointer travel past a slot boundary before it changes hands | `tab-strip-drag.ts:100,254-260` |
| Band margin ("in band") | pointer within `tab height` (32 px) of the strip box (x: strip rect; y: first tab's top..bottom) | `tab-strip-drag.ts:182,225` |
| Edge scroll band | 36 px at each strip end; step = `max(1, round(t*14))` px/frame, `t = 1 - distance/36`, rAF-driven | `components/layout/edge-scroll.ts:3-25` |
| Fallback tab width (pane handle ghost) | 160 px (+4 pitch when all tabs are pinned) | `tab-strip-drag.ts:88,179` |
| Ghost | portalled `fixed top-0 left-0 z-50 pointer-events-none`, positioned only by `transform: translate3d(x,y,0)` | `drag-ghost.tsx:40-66` |
| Ghost surface (`LIFTED`) | `control-surface outline outline-1 outline-border! shadow-md shadow-black/15 inset-shadow-[0_1px_var(--elevated-highlight)]`; always the light palette (`[data-drag-ghost]` forces `:root` tokens) | `drag-ghost.tsx:110-112`, `styles/theme.css:6-9` |
| Stack ghost (tree rows only) | up to 3 clones, offset 4 px both axes, back clones opacity 0.25, count badge `h-4 min-w-4 rounded-full bg-primary text-[10px]` at `-right-1.5 -top-1.5` | `drag-ghost.tsx:23-29,99-103,137-150` |
| Drop line | 2 px (`h-0.5`) `rounded-full bg-sidebar-drop-line` with a 4 px dot end-cap, `transition: top,left,width 60ms ease-out`, `z-60`; vertical variant for strips: x = tab edge - 1, top = tab top + 4, height = tab height - 8, width 2 | `drop-indicator.tsx:11-13,34-45` |
| Cursor | `grabbing` everywhere while dragging (`html[data-row-dragging]`); text selection blocked | `index.css` |
| Global attribute | `html[data-row-dragging]` set during drag; sidebar carousel overflow-x hidden | `use-sidebar-drag.ts:249`, `index.css` |

### 5.2 Behaviour while the pointer is inside the band (reorder in the strip)

1. At drag start the strip snapshots every tab slot rect and strip scroll (`tab-strip-drag.ts:157-170`).
2. The dragged tab's slot goes `visibility:hidden` (an empty gap) while in band (`:317`). The **ghost is a clone of the whole shell** (all members, separators, ×; `data-testid` stripped; surface replaced by `LIFTED`) (`:306-309`, `drag-ghost.tsx:160-175`).
3. Ghost x = pointer - grab offset, **clamped so it stays over the tabs** (not beyond the first/last slot, within the strip); ghost y pinned to the strip row (`shell.top`) while in band and **eased toward the free pointer across the 32 px margin** (`k = outside/margin`) so leaving the band has no jump (`:228-247,275-282`).
4. Target index = tab passes a neighbour when the ghost's leading edge crosses that neighbour's midpoint (`targetIndex`, `:54-62`); with hysteresis. Neighbours slide by one `pitch` (the dragged slot's width) via `translateX`, `150ms ease-out`; the dragged slot stays at 0 (`slideOffsets`, `:65-72`).
5. **Pinned tabs only reorder among pinned; unpinned among the rest** (`lowest/highest` bounds, `:173-177`).
6. A hit test against tree/pane targets is skipped while in band (hit = null, so no drop line, no pane zone) (`use-sidebar-drag.ts:283-286`).
7. Edge scrolling of the strip runs while the pointer is within 36 px of the strip's left/right and in band (`tab-strip-drag.ts:210-214,311-313`); strip scroll during the drag re-aims the slide (`:215-218`).
8. Release in band: `strip.drop(x)` -> the neighbour element + `'before'|'after'`; `commitRowDrop(... inRecents=true)` runs the Recents drop (`performRecentsDrop`, `components/sidebar/lib/pane-drop-actions.ts`): edges (`before/after`) reorder `viewOrder` (`reorderView`, pinned order re-settled); then `settle(x)` starts the dropped tab at the ghost's position and glides it into the slot (`transform 150ms ease-out`, set after one rAF) (`use-sidebar-drag.ts:344-352`, `tab-strip-drag.ts:334-343,131-143`).
9. No-op drop (same slot): `drop` returns null, nothing is written (`:74-85`).

### 5.3 Drops onto a tab from elsewhere

- **Middle of a tab** (mode `into`): every dragged chat is opened into that tab's view as a split (`openRecentsEntryThenMerge`, `pane-drop-actions.ts:16-35`).
- A chat dragged from the sidebar into the strip band: treated as an **external subject** = a virtual tab after the last; neighbours open a gap of one pitch where it will land (new-tab width = strip width / (n+1) clamped 112..240); on release the new tab animates from the ghost's position (`gapOffsets`, `insertIndex`, `settleNewTab`, `tab-strip-drag.ts:89-97,284-300`).
- **Working chats**: a working (agent-running) row refuses to drop onto Recents targets (policy `NO_MODES`) and a toast "Can't move <label> there" appears (`components/sidebar/lib/sidebar-drop-policy.ts:153`, `sidebar-drop-dispatch.ts:63-71`). A pane drag-up (pane handle) is allowed to open a view (`use-sidebar-drag.ts:155-157`).
- Drag a tab out of the band onto a **pane** of the current view: that is a pane drop: split zones, creates/adds a view member, never removes a view (`sidebar-drop-dispatch.ts:24-26`; zones in the layout spec).

### 5.4 Robustness rules the Rust engine must keep

Drag state mutated in place, no per-move re-render; pointer capture attempted only after the threshold (capturing earlier swallows `dblclick`); a release outside the window is detected by `buttons === 0` on the next move; window blur, `pointercancel` and teardown all end the drag and clear `data-row-dragging`; the click fired by the release is swallowed (`use-sidebar-drag.ts:56-64,260-270,361-367`).

## 6. Animation table (view strip + chrome)

| Element | Property | Duration | Easing | Trigger | Source |
|---|---|---|---|---|---|
| Tab shell hover/active fill, border, outline, shadow | colors (`transition-colors` on Tab/row; the shell itself only inherits `border`) | 150 ms (Tailwind v4 default) | `cubic-bezier(0.4,0,0.2,1)` | hover/activate | `components/ui/tabs.tsx:383` (Tab); the shell div has **no transition class**: fill change is instant (`view-row.tsx:172`) - VERIFY LIVE |
| Tab slide during drag | transform | 150 ms | ease-out | drag move | `tab-strip-drag.ts:87,220` |
| Dropped tab glide | transform | 150 ms | ease-out | release | `tab-strip-drag.ts:131-143` |
| Drop line move | top,left,width | 60 ms | ease-out | hit changes | `drop-indicator.tsx:197` |
| Working spinner | stepped `translateX` strip (`flicker-strip`) | per-asset | steps | working | `index.css`, `flicker-spinner.tsx` |
| Tooltip | opacity/scale/translate | tw-animate-css default 150 ms | ease | hover 150 ms delay | `tooltip.tsx:9` |
| Edge scroll | scrollLeft | rAF, 1..14 px/frame | n/a | pointer in 36 px band | `edge-scroll.ts` |
| Reduced motion | all transitions/animations 0.01 ms except spinner | | | OS pref | `index.css` `@media (prefers-reduced-motion)` |
| Tab appear/disappear | none (no enter/exit animation) | | | | no code |

## 7. Edge cases (view strip)

- 0 views: strip not drawn; panes' own top rows carry the chrome and the sidebar-reopen toggle (`view-tabs-row.tsx:124`, `tab-bar.tsx:210-215`).
- Strip toggle ownership: while the strip exists it owns the reopen toggle (`tab-bar.tsx` hides its own via `isAtTopEdge`; also the 20 px edge test).
- Lone tab: width 240 px max; with the sidebar closed it still starts after the toggle (28 px + 6 gap) on the left.
- More tabs than fit: every slot at 112, scroller scrolls; **active tab is not auto-scrolled into view** on the view strip (only the pane strip has that code, part 2) - VERIFY LIVE.
- A view whose chat has not loaded: label `"Loading…"`; draft chat `"New thread"`.
- Closing the showing view: next view of same project (and workspace) by most-recent pane, else stage (`view-state.ts:467-487`).
- Pinned order violated by any write: auto-repaired (`settlePinnedOrder`).
- Tab strip inside a parked/hidden workspace: zero-size row ignored for edge tests.

## 8. Persistence and ownership (this surface)

| Datum | Where it lives | Source |
|---|---|---|
| views, viewOrder, pinned flag, activeViewId, activeViewByProject, stage, bottomLayout, panes, activePaneId, mostRecentActivePaneIds, buffers | window pane store; persisted to **IndexedDB `workspace-layout`** row key `"window"` (version 4), debounced **300 ms** after any change; restored on boot and repaired (`view-repair.ts`) | `features/panes/stores/window-pane-store.ts:610-648`, `lib/persistence/workspace-layout.ts:6-26` |
| `tabListScope`, `sidebarPosition` | settings store (daemon-synced settings) | `features/settings/config/default-settings.ts:18-20` |
| chat title, working flag, workspace, repo icon of each member | daemon entities (chat records) read via the entity store | `view-row.tsx:65-87` |

**Daemon owns:** chat/workspace identity, titles, working state, branch names, change counts, settings. **Shell owns:** which views exist and their order/pinned flag/layout, active view/pane, strip scroll position, drag state, window geometry. Note: today view/layout state is persisted in the **webview's IndexedDB** (per-home webview storage, see `docs/plans/2026-11-10-webview-storage-per-home.md`), not in the daemon. The GPUI shell becomes the owner and must persist it in its own store and migrate or abandon the IndexedDB row (decision for the lead).

## 9. Intent vs observed bugs (owner rule: Rust implements the INTENT; visuals stay 1:1 for every state)

Sections 1-8 describe what the app does today. Where behaviour below differs from intent, the Rust side follows **Intent**. Items marked (owner) need the owner's confirmation. Paths relative to `app/panes/src/`.

### 9.1 View-tab drag reorder
**Intent.** Grab a tab, the others slide aside, release drops it in the gap; pinned tabs only move among pinned, unpinned among the rest; a no-op drop writes nothing; a drag never changes selection.
**Observed bugs / workarounds.** None wrong in outcome. Workarounds the engine carries because of the web view (not needed natively): delayed pointer capture so `dblclick` survives (`components/sidebar/hooks/use-sidebar-drag.ts:159-160`), swallowing the release click via a one-shot capture listener + `setTimeout 0` (`:56-64`), global `selectstart` block (`:158`), `buttons===0` recovery for releases outside the webview (`:260-270`).

### 9.2 Merge into a split (drop on a tab's middle)
**Intent.** Dropping a chat on a tab's middle opens it as a split of that tab's view; edges reorder.
**Observed bugs.** A *working* chat is refused on any Recents target, including the merge (`components/sidebar/lib/sidebar-drop-policy.ts:153`), while the comment at `use-sidebar-drag.ts:155-157` says a pane drop only opens a view and merging does not touch the agent. Intent (owner): a working chat may be merged and its view reordered; only moves that change its workspace placement while working stay deferred ("Moving after this turn", `sidebar-row.tsx:367-378`). The refusal toast text "Can't move <label> there" (`sidebar-drop-dispatch.ts:63-71`) gives no reason.

### 9.3 Move chats between tabs and panes
**Intent.** A split member can be dragged out to its own tab (pane handle grip, or the detach button), dropped onto another tab (merge), or onto a pane zone; closing one member closes only that chat's pane.
**Observed bugs.** None known in outcome. Note the ghost from the pane handle uses a fixed 160 px width when no strip exists (`tab-strip-drag.ts:88`).

### 9.4 Close, restore, reopen (views)
**Intent.** × closes the view (agents keep running); the user can bring a closed view back (reopen last closed).
**Observed bugs.** There is no way to restore a closed view: `mod+shift+t` reopens only documents (see pane spec 5.1/12). The chat is still in the sidebar, so the "restore" is a click there. Intent (owner): reopen-last-closed should include views (re-open the chat set as one view).

### 9.5 Pin (views)
**Intent.** Pin keeps a tab at the front, compact, and protects it from accidental close; unpin returns it to the start of the unpinned group.
**Observed bugs.** Not compact: no pinned branch in the slot style (`features/tabs/components/view-tabs-row.tsx:190-198`) despite the type comment (`features/panes/types/pane.ts:240`). Pinned views can still be closed via a menu Close if the menu were reachable. Pin is only reachable through the unreachable menu (9.8): **no working UI pins a view today**. Intent: Pin/Unpin in a reachable tab context menu; compact width is the owner's call (owner).

### 9.6 Working-chat refusal
See 9.2.

### 9.7 Strip scrolling
**Intent.** The showing tab is always visible; the strip can be scrolled with wheel/trackpad.
**Observed bugs.** The view strip never scrolls the showing tab into view (only the pane strip does, `features/tabs/components/tab-bar.tsx:237-250`); with more tabs than fit, a newly opened/activated view can be off-screen. Intent: scroll into view on activation.

### 9.8 View-tab context menu
**Intent.** Right-click a view tab: `Pin Tab`/`Unpin Tab`, `Close`, separator, then the chat's sidebar verbs (Rename, Move to..., Make a branch, Cancel move, Kill agent, Delete...) (`components/sidebar/hooks/use-view-tab-menu-items.tsx:8-22`, `components/sidebar/row-context-menu.tsx:120-150`).
**Observed bug.** The handler resolves the row by `closest('[role="treeitem"]')` (`row-context-menu.tsx:76`) but tab rows are `role="tab"` (`sidebar-row.tsx:273`), so nothing opens (and the in-strip branch `viewId` is dead). The native WebKit menu leaks instead (inventory). Rust implements the intended menu (text-only native-style menu; "Rename" only if owner wants in-tab rename, since in-place rename is disabled in tabs, `view-row.tsx:259`).

### 9.9 Workarounds and suspected bugs (this file)
| # | Item | Source | Verdict |
|---|---|---|---|
| 1 | tab context menu role mismatch | `row-context-menu.tsx:76` | bug, implement intent |
| 2 | pin not compact vs comment | `view-tabs-row.tsx:190`, `pane.ts:240` | mismatch (owner) |
| 3 | view strip has no scroll-into-view | `view-tabs-row.tsx` | bug |
| 4 | working chat refused on merge | `sidebar-drop-policy.ts:153` | suspected bug (owner) |
| 5 | reopen-closed ignores views | `buffer-slice.ts:266` | gap |
| 6 | focus ring suppressed by blur-on-click and mousedown preventDefault | `components/ui/tabs.tsx:372-380`, `sidebar-row.tsx:298-300` | web-view workaround, not needed natively |
| 7 | 20 px edge heuristic and two-frame wait to find the top-left row | `use-pane-top-row-edges.ts:6`, `use-mac-traffic-light-sync.ts:114-128` | workaround; native shell knows geometry |
| 8 | stale `data-drag` flattening of shadows during pane resize | `index.css` (`html[data-pane-resizing]`) | perf workaround, do not port |
| 9 | tab shell fill change has no transition | `view-row.tsx:172` | verify live; keep visuals as seen |
| 10 | dev-only `BuildReadout` anchor | `view-tabs-row.tsx:169` | cut |
