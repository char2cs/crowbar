# `workspace-branch-icon` (P3.50)

`web/src/components/layout/workspace-branch-icon.tsx` →
`crates/crowbar-ui/src/components/workspace_branch_icon.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> for the reason `avatar.md` gives.

The other `components/layout` foundation leaf P3.50 ports: the sidebar row's
status glyph, an eight-way branch (`working`, then `isPlaceholder`, then a
seven-way exhaustive `switch` on `WorkspaceStatus`) that renders exactly one
`@phosphor-icons/react` glyph or one `FlickerSpinner`, never a wrapper
around either — the same "root is the swapped element" shape `repo-avatar.md`
§0 and §8 record, this port's second instance of it.

Every "Compiles to" below came from running the app's own `web/src/index.css`
through its own `tailwindcss` 4.3.0 with the utility as a candidate.

## 0. The headline: seven statuses, five pictures, and a state axis with no branch at all

`deleted` and `pr-closed` both render `<GitFork ... text-red-500>` —
pixel-identical boxes from two different statuses, the same shape of finding
`file_tree_row.md` records for `GitStatus` ("six statuses, five colours").
The placeholder glyph (`isPlaceholder`, `text-amber-500` `<Warning>`) is
pixel-identical to `pr-conflicts`'s own `<Warning>` too — same component,
same class list, differing only in `role`/`aria-label`, neither of which
carries a field in the contract. So seven statuses plus the placeholder case
resolve to five distinct pictures, not eight.

The second, larger finding is about state rather than colour.
`workspace-branch-icon.tsx` takes exactly three props — `status`, `working`,
`isPlaceholder` — spreads none of them onward, and merges no `className`
anywhere in its own source or `WorkspaceAgentSpinner`'s. Checked
exhaustively: none of `hover`/`focus`/`selected` has a rule either. Every
other "no interaction rule" surface in this port (`crowbar-mark`, `kbd`,
`label`, `separator`, …) still keeps `StateFlag::Empty` real, because their
primitives accept a `className`/prop passthrough a call site could —
hypothetically — merge down to zero, even where none does live. This
component has no such seam anywhere, so it is the **first surface in the
port whose entire six-flag state axis is vacuous by construction** — not
"not modelled yet," but structurally unreachable, real or hypothetical.

This surface first shipped with a synthetic `WorkspaceBranchIcon::empty`
field anyway, added only to satisfy the registry invariant that then
required at least one non-mandatory flag to be real on every surface. A
follow-up (still P3.50) removed it and gave the invariant a second half
instead: `SurfaceParams::no_state_axis`, a declaration a surface makes on
purpose rather than a field with no seam behind it. See §7 for the shipped
shape and the mutations that back it.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-4` (every glyph, the spinner wrapper) | **16px** | `SIZE_4 = px(16.0)` | no reference — see §7 |
| `size-3.5` (`<FlickerSpinner>` inside the wrapper) | **14px** | `flicker_spinner::CallSite::WorkspaceIcon` | no reference |
| `text-foreground` (`Lock`, `GitBranch`, spinner wrapper) | `var(--foreground)` | `theme.foreground` | no reference |
| `text-amber-500` (`Warning`, placeholder and `pr-conflicts`) | `--warning: var(--color-amber-500)` in both tables | `theme.warning` — the one colour on this surface that **does** coincide with a token | no reference |
| `text-red-500` (`GitFork`, `deleted`/`pr-closed`) | raw palette utility; no `--` token in `theme.css` aliases it. Genuinely different from `theme.destructive`, which is `oklch(0.65 0.22 24)` in dark mode | `Color::RED_500`, `oklch(63.7% 0.237 25.331)`, hex `#fb2c36` — minted as a literal, not read from a token | no reference |
| `text-green-500` (`GitPullRequest`, `pr-open`) | raw palette utility; no `--` token aliases the green family at all (`success` is `emerald-500`) | `Color::GREEN_500`, `oklch(72.3% 0.219 149.579)`, hex `#00c950` | no reference |
| `text-violet-500` (`GitMerge`, `pr-merged`) | raw palette utility; no violet token exists anywhere in `theme.css` | `Color::VIOLET_500`, `oklch(60.6% 0.25 292.717)`, hex `#8e51ff` | no reference |

