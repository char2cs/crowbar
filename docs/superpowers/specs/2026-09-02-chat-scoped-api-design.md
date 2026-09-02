# Chat-scoped API — a workspace is never addressable

**Date:** 2026-09-02

**Status:** design spec, open. Backend. Consumed by the sidebar/pane work already
on `enhancement/unify-sidebar`, but nothing here is frontend work, and nothing
here is built yet.

**No migration.** Pre-production — every schema change below ships straight,
no dual-read, no backfill path for old shapes.

---

## 0. The bug this starts from

Live, on this daemon, right now: the workspace behind `feature/pricing-rounding`
has `owningChatId: ""`. No chat anywhere in the repo's chat list names it. It is
a real, reachable git worktree with nothing that can open it, because nothing
in the sidebar, the API, or the daemon addresses a workspace except through the
chat that owns it — and this one doesn't have one.

Traced to the source: `CreateFromImport`
(`api/internal/app/usecases/workspace/internal/hierarchy/import.go`) creates a
workspace row directly from a discovered git branch. No chat is minted in the
same breath. The only thing that could have caught this later —
`BackfillOwningChats` / `EnsureOwningChat`
(`api/internal/app/usecases/chat/internal/tree/backfill.go`) — is a **best-effort
reconciler**, not a guarantee: it runs at boot, it's already wired correctly,
and this workspace still slipped through, because import is the one creation
path nothing ever pointed it at.

That is the whole finding this spec acts on: **the invariant "a workspace
cannot exist without an owning chat" is currently enforced by a background
sweep noticing after the fact, not by the write path making the violation
impossible.** Every decision below exists to move that guarantee from
"reconciled" to "unrepresentable."

---

## 1. The laws

Eight rules. Everything else here is a consequence of one of them.

1. **A CHAT IS THE ONLY ADDRESSABLE UNIT.** No public route names a
   `workspaceId`. A consumer creates chats, reads chats, and acts on chats —
   nothing else has an id a request can carry.
2. **A WORKSPACE CANNOT OUTLIVE ITS CHAT BECAUSE IT CANNOT PRECEDE IT.** Every
   worktree-provisioning path — fork a new branch, import an existing one —
   mints its owning chat first and attaches the worktree after, in one call.
   `BackfillOwningChats` goes back to being what its own doc comment already
   says it's for: a one-time reconciliation for data that predates this
   guarantee, not something a live code path leans on.
3. **SIBLING USECASES NEVER IMPORT EACH OTHER.** `usecases/chat` and
   `usecases/workspace` stay two independent packages. Nothing new changes
   that — the resolver in §3 sits between them, it is not a bridge that lets
   one import the other.
4. **AN INTERFACE IS DECLARED WHERE IT IS CONSUMED.** The existing
   `WorkspaceReader{ Get(ctx, id) (domain.Workspace, error) }` in
   `terminal/handlers/handlers.go` is the precedent: a consumer names the
   narrow slice of behavior it needs, locally, and the container satisfies it
   with whatever concrete type actually implements it. No new code imports
   `usecases/workspace` or `usecases/chat` wholesale for one method.
5. **SHARED STATE IS SHARED; OWNED STATE IS OWNED.** git, review, files,
   search and identity are the worktree's own single truth — every chat
   sharing that worktree gets the same answer, and a write from one is visible
   to the others. Editor (LSP), terminal and provider are sessions one
   specific chat opened — the resolver still finds the worktree for a CWD, but
   the session itself is never shared with a sibling.
6. **THE CONTAINER IS THE ONLY PLACE THAT KNOWS CONCRETE TYPES.**
   `container.go` constructs the real repositories and the resolver once, and
   injects them wherever an interface from law 4 needs satisfying. No other
   file makes that decision.
7. **NO SPECIAL CASE FOR "HOME."** A repo's default branch does not get its
   own `Workspace.IsDefault`/`Kind: Home` fields. It is the first import
   Crowbar performs automatically when a repo is added, through the exact
   same path a user-triggered import uses.
8. **REUSE THE SCAFFOLD THAT ALREADY WORKS.** `createOwnWorktreeChat`
   (`usecases/chat/internal/tree/chats.go`) already does mint-then-place-then-
   attach correctly for fork. Import is a second way to do the *attach* step,
   not a second way to do the whole thing.

---

## 2. The domain fix: `CreateChat`'s worktree parameter

**Before:**

```go
CreateChat(ctx, workspaceID, providerID, parentID string, ownWorktree bool) (chatID, runnerID string, err error)
```

A bool holding what needs to be three states (none, fork, import) is already a
shoehorn the moment import joins as a real sibling rather than a special case
bolted on elsewhere. **After:**

