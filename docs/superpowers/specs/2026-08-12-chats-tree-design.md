# Chats tree — threads, folders, search

**Status:** design, awaiting review
**Branch:** `feature/chats-folders`
**Mock (normative for geometry and interaction):** https://claude.ai/code/artifact/9a84a3cb-2deb-4a20-9b34-17c93b0bbce3

The Chats panel is a flat, client-ordered list. The workspace tree beside it is a
real hierarchy with folders, drag, kept rows and a removal tray. This brings the
Chats panel onto that same footing and adds the one thing the workspace tree has
no equivalent of: chats that hold other chats.

---

## 1. The model

### 1.1 A chat holds threads

A chat may have child chats. A child is a **thread**: it hangs off its parent and
reads that parent's turns.

Not a fork. Nothing is copied at creation, and there is no snapshot. The thread
reads the parent's ledger as it stands whenever the thread's agent asks, so a
parent that keeps working is a parent the thread can keep reading.

The consequence is accepted deliberately: a thread's context is not reproducible
in isolation, because re-opening it later sees a parent that has moved. That is
the point of a shared context rather than a copy, and it is why the word "fork"
was rejected — it promises a snapshot the implementation does not take.

### 1.2 Context runs down, never up

- A thread reads **every chat above it**, nearest ancestor first.
- A parent **never** reads its threads.
- Siblings **never** read each other.

Three threads off one chat are three genuinely independent attempts. Nothing
merges them back automatically; if the parent wants what a thread found, the user
says so.

### 1.3 Position is the relationship

There is no `forkedAtTurn`, no badge, no thread count on the parent row. The
indent is the entire statement, exactly as it is for a nested workspace. Dragging
a chat under another chat makes it a thread of that chat; dragging it out to the
root makes it standalone again.

An earlier iteration drew a `⑂ 14` badge. It was removed because it described a
snapshot that does not exist, and because the first question asked of it was
"what does this mean?".

### 1.4 Folders, and why they are transparent

Folders group chats. A folder may hold chats and other folders, and it may live
**anywhere a chat can, including inside a chat** — a chat that has grown a dozen
threads needs to order them exactly as much as the root does.

This does not make the indent ambiguous, because **a folder holds no turns**. It
is not a thread and can never be mistaken for one, so it carries no context and
changes none:

> Context lineage is the ancestor chain **filtered to chats**. The walk steps
> straight through a folder.

Grouping a chat's threads into folders therefore cannot alter what any of them
reads. That is the property that lets folders go anywhere, and it is worth a test
of its own: a thread two folders deep under a chat must read exactly the same
lineage as a thread sitting directly under it.

Folders are per-workspace, matching how the workspace tree scopes its own.

---

## 2. Interaction

### 2.1 Rows

Every row is the workspace tree's row: `ROW_BASE` geometry (36px, `rounded-lg`,
`mx-1.5 my-0.5`, 13px/500), `ROW_ACTIVE`/`ROW_INACTIVE` treatments,
`ROW_INDENT_STEP` of 14px per level, `role="treeitem"`, trailing actions on
hover/focus/active only. Folder rows use the duotone open/closed pair, swapped
rather than tweened.

Chat rows keep their provider glyph, which becomes the flip-dot spinner while the
chat is working. Each row subscribes to its own `working` state — never a
panel-level subscription, which re-rendered the whole list on the feed's hottest
event.

### 2.2 Drag

Identical to the workspace tree, reusing its modules rather than re-deriving them:
`drop-rules.ts`, `drop-indicator.tsx`, `drag-ghost.tsx`, `drop-target-dom.ts`,
`edge-scroll.ts`.

- The grabbed row is **cloned and promoted to `ROW_ACTIVE`** — lifted is what
  active already draws. Source row at `opacity-40`.
- Up to three clones stack at 4px steps, the ones behind at 0.25; a count badge
  carries the rest.
- A **hairline** says *between*, a **fill** (`ROW_NEST_TARGET`) says *inside*.
  Never both.
- Edge bands: **20%** on a folder, **30%** on a chat. Re-parenting a chat changes
  what it reads, so it is the expensive miss and gets the harder target; dropping
  into a folder changes nothing but position, so it is cheap and gets the easier
  one.
- Spring-open on hovering a collapsed row's middle.
- `⌘`/`Ctrl` click multi-selects; a multi-drag carries rows in tree order.
- Drop on the footer trash deletes, no confirm, covered by the removal tray's
  undo.

### 2.3 Refusals

Any row may go into any other row. Chats hold threads and folders; folders hold
chats and folders. The only refusal is structural:

| Drag | Target | Allowed |
| --- | --- | --- |
| chat or folder | chat or folder | before / after / into |
| anything | itself or its own descendant | none |

A refusal draws **no mark at all** — no line, no fill.

### 2.4 Kept rows

Collapsing a row that holds the chat currently on screen keeps that chat visible,
hoisted one indent step under the collapsed parent, which shows the three holding
dots inside its glyph and offers the fold-away control. Mirrors `useKeptRows`.

### 2.5 Search

One field at the top of the panel, scoped to this workspace's chats. No scope
chip — the panel is workspace-scoped by construction, and on a project home that
workspace already *is* the big list.

