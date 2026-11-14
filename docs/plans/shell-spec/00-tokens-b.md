# Shell spec 00b: type, radii, shadows, motion, z-order, icons, primitives

Companion to `00-tokens.md` (colour). Paths are under `app/panes/src/`. Tailwind is 4.3.3 (`package.json:112`), spacing unit `--spacing: 0.25rem` = 4px. Desktop viewport is always >= 640px, so every `sm:` utility applies and the `max-sm:` / unprefixed-mobile sizes below are NOT used on desktop. Sizes quoted are the `sm:` ones.

## 1. Typography

| Item | Value | Source |
|---|---|---|
| UI face | `CalSansUI` variable, weight 300-700, `/fonts/CalSansUI.woff2` (62 716 B), fallback `ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` | `styles/theme-fonts.css:11-16`, `font-style-injector.tsx:47-49`, `config/typography-defaults.ts:3` |
| Brand face | `CalSans` Regular 400-600, `/fonts/CalSans-Regular.woff2` (37 412 B) | `theme-fonts.css:18-23` |
| Heading face (`font-heading`) | `Instrument Serif` 400 + italic (latin, latin-ext, @fontsource), fallback `Georgia, "Times New Roman", serif`. User-configurable | `theme-fonts.css:4-7`, `theme-vars.css:9-14`, `typography-defaults.ts:8` |
| Mono (`font-mono`, terminal, editor) | `Geist Mono Variable` (@fontsource-variable/geist-mono, incl. italic), fallback `ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace` | `styles/editor-theme.css:1-4`, `typography-defaults.ts:4,12` |
| Symbols | `Symbols Nerd Font Mono` and `Symbols Nerd Font` both from `/fonts/SymbolsNerdFontMono-Regular.woff2` (1.18 MB) | `theme-fonts.css:25-37` |
| Geist sans (selectable only) | `@fontsource-variable/geist` | `theme-fonts.css:3` |
| Font files not in repo | Geist, Geist Mono, Instrument Serif come from `node_modules/@fontsource*`; the Rust app must vendor these woff2 files | `package.json:30-33` |
| Base size | `html` has no font-size: 16px root. `body` text 16px unless a class says otherwise; no letter-spacing, no smoothing overrides anywhere in CSS | `index.css` base layer |
| UI font size setting | min 10, max 24, step 0.5, default 15 (code 14 + 1). `--app-ui-scale = size / 15` (4 decimals) | `lib/ui-font-size.ts:3-8,25-28`, `typography-defaults.ts:14-16` |
| `ui-text-*` scale (the ONLY scaled sizes) | xs `0.6875rem` = 11px, sm `0.75rem` = 12px, base `0.875rem` = 14px, lg `1rem` = 16px, xl `1.25rem` = 20px, each times `--app-ui-scale`; `ui-font` = `var(--app-font-family, var(--font-sans))` | `theme.css:104-109`, `index.css` utilities |
| Tailwind sizes (NOT scaled by the setting) | `text-xs` 12/16, `text-sm` 14/20, `text-base` 16/24, `text-lg` 18/28, `text-xl` 20/28 (size/line-height px); `text-[10px]` x7, `[11px]` x7, `[13px]` x8, `[15px]` x2, `[17px]`, `[19px]` x3, `[22px]`, `[32px]` used ad hoc in shell dirs | tailwind `theme.css`; grep of `components/layout`, `sidebar`, `features/{tabs,panes,settings,window}` |
| Weights | `font-normal` 400, `font-medium` 500 (buttons, labels, tabs), `font-semibold` 600, `font-bold` 700 | tailwind |
| Markdown base | `--md-base-font-size`, default 16px | `settings/lib/markdown-font-size.ts`, `font-style-injector.tsx:89` |

Frequency in shell dirs: `ui-text-sm` 32, `text-xs` 24, `text-sm` 15, `ui-text-xs` 6, `text-lg` 4, `text-base` 4, `font-mono` 17, `font-medium` 10, `font-heading` 7.

## 2. Spacing, radius, shadow (concrete)

Radius base `--radius: 0.625rem` = 10px (`theme.css:9`); scale `theme-vars.css:93-99`:

| class | px | formula |
|---|---|---|
| `rounded-sm` | 6 | 0.6r |
| `rounded-md` | 8 | 0.8r |
| `rounded-lg` | 10 | r |
| `rounded-xl` | 14 | 1.4r |
| `rounded-2xl` | 18 | 1.8r |
| `rounded-3xl` | 22 | 2.2r |
| `rounded-4xl` | 26 | 2.6r |
| `rounded-full` | 9999 | tailwind |

Tailwind default shadows (black, alpha as `/N` modifier replaces the colour alpha):