```go
type WorktreeMode int
const (
    WorktreeNone WorktreeMode = iota
    WorktreeFork
    WorktreeImport
)

type WorktreeSpec struct {
    Mode   WorktreeMode
    Branch string // WorktreeImport only
    Remote string // WorktreeImport only, "" = local
}

CreateChat(ctx, providerID, parentID string, worktree WorktreeSpec) (chatID, runnerID string, err error)
```

(`workspaceID` drops from the signature entirely — per law 1, nothing calls
this with one anymore.)

Internally, `createOwnWorktreeChat` keeps its existing shape — mint the bare
chat via `u.agent.MintChat`, place it in the tree, refuse before minting if the
parent is invalid — and only the last step branches:

- `WorktreeFork` → `u.agent.SpawnChatWithOwnWorktree(ctx, chatID, providerID)`
  (unchanged, this already exists)
- `WorktreeImport` → `u.agent.SpawnChatWithImportedWorktree(ctx, chatID,
  providerID, branch, remote)` (new — same contract as the fork call: attach a
  worktree to an already-minted, already-placed chat, discard the chat on
  failure)

`hierarchy/import.go`'s `CreateFromImport` stops existing as a workspace-first
path. What it currently does — discover a branch, prepare a worktree for it —
becomes the git-mechanical half of `SpawnChatWithImportedWorktree`; the
chat-minting half it never had comes from the scaffold in law 8.

**Repo home (law 7):** when a repo is added, Crowbar calls this exact same
`CreateChat` with `WorktreeSpec{Mode: WorktreeImport, Branch:
repo.DefaultBranch}` once, automatically. `Workspace.IsDefault` and
`Workspace.Kind == Home` are deleted — nothing checks them because nothing
sets them; a repo's home branch is a chat that imported `main`, same as any
other imported branch, and its lock status comes from the same
`WorkspaceStatusLocked` every protected branch already uses.

---

## 3. The resolver

One new package: `internal/app/usecases/worktree` (name open — see §7).

```go
type WorkspaceReader interface {
    Get(ctx context.Context, id string) (domain.Workspace, error)
}

type ChatAncestryReader interface {
    Ancestors(ctx context.Context, chatID string) ([]domain.Chat, error)
}

func Resolve(ctx context.Context, chatID string, chats ChatAncestryReader, workspaces WorkspaceReader) (domain.Workspace, error)
```

`Resolve` walks the chat's ancestry — itself first, then each parent in turn —
for the nearest chat that owns a worktree, and loads that workspace. A chat
with no worktree anywhere in its ancestry (a bubble hanging off nothing,
should that ever be reachable) is refused with a typed error, not a zero
value.

Both interfaces are declared **in this new package**, not imported from
`repositories/chat` or `repositories/workspace` directly — law 4. The concrete
types that satisfy them (the real repositories) are wired in by
`container.go` — law 6. This package imports neither `usecases/chat` nor
`usecases/workspace` — law 3.

Every handler that used to do `wsID := ctx.Param("wsId"); ws, err :=
h.wsReader.Get(ctx, wsID)` does `chatID := ctx.Param("chatId"); ws, err :=
h.resolver.Resolve(ctx, chatID)` instead. The handler's own declared
`WorkspaceReader` interface (law 4) doesn't change shape — only what's wired
in to satisfy it does.

---

## 4. The route surface

Full counts verified against the live route tables in this repo, not
estimated.

### 4.1 Two groups stop existing

| group | routes | what happens |
|---|---|---|
| `workspaces` | 13 | List/Detail die — a chat's own DTO carries git fields now (§5). Create/Import die as routes — both are `POST /chats` with a `WorktreeSpec` (§2). Delete dies — it was never anything but `DELETE /chats/:id` for a chat that happens to own a worktree. Lock, Sync, MergeIntoParent, Reparent, RebaseOntoParent, RetryProvision, DetachHolder **move to `chat`** as verbs on the thing actually being held. |
| `home` | 57 | `/home/files` (9), `/home/terminals` (4), `/home/chats` (36, a straight duplicate of the real `chat` group) all describe a state with no chat behind it — exactly what law 1 makes unreachable. **`/home/threads` (8) stays** — that's the code-review comment system, an unrelated concept sharing the word "thread." `GET /home` (1, bare) is deleted too — §7.3. |

### 4.2 Eight groups re-key `wsId` → `chatId`, split by law 5

**Shared workspace state** — one worktree, one answer, regardless of how many
chats ask:

