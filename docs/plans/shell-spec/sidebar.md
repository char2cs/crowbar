# Shell spec: sidebar, part A (anatomy, tokens, rows, panels)

Part B (menus, create panel, drag and drop, removal, peek/resize behaviour, shortcuts, persistence, port list, ownership, live-capture checklist): `sidebar-b.md`.

Conventions
- Sources are cited as `file :: symbol`, paths relative to `app/panes/src/`. Line numbers are deliberately not used (they drift; the symbol is greppable).
- Tailwind is v4 with default scale: 1 spacing unit = 4px; `text-xs` 12/16, `text-sm` 14/20, `text-base` 16/24; radii come from `--radius: 0.625rem` (10px): sm 6, md 8, lg 10, xl 14, 2xl 18 (`styles/theme-vars.css`). `sm:` variants apply at viewport width >= 640px, so on desktop `sm:` sizes are the live ones (Button `icon-sm` is 28px, tab 32px).
- Colour tokens are named, not resolved to hex here; the token table (hex per theme) is `00-tokens.md` (fork 1). Values quoted here are the ones defined in `styles/theme.css` / `styles/crowbar.css` and are repeated only where the sidebar depends on a derived recipe.
- "Chrome palette": the default theme is `crowbar` (`styles/crowbar.css`). The sidebar subtree carries class `chrome-surface`, which re-paints it with the DARK palette in BOTH light and dark mode (`styles/theme.css` selector `[data-theme='crowbar']:not(.dark) .chrome-surface`). Everything inside the sidebar therefore resolves `--foreground` = #f2f0ea, `--background` = #1c1c1a, `--muted-foreground` = oklch(0.834 0.008 278.6), `--border` = #333330, `--sidebar-element-idle` = 11% foreground, `--sidebar-element-hover` = oklch(1 0 0 / 10%). Floating surfaces portalled to body (menus, toasts, tooltips, dialogs, drag ghost) are NOT inside `chrome-surface` and use the active mode's palette (light: popover #ffffff / fg #1b1b19).
- Linux has no window vibrancy: `[data-platform='linux']` flattens `--chrome-bg` to oklch(0.182 0.005 106.7) (opaque) (`styles/crowbar.css`, `styles/theme.css`). The sidebar root is `bg-transparent` over that body fill, so the sidebar column is opaque #1c1c1a-ish on Linux; on macOS it would be translucent.
- Fonts: UI = `CalSansUI` (`public/fonts/CalSansUI.woff2`, weights 300-700, `--font-sans`, overridable by the `--app-font-family` setting); headings = `Instrument Serif` (`--font-heading`); mono = `Geist Mono Variable` (`--font-mono`, overridable by editor font). (`styles/theme-vars.css`, `styles/theme-fonts.css`). Icons: Phosphor (`@phosphor-icons/react`), default weight `regular` unless stated.
- `--app-ui-scale` (default 1) scales `ui-text-*` utilities only (xs 11px, sm 12px, base 14px, lg 16px, xl 20px). Most sidebar text uses fixed px (13px rows) and is NOT scaled.

## 1. Window-level frame

Source: `components/layout/ide-shell.tsx :: IdeShellBody`, `sidebar-split-pane.tsx :: SidebarSplitPane`.

```
IDEShell
 ConsoleDock                      (console dock; layout spec)
  SidebarProvider  h-full flex-1 bg-transparent text-foreground; owns Mod+B
   SidebarSplitPane  CSS grid, @container/split, h/w 100%
     col A  "sidebar"   (id crowbar-sidebar-pane)  overflow-hidden
     col B  "handle"    1px track, 8px-wide hit element centred in it
     col C  "content"   overflow-visible            (tabs bar + panes, other specs)
     [scrim button, only when narrow]
   SettingsDialog (lazy), FontStyleInjector, DetachHolderModal, PlaceholderToastWatcher
```

| Property | Value | Source |
|---|---|---|
| Grid, sidebar left | `min(var(--sidebar-track-width), 80%) 1px minmax(0,1fr)`, areas `"sidebar handle content"` | SidebarSplitPane |
| Grid, sidebar right | `minmax(0,1fr) 1px min(var(--sidebar-track-width), 80%)`, areas `"content handle sidebar"` | SidebarSplitPane |
| `--sidebar-track-width` | `open ? preferredWidth : 0` px; written imperatively during drag | SidebarSplitPane :: writeWidth |
| Default / min / max width | 294 / 250 / 640 px | `use-sidebar-panel.ts :: SIDEBAR_DEFAULT_PX, SIDEBAR_MIN_PX, SIDEBAR_MAX_PX` |
| Max share of window | 80% (content keeps >= 20%, `contentMinRatio` 0.2); clamp is `min(max, (container-1) * 0.8)` | SidebarSplitPane :: clampOpenWidth |
| Window pressure | The preference is never rewritten; the live track is squeezed by the `min(..., 80%)` and restores when room returns | use-sidebar-panel.ts comment, grid template |
| Sidebar side | `settings.sidebarPosition` `'left'` (default) or `'right'`; grid areas swap, children are not remounted | `features/settings/config/default-settings.ts`, IdeShellBody |
| Collapsed | track 0 px; sidebar subtree stays mounted (SidebarPeek is a wrapper, never a branch) | ide-shell.tsx comment, sidebar-peek.tsx |
| Open/close transition | NONE on the grid track (no `transition` on `gridTemplateColumns`); width changes instantly. VERIFY LIVE (capture list item A3). | SidebarSplitPane |
| Narrow window (< 640 px split container) | Sidebar becomes an overlay: `absolute inset-0 z-40 bg-background shadow-xl`, content takes all columns, resize handle hidden, scrim `absolute inset-0 z-30 bg-black/30` closes it on click; `Esc` closes it (if not `defaultPrevented`). | SidebarSplitPane :: NARROW_SIDEBAR, scrim button |
| Sidebar body root | `chrome-surface relative flex h-full min-h-0 flex-col overflow-hidden bg-transparent select-none` | ide-shell.tsx |