| class | value |
|---|---|
| `shadow-xs` | `0 1px 2px 0 rgb(0 0 0 / .05)` |
| `shadow-sm` | `0 1px 3px 0 rgb(0 0 0 / .1), 0 1px 2px -1px rgb(0 0 0 / .1)` |
| `shadow-md` | `0 4px 6px -1px rgb(0 0 0 / .1), 0 2px 4px -2px rgb(0 0 0 / .1)` |
| `shadow-lg` | `0 10px 15px -3px rgb(0 0 0 / .1), 0 4px 6px -4px rgb(0 0 0 / .1)` |
| `shadow-xs/5`, `shadow-sm/5`, `shadow-md/5`, `shadow-lg/5` | same geometry, every layer at 5% black |
| `shadow-xs shadow-black/10` (pill) | `0 1px 2px 0` at 10% black |
| dropdown (custom) | `0 14px 30px -24px rgba(0,0,0,.45)` (`ui/dropdown-styles.ts:25`) |
| toast/pane handle | `0 0 5px 1px rgba(0,0,0,.1)` (`panes/components/pane-handle.tsx:58`) |

"Inner edge" recipe on every card-like primitive (button, input, select, popover, dialog, context menu, card): a `::before` pseudo box, `inset:0`, radius = parent radius - 1px, with `box-shadow: 0 1px black/4%` (light) or `0 -1px white/6%` (dark). It reads as a 1px lower edge in light, upper highlight in dark. Hidden while disabled, focused, invalid or pressed. Light mode also uses `background-clip: padding-box` on these (`not-dark:bg-clip-padding`).

## 3. Motion and easing

Global rules:

- Tailwind default transition: 150ms, `cubic-bezier(0.4, 0, 0.2, 1)` (`--default-transition-*`). `ease-in-out` equals that curve; `ease-out` = `cubic-bezier(0,0,0.2,1)`.
- tw-animate-css `animate-in`/`animate-out`: duration `var(--tw-duration, 150ms)`, easing CSS `ease`, keyframes from/to the `--tw-enter-*`/`--tw-exit-*` opacity/translate/scale. `duration-100` on a popup sets 100ms for its animation. `fade-in-0` = from opacity 0, `zoom-in-95` = from scale .95, `slide-in-from-top-1` = from translateY(-4px) (n x 4px).
- framer-motion: only one wrapper, `<MotionConfig reducedMotion="user">` (`app-main.tsx:115`). Only `ui/menu-popover.tsx` imports framer-motion in the shell dirs; everything else is CSS.
- Reduced motion: `prefers-reduced-motion: reduce` forces `animation-duration`/`transition-duration: 0.01ms !important`, iteration 1, `scroll-behavior:auto`, except `.animate-spin` and `[data-essential-motion]` (`index.css:44-53`).

Animation table (every time-based motion found in styles + shell primitives):

