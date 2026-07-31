# `inline-error` (P3.10)

`web/src/components/ui/inline-error.tsx` →
`crates/crowbar-ui/src/components/inline_error.rs`.

**No reference.** See §0 — and the reason is stronger than "I could not drive
it".

## 0. ⛔ Reachability — 0 live, and the render guard **cannot become true**

`separator` and `skeleton` were reported as "0 live instances". This one is a
step further: it was *measured* to be unreachable, not merely unreached.

The single call site is `components/layout/workspace-tree.tsx:57`:

```ts
if (wsListData.status === 'error' && repos.length === 0) return <InlineError … />
```

Tracing `wsListData`:

| Step | File | What it establishes |
|---|---|---|
| 1 | `lib/store/loadable-slice.ts:64` | **Exactly one** writer of the error state: `catch { set({ data: failed(err) }) }`. It fires only when `cfg.fetcher` rejects. |
| 2 | `lib/store/workspace-list.ts:23` | This store's fetcher is `buildTreeFromCache` — two `getAllEntities` calls plus `buildScopedRepoTree`. |
| 3 | `lib/persistence/entity-cache.ts:30` | `getAllEntities` is `try { … } catch { return [] }`. |
| 4 | `lib/store/build-repo-tree.ts:126` | `buildScopedRepoTree` is a pure filter and grouping over arrays. |

**The fetcher cannot reject, so the status cannot become `error`, so the panel
cannot mount.**

Note the fetcher reads **IndexedDB and not the network** — so even a dead daemon
does not produce this state. The panel's `onRetry` calls the same `fetch()`.

### Confirmed in the running app, not only read

Two injections, neither of which touched the store or its cache:

1. `getAllEntities('not_a_real_store')` — makes `idb`'s `getAll` reject —
   returned **`[]`**.
2. `indexedDB.open` replaced with a thrower, then a read of a real store —
   returned **its rows**, because `getDB()` caches the handle, so a post-open
   IDB failure is invisible too.

Both went through the catch arm; neither propagated. `indexedDB.open` was
restored.

**No reference JSON was fabricated.** `git-row-dir`, `separator` and `skeleton`
are the precedent: rendered by the port, absent from the product.

## 1. Where the values came from

The utilities, resolved through the app's own compiled `tailwindcss` 4.3.0 and
then read back off a **probe element inserted into the live document** and
removed again — so the numbers include any stylesheet that would have overridden
them (which, unlike `callout-node`, none does). The probe was taken at the
sidebar's real **294px**, which is the panel's only call site.

| React / Tailwind | Compiles to | Probe | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|---|
| `p-6` | `calc(var(--spacing) * 6)` | 24px | `PADDING` | compared |
| `gap-2` | `calc(var(--spacing) * 2)` | 8px | `GAP` | compared |
| `flex-1 flex-col items-center justify-center` | — | 294 × 128 | `.flex_1().flex_col().items_center().justify_center()` | compared |
| `text-center` | `text-align: center` | — | **absent** — gpui has no text-align; the column's `items-center` is what centres the runs, and it is what the DOM measures too | invisible |
| `text-lg` (glyph) | `--text-lg` / `calc(1.75/1.125)` | 18px / 28px | `GLYPH_STEP` | compared |
| `opacity-50` | `opacity: 50%` | 0.5 | `GLYPH_OPACITY` | **invisible** — no field, and v1.7 hides an anchor only at *zero* |
| `text-sm font-medium text-foreground` | `--text-sm` / `calc(1.25/0.875)` | 14px / 20px / 500 | `TITLE_STEP`, `TITLE_WEIGHT` | compared |
| `font-mono text-[11px] text-muted-foreground` | arbitrary; no paired line-height | 11px / **16.5px**, box **16** | `DETAIL_STEP` | compared |
| `mt-1` on the control | `calc(var(--spacing) * 1)` | 4px | `RETRY_MARGIN_TOP` | compared |
| `<Button variant=outline size=sm>` | — | `sm:h-7` = 28 | `Size::Sm` + `Variant::Outline`, read not copied | compared |

## 2. Declarations

`CONTENT_SIZED` = glyph, title, detail, **retry**; `LINE_SIZED` = glyph, title,
detail, **not** retry.

- `items-center` on a flex column leaves the cross axis unstretched, so every
  child's width is its own content's — while the panel is `flex-1` and is in
  neither list.
- The retry control authors `h-8 sm:h-7`, which is `badge`'s rule: v1.6's test is
  "derived from the line box", not "paints text".
  `the_three_runs_are_line_sized_and_the_control_is_not` asserts both directions,
  so the control is the paired control that keeps the claim from being vacuous.

**The detail line is v1.6's exact case.** `text-[11px]` × the inherited `1.5`
ratio is **16.5**, and the probe measures the box at **16** — WebKit floored it.
Declaring `line_sized` is what turns that into a 0.5px quantisation instead of a
delta.

## 3. ⚠ `overflow` on this surface is a **wrap** test, not a clip test — and a hyphen is a break

The detail `<p>` carries no `truncate` and no `overflow-hidden`, so `clipped` is
`false` at every length and the box grows instead of clipping.

Two things were measured while getting the fixture right, and both are general:

1. **A hyphen is a break opportunity.** The `overflow` string was first
   `ECONNREFUSED-while-reading-the-workspace-entity-cache` — no spaces — and it
   **still wrapped**: `U+002D` is line-break class `HY`. Measured: the box came
   back 33px against 16.5, exactly two lines. *"Unbreakable" means no spaces
   **and** no hyphens*, which is stronger than the brief states and stronger than
   what `callout-node`'s single-word fixture satisfied by accident.

2. **gpui wraps an over-long unbreakable run where WebKit would let it
   overflow.** With `overflow-wrap: normal` and no break opportunity, CSS spills
   the run on one line; gpui breaks it to the box. That is a whole line box of
   disagreement on `bounds.h`, compared at ±0.5 — **such a cell could never
   converge**, and its cause would be the engines rather than the port.

So this surface's `overflow` fixture is the **longest run that still shapes on
one line**, which keeps all three content cells comparable. The
genuinely-too-long case is pinned by
`an_unbreakable_run_wider_than_the_box_wraps_here_and_would_not_in_webkit`,
driven through `--title` so it measures a real second rendering.

## 4. v1.8: no anchor set is declared, and the reason is the **build**

The detail line sits behind `import.meta.env.DEV`, so a dev build has five
anchors and a shipped one has four. That makes the set a property of the *build*
rather than of the surface, and v1.8 permits a declaration only in the second
case — a declared anchor that is legitimately absent is a refusal, not a delta.

The retry `<Button>` **is** renamed to `inline-error-retry` at the call site,
which is the other half of v1.8 and `git-row-badge`'s precedent: without it the
capture would carry `button`'s own id, which belongs to another surface.

## 5. What is not ported

| Thing | Status |
|---|---|
| `text-center` | **absent.** gpui has no `text-align`; the flex centring is what both engines actually measure here. |
| the retry control's `hover`/`focus`/`active` | **absent.** They are `button`'s, and the control is composed from `button`'s *resting* values. Driving them would move a box this surface does not own. |
| `aria-hidden` on the glyph | **absent.** No AX in this port (§10.4, dropped). |