- Filters in place; matching substring marked.
- Ancestors of a match are kept as **dimmed context**, so a hit never loses the
  folder or parent chat it belongs to.
- Descendants of a matched row are kept in full.
- Collapse state is ignored while a query is active.
- Result count and `esc to clear` under the field.

### 2.6 A row is lit only while its chat is on screen

`active` means: this chat is the **active tab of a pane that is on screen** —
that is, a pane which is a leaf of `rootLayout`. Read from the layout, not the
panes record: the store holds panes nothing renders (`bottomLayout` and its
`BOTTOM_PANE_ID` are drawn by no component), so enumerating `panes` would light a
row for a pane the user cannot see.

Not "has a tab" — the current `openChatIds` in `agent-chats-panel.tsx` lights a
row for any open buffer, so a chat in a background tab or a hidden pane stays lit
with nothing on screen to justify it.

Two states, not three: lit, or nothing. A chat parked in a tab you cannot see
gets **no mark of its own** — it is not on screen, and a column of half-signals
is harder to read than a column carrying one.

---

## 3. Persistence

Chat order today is `localStorage` per workspace
(`crowbar:agent-chat-order:<wsId>`). Hierarchy cannot live there: parentage
changes what a chat *reads*, so it is domain truth, not a view preference.

- **`ParentID string`** joins `domain.AgentChat`, defaulting to `""` (root). It is
  set by an asynx command and folded from an event like every other field on the
  aggregate.
- **Folders** are a new small aggregate, `AgentChatFolder`: `ID`, `WorkspaceID`,
  `ParentID`, `Name`, `CreatedAt`. Folders and chats share one id space for
  parentage, as workspaces and folders already do in the workspace tree — so a
  chat's `ParentID` may name either a chat or a folder, and lineage resolution
  must filter the chain to chats rather than assuming the parent is one.
- **Sibling order** moved to the daemon with parentage rather than staying in
  `localStorage`. Chats and folders interleave at every level, so an order held
  on one side of that pair cannot arrange the other, and the level a drop lands
  in has to be dense for the drop index to mean anything. `Order int` therefore
  joins the aggregate beside `ParentID`, and an operation renumbers every level
  it disturbs.

Cycles are refused at the command, not the UI: re-parenting under a descendant is
rejected server-side as well as drawn as a refusal.

**A move decides one parent and many indices, and the writes say exactly that.**
An operation plans from a single snapshot, and the chat half of that snapshot is
the asynchronous read model — so it can still be serving the placement a chat had
before the operation immediately before this one. Two rules keep that from
deciding anything:

- The SUBJECT's own row is read from the event log (`LoadChat`) and stamped over
  what the list said, because its stored parent is what the plan compares the
  destination against.
- Every OTHER row a densify touched is written with `SetOrder`, which carries an
  index and no parent at all. A renumber that restated the parent restated the
  snapshot's, and a multi-row drag is several placement calls in a row: the
  second one's renumber put the first one's chat back where it had just come
  from, silently un-threading it.

---

## 4. How a thread actually reads its parent

The machinery already exists.

- Every chat has an append-only **ledger** directory of turns
  (`api/internal/app/ledger`), written by Crowbar from vendor-CLI hooks.
- `get_chat_log` (`agenttools/tools_context.go`) already serves another chat's
  ledger to an agent, bounded and paged, scoped by position in the workspace tree.
- `contextInject` (`usecases/agent/agent.go`) is the single channel for handing a
  spawning agent a context document, per provider descriptor.

So a thread needs two things, neither of them new subsystems:

1. **At spawn**, `contextInject` names the thread's lineage — the ancestor chat
   ids, nearest first, **with folders filtered out** (§1.4) — and instructs the
   agent to read them with `get_chat_log` before starting. A pointer, not a paste: the thread reads the parent as it
   stands at the moment it asks, which is what "live" means here.
2. **In scope**, `get_chat_log` must permit a thread to read its ancestors. It is
   already workspace-scoped, and threads live in the same workspace as their
   parent, so this is expected to be a no-op — it must be **proved by test**, not
   assumed.

The reverse direction must be **refused**: a parent asking `get_chat_log` for a
descendant, and a sibling asking for a sibling, are both out of scope. That
refusal is the load-bearing half of §1.2 and needs its own tests.

---

## 5. Out of scope

- Catching up a thread with the parent's newer turns as an explicit action.
- Merging a thread's findings back into its parent.
- Cross-workspace search.
- Migration of existing flat chats: they all land at the root with no parent,
  which is correct, and pre-production means no migration path is owed.

---

## 6. Open questions

1. **Does re-parenting rewrite context retroactively?** Dragging a chat with fifty
   turns of its own under another chat makes it a thread. Does it then read that
   new parent's whole history, or only from the move onward?
   *Leaning:* from the move onward, announced as a turn in the thread — silently
   rewriting what a chat has already read is the version you cannot audit.
2. **Does deleting a parent take its threads?** Drag-to-bin currently carries the
   whole subtree, which here also destroys the context those threads were reading.
   *Leaning:* carry the subtree, with the removal tray's eight-second undo.