| Element | Property | Duration | Easing | Trigger | Source |
|---|---|---|---|---|---|
| Default `transition`/`transition-colors`/`-opacity` | listed props | 150ms | (.4,0,.2,1) | state change | tailwind |
| Dialog backdrop | opacity (0 <-> 1), blur 8px (`backdrop-blur-sm`) | 200ms (`transition-all duration-200`) | default curve | open/close (`data-starting/ending-style`) | `ui/dialog.tsx:86` |
| Dialog popup | scale .98 -> 1, opacity 0 -> 1 (nested dialog: scale `1-0.1*n`, opacity `1-n`) | 200ms | `ease-in-out` | open/close | `dialog.tsx:135` |
| AppDialog popup | scale, opacity, translate | 200ms | `ease-in-out` | open/close | `dialog.tsx:290` |
| Popover popup | width, height, scale (.98 -> 1), opacity | 150ms | default | open/close | `ui/popover.tsx:54` |
| Popover positioner | top,left,right,bottom,transform | 150ms | default (none when `data-instant`) | reposition | `popover.tsx:47` |
| Tooltip | opacity 0->1, scale .95->1, translate 4px from side | 150ms | `ease` | open after 150ms hover delay; exit reverses | `ui/tooltip.tsx:9` |
| Dropdown menu popup | opacity, scale .95, translate 8px from side | 100ms | `ease` | open/close | `ui/dropdown-menu.tsx:39` |
| Switch track | background-color, box-shadow | 200ms | default | toggle | `ui/switch.tsx:29` |
| Switch thumb | translate 150ms, border-radius 150ms, scale 100ms (delay 100ms), transform-origin 150ms | as listed | `ease` | toggle / pressed (press stretches x 1.1) | `switch.tsx:39` |
| Tabs / segmented indicator | width, translate | 200ms | `ease-in-out` | active tab change; `motion-reduce:none` on segmented | `ui/tabs.tsx:35`, `ui/segmented-control.tsx:33` |
| Tab / segment item | color, outline-color / color, background, box-shadow | 150ms | default | hover | `tabs.tsx:48`, `segmented-control.tsx:62` |
| Scroll-area scrollbar | opacity 0 -> 1 | 100ms in (no delay), out default 150ms after 300ms delay | default | hover/scroll | `ui/scroll-area.tsx:~38` |
| Skeleton | background-position -200% | 2s, -1s offset, infinite | linear | always | `theme-vars.css:75`, `ui/skeleton.tsx` |
| Caret blink (`animate-caret-blink`) | opacity keys 0/70/100% =1, 20/50% =0 | 1s infinite | ease-out | text caret | `theme-vars.css:76` |
| Toast success | scale 1 -> 1.025 (30%) -> .99 (60%) -> 1 | 320ms | (.5,1,.89,1) | toast added (odd/even class alternates so it re-fires) | `theme-vars.css:77-78`, `index.css` keyframes |
| Toast error | translateX 0, -3px, 3px, -3px, 0 at 0/25/50/75/100% | 280ms | (.5,1,.89,1) | toast added | `theme-vars.css:79-80` |
| Toast enter/exit (anchored, sidebar) | opacity, scale .98 | 150ms / 200ms (`duration-200` sidebar) | default | add/remove | `ui/toast.tsx:69`, `layout/sidebar-toast-overlay.tsx:69` |
| `label-in` | opacity 0 -> 1, translateY 3px -> 0, fill both | 140ms | (0,0,.2,1) | label appears | `theme-vars.css:81,98-100` |
| `row-action-in` | opacity 0 -> 1 | 120ms | `ease` | sidebar row action appears | `theme-vars.css:83` |
| `tray-drain` | scaleX 1 -> 0, forwards | 8s | linear | removal tray countdown | `theme-vars.css:85`, `layout/removal-tray.tsx` |
| Spinner (`animate-spin`) | rotate 360 | 1s | linear | loading | tailwind, `ui/spinner.tsx` |
| Flicker spinner | `translateX(-100%)` of an N-frame strip | per-svg (e.g. 0.72s), `steps(N)`, infinite; paused when off-screen | steps | loading / agent working | `index.css` `@keyframes flicker-strip`, `ui/flicker-spinner.tsx:77`, `ui/flicker-strip.ts` |
| Mobile nav stack | transform, opacity | 280ms | (.4,0,.2,1) | push/pop | `layout/nav-stack.tsx:25` (mobile only) |
| Sidebar peek card | transform | 200ms | (.4,0,.2,1) | peek in/out | `layout/sidebar-peek.tsx:140` |
| Drop indicator line | top, left, width | 60ms | ease-out (`motion-reduce:none`) | drag target change | `layout/drop-indicator.tsx:22` |
| Project switcher | width, margin, opacity | 150ms | default (`motion-reduce:none`) | switcher expand | `layout/sidebar-project-switcher.tsx:219` |
| Row chevron | transform rotate 90deg | 150ms | default | expand/collapse | `sidebar/sidebar-row-actions.tsx:246` |
| Pane content fill | colors | 150ms | default | focus | `panes/components/pane-container.tsx:230` |
| Pane accent ring | opacity | 150ms | default | focus | `panes/components/pane-accent-ring.tsx:44` |
| Pane sash | colors (hover `bg-border/60`) | 150ms | default | hover | `panes/components/pane-sash.tsx:200` |
| Settings rows | colors (hover `bg-muted/50`) | 150ms | default | hover | `settings/components/settings-section.tsx:256` |
| Add-host splash | translate, scale | 500ms | ease-out | route enter | `hosts/components/add-host-screen.tsx:42-49` |

Dissolve effect (`ui/edge-dissolve.tsx:14-22,40-68`): 7 stacked layers per zone, each `backdrop-filter: blur(N px)` N = 1,2,4,8,16,32,64, masked by a vertical gradient with stops (fractions of zone height): L1 [0,.1,.3,.4], L2 [.1,.2,.4,.5], L3 [.15,.3,.5,.6], L4 [.2,.4,.6,.7], L5 [.4,.6,.8,.9] (transparent, black, black, transparent), L6 [.6,.8], L7 [.7,1] (transparent -> black). Layer `top = max(0, H*first - blur)`, `bottom = 4-stop ? max(0, H*(1-last) - blur) : 0`. `edge="top"` is flipped with `scaleY(-1)`. Used by the chat header (`tabs/components/pane-top-row.tsx:64`).

## 4. Z-order (all shell surfaces)

Tailwind `z-N` is the literal number. Higher paints on top.

