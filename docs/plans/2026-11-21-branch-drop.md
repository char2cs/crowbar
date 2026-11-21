# Branch drop: reparent, then place (DND-005/006)

Spec: develop `web/src/components/sidebar/lib/drop-plan-tree.ts` (`planTreeRowDrop`),
`drop-actions.ts` (`fireRowPlacementCall`), `core/entities/run-operation.ts` (`whenSettled`),
`lib/store/applied-placement.ts` (`applyWorkspacePlacement`).

## Why
A dragged branch today goes out as a chat placement (`PATCH .../chats/{owner}/placement`). The daemon
refuses a parent in another workspace ("A thread stays in its chat's workspace"), so a branch can
never change its fork parent, and a branch filed by `PATCH .../workspaces/{ws}/placement` stays where
it was until a resync, because no frame carries a workspace's placement.

## What develop does
1. When the fork anchor of the new container differs from the branch's `parentId`: operation
   `workspace.reparent` = `POST .../chats/{owner}/reparent {newParentId: <workspace id>}` with
   `Idempotency-Key: <minted op id>`; 202. The next call waits until that operation settles.
2. Always: `PATCH .../workspaces/{ws}/placement {parentId?: folderId, order}`. Its answer
   `{workspace: {id, parentId, order}, shifted: [...]}` is applied into the entity store.
3. Locked or ownerless branches never reparent (policy keeps them at their anchor).

## Owner of the state
The store's `Entities` (per host) stays the one owner. The answer is the daemon's committed rows,
written into the same tables the stream writes; nothing is cached beside them.

## Changes (store crate only; no daemon change needed)
- `Call` gets an operation id: `Call::operation(id)` adds `Idempotency-Key` (daemonapi, header list
  exists). The id is minted with `daemonapi::uuid`.
- **Step kind 1, settle.** The runner, after sending a call that carries an operation id, awaits
  `hub.until(|state| settled(state, host, id))` before the next call. `settled` reads
  `operations[id].state` from the host's `Entities`, which the host stream fills from operation
  frames. `Hub::until` wakes on each `Hub::edit` (a `watch` channel), so this is the stream, not a
  poll. It checks the state first, so a frame that lands before the 202 answer is not missed.
- **Step kind 2, apply.** `After::Place` reads the last answer and writes `workspace` into
  `workspaces[ws].{folder_id, order}` and each `shifted` row into the chat, folder or workspace it
  names (develop `applyRows`). `Table::patch` edits a held row in place and keeps its version; a
  frame whose version is at least the held one replaces the row, so the patch never outlives the
  daemon's next frame for it.
- Planner (`plan/relocate.rs`): a branch subject computes `currentFork` (`ws.parent_id`) and the
  next anchor (walk the container up through folders to the nearest workspace, else the repo's
  default workspace; develop `workspaceAnchor`). Calls: optional reparent, then workspace placement.
- Policy (`sidebar/tree/reorder/drop_policy.rs`): an owned branch gets develop's `ALL_MODES`
  inside its repo; a locked/ownerless one keeps its anchor rules.

## Error path
- The reparent POST refused (4xx): the plan stops before the placement; "Move" failure toast with
  the daemon's reason, as now.
- The operation settles `failed`: the wait ends with `PlanError::Operation(problem.message)`;
  the placement is not sent; same toast. (develop rejects `whenSettled` with `OperationFailedError`.)
- The host leaves the state while waiting: the wait ends with `UnknownHost`. A dropped plan task
  (store shutdown) drops the wait with it and lets go of the hub. No timeout (develop
  has none: each outcome is a fact the daemon sends). A reconnect resync re-sends held operations.
- A branch whose owner chat is not recorded: `PlanError::ChatLoading` before any call (develop
  `OwningChatNotRecordedError`).
- Placement refused after a landed reparent: toast; the stream already shows the new fork parent.

## Red tests (through the public API)
1. `relocate`: a branch dropped into another branch's subtree plans `reparent` (with an operation
   id) then `workspaces/{ws}/placement`; a reorder under the same fork plans only the placement.
2. `runner`: the placement is not sent until the operation frame says `succeeded` (the test daemon
   records calls; the frame is applied via `hub.edit` after the first call); a `failed` frame stops
   the plan with the problem's message and no placement.
3. `runner`: the placement answer moves the workspace and its shifted siblings in `Entities`
   (rows read back through `tree()`), and a later frame with a higher version still replaces them.
4. `drop_policy`: an owned branch over a branch of another fork gets ALL; a locked one does not.