Vertical stack inside the sidebar body (top to bottom): header (42px) / project switcher (44px) / mode bar / panel (flex-1) / foot (40px). Toast overlay is absolutely positioned over the bottom.

### 1.1 Resize handle

`sidebar-split-pane.tsx :: SidebarSplitPane` (the `role="separator"` element).

| Aspect | Value |
|---|---|
| Hit area | 8px wide (`w-2`), full height, centred on the 1px grid track (`justify-self-center`), `z-10`, `cursor: col-resize`, `touch-action: none`, no outline |
| Visual | `::after` 1px wide line, full height, `transition-colors` (Tailwind default 150ms ease); rest: transparent; hover: `bg-border/60`; focus-visible: `bg-border` + `ring-1 ring-ring` |
| Pointer | primary button only; focuses the handle; measures container and sidebar once; moves are rAF-coalesced; `data-pane-resizing=1` set on `<html>` on first move (panes flatten shadows/radii, see `index.css` rule list) and removed + `pane-resize-end` event dispatched on release |
| Drag math | `w = startWidth + (side==='left' ? dx : -dx)`; if `w < min/2 (=125)` -> 0 (collapsed) else clamp to [min, maxByWindow]. Collapse and reopen happen live (width 0 while dragging, sidebar reappears when pointer passes 125 px). |
| Commit | on release: width rounded; `open = width > 0`; if open, `commitPreferredWidth` (clamped 250..640, persisted). Release when collapsed persists nothing but `open=false`. |
| Cancel | `pointercancel` and window `blur` restore `startWidth` |
| Double click | toggle: open -> collapse (width 0), closed -> `preferredWidth` |
| Keyboard (handle focused) | Left/Right adjust by 16 px (Shift: 64), direction mirrored for right dock; Home -> 0 (collapse); End -> max (640); Enter toggles collapse/preferred. aria: `aria-valuemin=0`, `aria-valuemax=640`, `aria-valuenow=live width`. Results below 0 collapse; others clamp. |

Header toggle / Mod+B / peek are other ways to open and close (section 2, part B 2).

## 2. Sidebar header (42px row)

`components/layout/sidebar-project-header.tsx :: SidebarProjectHeader`; height `lib/chrome-metrics.ts :: CHROME_ROW_HEIGHT_PX` = 32 + 2*5 = **42px**; whole row is a window drag region (`data-tauri-drag-region`). Hidden while a pushed nav screen exists (never happens, see port list).

| Item | Spec |
|---|---|
| Row | `flex items-center gap-1`, full width, `overflow-hidden`, padding: dock-left `pl-3 pr-2` (12/8), dock-right `pr-3 pl-2` and `flex-row-reverse` |
| macOS only | 72px spacer for traffic lights when dock-left (`IS_MAC`). Not on Linux. |
| Order (dock-left) | `[Settings] [host picker (flex-1, centred)] [Sidebar toggle]`; mirrored for dock-right |
| Settings button | Button ghost `icon-sm` => 28x28 (desktop), `rounded-sm` (6), `text-muted-foreground`, hover `bg-sidebar-element-hover` (text stays muted), icon Phosphor `Gear` 16px regular; tooltip "Settings" (bottom); click opens settings dialog (tab = last/default `appearance`) |
| Toggle button | same box; icon `SidebarToggleIcon` (`components/ui/sidebar-toggle-icon.tsx`: 24-unit viewBox, stroke 2 round, panel rect x3 y4 w18 h16 rx2.5, divider `M9 4v16`, two lines `M5.5 9h1.5` `M5.5 13h1.5`) at `size-4` (16px); dock-right mirrors with `scale-x-[-1]`; tooltip "Hide Sidebar"/"Show Sidebar" (bottom); aria "Hide sidebar"/"Show sidebar" |
| Host picker | only if `hostIds.length >= 2` (see 2.1) |
| Button base (Button ghost) | `rounded-lg border font-medium`, transparent border; focus-visible `ring-2 ring-ring ring-offset-1 ring-offset-background`; disabled 64% opacity; `pointer-coarse` hit expands to 44px (`components/ui/button-variants.ts`) |
| Tooltip | `WithTooltip`: delay 150ms, `timeout=0`, side offset 6, collision padding 8; bubble `ui-text-sm` (12px) `rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-foreground shadow-lg backdrop-blur-sm`; enter: `animate-in fade-in-0 zoom-in-95` + 4px `slide-in-from-*` (tw-animate-css defaults 150ms) (`components/ui/tooltip.tsx`) |
| When sidebar is collapsed | the header is hidden with it; the toggle that reopens it lives in the tabs bar (tabs spec: `features/tabs/components/view-tabs-row.tsx`, `tab-bar.tsx`) |