| z | Element | Source |
|---|---|---|
| -10 | pane-top-row dissolve | `tabs/components/pane-top-row.tsx:64` |
| 1 | workspace content (`z-[1]`), pane content box | `layout/ide-shell-workspace-content.tsx:23`, `panes/components/pane-container.tsx:230` |
| 2 | pane accent ring | `panes/components/pane-accent-ring.tsx:44` |
| 3 | file-row indent guide | `sidebar/sidebar-file-row.tsx:152` |
| 10 | sidebar split handle, project header buttons, view-row actions, pane sash, pane-top-row overlay, dragged sortable tab | `layout/sidebar-split-pane.tsx:334`, `layout/sidebar-project-header.tsx:57,81,83`, `sidebar/view-row.tsx:135`, `panes/components/pane-sash.tsx:199`, `tabs/components/pane-top-row.tsx:54`, `tabs/components/sortable-editor-tab.tsx:26` |
| 20 | tab drop indicator (2px bar, `bg-secondary`), daemon halt drag region | `tabs/components/tab-bar-item.tsx:91`, `window/components/daemon-halt-gate.tsx:12` |
| 30 | pane handle tab, narrow-width sidebar scrim (`bg-black/30`) | `panes/components/pane-handle.tsx:58`, `layout/sidebar-split-pane.tsx:355` |
| 40 | sidebar peek card, narrow-width sidebar overlay (container <=640px), pane drop tint (`bg-secondary/10`), mobile ide | `layout/sidebar-peek.tsx:127`, `layout/sidebar-split-pane.tsx:22`, `panes/components/pane-container.tsx:218` |
| 50 | dialog backdrop+viewport, popover, select, dropdown-menu, toast positioner, drag ghost, split-drop overlay, sidebar toast viewport, build readout | `ui/dialog.tsx:86,104`, `ui/popover.tsx:47`, `ui/select.tsx:87`, `layout/drag-ghost.tsx:54`, `panes/components/split-drop-overlay.tsx:60`, `layout/sidebar-toast-overlay.tsx:120` |
| 60 | tree drop indicator line (above ghost), fallback toast viewport | `layout/drop-indicator.tsx:18`, `layout/sidebar-toast-overlay.tsx:135` |
| 100 | daemon crash panel | `window/components/daemon-crash-panel.tsx:44` |
| 10040 | custom dropdown, context-menu popups, split-view root portal | `ui/dropdown-styles.ts:25`, `ui/context-menu.tsx:69,151`, `panes/components/split-view-root.tsx:90` |
| 99999 | tooltip | `ui/tooltip.tsx:57` |

Note: dialog (50) sits below context menu and dropdown (10040) and below tooltips, so menus opened inside a dialog float above it.

## 5. Icons (Phosphor `@phosphor-icons/react` 2.1, `Icon`/`...Icon` names)

Phosphor defaults: size `1em`, weight `regular`, `currentColor`. No `IconContext` is set anywhere. Button rule: svgs without a `size-*` class are 16px (sm:size-4), `-mx-0.5`, `pointer-events:none` (`ui/button-variants.ts:4`).