| group | before | after | routes |
|---|---|---|---|
| git | `/workspaces/:wsId/git/*` | `/chats/:chatId/git/*` | 32 |
| review | `/workspaces/:wsId/review*` | `/chats/:chatId/review*` | 6 |
| files | `/workspaces/:wsId/files/*` | `/chats/:chatId/files/*` | 8 |
| search | `/workspaces/:wsId/search*` | `/chats/:chatId/search*` | 2 |
| identity | `/workspaces/:wsId/identity` | `/chats/:chatId/identity` | 1 |

**Owned by one chat** — the resolver still runs, for a CWD, but the session
itself never shares:

| group | before | after | routes |
|---|---|---|---|
| editor (LSP) | `/workspaces/:wsId/{blame,lsp/*}` | `/chats/:chatId/{blame,lsp/*}` | 13 |
| terminal | `/workspaces/:wsId/terminals*` | `/chats/:chatId/terminals*` | 4 |
| provider | `/workspaces/:wsId/provider` | `/chats/:chatId/provider` | 1 |

(`/protected-branches`, also in the `provider` group, is repo-level and
doesn't move.)

### 4.3 What lands on `chat`

| verb | route | was |
|---|---|---|
| Lock | `POST /chats/:id/lock` | `POST /workspaces/:wsId/lock` |
| Sync | `POST /chats/:id/sync` | `POST /workspaces/:wsId/sync` |
| Merge into parent | `POST /chats/:id/merge-into-parent` | `POST /workspaces/:wsId/merge-into-parent` |
| Reparent | `POST /chats/:id/reparent` | `POST /workspaces/:wsId/reparent` |
| Rebase onto parent | `POST /chats/:id/rebase-onto-parent` | `POST /workspaces/:wsId/rebase-onto-parent` |
| Retry provision | `POST /chats/:id/retry-provision` | `POST /workspaces/:wsId/retry-provision` |
| Detach holder | `POST /chats/:id/detach-holder` | `POST /workspaces/:wsId/detach-holder` |

### 4.4 Untouched, on purpose

`projects`, `repos` (neither is a worktree), `threads` (review comments, not
agent chats), `health`, `system` (infrastructure), every `/settings/*` route
(user-level, no chat in sight).

---

## 5. Data model changes

- **`TerminalSession`** (`internal/domain/terminal_session.go`):
  `WorkspaceID string` → `ChatID string` (GORM index moves with it). No
  migration — pre-production.
- **`domain.Workspace`**: drop `IsDefault bool` and `Kind` (the `Home` value
  specifically — confirm no other `Kind` value depends on the same field
  before removing it wholesale). Everything else on the type is unchanged;
  law 2 doesn't require collapsing Workspace into Chat, and §6 records why
  that was considered and rejected.
- **Chat DTO** (wherever the API's chat response type lives): gains the git
  fields `Workspace`'s own DTO (`internal/api/v0/dto/workspace.go`) currently
  carries — branch, added/deleted, lock status, merge/PR state — populated
  only when the chat owns a worktree. The standalone workspace DTO is deleted
  once nothing serializes it.
- **WS/broadcast topics**: anything currently pushing lifecycle events on a
  workspace-scoped channel (confirmed at least for terminal sessions —
  `h.pushSession(..., wsID, projectID, repoID, ...)`) renames its topic to be
  chat-scoped. Not optional — the frontend's subscriptions break silently
  otherwise. Full audit of push sites is part of implementation, not fully
  enumerated here.

---

## 6. What was rejected, and why

| rejected | why |
|---|---|
| **Keeping `workspaceId` as a public, read-only field** somewhere in the API | Still lets a consumer name a resource independent of any chat — the exact thing law 1 exists to prevent, even if you can't act on it directly. |
| **Collapsing `Workspace` into `Chat`** — one type, git fields inline | A worktree is genuinely many-chats-to-one (a thread with no worktree of its own shares its parent's). Folding it in means duplicating or pointer-chasing shared state across every sibling chat instead of normalizing it into its own row. Two focused types joined by an id is the correct shape here, not a legacy accommodation. |
| **The resolver importing `usecases/workspace` directly** | Breaks law 3 for the sake of convenience. The existing `WorkspaceReader` precedent in `terminal/handlers` already proves the narrow-interface pattern works without it. |
| **`ownWorktree bool` with import as a special third case** | A bool can't honestly hold three states. Worth the signature change now that import is a real sibling to fork, not worth carrying forward because it already exists. |
| **A bolt-on `EnsureOwningChat` call inside `CreateFromImport`, left otherwise as-is** | The first fix considered for the bug in §0. Rejected: it keeps import as a workspace-first path patched to remember its chat, rather than a chat-first path that never has one missing. Fixes the symptom that was found, not the class of bug. |

---

## 7. Decisions closed

All five questions this design left open are answered, each checked against
the actual code rather than guessed. None of them is open any more.

1. **Route prefix — flat.** `/v0/chats/:chatId/...`, no project/repo nesting.
   Chat ids are globally unique; nesting would force a consumer to resolve
   ids it doesn't need for anything past the one creation call. The only
   routes that keep the `/projects/:projectId/repos/:repoId/` prefix are
   creation and repo-level listing (`POST/GET
   /projects/:projectId/repos/:repoId/chats`) — everything that follows is
   `/v0/chats/:chatId/...` on its own.
2. **The resolver package — `internal/app/usecases/worktree`.** Final, not a
   placeholder. Names what it resolves *to*; collides with neither existing
   usecase package.
3. **`GET /home` — deleted, no replacement.** Read the handler
   (`endpoints/home/handlers/home.go:17`): it does exactly one thing, return
   "the home workspace DTO for the project." That is the `IsDefault`/`Kind:
   Home` special case law 7 already deletes. A consumer who wants the chat
   that owns the repo's default branch already has it from the repo's own
   chat listing — it's the one `ChatTypeBranch` chat whose branch equals
   `repo.DefaultBranch`. No dedicated endpoint needed for a lookup that's a
   filter over data already returned elsewhere.
4. **WS/broadcast topics — rename, and fan out where the data is shared.**
   Confirmed a second push site beyond terminal's direct per-handler call: git
   writes trigger a file/git watcher broadcast (`endpoints/git/handlers/
   write.go`, "watcher broadcast" — a workspace-scoped stream, not called
   per-handler the way terminal's `pushSession` is). Every topic renames from
   workspace-keyed to chat-keyed, but the **shared bucket's topics (git,
   review, files, search, identity) fan out to every chat currently resolving
   to that workspace**, not just the one that triggered the write — siblings
   share the data, so they share the push. This needs the resolver's inverse:
   given a workspace id, which chat ids currently point at it. The **owned
   bucket's topics (terminal, editor, provider)** stay single-chat, a
   straightforward rekey.
5. **`Workspace.Patch` — deleted, both halves already have a home.** Read the
   handler (`endpoints/workspaces/handlers/hierarchy.go:65`): its placement
   half (`FolderID`/`Order`) already does nothing but resolve the owning chat
   and call `h.placer.PlaceChat` — that's pure duplication of the
   already-existing `PATCH /chats/:id/placement`. Its rename half
   (`body.Branch`) folds into the already-existing `POST /chats/:id/rename`:
   renaming a worktree-owning chat renames the branch, renaming a bubble
   renames the title — one verb, resolved by what the chat is, the same way
   Fork/Thread on a row resolve by what the row is. No new endpoint, no
   fallback.

---

## 8. Sequencing

1. **§2 first, on its own** — `CreateChat`'s `WorktreeSpec`, the import path
   reuse, the repo-home auto-import. This closes the actual bug from §0 and
   needs nothing else in this spec to be true first.
2. **The resolver (§3)**, built and tested against the two interfaces, wired
   into `container.go`, used by nothing yet.
3. **One pilot group** — terminal, per the earlier discussion in this
   thread — re-keyed end to end against the flat `/v0/chats/:chatId/...`
   prefix §7.1 already settles, proving the resolver and the fan-out
   mechanics (§7.4) before repeating the change five more times.
4. **The rest of the "shared" bucket** (git, review, files, search, identity),
   same mechanical change repeated.
5. **The rest of the "owned" bucket** (editor, provider).
6. **Delete `workspaces` and `home`** once nothing depends on either.
7. **Frontend follow-up** — out of scope here, but every workspace-keyed call
   site in `web/src/lib/api.ts` / `agent-api.ts`, and the `/ide/:projectId/
   :repoId/:wsId` route itself, depend on this landing first.

---

## 9. Numbers, lifted not invented

| value | source |
|---|---|
| orphaned workspace, `owningChatId: ""` | live daemon query, `feature/pricing-rounding`, id `7c613d8c-48b5-4c9e-93e8-aea9b0c57112` |
| `WorkspaceReader{ Get(id) }` precedent | `api/internal/api/v0/endpoints/terminal/handlers/handlers.go:73` |
| mint→place→attach scaffold | `api/internal/app/usecases/chat/internal/tree/chats.go:65` (`createOwnWorktreeChat`) |
| `BackfillOwningChats` / `EnsureOwningChat` | `api/internal/app/usecases/chat/internal/tree/backfill.go` |
| import path with no owning-chat call | `api/internal/app/usecases/workspace/internal/hierarchy/import.go` |
| `usecases/chat` / `usecases/workspace` do not import each other today | verified via grep, both directions, zero matches |
| route counts (§4) | counted directly from every `internal/api/v0/endpoints/*/routes.go` in this repo |
| total surface | ~214 routes today, ~159 after |