### 2.1 Host picker (`components/layout/sidebar-host-picker.tsx`)

- Trigger: `inline-flex h-6 min-w-0 items-center gap-1.5 rounded-full px-2 text-xs font-medium text-muted-foreground`, hover `bg-sidebar-element-hover text-foreground`. Content: `HostStatusIcon` (16px; Phosphor `Desktop` for local, `Cloud` for remote, with status dot) + name `max-w-24` (96px) truncate. `title` = "Local|Remote · <state> · Switch host". aria "Host: <name>".
- Menu (`DropdownMenu`, `align=center`, `min-w-56` 224px, `w-max`): one row per host: status icon, column (name; secondary line 12px muted = "This machine" or endpoint), trailing "Current" (12px muted) for the shown host. Selecting calls `selectHost` (filter + land on remembered/latest project of that host).
- Status dot (`features/hosts/components/host-status-icon.tsx :: HostStatusDot`): 5px circle (`size-1.25`) at `-right-px -bottom-px`; the host icon is masked with a radial cut-out (`circle at calc(100% - 1.5px) calc(100% - 1.5px)`, transparent < 3.75px, opaque > 4.25px). Fill by tone: online `bg-success`; offline `bg-destructive`; needs-attention `bg-warning-foreground` (light) / `bg-warning` (dark); unsettled = 1px ring `border-muted-foreground/65`, no fill. State -> tone: up, no-live-stream -> online; down, not-available -> offline; needs-repair -> needs-attention; unknown, probing -> unsettled. Labels: `features/hosts/lib/host-tone.ts :: LABEL` ("Status unknown", "Checking…", "Online", "Offline", "Needs pairing again", "Online — no live stream", "Host not available").

## 3. Project (space) switcher line (44px)

`components/layout/sidebar-project-switcher.tsx :: SidebarProjectSwitcher`. Rendered only if there are projects or an add handler.

| Item | Spec |
|---|---|
| Row | `h-11` (44px), `px-3`, `gap-2`, `overflow-hidden`, `group/switcher`; inner left block `flex min-w-0 flex-1 gap-1` |
| Active project tile | 28x28 (`size-7`), `rounded-[8px]`, `-ml-1` (-4px); holds `EditableProjectIcon` (20px mark `size-5`; click opens icon popover, section part B 6) |
| Active project name | `font-heading` (Instrument Serif) `text-[19px] leading-6 (24px) font-normal tracking-tight (-0.025em)`, truncate, colour foreground. Double-click starts rename (`InlineRenameInput` with the same text classes, so no reflow). Enter confirms, Esc cancels, blur confirms; rename calls `performRenameProject`. Switching space mid-edit drops the editor (state keyed to project id). |
| Remote host line (only for non-local active project) | under the name; `text-[10px]/[12px]`, button, `gap-1`, `rounded-sm`, host icon `size-3` (12px, bold), text truncate: online -> name in muted (hover foreground); otherwise "<name> · <state word>" in the tone's text colour (success-foreground/ destructive-foreground / warning-foreground light; success/destructive/warning dark). Click opens Settings > Hosts. Tooltip "Host settings: <name>, <state>". |
| `+` (Add space) | Button ghost `icon-sm` with TILE box 28x28 `rounded-[8px]`, icon `Plus` 16px, `text-muted-foreground`, hover `bg-sidebar-element-hover`; tooltip "Add space". With more than one project it is collapsed to width 0, `-ml-2` (cancels the row gap), border 0, opacity 0, and expands to `w-7 ml-0 opacity-100` on `group-hover/switcher` or `group-focus-within/switcher` (also always shown on `pointer-coarse`). Transition `width, margin, opacity` **150ms** (Tailwind default easing), `motion-reduce:transition-none`. With exactly one project it is always visible. |
| Other-project marks | one 28x28 tile per non-active project, in project order, right of `+`; `rounded-[8px]`, hover `bg-sidebar-element-hover`; the icon is wrapped at `opacity-80` -> `100%` on hover (`group-hover/mark`), `ProjectIconMark` size lg (20px; emoji `text-lg`; fallback Phosphor `Books` 16px; image `rounded-md object-cover`); tooltip + aria = project name (remote: "<name>, <host> <state>"); click -> select project. Remote ones wear the status dot on the icon. |
| Overflow | the name truncates first; marks never shrink (`shrink-0`) |
| Project list shown | `projects` filtered to the shown host when >= 2 hosts (host filter) |

Select project => route `/ide/$projectId/home`; this is how the carousel scrolls (4.1). Mod+Shift+1..9 jump to a project by position (9 = last).

## 4. Mode bar (Chats / Files / Changes)

`components/layout/sidebar-mode-bar.tsx :: SidebarModeBar` Mounted by `sidebar-body.tsx :: SidebarBody` directly under the project switcher and above the three panels. Tabs primitive `components/ui/tabs.tsx`.