| Icon (weight, size) | Where | Source |
|---|---|---|
| ChatsCircle / FolderOpen / GitBranch (14px; `fill` when active else `regular`) | sidebar mode switch Chats / Files / Changes | `layout/sidebar-mode-bar.tsx:38`, `layout/sidebar-panel-tabs.ts:1,13-30` |
| GearIcon 16 | sidebar header settings | `layout/sidebar-project-header.tsx:62` |
| SidebarToggleIcon (custom) | sidebar header and tab bar toggle | `ui/sidebar-toggle-icon.tsx`, `tabs/components/tab-navigation-buttons.tsx:36` |
| PlusIcon 16 | add space | `layout/sidebar-project-switcher.tsx:229` |
| BooksIcon (size per scale) | project glyph fallback | `layout/project-icon-mark.tsx:52` |
| Folder / FolderOpen (duotone, 20 large / 16 small), GitBranch (fill), ChatsCircle (regular), Lock (fill) | sidebar row glyph | `sidebar/sidebar-row-glyph.tsx:71-96` |
| GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning (all `fill`; amber-500 warning, red-500 fork, green-500 PR, violet-500 merge) | workspace status | `layout/workspace-branch-icon.tsx:43-102` |
| ArrowClockwise, ArrowElbowDownRight, DotsThree, GitBranch, LinkBreak, X (all `bold`) | sidebar row hover actions | `sidebar/sidebar-row-actions.tsx:77-219` |
| CaretRight rotate 90 (size-3) | row expander | `sidebar/sidebar-row-actions.tsx:246` |
| PlusIcon size-4, DotsThreeIcon size-4 bold, FolderOpenIcon, FolderIcon, TrashIcon | sidebar foot | `sidebar/tree-foot.tsx:56-88` |
| ArrowClockwise, ArrowElbowDownRight, DownloadSimple, Folder, GitBranch, LinkBreak, Lock, LockOpen, PencilSimpleLine, Terminal, Trash, X | row context menu items | `sidebar/lib/row-menu.ts:1-14` |
| PushPinIcon size-3.5 `fill`; XIcon 12 | view row pin and close | `sidebar/view-row.tsx:117,126,148` |
| WarningCircle | unread notice | `sidebar/unread-notice.tsx` |
| CheckCircle, CircleNotch (spin), Info, WarningCircle, Warning; XIcon h-3.5 w-3.5 | toasts | `layout/sidebar-toast-overlay.tsx:13-19,105` |
| LockIcon size-3, PushPin, X | tab item | `tabs/components/tab-bar-item.tsx:143` |
| PlusIcon, CaretDownIcon size-3.5, CodeIcon, SquareSplitHorizontal, GitPullRequest | tab bar buttons | `tabs/components/tab-add-button.tsx:28`, `tab-overflow-menu.tsx:50`, `open-editor-button.tsx:20`, `split-toggle-button.tsx` |
| Copy, FolderOpen, PencilSimpleLine, PushPin, PushPinSlash, ArrowCounterClockwise, TerminalWindow, X | tab context menu | `tabs/components/tab-context-menu.tsx` |
| Globe, GitPullRequest, Package, Plus, TerminalWindow | tab buffer icons | `tabs/components/tab-buffer-icon.tsx` |
| ChatsCircle (regular), GitBranch (regular) | chat branch header | `tabs/components/chat-branch-header-view.tsx:119` |
| DotsSixVerticalIcon 10 (rotate 90), ArrowsOutSimpleIcon 10, XIcon 8 | pane handle | `panes/components/pane-handle.tsx:69,80,91` |
| TerminalWindowIcon, GlobeIcon, FilePlusIcon, GitPullRequest | new-tab page | `panes/components/new-tab-view.tsx:55-66` |
| PaintBrush, CodeBlock, TreeStructure, GitBranch, TerminalWindow, Globe, Robot, Pulse, Keyboard, HardDrives (Hosts, Tauri only); Bug (dev only) | settings tabs (all `duotone`, 16px) | `settings/components/settings-tab-items.ts:1-14,30-45`, `settings-layout.tsx:44,57`, `settings-vertical-tabs.tsx:127` |
| CaretDown, MagnifyingGlass, ArrowCounterClockwise (reset), Info, Skull, Plus, Trash, DotsSixVertical (bold) | settings content | `settings/components/settings-layout.tsx`, `settings-section.tsx`, `tabs/*` |
| CloudIcon, DesktopIcon (weight dynamic) | host status | `hosts/components/host-status-icon.tsx:82` |
| XIcon, CaretDownIcon/UpIcon/UpDownIcon, CaretRightIcon, CheckIcon | dialog close, select, submenu, checkbox | `ui/dialog.tsx`, `ui/select.tsx:26,40`, `ui/context-menu.tsx:62` |
| Pencil, Upload, Smiley, Star, Trash (size-3) | icon popover | `layout/icon-popover.tsx` |

## 6. Primitive anatomy (desktop sizes, px)

Common: `border` colour `--border`; disabled `opacity .64`; focus ring `focus-visible:ring-2 ring-ring ring-offset-1 ring-offset-background` (2px ring colour `--ring`, 1px offset in `--background`); inputs use 3px ring `ring-ring/24` and `border-ring`. Invalid: `border-destructive/36`, focused 64% with ring `destructive/16` (dark 24%).

**Button** (`ui/button-variants.ts`, `ui/button.tsx`): inline-flex, gap 8 (sm size gap 6, xs gap 4), font-medium, `text-sm` (xs: text-xs; xl: text-base), border 1px, radius `lg` 10 (xs and icon-xs `md` 8), cursor pointer, `transition-shadow`.
Sizes (height / padding-x including the 1px border): default 32 / 11; sm 28 / 9; xs 24 / 7; lg 36 / 13; xl 40 / 15; icon 32x32; icon-sm 28; icon-xs 24; icon-lg 36; icon-xl 40. Icon 16px (xs 14, xl 18). `loading`: text transparent + centred Spinner.
Variants: default `bg-primary text-primary-foreground border-primary shadow-xs shadow-primary/24` + `inset 0 1px white/16%`; hover `bg-primary/90`; active/pressed inset `0 1px black/8%`, shadow none. destructive same with `--destructive`, text white (dark: `--destructive-foreground`). outline `border-input bg-popover shadow-xs/5`, hover `bg-accent/50`, dark `bg-input/32` (hover/pressed `/64`). destructive-outline text `--destructive-foreground`, hover border `destructive/32` bg `destructive/4`. ghost transparent, hover/pressed `bg-accent`. secondary `bg-secondary` hover `/90` active `/80`. link underline on hover (offset 4). `active` prop adds `bg-accent/20`. Pointer-coarse hit area 44x44 (touch only).

