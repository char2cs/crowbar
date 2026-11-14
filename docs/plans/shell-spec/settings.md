# Settings modal spec (1/3): chrome, navigation, row anatomy, controls, behaviour

Source root: `app/panes/src` (abbrev. `src/`). Tailwind classes resolved at 1 rem = 16 px, UI scale 1 (default `uiFontSize` 15, see "UI scale"). Colour tokens are named (`--popover`, `--muted`, ...); hex per theme lives in `00-tokens.md` (fork 1). Items marked **VERIFY** were derived from class strings and must be checked with `getComputedStyle` in the live app (checklist in settings-schema.md section 6).

Related files: `settings-pages.md` (every page, field by field), `settings-schema.md` (proposed schema, port list, ownership, capture checklist).

## 1. Open / close

| Trigger | Source |
|---|---|
| Gear icon button in the sidebar header (16 px `GearIcon`, `icon-sm` ghost, tooltip "Settings", aria-label "Settings") calls `openSettingsDialog()` -> tab `appearance` | `src/components/layout/sidebar-project-header.tsx:52-63`, `src/features/window/stores/ui-state-store.ts:107-108` |
| Host picker "host settings" entry opens tab `hosts` | `src/components/layout/sidebar-project-switcher.tsx:78` |
| Native sidebar message `openSettings` (phone shell only) | `src/features/window/hooks/use-native-sidebar.ts:446-448` |
| **No keyboard shortcut and no menu item exist.** Keymap registry has no settings command. | `src/features/keymaps/registry.ts` (grep: none); Tauri src has no Settings menu |
| Close: header X button, Esc, click on backdrop (Base UI Dialog defaults, not overridden), `onClose` sets `isSettingsOpen=false` | `src/components/ui/dialog.tsx:214-217,256-263`; `src/components/layout/ide-shell.tsx:154-159` |

State machine (`src/features/window/stores/ui-state-store.ts:34-110`, `settings-dialog.tsx:24-46`):

| State | Transition | Effect |
|---|---|---|
| closed (`isSettingsOpen=false`, dialog unmounted: `ide-shell.tsx:154`) | `openSettingsDialog(tab?)` | `isSettingsOpen=true`, `settingsInitialTab = tab ?? 'appearance'` |
| open | tab click/arrow | local `activeTab` set AND `settingsInitialTab` set (last tab is remembered only within the session, not persisted) `settings-dialog.tsx:38-41` |
| open | `openSettingsDialog(other)` while open | effect re-syncs `activeTab` to `settingsInitialTab` (deep link while open) `:31-35` |
| open | close | `clearSearch` runs on unmount `:44-46`; search text is never kept |

Reopening via the gear always resets to Appearance (`tab ?? 'appearance'`), so "remember last tab" in the dialog comment only holds while the dialog stays mounted.

## 2. Window chrome (AppDialog)

All from `src/components/ui/dialog.tsx` (AppDialog `:201-279`, backdrop `:16-32`, viewport `:34-48`) with the Settings overrides at `src/features/settings/components/settings-dialog.tsx:52-68`.