| Item | Spec |
|---|---|
| Container | `@container flex shrink-0 items-center px-2 py-1.5` (8px sides, 6px top/bottom); container queries respond to the container width = sidebar width minus 16 |
| TabsList | `w-full rounded-lg bg-sidebar-element-idle p-0.5 gap-x-0.5 text-foreground/70`; `flex items-center justify-center` |
| Tab | `h-8` (desktop `sm:h-8`; 36 below 640px viewport), `flex-1 grow`, `rounded-md` (8) with 1px transparent border, `px-[calc(2.5*4px - 1px)]` = 9px, `gap-1` (4px), `text-sm` (14px) `font-medium`, `whitespace-nowrap`; icon Phosphor 14px: `fill` weight for the active tab, `regular` otherwise (ChatsCircle, FolderOpen, GitBranch) |
| Label visibility | hidden by default; active tab label shows when container >= 280px; all labels show when >= 420px |
| Colours | inactive: muted (via `text-foreground/70` on list; `hover:text-muted-foreground`); active: `text-control-foreground` (#1b1b19 on the raised pill) with `--muted-foreground` re-pointed to `control-muted-foreground` |
| Active pill (sliding indicator) | `TabsPrimitive.Indicator`, behind (`-z-1`), `h/w = active tab size`, `translate-x = tab left`; `transition-[width,translate] duration-200 ease-in-out`; recipe `SELECTED_PILL` = `control-surface rounded-lg border shadow-xs shadow-black/10 inset-shadow-[0_1px_var(--elevated-highlight)]` (`components/ui/selected-pill.ts`); `control-surface` = bg `--control-surface` #fcfcfa, border/outline same colour, text #1b1b19 (`index.css @utility control-surface`) |
| Disabled | Files is disabled (64% opacity, no pointer) when the focused workspace context has no `workspaceId`; Changes is not drawn at all unless a repo workspace is focused (`hideWhenUnavailable`); Chats always |
| Which mode shows | `useShownSidebarTab`: stored `activeTab` if available else Chats (derived, not written back) (`components/layout/sidebar-panel-tabs.ts`) |
| Follow focus | setting `sidebarFollowsFocus` (default false): focus entering a `[data-sidebar-surface]` pane of another kind switches mode, only after the user's first pointerdown/keydown (`use-sidebar-focus-mode.ts`) |
| Keys | Mod+1 Chats, Mod+2 Files, Mod+3 Changes (remappable; ignored if the mode is unavailable) (`features/keymaps/registry.ts`, `hooks/use-sidebar-tab-keyboard.ts`) |

Plan doc with rationale: `docs/plans/2026-10-24-sidebar-mode-switch.md`.

## 4.1 Panels container and carousel

`components/layout/sidebar-body.tsx :: SidebarBody`: three sibling panels (`chats`, `files`, `git`) each `flex min-h-0 flex-1 flex-col overflow-hidden`; inactive ones get the `hidden` attribute, never unmount (scroll positions and folds survive). Each panel is wrapped in an `ErrorBoundary`.

Chats panel (`components/layout/sidebar-tree-surface.tsx :: SidebarTreeSurface`) = `SpaceScroller` (`components/sidebar/space-scroller.tsx`):

| Aspect | Value |
|---|---|
| Container | `flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory]`, scrollbar hidden (`scrollbar-width:none`) |
| Panel (one per project of the shown host) | `min-w-full`, `scroll-snap-align:start`, `flex-col overflow-hidden`; contains a vertical `ScrollArea` |
| Programmatic scroll | active project change -> `scrollTo({left: index*clientWidth, behavior:'smooth'})` (browser smooth; ~300ms WebKit). Resizing the sidebar re-aligns instantly (`scrollLeft = index*clientWidth`), ignored at width 0. |
| User swipe | only horizontally dominant wheel gestures (`|dx| > |dy|`) or touch arm "user gesture"; then the snapped index becomes the active project (route change). Any other scroll change snaps back to the active project. |
| During a row drag | `html[data-row-dragging] [data-sidebar-carousel]` gets `overflow-x:hidden; will-change:transform` so a drag past the edge cannot scroll a panel away (`index.css`) |
| ScrollArea (vertical) | Base UI scroll area; viewport `h-full`; scrollbar `m-1`, thumb track 6px wide (`w-1.5`), `opacity-0` at rest, `opacity-100` while hovering or scrolling with `duration-100`, hide `delay-300` (default 150ms transition); thumb `rounded-[var(--app-scrollbar-radius)]` bg `--app-scrollbar-thumb` (oklch(0.55 0 0 / 42%)), hover `--app-scrollbar-thumb-hover` (`components/ui/scroll-area.tsx`, `styles/theme.css`) |

Contents of one space panel, in order (`space-scroller.tsx :: SpacePanel`): `UnreadNotice`, `ActiveSessionsSection` (touch devices only: renders nothing on desktop), `SidebarTree`, `TreeFoot`. Greying when the project's host is unreachable (`space-host-scope.tsx`): inside the scope `--foreground` and `--background-inverse` are re-pointed to `--muted-foreground` and the count colours to `--foreground-inverse`; rows still accept clicks.

When `creatingSpace` is true `CreateSpacePanel` replaces the scroller entirely (part B 5).

Loading: until the `projects` scope snapshot is synced, `SidebarSkeleton` (inline in sidebar-tree-surface.tsx): `flex flex-col gap-2 p-3`, 5 `Skeleton` bars `h-5` (20px) with widths 75%, 50%, 66.7%, 50%, 60%. Skeleton = `rounded-sm`, gradient `linear-gradient(120deg, transparent 40%, highlight, transparent 60%)` over `--muted`, background-size 200%, fixed attachment, animation `skeleton 2s -1s infinite linear` moving `background-position` 0 -> -200% (`components/ui/skeleton.tsx`, `styles/theme-vars.css`); highlight = white 64% (light) / 4% (dark). (A second, richer `components/layout/sidebar-skeleton.tsx` is used only as the Files panel Suspense fallback.)

Unread notice (`components/sidebar/unread-notice.tsx`): role alert row `flex gap-2 px-3 py-1.5 text-xs text-muted-foreground`; icon `WarningCircle` 14px (`size-3.5`, `mt-0.5`, `text-warning-foreground` light / `text-warning` dark); lines "Couldn't load this project's chats" / "Couldn't load chats in <repo>"; ghost `xs` "Retry" button (re-subscribes the `projects` scope).

## 5. Tree rows

### 5.1 Base capsule (`components/layout/workspace-row-base.ts`)

| Property | Value |
|---|---|
| `ROW_BASE` | `flex cursor-pointer select-none items-center gap-1.5 rounded-lg border contain-layout px-1.5 text-[13px] font-medium outline-none h-9 mx-1.5 my-0.5` => height **36px**, margin 6px left/right, 2px top/bottom (pitch **40px**), padding-x 6px, gap 6px, radius 10px, 1px border (transparent at rest), font 13px weight 500 (line-height inherited; irrelevant, fixed height + centring) |
| Indent step | `ROW_INDENT_STEP` = **14px** per depth, applied as `margin-inline-start` on an outer wrapper; changes animate: `transition: margin-inline-start 0.1s ease-in-out` (motion-safe) (`ROW_INDENT_TRANSITION`) |
| Glyph box | `ROW_GLYPH_BOX` = `inline-flex size-4 shrink-0 items-center justify-center` (16px); repo header row uses `size-5` (20px) |
| Resting colours | text `--foreground`; fill none; `hover:bg-sidebar-element-idle` (`ROW_INACTIVE`). Open (has a view) rows rest on the same fill: `ROW_HAS_VIEW_IDLE = bg-sidebar-element-idle` (always applied in the tree: `SidebarTreeRow` passes `hasViewIdle`). |
| Focus | `focus-visible:ring-2 ring-ring ring-offset-1 ring-offset-background`; a pointer click blurs the row so trailing controls do not stay lit |
| Nest-drop target | border `--sidebar-drop-nest-edge`, fill `--sidebar-drop-nest` (colour-mix secondary 65% / 30%), text foreground; replaces rest/hover colours |
| Dragging source | `opacity-40` |
| Touch | `touch-action: pan-x pan-y` |
| Hover vs press | no separate pressed style; `--sidebar-element-press` token exists but rows do not use it |

### 5.2 Row anatomy and states (`components/sidebar/sidebar-row.tsx :: SidebarRow`)

`[glyph 16][label flex-1 truncate][trailing cluster]` inside the capsule. `role=treeitem`, `aria-level=depth+1`, `tabIndex=0`, Enter/Space opens (only when the row itself is focused), click opens (unless an inline editor is active). No arrow-key tree navigation exists in the chats tree.

Row variants (`SidebarRow` early returns):

| Variant | Condition | Rendering |
|---|---|---|
| Operation / draft | `row.operation` or `row.draft` -> `operation-sidebar-row.tsx` | ROW_BASE with transparent border, not interactive unless draft (`pointer-events-none opacity-60`). Glyph: failed `✕` 12px destructive; running `FlickerSpinner` 14px; draft shows normal glyph. Label: draft -> inline name input (placeholder `branch-name`, mono for branch kind); else `font-mono text-[13px] text-muted-foreground` "New thread" or branch label/"New workspace". Failed: "failed" (12px destructive), reason (12px muted, max 50% width, truncate), `✕` Dismiss (12px muted, hover foreground). |
| Removing | `row.removal` -> `sidebar-row-removing.tsx` | ROW_BASE + ROW_INACTIVE, `relative`; glyph, label (mono for branch, italic if provisional), optional pill `+N` (`rounded-full bg-sidebar-element-idle px-1.5 py-px text-[11px] tabular-nums text-muted-foreground`), seconds figure (`text-[11px] tabular-nums muted`), Keep button (`ROW_SUB_ACTION`, undo-arrow svg 12px, stroke 2 round; label "Keep", title "Keep"), and the drain bar `absolute inset-x-2 bottom-[3px] h-0.5 (2px) rounded-full bg-destructive origin-left animate-tray-drain` (8s linear forwards, scaleX 1 -> 0; `data-essential-motion` so it runs under reduced motion). Not draggable, no drop attributes. |
| Normal | otherwise | below |

Normal row states:

| State | Visual |
|---|---|
| rest | glyph + label, transparent border; label muted only if `hasView && !hasViewIdle` (never in the tree) |
| hover / focus-within | fill `--sidebar-element-idle`; trailing cluster appears (`hidden` -> `inline-flex` on `group-hover` / `group-focus-within`; takes layout space only then, so label truncation changes) |
| open (has pane/view) | resting fill `--sidebar-element-idle` |
| working (agent turn or operation on its workspace) | glyph replaced by `FlickerSpinner` `size-3.5` (14px) in the 16px box (never beside the glyph) |
| provisional title | label italic (`labelProvisional`) |
| renaming | label replaced by `InlineRenameInput` (below); trailing cluster still rendered; row click ignored; drag disabled |
| branching (Make a branch) | label replaced by `BranchOutInput`; trailing cluster hidden; shows error line below the input (`ROW_SUBLABEL` + destructive) if the daemon refuses the name |
| pending move (chat dropped into another workspace mid-turn) | two lines: title + sublabel "Moving to <label> after this turn" / "Moving after this turn" (ROW_SUBLABEL); context menu gains "Cancel move" |
| dragging | opacity 40% |
| nest target | see 5.1 |

Labels: `data-sidebar-row-label` marks the label span for double-click rename. Branch rows use `font-mono` for the label when single-line. Two-line branch rows (unlocked branch row owning a workspace whose chat title differs from branch name): column `flex-col justify-center`; line 1 title (13px/medium); line 2 = `ROW_SUBLABEL` = `truncate font-mono text-[10.5px]/[13px] tabular-nums text-muted-foreground`: `<branchName> +<added> -<deleted>`; counts coloured `--sidebar-count-added` (#4e7a36 light palette; #b9cf98 in chrome/dark palette) and `--sidebar-count-deleted` (#b8422c; #f1a493 in dark), omitted when 0; counts formatted by `components/layout/format-change-count.ts`.

Inline rename input (`components/sidebar/inline-rename-input.tsx`): `<input>` `min-w-0 flex-1 truncate bg-transparent text-left text-[13px] font-medium outline-none` (+ `font-mono` for mono rows); no border or caret styling of its own (caret = default); autofocus + select-all + scrollIntoView(nearest); Enter confirms (trim; empty -> cancel); Esc cancels; blur confirms (or cancels with `cancelOnBlur`, used by Make-a-branch); a rejected confirm keeps/reopens the input. `aria-invalid` when an error is shown.

### 5.3 Glyph selection (`components/sidebar/sidebar-row-glyph.tsx :: SidebarRowGlyph, RowGlyph`)

Priority: working/busy -> FlickerSpinner; repo header (`repoIcon`, project-home, editable) -> `EditableRepoIcon` size lg (20px box, see repo icon below); read-only header -> `RepoAvatar`; then `RowGlyph`:

| Row | Glyph (16px; 20px for repo header) |
|---|---|
| folder | Phosphor `FolderOpen` (expanded) / `Folder` (collapsed), weight duotone |
| branch with a landed workspace (`status`) | `WorkspaceBranchIcon` (`components/layout/workspace-branch-icon.tsx`): placeholder (needs provisioning) -> `Warning` fill amber-500 with title=reason; `locked` -> `Lock` fill, foreground; `new` -> `GitBranch` fill, foreground; `pr-conflicts` -> `Warning` fill amber-500; `deleted` and `pr-closed` -> `GitFork` fill red-500; `pr-open` -> `GitPullRequest` fill green-500; `pr-merged` -> `GitMerge` fill violet-500 |
| locked branch without workspace yet | `Lock` fill |
| owns worktree (fork) | `GitBranch` fill |
| plain chat | `ChatsCircle` regular |

FlickerSpinner (`components/ui/flicker-spinner.tsx`, `flicker-strip.ts`, assets `components/ui/spinners/*.svg`): one random flip-dot SVG per instance, baked into a horizontal sprite strip animated with `animation: flicker-strip <duration from asset> steps(<frames>) infinite` (translateX 0 -> -100%); box `relative inline-flex size-4 overflow-hidden`; colour `currentColor`; pauses (`animation-play-state: paused`) while off screen (IntersectionObserver); exempt from reduced motion. Reference needs a deterministic spinner for parity: record asset choice per capture.

Repo icon marks (`components/layout/repo-icon-mark.tsx`, `repo-avatar.tsx`): sizes sm 16 / lg 20 / xl 24 (px box; letter 10/11/13px bold, emoji 12-14 / 14 / 18-20px); letter tile = `rounded-sm px-0.5 font-bold text-primary-foreground` over avatar colour class (`avatar-rose|amber|emerald|cyan|indigo|violet|slate|pink` = Tailwind `*-700`, `index.css`); image `rounded-md object-cover` (fallback to the letter tile on error); emoji avatars use `emoji:` URL prefix. Pointer hover on an editable mark shows a pencil overlay (`rounded-md bg-black/45`, Phosphor `PencilSimple` 10px white) (`icon-popover.tsx`).

### 5.4 Trailing action cluster (`components/sidebar/sidebar-row-actions.tsx`, verbs in `components/sidebar/lib/row-menu.ts :: rowButtonVerbs`)

Each button: `ROW_SUB_ACTION_HOVER` = hidden until row hover/focus-within, then `inline-flex shrink-0 rounded-lg p-1.5` (6px padding) with 12px icon (`size-3`; `...` is `size-3.5` = 14px) => 24x24 hit, `text-muted-foreground`, hover `bg-sidebar-element-hover text-foreground`, focus-visible `ring-1 ring-ring`. Clicks stop propagation and blur; pointerdown stops propagation (no drag arm).

Order left to right (all weight `bold`): Detach (`LinkBreak`; branch with workspace and `heldByPath`), Retry (`ArrowClockwise`; branch with workspace, placeholder, no holder), repo `...` menu (`DotsThree`; repo header row only), Remove (`X`; chat, folder, unlocked branch; never beside repo menu), Thread (`ArrowElbowDownRight`; unless placeholder), Fork (`GitBranch`; folder owning worktree or branch; unless placeholder), Make-a-branch (`GitBranch`; repo thread, no worktree, no pending move), Close (`X`; only where `onClose` is given: not in the chats tree), Fold chevron (svg path `M6 3l5 5-5 5`, 16-unit viewBox, stroke 2 round, `size-3`, `rotate-90` when expanded, `transition-transform` 150ms default; only if the row has children; on desktop it is ALSO hover-revealed). aria labels: "Detach <b>", "Retry <b>", "More actions for <label>", "Remove <label>" or "Remove branch <b> and its worktree", "Thread <label>", "Fork <label>", "Make this thread a branch", "Close <label>", "Expand|Collapse <label>". Titles on Remove, Retry/Detach (placeholder reason), Make-a-branch.

Press-and-hold drag arms from anywhere on the row except these buttons and the inline inputs (part B 7).

## 6. Tree foot (per space, inside the scroll area, after the last row)

`components/sidebar/tree-foot.tsx :: TreeFoot`:
- Divider `mx-3 my-1 h-px bg-foreground/15` (12px side margins, 4px vertical, 1px).
- Two `FootRow` buttons using ROW_BASE + ROW_INACTIVE (so: 36px high, 6px side margins, 10px radius, hover fill idle) with `w-[calc(100%-0.75rem)] text-left text-muted-foreground`; glyph box 16px: "New thread" (Phosphor `Plus` 16px) and "More…" (`DotsThree` bold 16px).
- New thread: blur, then `handleCreateHomeThread(projectId, homeWorkspaceId)`; with no home workspace resolved yet: toast error "Can't start a new thread yet".
- More…: opens a context menu below the button (`x = rect.left`, `y = rect.bottom + 4`) with: "Import a repo" (`FolderOpen`; opens the Add repository modal), "Create a new folder" (`Folder`; `performCreateHomeFolder`), "Delete space" (`Trash`, destructive text colour; starts a project removal hold, which asks for confirmation, part B 8).
- Empty space: the foot sits at the top of an empty tree (no separate empty-state graphic).

## 7. Files panel (mode `files`)

`components/layout/sidebar-files-panel.tsx` -> `features/file-explorer/file-explorer/components/file-explorer-tree.tsx` (virtualised via `use-file-explorer-visible-rows.ts`).

| Item | Spec |
|---|---|
| Row | `SidebarFileRow` (`components/sidebar/sidebar-file-row.tsx`): ROW_SHAPE capsule (rounded-lg border px-1.5 13px/500 gap-1.5) at **26px** height (`SIDEBAR_FILE_ROW_HEIGHT`), indent `6 + depth*14` px left padding; leading `DirectoryChevron` (box 14px `size-3.5`; Phosphor `CaretRight` bold 10px, `rotate-90` open, 150ms); icon column 16px with `FileExplorerIcon` at 14px; label truncate; git status letter badge after label; trailing slot `ml-auto gap-1.5 pl-1.5` |
| Row states | rest: ROW_INACTIVE hover fill; selected/active: ROW_HAS_VIEW_IDLE fill; drop target: ROW_NEST_TARGET; ignored file: opacity 50%; cut: italic opacity 40%; search match: `file-tree-search-match` + `<mark class=file-tree-search-highlight>` on matched text; transient feedback text (12px, green-500 or destructive) "✓ path copied", "✓ rel copied", "✓ created" |
| Indent guides | `TreeGuides`: 1px vertical lines `left = baseIndent + level*14 + 7px`, colour `color-mix(srgb, muted-foreground 18%, transparent)`, top/bottom inset 4px at where a level begins/ends (`file-explorer-tree.css`) |
| Search | filter box opens on `/` (no modifiers) or Mod+F while the tree has focus (`use-tree-container-events.ts`); `SidebarHeader`: input `size sm` placeholder "Search", search icon 14px at `start-2.5`, and a 26px-square outline filter button (Phosphor `Funnel` 14px, `text-secondary` when any filter is off) opening a menu: Hidden Files, Gitignored Files, separator, Git Status (checkmarks). Enter/Shift+Enter step matches; Esc closes. |
| Keyboard | Up/Down/Home/End move; Right expands (or into folder), Left collapses (or to parent), Enter opens, F2 renames, Delete removes (confirm), Mod+C/X/V copy/cut/paste (`use-tree-container-events.ts`) |
| Context menu (right click) | dir: New File, New Folder, Upload File (hidden on locked workspace), Refresh, Open All Files, Collapse All, Open in Terminal, Generate Image (if handler), Add Folder to Workspace (root only), separator; file: Open, Copy Content, [env-template items], Properties, separator; both: Copy Path, Copy Relative Path, Reveal in Finder (local only), then management items (Duplicate, Rename, Delete; hidden for the workspace root) (`use-file-explorer-context-menu.tsx`) |
| Drag and drop | rows draggable to move files (nest highlight on target folder) and to panes (`use-file-explorer-drag-drop.ts`) |
| Empty / loading | "No folder open" or "Folder is empty" / search messages via `SidebarEmptyActionState` (`flex min-h-24 flex-col items-center justify-center gap-1.5 px-3 py-6 text-center`, message `ui-text-sm leading-[1.35]` muted, optional description 12px max 24ch, optional ghost action `h-6`); loading shows `SidebarSkeleton` (2 chat-like rows, divider, 2 repo blocks each with 2 child rows; bars `h-3`/`h-5 w-5 rounded-md`) |

## 8. Changes panel (mode `git`, only with a repo workspace focused)

`features/git/components/git-panel.tsx`, `changed-files-tree.tsx`, `review-merge-box.tsx`, `commit-changes-form.tsx`. Whole panel scrolls (`overflow-y-auto`).

1. Branch pill: `mx-1.5 my-0.5` + SELECTED_PILL, inner `flex h-9 items-center gap-2 px-2 text-[13px] font-medium`: `GitBranch` 14px muted, branch name `font-mono` truncate, then `→` muted and parent branch mono muted.
2. Git error (role alert): `mx-3 my-2`, "Git can't read this workspace" (12px/medium) + message (11px... `ui-text-xs`, muted).
3. Merge box `Card` `mx-1.5 my-1 gap-0 rounded-xl`: header `p-3` title `ui-text-sm` with `GitMerge` 16px muted, "Merge into <parent>" or "Merged into <parent>"; content `p-3 gap-2`: optional commit form (when uncommitted changes block merge): checkbox list (`max-h-40` scroll, 12px), textarea "Describe the change…" (`min-h-16 ui-text-sm`, Mod+Enter commits), button "Commit N change(s)" / "Committing…"; Select (strategy: "Create a merge commit" / "Squash and merge" / "Rebase and merge"), primary small full-width button (labels "Create merge commit" / "Squash & merge" / "Rebase & merge", "Merging…"), checkbox "Delete this branch and its chats after merging" (off by default), muted reason line (blocked reasons: "Nothing to merge yet — this branch has no changes.", "N uncommitted change(s) — commit them first.", "Conflicts with <p> — resolve them first.", "<p> is protected — open a pull request instead.", "This branch has no parent to merge into.", "N open thread(s) must be resolved before merging."), destructive error line, "Stop and merge" outline button when an agent is working.
4. Header text "Changed — N file(s)": `ui-text-xs` (11px) muted, `px-3.5 pt-2 pb-1`.
5. Changed-files tree (virtualised, overscan 8): `SidebarFileRow` 26px rows; folders (chain-compacted names, sorted by name, then files sorted by path) with tri-state "viewed" checkbox (positioned over leading edge) and `viewed/total` trailing (12px muted); files with checkbox, icon, label (muted when viewed else git-status colour), status letter badge, trailing `+N -N` (mono 11px, count tokens; `gap-0.5` when `compactGitStatusBadges`) and open-thread count `Badge info sm`. Click folder toggles collapse (local state, resets on remount); click file opens Branch Review pane revealed at that file. File rows draggable when repoPath.
6. Empty: "No repository open" centred 13px muted when no workspace resolved.

## 9. Foot (40px)

`components/layout/sidebar-foot.tsx`: a `relative h-10 shrink-0` spacer. Only content: the opt-in "build readout" net-graph overlay (`buildReadout` setting, default ON only in dev builds; `components/layout/build-readout.tsx`, text colour #E0E0D0, mono, centred, max 42ch, no pointer events). Release default: empty 40px strip. The inventory marks the readout CUT (dev tooling); keep the 40px spacer unless the layout agent reflows the foot.

## 10. Sidebar toasts (`components/layout/sidebar-toast-overlay.tsx`)

- Open sidebar: viewport `absolute inset-x-0 bottom-0 z-50 flex flex-col-reverse gap-2 p-2` inside the sidebar; max 3 visible (pinned toasts, `timeout: 0`, always kept; transients fill remaining slots newest-first).
- Collapsed sidebar: `Toast.Portal` viewport `fixed bottom-4 left-4 (right-4 when dock-right) z-60 w-72 (288px) flex-col gap-2`, no limit.
- Toast: `relative w-full overflow-hidden rounded-lg border bg-popover px-3.5 py-3 text-sm (14px) text-popover-foreground shadow-lg/5`; enter/exit `opacity` 200ms (`data-starting-style`/`data-ending-style` opacity 0). Content `flex-col gap-2`; head row `gap-2 pr-6`: icon (16px wide; Phosphor WarningCircle=error destructive, Info=info, CircleNotch=loading spin 80% opacity, CheckCircle=success, Warning=warning), title `font-medium`, description muted, break-words; optional action button `xs` right-aligned; close `X` 14px at `top-3 right-3`, opacity 50% -> 100% hover, `hover:bg-muted rounded p-0.5`. Swipe to dismiss: left (right when dock-right) or down.
- Keyed toasts replace each other (connection indicator uses key `connection-indicator`: "Backend unavailable / Reconnecting…" after a 500ms debounce, pinned; "Reconnected / Backend connection restored."; migrating: "Crowbar is upgrading its data" + "Step i of n: name") (`components/layout/connection-indicator.tsx`).
- Keyframes used by `components/ui/toast.tsx` (the non-sidebar toast renderer; the sidebar overlay does not apply them):  `toast-success` (scale 1 -> 1.025 -> 0.99 -> 1, 0.32s cubic-bezier(.5,1,.89,1)) and `toast-error` (translateX 0,-3,3,-3,0 px, 0.28s same easing) in `styles/theme-vars.css`/`index.css`.
