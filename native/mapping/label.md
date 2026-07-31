# `label` (P3.6)

`web/src/components/ui/label.tsx` →
`crates/crowbar-ui/src/components/label.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

One class list over `@base-ui/react`'s `useRender`, no variants, no library
styles. Every "Compiles to" below came from running the app's own
`src/index.css` through its own `tailwindcss` 4.3.0 with the utility as a
candidate.

**Reference:** `/tmp/p3-ref-label.json`, captured live from the settings
dialog's **Typography** section header at a 1714px viewport.

## 0. The headline: the `sm:` trap fires, and it kills the **call site's** class

`native/MAPPING.md` records the trap in one direction: *a call site's unprefixed
class can be dead above 640px, because tailwind-merge keeps both and Tailwind
emits the `sm:` variant later*. On this component the same mechanism fires and
the conclusion is the **opposite one** — it is the call site that loses.

The primitive carries `text-base/4.5 sm:text-sm/4`. Every live label is a
settings row or header that adds its own `ui-text-sm` or `ui-text-base`, which
`src/index.css` defines as `@utility` rules over CSS variables. Measured live on
the running app:

```text
--ui-text-sm      calc(0.75rem * 1)   = 12px
--ui-text-base    calc(0.875rem * 1)  = 14px
rendered font-size, all 12 live labels = 14px   line-height 16px
```

The six labels carrying `ui-text-sm` render at **14px, not 12px**. The `sm:`
variant is emitted in a later layer than the `@utility`, so `sm:text-sm/4` wins
at every real window width and the call site's `ui-text-sm` is **dead**.

A port that had read the call site and concluded 12px would have been wrong by
2px on every settings row. The reference settles it:

```json
"font": { "size": 14, "weight": 500, "family": "CalSansUI", "line_height": 16 }
```

The lesson `MAPPING.md` already states — measure, do not infer — holds; what is
new is that the class that misleads can be the *call site's* rather than the
primitive's, so "check the primitive for a `sm:`" is not a sufficient habit.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | `display: inline-flex` | `.flex()` | live computed `display` is **`flex`** — every live label is a flex item |
| `items-center` | `align-items: center` | `.items_center()` | not a field |
| `gap-2` | `calc(--spacing * 2)` = **8px** | `GAP` | not a field — inert with a single text child |
| `text-base/4.5` | 16px on `calc(--spacing * 4.5)` = **18px** | `BASE_STEP` | **dead ≥ 640px** |
| `sm:text-sm/4` | 14px on `calc(--spacing * 4)` = **16px** | `SM_STEP` | `font.size` 14, `font.line_height` 16 |
| `font-medium` | `font-weight: 500` | `WEIGHT` | `font.weight` = 500 |
| `text-foreground` | `color: var(--foreground)` = `oklch(0.97 0 0)` | `theme.color_foreground` | `fg` = `#f5f5f5ff` |
| — (no background) | | | `bg` = `#00000000` |
| — (no radius, no border) | preflight's `border: 0 solid` | | `radius` 0, `border.w` 0 |

## 2. Declarations

| | Value | Why |
|---|---|---|
| `content_sized` | **true** | `inline-flex` with no authored width and no stretch — the used width *is* the run's max-content width |
| `line_sized` | **true** | Nothing authors a height, so the box height *is* the run's line box |

`line_sized` is the interesting one, because `badge` and `kbd` both refuse it.
`ANCHORS.md` v1.6's test is whether the height is **derived from** the line box,
not whether the element paints text: `badge` authors `h-5`/`h-4`, `kbd` authors
`h-5`, and this authors nothing.

**On the captured cell the declaration is numerically free** — `bounds.h` is 16
and `font.line_height` is 16, so both comparisons ask the same question and
neither can fail where the other passes. It is declared anyway because it is the
true shape, and because it is what keeps the anchor honest if the type step ever
moves off an integer line box. `row_layout/label.rs` asserts the equality
directly, with `kbd`'s 4px gap as the control that stops it being a property
every box satisfies.

**The declaration is conditional on both sides.** A `<Label>` with no children
paints no run, and v1.6 makes `line_sized` valid only on an anchor carrying a
`font` — the differ refuses such a document *by name*. So `label.tsx` emits
`data-oracle-line-sized` only when `Children.count(props.children) > 0`, and
`Label::render` drops it in exactly the same case. Two extractors agreeing
inside a silence is the whole point of the file this rule lives in.

## 3. Call sites

Both live importers are settings UI, and they differ in exactly **one** visual
property:

| Call site | className | Effect on the primitive |
|---|---|---|
| `settings-section.tsx` header | `ui-font ui-text-base font-medium text-foreground` | none — restates the primitive's own weight and colour; `ui-text-base` is dead |
| `settings-section.tsx` row, `tabs/sortable-provider-row.tsx` | `ui-font ui-text-sm … font-normal text-foreground` | **`font-normal` beats `font-medium`**; `ui-text-sm` is dead |

`ui-font` resolves to the same `CalSansUI` the primitive would have inherited.
So across the whole product a call site moves the **weight** and nothing else,
which is why `CallSite` carries a weight and no other field.

## 4. Reachability

**12 live `[data-slot="label"]`**, all in the settings dialog's Appearance tab,
reached by clicking the `Settings` button. Six headers at weight 500 and six rows
at weight 400, so both call-site branches are live. One header (`Theme`) measures
`0 × 0` — it sits in the section that `first:[&>.settings-section-header]:hidden`
suppresses — which is a live `visible: false` cell that was **not** captured and
is worth knowing exists.

## 5. What is **not** modelled

* `cursor-default` and `min-w-0 flex-1 truncate` on the provider row: cursors are
  not a contract field, and the truncation belongs to that call site's flex row
  rather than to the label.
* `--width` is **vacuous**: the box is `inline-flex` with no authored width and
  no stretch, so nothing on the command line moves it.