| Element | Spec |
|---|---|
| Backdrop | fixed inset 0, z-50, `bg-black/32` (black at 32% alpha), `backdrop-blur-sm` (blur 8 px), `transition-all duration-200` (150 ms default easing is overridden to 200 ms, timing fn = Tailwind default `cubic-bezier(.4,0,.2,1)`), enter/exit: opacity 0 -> 1 (`data-starting-style` / `data-ending-style`). `forceRender` so it always dims. |
| Viewport | fixed inset 0, z-50, `grid grid-rows-[1fr_auto_3fr]`, `justify-items-center`, `p-4` (16 px). Popup is `row-start-2`: the free vertical space is split 1 : 3 (top : bottom), i.e. the modal sits above centre. Top gap = 16 + (H - 32 - modalH) / 4. |
| Popup size | `h-[74vh] max-h-[820px] w-[90vw] max-w-[1120px] min-w-0`; also base `w-full`, `max-w-[calc(100%-2rem)]`. Under a 720 px wide window: `h-[86vh] w-[calc(100vw-32px)]` (`max-[720px]`). |
| Popup surface | `bg-popover text-popover-foreground`, `rounded-xl` (`--radius-xl` = 14 px; `--radius` = 10 px, `src/styles/theme.css:9`, multipliers `theme-vars.css:80-86`), `shadow-lg/5` (Tailwind `shadow-lg` = `0 10px 15px -3px / 0 4px 6px -4px` at 5% alpha), `overflow-hidden`, `border-0` (Settings removes the border; AppDialog itself has none), flex column, `outline-none`. |
| Popup motion | `transition-[scale,opacity,translate] duration-200 ease-in-out`; enter/exit change **opacity only** (0 <-> 1). No scale class is present in AppDialog (unlike `DialogPopup`), so there is no scale/slide. Exit anim likely does not play because the parent unmounts on `onClose` (**VERIFY** with a frame capture). |
| Header | `shrink-0 flex items-center gap-2 border-b px-4 py-3`; Settings adds `[&>div:first-child]:border-b-0` (the header's border is removed). Header height = 12 + content + 12 (content = the 28 px close button -> 52 px; **VERIFY**). Under 720 px: becomes `grid grid-cols-[minmax(0,1fr)_auto] gap-2`. |
| Title | `DialogPrimitive.Title`, `font-heading font-normal text-sm leading-none` = Instrument Serif (default `--font-heading`, `theme-vars.css:7-13`) 14 px, line-height 1. Text "Settings" (hidden under 720 px, where a dropdown replaces it). |
| Header actions | wrapper `ml-2 flex items-center gap-2`; holds the search field (`w-64` = 256 px; `max-[720px]:w-44` 176 px; `max-[520px]:w-32` 128 px). |
| Close button | Base UI `Dialog.Close` rendered as `Button variant="ghost" size="icon-sm"` with `ml-auto shrink-0`; `size-7` (28 px, since `sm:` breakpoint >= 640 px applies; `size-8` below), icon phosphor `XIcon` (16 px default), sr-only "Close". Hover: `bg-accent`. `rounded-lg` (10 px). `src/components/ui/button-variants.ts:4,15,33`. |
| Content slot | `flex-1 min-h-0` + Settings passes `flex p-0` (no padding, no overflow here; the children manage scroll). |
| Footer | none (settings apply live). |
| Focus | Base UI Dialog: modal, focus trapped, returns focus on close (library default; **VERIFY** initial focus target). |
| Reduced motion | `@media (prefers-reduced-motion: reduce)` forces all transition/animation durations to 0.01 ms (`src/index.css:34-43`). |

Stacking: dialogs opened from inside Settings (confirm, Add host) are separate Base UI dialogs at z-50 and dim the modal again (`dialog.tsx:27` `forceRender` comment; nested popups use `--nested-dialogs` scale/opacity only in `DialogPopup`, not AppDialog).

## 3. Body layout

`SettingsPanels` (`src/features/settings/components/settings-layout.tsx:88-116`):

```
[ rail 208 px | content flex-1 ]      container: flex h-full w-full min-w-0 overflow-hidden
```

| Region | Spec |
|---|---|
| Rail | `w-52` = **208 px**, `shrink-0`; hidden under 720 px (title dropdown stands in). Inner list: `role=tablist aria-orientation=vertical aria-label="Settings sections"`, `flex-1 space-y-0.5` (2 px gap) `overflow-hidden p-2` (8 px padding) `settings-vertical-tabs.tsx:64-72`. The list does not scroll natively; a wheel handler (`onWheelCapture`) scrolls it programmatically when content overflows (`:40-49`). No divider line between rail and content, no rail background (same `bg-popover`). |
| Content | `id=settings-panel-<tab> role=tabpanel tabIndex=-1`, `min-w-0 flex-1 overflow-y-auto p-3` (12 px; `max-[720px]:p-2` 8 px). Scrollbar = global app scrollbar tokens (`--app-scrollbar-size 11px`, thumb `oklch(0.55 0 0 / 42%)`, hover 58%, radius 999, `src/styles/theme.css:111-117`). Each page root is `space-y-4` (16 px between sections). Sets `--app-ui-control-font-size: var(--ui-text-sm)`. |
| Tab change | content swaps instantly (no animation); scroll position is not preserved (new subtree). |

Responsive: breakpoint 720 px (rail -> dropdown), 640 px (row stacks: label above control, control full width), 520 px (search 128 px). Desktop shell windows are normally wider than all three; the native shell must still honour them if the window can be resized below 720 px (`app/mobile/src-tauri/tauri.conf.json:26-27` sets the initial window to 1200x800; no minimum is set there, **VERIFY** the desktop config).

### Dropdown title (under 720 px)
`SettingsTitle` `settings-layout.tsx:31-66`: trigger `h-7 max-w-48 gap-1.5 rounded-md border border-border/70 bg-card/50 px-2`, active tab icon 16 px duotone + truncated label + `CaretDown` 14 px; hover `bg-muted`. Menu `min-w-44`, items icon 16 + label, active item `bg-muted text-foreground`.

## 4. Rail tab anatomy

`src/features/settings/components/settings-vertical-tabs.tsx:74-131`, items `settings-tab-items.ts:28-51`.

| Property | Value |
|---|---|
| Element | `Button variant=ghost compact`, `role=tab`, `id=settings-tab-<id>`, `aria-selected`, `aria-controls=settings-panel-<id>`, roving tabindex (0 on active, -1 others) |
| Box | `w-full h-auto justify-start gap-2.5 rounded-xl px-2.5 py-1.5 text-left`: gap 10 px, radius 14 px, padding 6/10 px, 1 px transparent border (Button base `border`) |
| Icon | phosphor, `size-[18px]`, `weight="duotone"`, colour `currentColor` |
| Label | `font-heading` (Instrument Serif) `text-[19px]`, truncate, line-height inherited from `Button`'s `text-base` (1.5) -> about 28.5 px. Row height therefore about 28.5 + 12 + 2 = 42.5 px (**VERIFY**). Button has `font-medium` (500) |
| Idle | `text-muted-foreground`, transparent bg |
| Hover | `bg-accent text-foreground` (accent is already a translucent overlay: `oklch(0 0 0 / 8%)` light, `oklch(1 0 0 / 10%)` dark; `theme.css:32,196`) |
| Active | `bg-primary/12 text-foreground` (primary at 12% alpha; primary `#606c38` light, `#98a86a` dark) |
| Focus | Button base has `outline-none`; no visible focus ring on rail tabs (**VERIFY**; keyboard focus appears only via the hover/active tint) |
| Transition | Button base `transition-shadow` only (no background transition) |
| Keyboard | ArrowDown/ArrowRight next, ArrowUp/ArrowLeft previous (wraps), Home first, End last; each press selects AND focuses (activation follows focus) via `requestAnimationFrame` focus `:51-62,89-112` |
| Empty | text "No matching settings", `ui-text-sm`, muted, centered, `p-2` (when search filters out every tab) |

Tab list in order (ids are persisted keys, `ui-state-store.ts:8-20`; labels/icons `settings-tab-items.ts`):

| # | id | Label | Phosphor icon | Gate |
|---|---|---|---|---|
| 1 | appearance | Appearance | PaintBrush | always |
| 2 | editor | Editor | CodeBlock | always |
| 3 | file-explorer | Files | TreeStructure | always |
| 4 | git | Git | GitBranch | always |
| 5 | terminal | Terminal | TerminalWindow | always |
| 6 | browser | Browser | Globe | always |
| 7 | providers | Agents | Robot | always |
| 8 | active-sessions | Active sessions | Pulse | always |
| 9 | keybindings | Keybindings | Keyboard | always |
| 10 | hosts | Hosts | HardDrives | `isTauri()` (`src/lib/crowbar-bridge.ts:387`): true in the desktop app; the native shell always shows it |
| 11 | developer | Developer | Bug | `import.meta.env.DEV` only. **Do not port.** |

Note the display order: Keybindings is last of the shared list, Hosts follows it (not alphabetical, not the order in the `SettingsTab` union).

## 5. Search

Header field: `Input size=sm leftIcon=MagnifyingGlass`, placeholder "Search settings...", id/name `settings-search`, width 256 px (`settings-layout.tsx:69-85`). Input anatomy (`src/components/ui/input.tsx`): outer span `rounded-lg` (10 px) 1 px `border-input` (= `--border`), `bg-background` (`dark:bg-input/32`), `shadow-xs/5`, text `text-base sm:text-sm` (14 px); inner input `sm:h-6.5` = 26 px, `px-[calc(2.5*4-1px)]` = 9 px, left padding `ps-7` = 28 px when icon (icon 14 px at `start-2` = 8 px, `text-muted-foreground`); placeholder `text-muted-foreground/72`; focus: `border-ring` + 3 px ring `ring-ring/24`; invalid: `border-destructive/36` (focused `/64`, ring `destructive/16`); disabled opacity 64%; transition-shadow.

Behaviour:
1. Typing calls `setSearchQuery` (`src/features/settings/store.ts:103-108`) which runs `runSearch` immediately (no debounce). The 51-record index (`config/search-index.ts`, lazy-imported on first query) is scored with `scoreSearchQuery` (`src/utils/search-match`): weights label 11, keyword 6 each, description 1, section 1. Results sorted by score. Only the **set of tabs that have at least one result** is used.
2. Rail shows only matching tabs (`settings-tab-visibility.ts`); tabs absent from the index (**keybindings, hosts, active-sessions**) disappear during any search. Index tab counts: appearance 7, browser 5, editor 18, file-explorer 7, git 2, providers 4, terminal 8.
3. If the active tab is not first visible, the first visible tab is activated automatically (`settings-vertical-tabs.tsx:31-38`).
4. In the page, every `SettingRow` filters itself again with a **different matcher**: case-insensitive substring over label and string description (`settings-row-search.ts`); non-matching rows render nothing; a section whose rows are all hidden is `hidden` including its heading (`settings-section.tsx:50-66`). Rows with a ReactNode description match on label only. Known quirk: index-level and row-level matchers disagree (a keyword-only hit selects a tab whose rows are all hidden -> blank page).
5. Search clears on close. There is no clear button and no Esc-to-clear (Esc closes the dialog).

## 6. Section and row anatomy (desktop; ignore every `IS_MOBILE` branch)

### Section (`src/features/settings/components/settings-section.tsx:45-104`)

| Part | Spec |
|---|---|
| `<section>` | `px-1 py-0.5` (4/2 px); `data-settings-section=<title>`; `hidden` when search active and no row visible |
| **First section of a page hides its header** | class `first:[&>.settings-section-header]:hidden` (`:74`): the first `Section` child of a page shows no title or description. Pages whose first section is a titled group therefore start straight with rows (Appearance "Theme", Editor "Editor", Files "Display", Git "Git View", Terminal "Typography", Browser "Navigation", Keybindings "Preset" incl. its description). Agents, Active sessions and Hosts draw their own 32 px `h3` in the body to compensate. |
| Header | `settings-section-header mb-2 px-1 py-1.5` (8 px bottom margin) |
| Title | `Label` with `font-heading`, size `var(--settings-section-title, 32px)` = **32 px** (the variable is never defined), `text-foreground`, base Label classes `inline-flex items-center gap-2 font-medium`. Line-height: Label's `sm:text-sm/4` is merged away by the size override; effective line-height **VERIFY** |
| Description | `p.ui-font.ui-text-sm.text-muted-foreground` (12 px) |
| Body | `SettingsCard` = `div.space-y-2` (8 px between rows) on desktop |

### Row (`SettingRow`, `:120-312`)

```
| [label + reset] ..................... [control] |   flex justify-between items-center gap-3
| [description]                                   |
```

| Part | Spec |
|---|---|
| Root | `div role=presentation flex justify-between items-center gap-3 rounded-lg px-1 py-2 select-none transition-colors hover:bg-muted/50 focus-within:bg-muted/50` -> radius 10 px, padding 8 px vertical / 4 px horizontal, hover and focus-within wash `--muted` (light `oklch(0 0 0 / 4%)`, dark `#262624`) at 50% alpha, color transition 150 ms ease (Tailwind default). Under 640 px: `flex-col items-stretch gap-2`. |
| Text column | `min-w-0 flex-1` |
| Label line | `flex items-center gap-1.5` (6 px): `Label.ui-font.cursor-default.font-normal.text-foreground.ui-text-sm` (12 px / weight 400, family = UI font, default CalSansUI) + optional `labelAccessory` + optional reset button |
| Reset button | wrapper `span.flex.size-5.items-center.justify-center` (20 px), `Button variant=ghost compact` (size default) containing phosphor `ArrowCounterClockwise` (16 px default), `aria-label "Reset <label>"`, tooltip "Reset <label>" (150 ms delay, top). When value equals default: `pointer-events-none invisible` (space is kept; it never collapses). Click writes the default via `updateSetting`. **VERIFY** the real hit size: the button is `h-8` (32 px) inside a 20 px box (overflows/clips?). |
| Description | `ui-font ui-text-sm cursor-default text-muted-foreground` (12 px, no margin), wraps |
| Control column | `shrink-0` (full width and shrinkable under 640 px); fixed widths come from `SETTINGS_CONTROL_WIDTHS` (below) |
| Row click | Mouse-only: a click on empty row space forwards to the row's primary control (`:183-237`): segmented -> next item; combobox/expander -> open; text input -> focus + select; others -> focus + click. No keyboard equivalent (the control itself is tabbable and gets `aria-labelledby`/`aria-describedby` from the row, `:167-178`). |
| `layout="stacked"` | only differs on mobile; on desktop no effect |

`SETTINGS_CONTROL_WIDTHS` (`settings-control-widths.ts`): compact 112 px, default 144, wide 176, xwide 224, number 112, numberCompact 96, text 192, textWide 224 (all `max-w-full`).

## 7. Controls (exact anatomy)

Common: `--radius-lg` = 10 px; 1 px border; focus ring = 3 px `ring-ring/24` + border `--ring` (light `neutral-400`, dark `neutral-500`); disabled = opacity 64%.

### 7.1 Switch (`src/components/ui/switch.tsx`)
Size prop `sm` only sets `data-size`; geometry is breakpoint-based: at >= 640 px `--thumb-size` = 16 px.
Track: height thumb+2 = **18 px**, width thumb*2-2 = **30 px**, `rounded-full`, padding 1 px. Unchecked `bg-input` (= `--border`, light `#dddcd6`, dark `#333330`); checked `bg-primary`. Transition `background-color, box-shadow 200 ms` (ease default). Thumb: 16 px circle `bg-background`, `shadow-sm/5`, `will-change-transform`; checked: `translate-x(thumb-4px)` = 12 px and `origin` moves to `thumb-size 50%`; transitions `translate .15s, border-radius .15s, scale .1s (delay .1s), transform-origin .15s`. Pressed (mouse down on switch or its label): thumb stretches `scale-x-110` and radius becomes `16px/17.6px`. Focus-visible: `ring-2 ring-ring ring-offset-1 ring-offset-background`. Disabled: `cursor-not-allowed`, 64%.

### 7.2 Select trigger (`src/components/ui/select.tsx:10-45`, `setting-select.tsx`)
`size="sm"`: `min-h-8` (32 px) but `sm:min-h-7` = **28 px**; `min-w-36` (144 px) overridden by width class; `rounded-lg`, `border border-input`, `bg-background` (`dark:bg-input/32`), `shadow-xs/5`, padding-x `calc(2.5*4-1)` = 9 px, gap 6 px, text `text-base sm:text-sm` = 14 px, `--ui-text-sm` is NOT applied here. Value `flex-1 truncate`, placeholder muted. Trailing icon `CaretUpDown` 16 px, `-me-1`, 80% opacity. Inset top highlight via `before` pseudo: `shadow-[0_1px_black/4%]` (dark: `0_-1px white/6%`), removed on focus/disabled/invalid/pressed. Focus-visible: `border-ring`, 3 px ring `ring-ring/24`. Invalid: `border-destructive/36`. Transition `box-shadow` (150 ms).

Popup: portal, `z-50`, positioner `alignItemWithTrigger=true` (the selected item overlays the trigger, macOS-style), `align=start`, `sideOffset=4`; panel `rounded-lg border bg-popover shadow-lg/5`, min-width = trigger width, inner list `max-h-(--available-height) overflow-y-auto p-1` (4 px); scroll arrows 24 px tall with a fade gradient from `--popover`. Item: `grid grid-cols-[1rem_1fr] gap-2 min-h-7 (sm) rounded-sm (6 px) py-1 ps-2 pe-4`, text 14 px; highlighted `bg-accent text-accent-foreground`; selected shows a 16 px check (stroke 2) in column 1; disabled 64%. Motion: none beyond Base UI defaults (**VERIFY**).

### 7.3 Number input (`src/components/ui/number-input.tsx`)
Container `div[data-slot=number-input] flex items-center gap-1` (4 px) with the width class. Children: `-` button, field, `+` button.
- Buttons: `Button variant=ghost` default size (`sm:h-8` = 32 px, padding-x 11 px, `shrink-0`), icon `Minus`/`Plus` `size-3` (12 px) for `size=xs`; disabled when at min/max. **VERIFY**: with widths 96-112 px the buttons (about 36 px each + 4 px gaps) would leave only about 16-32 px for the field (`min-w-[5ch]`); the real computed widths decide how the native control must be drawn.
- Field: `type=text inputMode=decimal`, height `h-6` (24 px) for `xs`, `ui-text-sm` (12 px), `px-2`, `rounded-md` (8 px), `border border-border`, `bg-muted`, `text-center tabular-nums text-foreground`, `min-w-[5ch] flex-1`, `outline-none`, disabled 50%. No visible focus style beyond the browser default (none).
- Behaviour: typing updates the setting immediately for every parseable partial value (`handleInputChange` calls `onChange` per keystroke; the store clamps only where `normalizeSettingValue` knows the key, otherwise the clamp happens on blur). Blur: empty or `-` -> 0 then clamp to `[min,max]`, else parse and clamp, reformat with `precision` of `step` (strips trailing zeros). ArrowUp/ArrowDown step by `step` (default 1), clamped. +/- buttons step and clamp. Display uses `Math.round` when step is integral.

### 7.4 Text input (`Input size=sm`) as in section 5; for Browser homepage/custom URL width 224 px; `aria-invalid` for the invalid custom URL.

### 7.5 Textarea (file filters, `file-tree-settings.tsx:152-168`)
`ui-font ui-text-sm w-48 (192 px) max-w-full resize-none rounded-md border border-border bg-muted px-2 py-1.5 text-foreground outline-none placeholder:text-muted-foreground focus:border-ring`, `rows=2`. Commit on blur or Enter (Shift+Enter inserts newline) -> split on `,`, trim, drop empties -> array (`parsePatterns :14-18`). External changes re-sync the text as `join(', ')`.

### 7.6 Button in a row ("Clear data"): `variant=outline size=sm` (`h-7` = 28 px, padding-x 9 px, `rounded-lg`, `border-input`, `bg-popover`, hover `bg-accent/50`, dark `bg-input/32`, hover `bg-input/64`), text 14 px weight 500.

### 7.7 Tooltip (`src/components/ui/tooltip.tsx:8-16`)
`ui-text-sm` (12 px), `rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 shadow-lg backdrop-blur-sm`; open delay 150 ms; `animate-in fade-in-0 zoom-in-95` + directional slide 4 px (tw-animate-css defaults: 150 ms); close reverses. z-index 99999.

### 7.8 Badge (descriptor checks) `src/components/ui/badge.tsx`: `rounded-sm` (6 px), height 18 px (`sm:h-4.5`), padding-x 3 px, `text-xs` 12 px weight 500; `success`: `bg-success/8 text-success-foreground` (dark: `/16`, `text-success`); `warning`, `error` likewise with `--warning`/`--destructive`.

## 8. Animation table

| Element | Property | Duration | Easing | Trigger | Source |
|---|---|---|---|---|---|
| Backdrop | opacity (+ blur present throughout) | 200 ms | cubic-bezier(.4,0,.2,1) | open/close | `dialog.tsx:23` |
| Popup | opacity (scale/translate declared but unused) | 200 ms | ease-in-out | open/close | `dialog.tsx:227` |
| Row hover wash | background-color | 150 ms | cubic-bezier(.4,0,.2,1) | pointer enter/leave, focus-within | `settings-section.tsx:256` |
| Switch track | background-color, box-shadow | 200 ms | default | toggle/focus | `switch.tsx:29` |
| Switch thumb | translate, border-radius, transform-origin | 150 ms | default | toggle | `switch.tsx:32` |
| Switch thumb | scale | 100 ms, delay 100 ms | default | press | `switch.tsx:32` |
| Select/Input focus ring | box-shadow | 150 ms | default | focus | `select.tsx:11`, `input.tsx:45` |
| Tooltip | opacity + scale 95->100 + 4 px slide | 150 ms | tw-animate default | hover after 150 ms | `tooltip.tsx:9` |
| Reset button | visibility (no animation) | - | - | value != default | `settings-section.tsx:282` |
| Agents row drag | transform/transition from dnd-kit | dnd-kit default 250 ms ease | - | drag | `sortable-provider-row.tsx:36` |
| Rail tab, tab content swap | none | - | - | - | - |

All collapse to 0.01 ms under `prefers-reduced-motion: reduce`.

## 9. UI scale and typography

- `uiFontSize` (px, 10..24, step 0.5, default 15) -> `--app-ui-scale = uiFontSize / 15` (4 decimals) set on `<html>` (`font-style-injector.tsx:84-92`, `ui-font-size.ts`). `--ui-text-xs/sm/base/lg/xl` = 11 / 12 / 14 / 16 / 20 px x scale (`theme.css:105-109`). Only `ui-text-*` utilities scale; Tailwind `text-sm/base/xl`, `h-8`, paddings and icon sizes do **not**. The native modal must treat these as two classes of metrics.
- Font stacks (`font-style-injector.tsx:9-27`): UI = `"<uiFontFamily>", ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` (default CalSansUI, bundled); heading = `"<headingFontFamily>", Georgia, "Times New Roman", serif` (default Instrument Serif); mono = `"Geist Mono Variable", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace`. Font files: `app/panes/public/fonts`, declared in `src/styles/theme-fonts.css`.
- Where the heading family appears in the modal: dialog title (14 px), rail labels (19 px), section titles (32 px), page-level `h3` (32 px).

| Text | Family | Size | Weight | Colour |
|---|---|---|---|---|
| Dialog title | heading | 14 px | 400 | `--popover-foreground` |
| Rail label | heading | 19 px | 500 (Button `font-medium`; Instrument Serif has 400 only, so 400 renders; **VERIFY**) | muted / foreground |
| Section / page title | heading | 32 px | 500 label (same caveat) | `--foreground` |
| Row label | UI | 12 px x scale | 400 | `--foreground` |
| Row description | UI | 12 px x scale | 400 | `--muted-foreground` |
| Control text (select, input, buttons) | UI | 14 px (not scaled) | 400/500 | `--foreground` |
| Number field | UI | 12 px x scale | 400, tabular | `--foreground` |
| Keybinding chip | mono | 12 px x scale (`ui-text-sm`) | 500 | foreground |
| Muted note | UI | 12 px x scale | 400 | `--muted-foreground` |

## 10. Colour tokens used (names; hex in 00-tokens.md)

Light (`:root`, `src/styles/theme.css:10-53`) / dark (`.dark`, `:195-210`): `--popover` white / `#1c1c1a`; `--popover-foreground` `#1b1b19` / `#f2f0ea`; `--foreground` `#1b1b19` / `#f2f0ea`; `--muted-foreground` `oklch(0.4 0 0)` / `oklch(0.834 0.008 278.6)`; `--muted` `oklch(0 0 0 / 4%)` / `#262624`; `--accent` `oklch(0 0 0 / 8%)` / `oklch(1 0 0 / 10%)`; `--primary` `#606c38` / `#98a86a`; `--border` = `--input` `#dddcd6` / `#333330`; `--ring` neutral-400 / neutral-500; `--destructive` `#b8422c` / `#f1a493`; `--destructive-foreground` `#7c2717` / `#3b211c`; `--success` `#4e7a36` / `#b9cf98`; `--warning` `#d9a441` / `#e9c97f`. `--background` `#fcfcfa` / `#1c1c1a`. Themes: `crowbar` (default) and `frosted` (`src/extensions/themes/theme-registry.ts:12-24`). The Crowbar theme paints chrome dark even in light mode, but the settings modal body uses `bg-popover` (not a chrome-surface), so it follows the real light/dark mode.

## 11. Intent versus observed bugs (owner rule: record intent, never port quirks)

Visuals above are 1:1. For behaviour, each row states the **Intent** the native shell must implement and the **Observed** workaround or bug that must NOT be ported. "Wiring" was checked by grepping non-settings consumers of each key (`app/panes/src`, excluding tests).

| # | Area | Intent (port this) | Observed (do not port) | Source |
|---|---|---|---|---|
| 1 | Keybindings presets | The user picks a base keymap; per-command overrides win. Ship the presets that are real. | The page says IDE presets are "not yet provided" and offers "Compact (alternate)", whose only content is moving Reopen closed tab to `mod+shift+r`, a proof that switching re-resolves. A select plus a disclaimer is a stub. Port only the override model; show a Preset select only if owner defines real presets, never the disclaimer. | `keybindings-settings.tsx:152-155`, `keybinding-presets.ts:1-2` |
| 2 | Keybindings "display-only" rows | Every shortcut shown is rebindable. | Save and Save all (Editor category) render a disabled button plus a "display-only" chip because `use-save-keyboard.ts` hardcodes them. The native keymap must route these through the registry and drop the chip, or hide the rows. Project switch `mod+shift+digit` is also hardcoded and invisible. | `registry.ts:172-186`, `use-save-keyboard.ts` |
| 3 | Keybindings capture | A chord is captured and saved; Esc cancels. | Preset section description is never visible (first-section header hidden); any bare key (no modifier) can be saved and shadows typing; conflicts only warn. Intent for conflicts/bare keys is undefined: owner to decide before porting. `KeymapOverrides` comment says null means unbound but the type is string. | `keybindings-settings.tsx:137-139`, `keymaps/types.ts:32` |
| 4 | Search | One matcher decides which pages and rows match. | Two matchers (scored 51-record hand-kept index for tabs, substring for rows) disagree, so a keyword-only hit selects a tab whose rows are all hidden (blank page); Keybindings/Hosts/Active sessions are missing from the index; an effect force-switches the active tab on each query change. Replace by a single schema-derived matcher. | `store.ts:103-150`, `settings-vertical-tabs.tsx:31-38`, `settings-row-search.ts` |
| 5 | Section header rule | Each page's first group shows its title. | Header of the first section is CSS-hidden, and three pages re-draw an `h3` to compensate; Preset's description is lost. Parity of look: first block shows no title (keep for 1:1), but model it as an explicit `showHeader:false` per section, not a `:first-child` rule. | `settings-section.tsx:74`, `providers-settings-view.tsx:83-87` |
| 6 | Font pickers | Choose among available fonts; a stored font that is not available falls back to the default once at load. | The picker is a resync effect that rewrites the setting from inside render of a settings page whenever the list changes; "(bundled)" and "(custom)" labels, `validateFont` (always true) and the Nerd-font filter are dead because OS fonts are not enumerable (list is the 4 bundled fonts; the terminal list is one entry). Native shell may enumerate system fonts; that is a new decision, not parity. | `font-selector.tsx:21-111`, `font-store.ts:108-135`, `terminal-settings.tsx:33-47` |
| 7 | Number field | Edits commit once with clamping. | Each keystroke calls `onChange` unclamped before blur clamps; `0` is written for empty on blur even where 0 is out of range; two local state copies mirror one value (an effect resyncs them). Port: parse on commit (Enter, blur, step), clamp to min/max, never write intermediate values. | `number-input.tsx:299-352` |
| 8 | Reset affordance | A value that differs from default shows a reset icon; click restores it. | Icon box is 20 px but wraps a 32 px ghost button; the icon is `invisible` instead of absent; a few rows (Agents permission level, Hosts, Clear data) have none. | `settings-section.tsx:273-288` |
| 9 | Last tab memory | Opening Settings from the gear starts on Appearance; deep links choose a tab. | A comment claims the last tab persists across open/close but `openSettingsDialog` resets to `appearance`; two copies of the active tab (`activeTab` and `settingsInitialTab`) are kept in sync by an effect. One owner suffices. | `settings-dialog.tsx:27-41`, `ui-state-store.ts:107` |
| 10 | Exit animation | Modal fades out (200 ms). | Parent unmounts the dialog on `onClose`, so the exit style probably never runs. Needs live frame check; implement the intended fade-out. | `ide-shell.tsx:154-159`, `dialog.tsx:214-217` |
| 11 | Browser page | "Clear data" is enabled iff the platform can clear data, known at open. | Capability is fetched asynchronously after mount, so the row first shows the "Not available on this system..." text and Clear data disabled, then flips (the inventory noted this tab "raced"). Resolve capability before first paint. | `browser-settings.tsx:41-56` |
| 12 | Browser custom URL | Invalid template is flagged; the default engine is used until valid. | Raw text is written to storage on every keystroke even when invalid (description text switches instead of blocking). Acceptable; store as typed, validate on use. | `browser-settings.tsx:57-58,108-115` |
| 13 | Text-list fields | Patterns apply on commit. | `useEffect` resyncs the draft string from the store on every external change; commit on blur AND Enter both exist. Port as one draft with commit on blur/Enter. | `file-tree-settings.tsx:24-45` |
| 14 | Agents list | Reorder and toggles persist to the daemon, last action wins. | Optimistic write plus a global write-generation fence and a separate GET fence exist only to stop stale responses from rewinding state; "Retry" and the three-state loading text are the real intent. Daemon should return the resolved list; shell keeps one in-flight write queue. Permission level has its own duplicate fence (`writeGeneration`). | `use-providers-settings.ts:64-87`, `default-permission-level-setting.tsx:185-209` |
| 15 | Hosts store | One host list shared by every window. | Persisted via `localStorage` with a manual cross-window `rehydrate` on the `storage` event and `current()` merge to avoid lost writes: a workaround for N webviews. In the native shell the host list has one owner (Rust, Agent C). | `host-config-store.ts:50-94` |
| 16 | Settings persistence | Settings survive restart; panes see changes. | One localStorage key per setting, 300 ms debounce, and a `storage`-event sync for the phone page. Native: one owner writes a settings file and pushes changes; no debounce window where a crash loses a value. Also never write defaults for unstored keys (keeps later default changes effective); keep that rule. | `settings-persistence.ts`, `settings-storage-sync.ts` |
| 17 | Build readout row | (Undecided) | Row exists in release, default off, controls a dev readout the inventory marks CUT. | `appearance-settings.tsx:232-243` |

### Settings not wired to a live consumer in the native shell (do not port as live controls)

Grep found at least one consumer for every row key. Status by consumer:

| Group | Keys | Status |
|---|---|---|
| Monaco / Plate editor options | fontFamily, fontSize, editorLineHeight, tabSize, wordWrap, lineNumbers, renderWhitespace, renderIndentGuides, semanticHighlighting, highlightOccurrences, showMinimap, autoSave, formatOnSave, inlineBlame, autoCompletion, parameterHints, markdownFontSize | Wired only to the web editor (`features/editor/*`, `--md-base-font-size`). The inventory classes the editor stack REDESIGN. If the editor pane stays a web pane, the Editor page is live but the values are pushed from the shell to that pane; if no editor ships, drop the page. Do not render controls that write keys nobody reads. |
| Terminal | terminalFont*, terminalLineHeight, terminalLetterSpacing, terminalScrollback, terminalCursor* | Wired to xterm (`features/terminal`). The native terminal (Wave 3 J) must read all eight or the rows are dead. |
| No UI row, no consumer outside settings | windowTransparency (only a root attribute), coreFeatures, sidebarWidth (layout owns widths), buildBadgeOverride, syncSystemTheme, autoThemeLight, autoThemeDark, chatSplitPresentationEnabled (dev), diffViewMode (review pane toggle) | Not controls; keep or drop with their consumers, never as settings rows. `headingFontFamily` and `markdownFontSize` have no TS consumer: they reach the UI only via CSS vars (`--app-heading-font-family`, `--md-base-font-size`). |
| `compact` Button prop, `--app-ui-button-*`, `--app-ui-badge-height`, `--settings-section-title` | | Defined, consumed nowhere (visual no-ops). |

### Workarounds and suspected bugs (consolidated)

1. First-section header hidden by `:first-child` CSS, compensated by hand-drawn headings; hides Preset's description (rows 3, 5).
2. Preset select + "not yet provided" disclaimer + Compact proof preset (1).
3. "display-only" keybinding rows, hardcoded save/save-all and project-switch chords (2).
4. Two search matchers; blank page on keyword-only hit; auto-switching tab effect; three pages missing from index (4).
5. `FontSelector` resync effect rewriting settings; dead validation and Nerd-font code; single-entry terminal font list (6).
6. Number field commits unclamped per keystroke; mirrored local state with resync effect (7).
7. Reset icon box 20 px vs 32 px button; invisible instead of absent (8).
8. Two copies of the active tab; "remembered tab" comment is false (9).
9. Exit animation probably never plays (10).
10. Browser capability fetched after paint, flashes disabled text (11).
11. Draft-string resync effects for pattern textareas (13).
12. Generation fences for provider and permission writes (14).
13. Host list cross-window rehydrate workaround (15).
14. Per-key localStorage with debounce and storage-event sync (16).
15. Build readout row in release (17).
16. Unused CSS variables and `compact` prop.
17. `NumberInput` layout may overflow its 96-112 px box (see section 7.3, VERIFY).
