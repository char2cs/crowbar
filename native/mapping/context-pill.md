# `context-pill` (P3.58)

`web/src/components/layout/context-pill.tsx` →
`crates/crowbar-ui/src/components/context_pill.rs`,
`crates/crowbar-app/src/surfaces/context_pill.rs`,
`crates/crowbar-app/src/row_layout/context_pill.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — see the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method), not off a
live capture.

## 0. What this file is, and what it is not

`context-pill.tsx` is `ContextPill()`: the sidebar's "you are here" pill,
always mounted, rendering one of three pictures (`kind: 'workspace' | 'home'
| 'project'`) or nothing at all (`kind: 'empty'`). Clicking it opens a
centred command dialog carrying `WorkspaceSwitcherMenu` — that dialog's own
chrome is `command.tsx`'s already-captured surface
(`crowbar_ui::components::command`, whose own fixture is documented as *"the
live workspace switcher, reached from the Context Pill"*), and its own item
content is `workspace-switcher.md`. This file's own job is the pill itself.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `px-2` (wrapper) | 8px | `OUTER_PADDING_X` |
| `pb-1` (wrapper) | 4px | `OUTER_PADDING_BOTTOM` |
| `pt-0` (wrapper) | 0px | literal `px(0.0)`, no constant |
| `gap-2` (trigger) | 8px | `TRIGGER_GAP` |
| `px-3` (trigger) | 12px | `TRIGGER_PADDING_X` |
| `py-1.5` (trigger) | 6px | `TRIGGER_PADDING_Y` |
| `rounded-lg` (trigger) | `theme.radius_lg` | reused, not re-derived |
| `bg-sidebar-element-idle` | `theme.sidebar_element_idle` | reused |
| `gap-0.5` (label stack) | 2px | `STACK_GAP` |
| `text-xs` (small line) | 12px font, **16px line** (Tailwind's own paired ratio `calc(1/0.75)`, independent of font) | `SMALL_TEXT` / `SMALL_LINE_HEIGHT` |
| `text-[13px]` (large line) | 13px font, **18px line** — see §2 | `LARGE_TEXT` / `LARGE_LINE_HEIGHT` |
| `size="lg"` on `<RepoAvatar>` | `repo_avatar::Size::Lg` | reused directly (`RepoAvatar::render`) |
| `<Library size={14}>` | 14px, unpainted | `LIBRARY_SIZE` |
| `hover:bg-sidebar-element-hover` | a colour-only rule | not modelled — no field on this contract |

## 2. `text-[13px]`'s own line height is transferred, not re-measured

`text-[13px]` carries no paired `line-height` utility, so its box is CSS
`normal` — resolved through the font's own metrics, not a number Tailwind
states. `context-pill.tsx`'s trigger and `workspace-switcher.tsx`'s
`CommandItem` both set this exact font size under the *same* font family:
the trigger's own `font-mono` and `command.tsx`'s `font-editor` are both
`var(--editor-font-family)`, confirmed by `command.rs`'s own module docs
("the same variable, read through a different custom property"). So
`workspace_switcher::CONTENT_HEIGHT`'s already-documented 18px (*"a 13px
label's own line box"*) applies unchanged here — recorded as
`LARGE_LINE_HEIGHT`, not re-derived from a second measurement of the same
font at the same size.

## 3. `scale-110` is not modelled

`context-pill.tsx` wraps its trailing glyph in `<span className="flex
shrink-0 scale-110">`. CSS `transform: scale()` does not participate in
layout — the scaled element keeps its own untransformed box for every
purpose a box tree cares about — but it **does** move
`getBoundingClientRect()`'s answer, which reports the painted, post-
transform box. gpui has no paint-time-only scale on `Styled` that leaves the
layout box alone the way CSS's does. `ContextPill::render` renders the icon
at its own natural extent; a future oracle comparison on this one anchor's
`w`/`h` will show a `size × 0.10` delta that is the contract's own gap, not
a port defect.

## 4. `kind: 'empty'` is not modelled

`ContextPill()` returns `null` in that cell — there is no element to
measure, so `crowbar_ui::components::context_pill::ContextPill` has three
variants, not four.