**Tooltip** (`ui/tooltip.tsx`): hover delay 150ms, provider `timeout=0`, `disableHoverablePopup`, `sideOffset 6`, `collisionPadding 8`, z 99999, `ui-text-sm`, padding 10x6, radius 10, border `border/70`, bg `card/95`, `shadow-lg`, `backdrop-blur-sm` (8px), nowrap, `pointer-events:none`. Shortcut variant: flex gap 8 + `kbd`. Primary variant: `bg-primary text-primary-foreground text-xs` radius 8 padding 12x6.
**Keybinding kbd** (`ui/keybinding.tsx:12`): `ui-text-sm`, min-height 16, padding-x 6, radius 8, 1px `--border`, `bg-card`, `text-muted-foreground`, inner shadow `inset 0 -1px 0 rgba(0,0,0,.12)`; keys joined with `+` (no separator on macOS).

**Popover** (`ui/popover.tsx`): sideOffset 4, radius 10, border, `bg-popover`, `shadow-lg/5`, viewport padding 16x16 (tooltip style: radius 8, text-xs, `shadow-md/5`, padding y 4 x 8), `max-w available-width`.

**Dropdown menu** (`ui/dropdown-menu.tsx`): min-w 128, `w` = anchor width, padding 4, radius 10, `bg-popover`, `ring-1 ring-foreground/10`, `shadow-md`. Item: padding 6x4 (`px-1.5 py-1`), gap 6, radius 8, `text-sm`, focus `bg-accent text-accent-foreground`, destructive focus `bg-destructive/10` (dark `/20`), disabled `.5`, svg 16. Label `text-xs font-medium muted` padding 6x4. Separator `-mx-1 my-1 h-px bg-border`. sideOffset 4, align start.

**Context menu**: in the desktop app the menu is the native OS menu (`showNativeContextMenu`), the styled popup is only a fallback (`ui/context-menu.tsx:226-248`). Fallback: min-w 180, radius 10, border, `shadow-lg/5`, padding 4; item min-height 28, padding 8x4, radius 6, gap 8, `text-sm`, highlight `bg-accent`; separator `mx-2 my-1 h-px`; submenu right/start offset -4; shortcut `text-xs tracking-widest muted/72`.

**Custom dropdown** (`ui/dropdown-styles.ts:25-50`): fixed, min-w 240, max `min(480, 100vw-16)`, radius 14, border, `bg-card/95`, padding 4, blur 8; item `ui-text-sm` padding 10x6 radius 10 gap 12, hover/focused `bg-muted`; section label padding 10x4 muted; separator `my-0.5 border-t border-border/70` (`menu-items-list.tsx:46`).

**Select** (`ui/select.tsx`): trigger min-h 32 (sm 28, lg 36), padding-x 11 (sm 9), radius 10, `border-input bg-background shadow-xs/5` (dark `bg-input/32`), `text-sm`, caret `CaretUpDown` 16 at 80% opacity, min-w 144. Popup: radius 10, border, `bg-popover`, `shadow-lg/5`, list padding 4, `alignItemWithTrigger` true, sideOffset 4, scroll arrows 24px high with gradient from popover. Item: grid `1rem 1fr`, min-h 28, padding-left 8, right 16, py 4, gap 8, radius 6, `text-sm`, highlighted `bg-accent`.

**Input** (`ui/input.tsx`): wrapper radius 10, border `input`, `bg-background`, `shadow-xs/5`, `text-sm`; control height 30 (sm 26, lg 34) plus 2px border = 32 / 28 / 36; padding-x 11 (sm 9); placeholder `muted/72`; left-icon padding 32 (sm 28), icon 16 (sm 14); autofill `bg-foreground/4`; dark `bg-input/32`. Number input heights xs 24, sm 28, md 32, icons 12/14/16, text `ui-text-sm` (md `ui-text-base`) (`ui/number-input.tsx:7-27`).

**Switch** (`ui/switch.tsx`): thumb 16, track 18 high x 30 wide, padding 1, checked `bg-primary`, unchecked `bg-input`, thumb `bg-background shadow-sm/5`, travel 12px; pressed thumb widens x1.1.
**Checkbox** (`ui/checkbox.tsx`): 16x16, radius 4, border `input`, `bg-background`, focus ring 2px offset 1.
**Label**: `text-sm/4` (14/16), medium, gap 8.