`RED_500`/`GREEN_500`/`VIOLET_500` are resolved through the same `OKLab`
pipeline `crowbar-ui/tools/gen-theme.py` uses and checked against Tailwind's
own published hex, the way `theme/token.rs`'s existing literals (`WHITE`,
etc.) already are.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `shrink-0` (every glyph, the wrapper) | `flex-shrink: 0` | `.flex_shrink_0()` | no reference |
| `flex items-center justify-center` (`WorkspaceAgentSpinner`'s wrapper only) | flex, both axes centred | `.flex().items_center().justify_center()` | no reference |
| the six glyph SVGs themselves | **no wrapping span.** `<GitBranch>` etc. render directly, with `size-4 shrink-0` on the SVG; Tailwind's preflight sets `svg { display: block }` | a **plain block `div`** — `.flex_shrink_0().w(SIZE_4).h(SIZE_4)`, no `.flex()` — `dropdown-menu.md`'s "a plain div child of a div" case, not `git_status_row`'s icon (which *is* wrapped) | no reference |
| every glyph SVG (a Phosphor icon component) | a vector line drawing | an **empty box** — no native equivalent exists (see §3) | no reference |

## 3. No gpui equivalent / not ported

| React | Why | What the port does |
|---|---|---|
| `Lock`, `GitBranch`, `Warning`, `GitFork`, `GitPullRequest`, `GitMerge` (`@phosphor-icons/react`) | `vendor/gpui-component-assets`'s bundled icon set (99 SVGs, enumerated) carries none of them — no `lock`, no `git-*` of any kind, and `triangle-alert.svg` is a *different* glyph from `Warning`, not a substitute | **empty boxes**, the same call `avatar.rs`'s `.image()` and `git_status_row`'s `.icon()` make, one step further: there is no partial substitute available at all, not even a wrong one |
| `role="img"` / `aria-label` (the placeholder `<Warning>`) | accessibility metadata, no visual effect | not ported — it is exactly what makes the placeholder glyph and `pr-conflicts`'s otherwise pixel-identical, see §0 |
| `weight="fill"` (every Phosphor icon prop) | selects which of Phosphor's SVG variants is used | not applicable — the port draws no SVG at all, so the variant choice has nothing to act on |

## 4. Painted but invisible to the oracle

**Nothing.** `workspace-branch-icon.tsx` has no shadow, no ring, no
transition, and no opacity — every field it paints (extent, colour) is one a
box records, and the box itself paints no bitmap or vector for the oracle to
disagree about the interior of.

## 5. Anchoring

| Construct | Decision |
|---|---|
| the root | [`workspace_branch_icon::ID`], `"workspace-branch-icon"` — shared by all six glyph branches and the spinner wrapper, the same "no persistent wrapper" shape `repo-avatar.md` records |
| the nested spinner | `working` cells carry a **second** anchor, `flicker-spinner` — `flicker_spinner.rs`'s own id, reused rather than reimplemented. A working cell is the only cell on this surface with two anchors; every glyph cell has exactly one |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty.** Every box is authored `size-4`; nothing takes its size from what it contains, and no branch paints a text run |
| `data-oracle-id` | **absent.** Checked, not assumed, the same finding `repo-avatar.md` records for its own file: no element, in any of the eight branches, carries one — nor does `WorkspaceAgentSpinner`'s wrapping `<span>`. `flicker-spinner.tsx` *does* carry its own id, but that id lives on a different file this component wraps, not on anything `workspace-branch-icon.tsx` itself renders. See §7 |

## 6. Traps

| Trap | What actually happens |
|---|---|
| **Wrapping a glyph's box in `.flex()`, by analogy with `git_status_row`'s icon.** | `workspace-branch-icon.tsx` renders the SVG element directly with `size-4 shrink-0` on it — no centring span. Preflight blockifies the SVG, so the reference's own computed `display` is `block`. Adding `.flex()` here draws a picture no live call site produces |
| **Treating the eight branches as eight pictures.** | `deleted`/`pr-closed` share `GitFork`; the placeholder case and `pr-conflicts` share `Warning`. Five pictures, not eight — see §0 |
| **Assuming `text-red-500`/`text-green-500`/`text-violet-500` read a `Theme` field.** | None of the three has a `--` custom property in `theme.css` to read. `text-amber-500` is the **one** exception (`--warning` aliases it in both tables) — treating all four colours the same way is the trap `RED_500`/`GREEN_500`/`VIOLET_500` exist to avoid |
| **Assuming `theme.destructive` is `red-500`.** | It is `red-500` in the *light* table only; the dark table redefines `--destructive` to `oklch(0.65 0.22 24)`, a different number. `Color::RED_500` is unconditional, the way `Color::WHITE` is |
| **Believing `--flags empty` moves anything on this surface, because every other `empty` in the port is real.** | It does not — see §7. `avatar.rs`'s, `flicker_spinner.rs`'s, and `crowbar_mark.rs`'s own `empty` fields each read a real, checkable prop the primitive genuinely accepts at an edge value no live caller chooses. This surface has no such prop, `WorkspaceBranchIcon` has no `empty` field at all, and `Params::icon` does not read `cell` — `--flags empty` parses (the vocabulary is shared) and renders the resting picture, full size, on both sides |
| **Expecting an earlier vendored copy of this port to compile.** | Before the P3.50 follow-up, `WorkspaceBranchIcon` carried a synthetic `empty: bool` field wired through to the nested `FlickerSpinner`'s own `empty`. Both are gone; `SpinnerCallSite::WorkspaceIcon`'s `FlickerSpinner` is now constructed with `empty: false`, unconditionally |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it, and this surface has more
to record than most.

- **No live oracle capture exists for this surface at all**, the same gap
  `repo-avatar.md` records for its own file and for the same reason:
  `workspace-branch-icon.tsx` carries no `data-oracle-id` anywhere, on any
  branch, and no call site adds one either. A follow-up prerequisite is
  required before a parity run can reach this surface — see
  `repo-avatar.md` §7 for the shape of that item.
- **The state axis is structurally vacuous, and the registry now says so on
  purpose rather than through a fabricated field.** §0 already establishes
  that none of the six §8.3 flags has even a *hypothetical* live branch here
  — this component takes no `className`, spreads no props, and (checked
  exhaustively) has no `hover:`/`focus:`/`data-active` rule anywhere. This
  surface first shipped keeping `WorkspaceBranchIcon::empty` real anyway,
  purely so `surface.rs`'s registry invariant — which then required at least
  one non-mandatory flag to be real on every surface — passed. A follow-up
  removed the field and gave the invariant a second half instead:
  `SurfaceParams::no_state_axis`, which this surface's `Params` returns
  `true` from, and which `crate::surfaces::ALL`'s own test now requires be
  `true` exactly when zero flags are real and `false` whenever one is — so
  it cannot be added carelessly, and it cannot go stale after a later real
  flag lands without failing a test. `FlickerSpinner`/`Avatar`'s own `empty`
  fields do **not** qualify for it: each reads a real, checkable prop the
  primitive genuinely accepts at an edge value no live caller chooses, which
  is a different thing from having no seam at all. See
  `surface.rs`'s `no_state_axis` doc comment for the exhaustive account.
- **Five of seven statuses have no reference either way**, for the same
  reason as `repo-avatar.md`'s entire §1: no anchor exists to capture
  against.

## 8. Cross-component notes added by this component

Things learned here that are **not** about `workspace-branch-icon`.

| Note | |
|---|---|
| A component can have **no** hypothetical state branch, not just no live one | every earlier "no interaction rule" surface (`crowbar-mark`, `kbd`, `label`, `separator`, `avatar`) still kept one §8.3 flag real through a prop the primitive accepts but no live caller exercises. This is the first surface where that escape hatch does not exist — the prop surface itself is closed |
| A registry invariant that requires a declaration, not just a count, survives the case that broke the count | `real > 0` alone cannot distinguish "this surface genuinely has no seam" from "this surface's real flag was dropped by accident" — both read as `real == 0`. `SurfaceParams::no_state_axis` adds the second fact the count was missing, checked both directions, so the failure mode the original assertion caught (a real axis silently marked unmodelled) stays caught, and the new one it could not express (a fabricated flag with no seam behind it) is caught too |
| Sharing one anchor id across every branch of a swapped-root component (`repo-avatar.md` §8's finding) generalises to a component with a **nested** anchor too | a working cell here carries two anchors (`workspace-branch-icon` and `flicker-spinner`); now that neither field collapses, both simply render at their own authored size, but the wiring precedent — a parent's declared state reaching everything it nests, not just its own box — still holds for whichever surface needs it next |
