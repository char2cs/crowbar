# `tree-row` (P3.30) — not a distinct surface; already covered twice over

`web/src/components/ui/tree-row.tsx` (45 lines) → **no new file in `crowbar-ui` or
`crowbar-app`.** This item's own two prior findings (`native/QUEUE.md` **F1** and
**F3**) already say why building it in isolation would be wrong, and reading the
two surfaces that already exist confirms the rest: `TreeRow`'s only DOM output —
one `<button>` — is fully rendered, anchored and verified under two other names.

---

## The question this item asks, answered with evidence

**Is `tree-row` a distinct surface, or is it covered by the already-ported and
verified `file-tree-row`?** Covered — and not only by `file-tree-row`. `TreeRow`
has exactly two live consumers (F1's table), and **both are already ported
components with passing gates**:

| Consumer | Port | Anchor for `TreeRow`'s `<button>` | Status |
|---|---|---|---|
| `SidebarTreeRow` (`sidebar-tree.tsx`) | `crates/crowbar-app/src/{surfaces,row_layout}/git_status_row.rs` | `git-row-button` | Part of the **Phase 1 gate** — `PHASE1-REPORT.md`: `git-status-row · dark · short · hover` and `· focus` both **PASS — 0 deltas over 8 anchors** |
| `FileExplorerTreeItem` (`file-explorer-tree-item.tsx`) | `crates/crowbar-app/src/{surfaces,row_layout}/file_tree_row.rs` | `file-row-button` | Verified under P1.5 (`native/mapping/file-tree-row.md`); `file_tree_row.rs`'s own module docs record it as "the **state** gate" |

There is no third render path. `TreeRow` in isolation (F1) renders nothing a
user ever sees; every pixel it is responsible for is already reachable through
one of the two rows above.

### The shared module already says so, in its own docs

`crates/crowbar-ui/src/components/sidebar_tree.rs`'s module doc, written before
this item existed:

> This is the native half of `web/src/components/ui/sidebar-tree.tsx` +
> `web/src/components/ui/tree-row.tsx`, **as those two are actually rendered**

and `file_tree_row.rs`'s:

> The native half of `…file-explorer-tree-item.tsx` rendered through
> `components/ui/tree-row.tsx` — the same `TreeRow` base
> [`super::git_status_row`] uses, and deliberately so.

`sidebar_tree::row_button` (used by `git_status_row.rs`, anchor `git-row-button`)
and `file_tree_row::FileTreeRow::button` (anchor `file-row-button`) are two
independent re-implementations of the **same** React element — `TreeRow`'s
`<button>` — because the two call sites sit inside different CSS cascades (F3's
correction: the file explorer tree is inside `.file-tree-container`, the git
status panel is not) and therefore render two genuinely different boxes from
one component. Both are already built, tested and — for the git row — gated.

### Confirming the indent-arithmetic angle the brief raised

`TreeRow` exports `TREE_ROW_BASE_INDENT = 14` and `TREE_ROW_INDENT_SIZE = 14` as
its own default parameter values:

```tsx
export function TreeRow({
  depth = 0,
  indentSize = TREE_ROW_INDENT_SIZE,
  baseIndent = TREE_ROW_BASE_INDENT,
  …
```

Checked both call sites for whether either one ever falls through to these
defaults:

* `SidebarTreeRow` passes `baseIndent={SIDEBAR_TREE_BASE_INDENT}` (10) and
  `indentSize={SIDEBAR_TREE_INDENT_SIZE}` (14) explicitly.
* `FileExplorerTreeItem` passes `baseIndent={FILE_TREE_BASE_INDENT}` (10) and
  `indentSize={indentSize}` (a prop threaded from `settings.fileTreeIndentSize`,
  default 16) explicitly.

**Both overrides. Neither live consumer ever reaches `TreeRow`'s own defaults.**
`TREE_ROW_BASE_INDENT`/`TREE_ROW_INDENT_SIZE` (14/14) are dead code — exported,
documented-looking, and unreachable. The *arithmetic* `baseIndent + depth ×
indentSize` is real and is exactly what both ports already implement:
`sidebar_tree::leading_padding` (10 + depth×14) and
`file_tree_row::leading_padding` (10 + depth×16, `file_tree_row.rs`'s own
"two numbers that are not the git row's" section). There is no third value to
port.

---

## Why nothing is built here

Building a `crowbar-ui::components::tree_row` (or a `--surface tree-row`) now
would do one of two things, both wrong:

1. **Render `TreeRow` "in isolation."** Per F1/F3, that means porting the
   *unscoped* class list — `rounded-md`, `hover:bg-muted`, `border-none`,
   `bg-transparent` read as written — which is precisely the shape F3 proves is
   **wrong** for both live cascades (2px vs 8px radius, 0 vs 1px border,
   `hover:bg-muted` dead in both). A gate on that surface would converge on
   numbers no user ever sees and prove nothing, the same failure §8.1 and F1
   both name.
2. **Duplicate one of the two existing ports under a third name.** Whichever
   cascade I picked, the result is a second, unmeasured copy of a component that
   already has a passing gate (git row) or a documented one (file row) — new
   anchors that add no information, at the cost of a second place for the two
   numbers to drift apart.

Per this item's own instruction — "either answer is fine with evidence; a guess
is not" — the evidence above is that `tree-row` is not a third surface. No files
under `crowbar-ui/src/components/`, `crowbar-app/src/surfaces/` or
`crowbar-app/src/row_layout/` were added for it, and none of `git_status_row.rs`,
`file_tree_row.rs` or `sidebar_tree.rs` were touched — they already carry the
correct, verified answer.

## What this changes about the "8 remaining" count

`native/QUEUE.md`'s "Position: 40 ported · 8 remaining" line (dated at Wave 5's
close) lists `tree-row` as one of the eight. Per the evidence above that line is
stale on this one point — `tree-row`'s only DOM surface (the `<button>`) has been
ported and, on one of its two call sites, gate-verified since Phase 1, before
that count was written. Left for the owner to reconcile in `QUEUE.md`, which
this branch does not touch.