**Tabs** (`ui/tabs.tsx`): list `bg-muted` radius 10 padding 2 gap 2 `text-muted-foreground/72`; tab height 32, padding-x 9, radius 8, medium `text-sm`, icon 16, gap 6; active text `--control-foreground`; sliding indicator = `SELECTED_PILL` (`ui/selected-pill.ts`: control-surface fill, radius 10, 1px border same colour, `shadow-xs shadow-black/10`, inset `0 1px var(--elevated-highlight)`), 200ms ease-in-out. `Tab` variants: pill (rounded-full, active control-surface + shadow, inactive hover `bg-accent`), ghost (radius 6, hover/active `bg-sidebar-element-hover`), underline (2px `bg-primary` bar `inset-x-1.5`, radius 1). `Tab` blurs on mouse click and prevents default on mousedown (no focus ring after click).
**Segmented control** (`ui/segmented-control.tsx`): list `bg-muted` radius 10 padding 2 gap 2; item height 30 (sm 26, lg 34) padding-x 9 (sm 7, lg 11), radius 8, `text-sm`, inactive `muted/72`, active `text-foreground`, indicator `bg-background shadow-sm/5` (dark `bg-input`) radius 8.

**Dialog** (`ui/dialog.tsx`): backdrop `black/32` + 8px blur; viewport grid rows `1fr auto 3fr`, padding 16 (the popup sits in the upper third); popup max-w 512 (`max-w-lg`), radius 18, border, `bg-popover`, `shadow-lg/5`; header padding 24 gap 8; title `font-heading text-xl` (20px, leading none); description `text-sm muted`; footer `border-t bg-muted/72` padding 16x24, gap 8, buttons right aligned; close = ghost icon button 32 at top 8 end 8 with X 16. `AppDialog`: radius 14, `bg-popover`, max width sm 384 / md 448 / lg 672 / xl 896, header `border-b px-4 py-3` gap 8 title `font-heading text-sm`, close icon-sm 28, body padding 16, footer `border-t bg-muted/50 px-4 py-3`. Alert dialog: `ui/alert-dialog.tsx`.

**Scroll area** (`ui/scroll-area.tsx`): scrollbar 6px wide (horizontal 6 high), margin 4, thumb `--app-scrollbar-thumb` (`oklch(.55 0 0 / 42%)`, hover 58%), radius 999, hidden until hover/scroll; edge fade mask 24px. Terminal scrollbar uses 11px track with 3px transparent thumb border (`--app-scrollbar-*`, `theme.css:110-115`).
**Separator**: 1px `--border`. **Skeleton**: radius 6, bg `--muted` with a 120deg highlight stripe (white 64% light / 4% dark), 200% size.
**Avatar** 32px round `bg-background text-xs medium`, fallback `bg-muted`. **Card** radius 18, border, `bg-card`, `shadow-xs/5`.
**Toast** (anchored, `ui/toast.tsx`): radius 10, border, `bg-popover`, `shadow-lg/5`, padding 14x12, `text-sm`, icon 16 colour by type (destructive/info/success/warning), loading spins at 80% opacity; compact tooltip toast radius 8 padding 8x4 `text-xs`. Sidebar overlay (`layout/sidebar-toast-overlay.tsx`): max 3 visible (pinned `timeout:0` toasts always kept), container padding 8 gap 8, newest on bottom (`flex-col-reverse`); when sidebar is hidden, fixed 288px wide at bottom 16, left or right 16; swipe-dismiss left/right (side) or down.
**Sidebar empty state** (`ui/sidebar.tsx:177-213`): min-h 96, padding 12x24, gap 6, centred, `ui-text-sm`, line-height 1.35, 28px icon box, tones neutral/error/success; action button `ui-text-xs h-6 px-2`.

## 7. Workarounds and suspected bugs (intent vs quirk)

