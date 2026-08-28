# The unified sidebar — backend implementation spec

**Date:** 2026-08-28

**Status:** implementation spec. Additive only.

**Baseline.** [`2026-08-23-unified-sidebar-design.md`](./2026-08-23-unified-sidebar-design.md)
is the model — row taxonomy, placement tree, verbs, wire contract, eight
invariants, and an eight-stage sequencing in its §7. This document revises none
of it. [`2026-08-28-sidebar-and-pane-surface-design.md`](./2026-08-28-sidebar-and-pane-surface-design.md)
is the frontend surface built on top of that model; it is the reason this
document exists, and it introduces exactly two backend requirements the model
spec did not anticipate. Everything else in the surface spec is frontend
wiring over the model spec's existing wire contract — checked directly:
pane/tab/view layout is entirely frontend-persisted (IndexedDB, via
`pane-slice.ts` / `buffer-slice.ts` → `workspace-layout.ts`), and no
`PaneGroup` / `Arrangement` / `view` concept exists anywhere under
`internal/domain` or `internal/app`. This document is the two additions, not a
restatement of the model.

**Scope:** two additions to the model spec's verbs, invariants and wire
contract. Both slot into stages already in its §7 sequencing.

---

## 1. Addition: aggregate file-count on a subtree delete

**Why.** 2026-08-28 §9 requires an idle delete's confirm to name what goes:
*"the card knows the changed-file count, the subtree walk knows the chat
count."* The chat count is free — `removal-plan.ts` already walks the
client-held tree for it (`descendantsOf`) and nothing here changes that; the
model spec's `GET /repos/:rid/tree` keeps serving the whole forest for it to
walk.

The file count is not free. Today one workspace is one delete target, so a
single `GET /repos/:rid/workspaces/:wsId` (model spec §5.1) covers it. Once a
deleted subtree can contain several independent workspace-owning rows — a
folder over two worktree chats, say — showing "N uncommitted files" means
summing git status across all of them. Doing that as N sequential
`workspaces/:wsId` calls on every delete confirm is a round-trip fan-out the
model spec's contract doesn't cover and the UI shouldn't pay for.

**Change.** One new route, resource-only like its sibling:

```
GET /repos/:rid/chats/:id/delete-preview   {chatCount, fileCount}
```

Walks the same subtree `DeleteCascade`'s successor would take, sums
`WorkingTreeSummary` (`added`/`deleted`, already computed per workspace today
— `worktree.go:898`) across every workspace-owning row it finds, and returns
the two counts in one call. No new domain type: `fileCount` is a derived sum,
never stored.

The model spec's own wire contract (§5.1) never lists a delete route at all.
This document adds it alongside the preview, since the two are naturally one
call site:

```
DELETE /repos/:rid/chats/:id
```

Cascades exactly as `DeleteCascade` does today, taking the whole subtree
(model spec invariant 4).

**Where it lands.** Model spec stage 7 ("Routes... Last on the backend") —
the delete route and its preview are new surface, not a rename, but they
depend on stage 3's row taxonomy and tree walk to know what "subtree" means
for a `chat`-type row, so they cannot land before it. The subtree walk and the
git-status sum are implemented once, in `usecases/chat/internal/tree`
alongside the placement walk, not duplicated in the handler.

---

## 2. Addition: working-guard on reparent and delete

**Why.** 2026-08-28 §9: *"A working chat is not deletable. REFUSED, not
confirmed."* This is not "delete needs what move already has" — checked in
code, **neither verb has a working-guard today.** `guardReparent`
(`api/internal/app/usecases/worktree/worktree.go:1238`) checks only
self-parent, unprovisioned parent and leaf-only; `DeleteCascade` (:1263)
checks only `WorkspaceStatusLocked`. The model spec's own invariant 5 ("a
working row does not move") is written but unbuilt — its stage 4 is still
open. This document extends that same, still-open stage to cover delete too,
rather than treating delete as a second gap.

The primitive already exists: `inflight.Work` in
`api/internal/app/usecases/chat/chat.go:240`, specifically
`(*turnstate.Work).Observe(chatID) (working, known bool, changed <-chan
struct{})` — the exact signal that feeds the frontend's `agentChats.working`
over the WS today. It is keyed per chat, which is why the guard belongs in
`usecases/chat`, not in `usecases/workspace`: a subtree is a set of `chat`-type
rows, only some of which own a workspace, and "working" is a property of a
chat, never of a workspace.

**Change.** One guard, two call sites:

```go
// usecases/chat/internal/tree
func guardNotWorking(ctx context.Context, subtreeIDs []string, work *inflight.Work) error {
    for _, id := range subtreeIDs {
        if working, _, _ := work.Observe(id); working {
            return ErrSubtreeWorking
        }
    }
    return nil
}
```

Called from both the placement verb (reparent) and the new delete verb (§1),
over the same subtree walk each already performs. Unlike the locked-branch
refusal on delete, this one is never confirmable past — the caller gets
`ErrSubtreeWorking` and stops; there is no "delete anyway."

**New invariant**, extending the model spec's §6 list:

> **9. A working row refuses both move and delete, unconditionally.** Working
> means the row itself or any row in the subtree the verb would take. Delete's
> refusal has no override; move's is the same population as invariant 5.

**Where it lands.** Model spec stage 4 ("Drop rules move server-side... the
working-row refusal") — this document only widens that stage's scope to
include the new delete verb from §1, since both verbs walk the same subtree
and both need the same check. No separate stage.

---

## 3. Sequencing delta

| stage (per 2026-08-23 §7) | this document's addition |
|---|---|
| 4 | `guardNotWorking` covers reparent (as already planned) **and** delete |
| 7 | `DELETE /repos/:rid/chats/:id` and `GET .../delete-preview` are new routes, not renames; both depend on stage 3's tree walk |

No new stage, no new package, no new domain type.

---

## 4. Success criteria (addendum)

- `DeleteCascade`'s successor and the reparent verb both return
  `ErrSubtreeWorking` for a subtree containing any working chat, and neither
  has a bypass.
- `delete-preview` returns in one call regardless of how many workspace-owning
  rows the subtree contains.
- The new invariant has a test that fails when it is inverted, per the model
  spec's own §6 convention.
