# `file-tree-row` — the inline-editing capture hole (P3.12)

`web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx`
→ `crates/crowbar-ui/src/components/file_tree_row.rs`,
`crates/crowbar-app/src/surfaces/file_tree_row.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

**This file is not the surface's mapping.** The port itself is P1.5 and its
measurements are in `native/QUEUE.md` — the pseudo-element background, the
`content_sized` / `line_sized` declarations on `file-row-name`, the 63 archived
pairs. This file records **one hole**, found in P3.12 by a web test going red,
so that the next person to reach for the editing state finds the reasoning
instead of re-deriving it.

---

## The hole, stated plainly

**A file explorer row that is being renamed or created inline cannot be captured
as a `file-tree-row` reference today, and tagging it is not the fix.**

`FileExplorerTreeItemComponent` has two returns. The second is the surface. The
first — taken when `file.isEditing || file.isRenaming` — is a different row:

```text
<div class="file-tree-item">           ← no data-oracle-id
  <div class="file-tree-guides">…</div>  ← renderTreeGuides(false): guides, untagged
  <div class="file-tree-row">
    <FileExplorerIcon/>                ← no data-oracle-id
    <Input/>                           ← data-oracle-id="input-control"
                                       ←   └ data-oracle-id="input"
  </div>
</div>
```

Every `file-row-*` anchor is withheld from that branch on purpose. The two that
*are* there are not this surface's: they are the `input` primitive's own, added
by P3.4 (`native/mapping/input.md`), and they are correct — `--surface input`
depends on them.

### 1. The reference cannot be captured at all

`extractSnapshot` roots on `[data-oracle-id="file-row-item"]` and takes
`roots[index]`. The editing row carries no such attribute, so it is **not in
that list at any index**. There is no `--index` that reaches it. In an empty
document the extractor throws `root anchor "file-row-item" #0 not found`; in a
live tree — the only place an editing row exists — it silently captures some
*other*, non-editing row instead. That second outcome is the dangerous one and
is the reason this is written down.

### 2. Tagging the editing root would reproduce the exact defect v1.8 exists for

Put `data-oracle-id="file-row-item"` on that first `<div>` and the walk —
`rootEl.querySelectorAll('[data-oracle-id]')`, everything beneath the root —
pulls `input-control` and `input` into a `file-tree-row` snapshot. That is the
`resizable` shape: a root that *contains another anchored subtree*, whose
capture swallowed the whole sidebar. ANCHORS.md v1.8 decided the rule for it:

> a snapshot contains **exactly the surface's own anchors, each at most once**.
> An anchor beneath the root that belongs to *another* surface is not part of
> this one.

Two anchors from the `input` surface inside a `file-tree-row` snapshot violate
that directly, and the native side would never emit them: `file_tree_row.rs`
renders `ID_ITEM`, `ID_BUTTON`, `ID_ICON`, `ID_NAME` and the guides, and nothing
else.

### 3. v1.8's remedy is **not available to this surface**

The remedy v1.8 supplies is a declared anchor set in
`oracleSurfaceScope` (`web/src/lib/oracle/extract.ts`). It comes with a
constraint, and the constraint bites here:

> A surface may declare its set only when the set is a property of the surface
> rather than of the cell.

`file-tree-row`'s set is a property of the **cell**: `file-row-guide-{n}` exists
once per indent level, so the set at `--depth 1` is not the set at `--depth 3`.
`extract.ts` already says so in as many words, listing this surface among the
three deliberately absent from the table. Both ways of forcing a declaration are
worse than the hole:

| forced declaration | what happens |
|---|---|
| a fixed list **including** `file-row-guide-0…n` | `oracleSelectDeclaredAnchors` throws *loud-missing* on every capture at another depth — it rejects honest captures, which is the failure v1.8 explicitly refused to introduce |
| a fixed list **excluding** the guides | the guides are dropped from the reference without comment, the native side still renders them, and the differ reports anchors present on one side only — a real message naming the wrong cause |

So closing this hole needs the contract to grow a way to express a scope that is
not an exhaustive list. **That is a contract decision and it is not taken here.**
Noted as an open question, not a proposal: v1.8 already rejected one
prefix-shaped filter (`carousel-*`, hand-applied at the call site) as "a
workaround that depends on a coincidence", and whether a *per-surface* predicate
is a different thing or the same thing wearing a hat is the owner's call.

### 4. Even with a reference, there is nothing to compare against

Independent of all of the above: **the native surface has no editing variant.**
`crates/crowbar-ui/src/components/file_tree_row.rs` renders one shape — there is
no `Input` in it — and `surfaces/file_tree_row.rs` exposes no option that would
select one (`Params` is empty; the surface's only knobs are `--depth`,
`--prev-depth`, `--next-depth`, `--git-status`, parsed by the shared parser).
Porting the editing row is a piece of work that has not been done and is not
queued. Both blocks are real; fixing either one alone changes nothing.

---

## What it actually costs, measured against what it does not

**It is not a §8.3 matrix cell.** The state vocabulary is fixed at
`empty | loading | error | hover | focus | selected` (ANCHORS.md v1.1, mirrored
in `row_surface::StateFlag`), and `editing` is not one of them. So no cell of
the matrix is unreachable and no archived run is affected: the 63 pairs in
`native/oracle/runs/` are all captures of the second branch and stay
byte-for-byte comparable.

**It corrupts no capture that is possible today.** An editing row is a *sibling*
of every `file-row-item`, never a descendant, and the walk only descends. So
`input-control` and `input` cannot enter any `file-tree-row` snapshot as things
stand — the risk is entirely in the future, at the moment someone tags that
branch expecting it to work.

**What is genuinely lost** is a visual state of a shipped component that the
oracle cannot see at all: the inline rename/create row, which is where the file
tree paints a text field, a focus ring and a bottom border it paints nowhere
else. If the native port ever grows that row, this file is the reason its gate
does not exist yet.

---

## Traps

**Do not "fix" this by removing `input.tsx`'s anchors.** They are correct and
`--surface input` is built on them. The anchors are not the defect; the walk
having no way to exclude another surface's subtree is.

**Do not assert "the editing branch has no anchors."** It did until P3.4 and the
web test said so as a count of *all* anchors; that assertion went red the moment
the `input` primitive was anchored, and the premise it was standing in for —
*no second `file-row-item` root* — was never violated. The test now states the
premise directly and pins the remaining set, so a third primitive's anchors
arriving in that branch still fails:
`web/src/__tests__/lib/oracle/file-row-anchors.test.tsx`, *"leaves the
inline-editing row untagged by any file-row anchor"*.