| Item | Source | What it is | Native port |
|---|---|---|---|
| `html[data-pane-resizing]` flattening (radius 0, no shadow, blur off, masks off) | `index.css:140-160` | WebKit re-rasterisation cost during sash drag | Not needed as a rule; intent = "drag must stay at frame rate". Keep pane chrome visually identical at rest. |
| `html[data-row-dragging]` user-select none + `cursor: grabbing` everywhere; carousel `overflow-x:hidden; will-change:transform` | `index.css:166-186` | WebKit drag workarounds | Intent kept (grabbing cursor while dragging, no text selection); the carousel scroll guard is not needed. |
| `@property --sidebar-track-width`, `--console-size` non-inherited | `index.css:15-25` | perf trick: restyle one element per drag frame | Not needed. Intent: sidebar width and console size are live drag values. |
| `[data-pane-container] { overflow: visible !important }` | `index.css:136` | lets pane shadow follow rounded content | Intent: pane shadow/ring is not clipped. |
| Reduced-motion blanket `0.01ms !important` | `index.css:44-53` | WCAG 2.3.3 | Intent kept: honour the OS setting; spinners exempt. |
| `transition: background-color 5000000s` on input | `ui/input.tsx:29` | suppresses browser autofill yellow | Not needed. |
| `Tab` preventDefault on mousedown + blur on click | `ui/tabs.tsx:92-100` | WebKit shows `:focus-visible` after trackpad click | Intent: no focus ring after pointer use, ring on keyboard focus only. |
| Duplicate token declarations: `--animate-toast-*` and toast keyframes in both `theme-vars.css:77-80,~100` and `index.css` `@theme inline`; `--color-destructive-foreground` and `--color-highlight` also in `index.css` | files named | duplicates | Port once. |
| `--highlight: oklch(0.852 0.199 91.936)` (same in light and dark) | `index.css` bottom | purpose unclear; used by `--color-highlight` | Port only if a shell component reads it; grep before porting. |
| Light `--muted-foreground` is neutral `oklch(0.4 0 0)` (#484848) while the ink hue elsewhere is ~107deg; dark `--muted-foreground` and syntax/terminal dark tokens carry hue ~266-278deg (blue-grey) | `theme.css:28,192,250-293` | leftover hue from the old blue accent system (comment at `theme.css:187-189`) | Keep the values for 1:1; flag for owner. |
| `--background-inverse`/`--foreground-inverse` "kept in sync with .dark by hand" | `theme.css:13-16,173-177` | duplicated literals | Compute from the opposite theme. |
| `--pane-accent: var(--secondary)` in base vs `var(--brand)` in crowbar light | `theme.css:73`, `crowbar.css:41` | both equal `#606c38` light; differs in nothing visible except width | Port as one token. |
| `--card: var(--color-white)` light but `--popover` white and `--background` `#fcfcfa`: popovers and cards are pure white in light | `theme.css:17-20` | intentional lift | Keep. |
| Unused / suspect tokens in shell scope: `--chart-*`, `--sidebar-primary*`, `--sidebar-ring`, `--code*`, `--git-renamed`, `--brand-ground` (phone first-run) | `theme.css` | no shell surface reads them (verify with grep per surface before porting) | Do not port unless a ported component uses them. |
| Mobile/iOS branches: `data-platform='ios'`, `data-chrome-page`, `@max-[640px]` sidebar overlay, `max-sm:` dialog bottom sheet | `theme.css:310-321`, `crowbar.css:45-49`, `ui/dialog.tsx:131-137` | phone shell | Not for desktop GPUI. The narrow-window sidebar overlay (container <= 640px, `sidebar-split-pane.tsx:22,355`) IS a desktop behaviour; confirm with the sidebar spec. |
| `Reveal in Finder` label, macOS-isms on Linux | inventory | wording | Out of token scope. |
| Dev-only: Developer settings tab, build readout (`z-50`) | inventory | debug build only | Cut. |
| Dialog backdrop has `forceRender` for nested dialogs | `ui/dialog.tsx:90` | Base UI quirk | Intent: every modal dims what is behind it, including nested modals (opacity of the lower popup `1-n`, scale `1-.1n`). |

## 8. Tokens to verify live (getComputedStyle)

On `document.documentElement` (crowbar, system mode, both light and dark): every `--*` token in section 2 of `00-tokens.md`, plus `--radius`, `--app-ui-scale`, `--ui-text-xs/sm/base/lg/xl`, `--app-font-family`, `--app-heading-font-family`, `--editor-font-family`, `--app-scrollbar-*`, `--pane-accent-width`; attributes `data-theme`, `data-theme-type`, `data-platform`; `body` background-color (expect `rgb(18,18,16)`) and `font-family`/`font-size`.
Inside `.chrome-surface` (sidebar root `ide-shell.tsx:85` and `view-tabs-row.tsx:73`) in light mode: `--background`, `--foreground`, `--accent`, `--muted`, `--sidebar-element-idle/hover/press`, `--elevated-highlight`, `--control-surface`, `--chrome-bg`.
Elements (width, height, padding, border, radius, box-shadow, font, transition): default `Button` and `icon` Button (expect 32 and 32x32); `Input` control (30 + 2 border); `Select` trigger (32); `Switch` root (30x18) and thumb (16); tabs list/tab (32 high, 9 padding); segmented item (30); mode switch in sidebar (`SidebarModeBar`: list padding 2, tab height 32); tooltip popup (padding 10x6, radius 10, bg `card/95`); dropdown-menu popup (padding 4, radius 10, ring); dialog popup (radius 18, max-w 512) and backdrop (`rgba(0,0,0,.32)`, blur 8); toast root; scroll-area thumb; `kbd`.
Also dump `getComputedStyle(el).transitionDuration/TimingFunction` of the dialog popup (200ms ease-in-out), tooltip (animation 150ms ease), menu popup (animation 100ms), switch thumb, and the sidebar peek card (200ms).