## 5. Anchoring

`context-pill.tsx` carried no `data-oracle-id` before this item. Two are
added:

* `context-pill` on the outer wrapper `<div>` — this surface's own root.
* `context-pill-trigger` on the `<Button>` inside `CommandDialogTrigger`'s
  `render` prop, overriding `button.tsx`'s own `data-oracle-id: 'button'`
  default. Namespaced for the same reason `sidebar-project-header`'s own
  four buttons are: nesting `Button::render`'s own `anchors.root(...)`
  inside this surface's root would contest which anchor `ANCHORS.md` §4
  means, so this port builds its own box from `button::Size`/`RadiusClass`'s
  public values instead — and the React id has to stop colliding with the
  generic `button` surface for the same reason.

Composed, not authored here: `repo-avatar` (`RepoAvatar::render`),
`workspace-branch-icon` (`WorkspaceBranchIcon::render`, and, on the
`working` branch, `flicker-spinner` one level deeper). No
`oracleSurfaceScope` entry needed — every nested anchor this composition can
reach is one it actually paints, never left unpainted the way
`sidebar-project-header`'s toggle icon is.

## 6. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — every box here is authored
(padding/gap), never a bare text run's own line box exposed as the anchor's
height.

## 7. The state axis

Every one of the six §8.3 flags is unmodelled — the trigger's own
`hover:bg-sidebar-element-hover` is a colour-only rule with no field on this
contract, and there is no `focus:`/`data-active` rule at all.
`--kind`/`--status`/`--working`/`--avatar` are this surface's own axis
instead, the `fps-overlay` shape. `Params::no_state_axis()` returns `true`.

## 8. `row_layout` coverage

* the default cell carries the root, the trigger, and the status glyph's own
  nested anchor, never `repo-avatar`
* `--avatar` swaps the status glyph for `repo-avatar`; `--working` swaps it
  back to the spinner even with `--avatar` set (the two-step precedence)
* `--kind home`/`--kind project` never carry `repo-avatar`
* the trigger sits inset by the wrapper's own `px-2`/`pt-0`
* the root's own width tracks `--width` exactly

## 9. Reachability

`ide-shell.tsx` → the sidebar column, mounted unconditionally above
`sidebar-tab-bar.tsx`.

---

## ❌ VERDICT — FAIL, 3 deltas over 2 anchors (2026-08-03, taken by me)

```
context-pill.bounds.h:          52.0, expected 51.0   (Δ +1.0)
context-pill-trigger.bounds.h:  48.0, expected 47.0   (Δ +1.0)
context-pill-trigger.border.w:   0.0, expected  1.0   (Δ -1.0, exact)
```

Anchor sets match exactly (2 vs 2), so this is geometry rather than scope.
Canary `native-short.json` byte-identical immediately before. Returned to P3.58.

### The drive that produced the reference — ANCHORS **v1.14**

```
reference:  live Tauri app, route #/ide/<id>/home, dark, viewport 1714,
            context pill in its HOME kind (text "oracle-fixturehome",
            exactly one <svg>, no nested oracle ids beyond the trigger)
            captured via import('/src/lib/oracle/extract.ts')
native:     crowbar-app --surface context-pill --kind home \
                        --width 344 --viewport-width 1714 --theme dark
```

**`--kind home` is the point of this note.** My first run used the surface's
default `workspace` kind and produced a spurious `workspace-branch-icon`
anchor-presence delta — a *fourth* delta that was mine, not the port's. The
anchor-presence line is what exposed it: a cell mismatch shows up as a **set**
difference before it shows up as a geometry one.

### The live measurement the deltas rest on

The trigger's computed border is **`1px solid rgba(0, 0, 0, 0)`** — a real 1px
border whose colour is transparent. **A transparent border still occupies
width**, and §5 compares `border.w` **exactly**, because ±0.5 on a 1px border is
a 50% error and plainly visible.

**Neither `row_layout` test could have caught this**: they run under a
`NoopTextSystem` and pass either way. Only the live capture sees it.
