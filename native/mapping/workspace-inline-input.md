# `workspace-inline-input` (P3.62)

`web/src/components/layout/workspace-inline-input.tsx` →
`crates/crowbar-ui/src/components/workspace_inline_input.rs`,
`crates/crowbar-app/src/surfaces/workspace_inline_input.rs`,
`crates/crowbar-app/src/row_layout/workspace_inline_input.rs`.

> Cluster 7, "small tree-row controls" (`native/mapping/layout-denominator.md`
> §8): `workspace-inline-input.tsx` · `placeholder-row-actions.tsx`.

**No captured reference.** This item adds the `data-oracle-id`s to the React
source as part of landing the port (below) — until an oracle run is taken
against them, the values here come from the app's own compiled Tailwind and
a probe element inserted into the live document, removed again, the method
`native/MAPPING.md` and `inline-error.md` both fix. **Verdicts are the
queue's**, not this item's — no snapshot JSON was captured or fabricated.

## 0. The three live call sites, and what each one drives

| Call site | `defaultValue` | `kind` | `resolveExisting` passed? |
|---|---|---|---|
| `workspace-tree-item.tsx`'s rename (`isRenaming`) | `workspace.branch` | `identifier` (default) | no |
| `workspace-tree-item.tsx`'s create-child, `repo-section.tsx`'s create | `''` | `identifier` (default) | **yes** |
| `repo-section.tsx`'s repo rename | `repo.name` | `identifier` (default) | no |
| `agent-chat-row.tsx`'s chat-title rename | `title` | `prose` | no |

**The hint is reachable from exactly two call sites, both `identifier`.** No
live call site passes both `kind="prose"` and `resolveExisting`, so a
`prose`-with-hint cell is a real combination this surface can drive but one
`web/src` never produces — flagged rather than silently treated as covered.

## 1. The field is `input.rs`'s exact finding, one door over

`web/src/components/ui/input.tsx`'s own module docs establish that an
`<input>` is a void DOM element — `childNodes.length === 0`, so
`web/src/lib/oracle/extract.ts`'s `oracleOwnText` returns `''` and the
reference's record for it carries only the box (`bounds`, `bg`, `visible`,
`radius`, `border`). This component's own `<input>` is **not** that
primitive — grepped, and confirmed by reading the real JSX: a bare `<input>`
tag, no `Input` import — but it is exactly the same DOM element, so the
finding transfers unchanged. The port opts `ID_FIELD` in through
`AnchorSink::boxed` only, never `.boxed_text`/`.text`, and paints the
value/placeholder string as a plain unanchored child — `input::Input::field`'s
own doc comment, restated: *"The string is a plain unanchored child."*

## 2. The hint stretches, and wraps, and both are measured rather than assumed

Probed live (real classes, the app's own compiled Tailwind) at 248px — the
field's own real content width inside the sidebar's `ROW_BASE` row: 294px
sidebar, less `mx-1.5 px-1.5` twice (24px) and the leading `WorkspaceBranch
Icon` (16px) plus its `gap-1.5` (6px): `294 − 24 − 22 = 248`.

| Probe | Result |
|---|---|
| Hint `getBoundingClientRect().width` at every value length tried | **equals the root's** — the button stretches, it does not size to its text |
| `'main' already has a workspace — open it` (40 chars after `.trim()`) | wraps to **two lines**, `h: 32` |
| `'x' already has a workspace — open it` (37 chars, one-char branch) | stays on **one line**, `h: 16` |

**`text-left` is the tell.** The root is `flex flex-col` with no
`items-center`, so `align-items` computes to its `stretch` default and a
block-level flex item (which a `<button>` becomes once it is a flex child)
stretches across the cross axis unless it opts out. `text-left` is otherwise
a no-op on a content-sized box; it stops being one exactly when the box is
wider than its text, which is what stretching produces. Because the wrap
point sits at ~37–40 characters and the fixed frame around a branch name
(`'` + `' already has a workspace — open it`) is already 36 characters, **any
git branch name of ordinary length wraps this hint to two lines** — a
one-or-two-character branch is the only case measured to stay on one line,
not a shape any real branch takes. So unlike `inline-error.tsx`'s single
unbreakable run, this hint's *typical* rendering is the two-line wrap, and
`ID_HINT` is declared **neither** `content_sized` (it stretches) **nor**
`line_sized` — v1.6 is a claim about a box derived from *one* line box, and a
wrapped paragraph's height is derived from as many line boxes as it happens
to wrap to on each engine, which is a different and less safe claim.

## 3. Values

| React / Tailwind | Compiles to / Probed | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `mt-0.5` (hint) | 2px | `HINT_MARGIN_TOP` | compared |
| `text-[13px]` (field), inherited line-height | 13px / `19.5px` box (probed, exact) | `FIELD_TEXT_SIZE`, `FIELD_HEIGHT` | invisible — no text field on this anchor at all |
| `text-[11px]` (hint), inherited line-height | 11px / `16.5px` line, WebKit floors to `16` — identical to `inline_error::DETAIL_STEP` | `HINT_TEXT_SIZE` | compared, not `line_sized` (§2) |
| `placeholder:text-muted-foreground/40` | 40% | `PLACEHOLDER_ALPHA` | invisible (field carries no `fg`) |
| `text-muted-foreground/70` (hint) | 70% | `HINT_ALPHA` | compared |
| `font-mono` (field, `identifier` only; hint, always) | — | `theme.font_mono` | invisible on the field, compared on the hint |

`line-height: 1.5` is unitless, so `FIELD_HEIGHT` does not depend on `kind` —
`font-mono` and the ambient sans produce the identical box at the identical
size, asserted directly
(`the_field_height_does_not_depend_on_kind`/`prose_kind_does_not_move_the_field_height`).

## 4. Declarations

`CONTENT_SIZED = []`, `LINE_SIZED = []` — see §1 for the field (no text field
exists to declare either way) and §2 for the hint (stretches, and wraps to a
variable line count).

## 5. The state axis

`empty` is real: `defaultValue=''` — the create-child call sites' actual
starting state — shows the placeholder instead of a value. `--hint` is this
surface's own option rather than a §8.3 flag: no word in that vocabulary
names "a collision was found," and it is driven directly rather than by
simulating the `resolveExisting` lookup that would organically produce it —
the `nav-stack`/`sidebar-peek` precedent (`layout-denominator.md` §4) for a
boolean a real store/pointer action would otherwise compute.

`hover`/`focus`/`selected`/`loading` are unmodelled: the source carries no
`hover:`/`focus-visible:`/`data-active` rule of its own on either element —
the hint's `hover:text-foreground` has no reference (synthetic pointer events
are denied on this project's machines, `button.rs`'s standing finding) — and
nothing here loads.

`--viewport-width` is vacuous: neither element carries an `sm:` rule,
asserted directly (`viewport_width_moves_nothing_on_this_surface`).

## 6. What is not ported

| Thing | Status |
|---|---|
| the field's caret and selection highlight | **absent** — `input.rs`'s own finding: neither has an element, neither can carry an anchor |
| `onBlur`/`onKeyDown` (Enter/Escape) confirm-or-cancel | **absent** — interaction, Phase 5's remit, not a static cell's |
| the hint's `hover:text-foreground` | **absent**, no reference (see §5) |

## 7. Reachability

Three importers, all in `components/layout`: `workspace-tree-item.tsx` (two
call sites — rename and create-child), `repo-section.tsx` (two call sites —
repo rename and create-child), and `agent-chat-row.tsx` in
`features/agent/components/` (chat-title rename). §0 has the full table.
