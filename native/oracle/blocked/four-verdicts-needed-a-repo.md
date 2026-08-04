# RESOLVED — the four verdicts that "needed a repo in the fixture"

**Raised:** across several iterations, as `repo-avatar`, `workspace-branch-icon`,
`repo-icon-popover` and `repo-import-dialog` each reached the ledger as ⏸
*"needs a repo in the fixture"*.
**Resolved:** 2026-08-03. **The daemon had the repo the whole time.** Nothing
about the fixture was missing; the webview could not read its own cache.

## Root cause, proven rather than inferred

`web/src/lib/persistence/idb.ts` opens `openDB('crowbar', 7, …)`. The database
on this origin was at **version 9**, written by a *newer* build sharing
`localhost:5173`. IndexedDB refuses to open at a lower version than the one on
disk:

```
getDB()               -> VersionError: An attempt was made to open a database
                         using a lower version than the existing version.
indexedDB.open(…, 7)  -> VersionError: (identical)
```

Both spellings were run in the live webview; the second exists so the failure
cannot be blamed on the `idb` wrapper. **Every** entity-cache call routes
through `getDB()`, so from that moment:

- `upsertEntity` — `catch { /* best-effort cache write */ }` → silent no-op
- `getAllEntities` — `catch { return [] }` → silently empty
- `removeEntity` — same

The sidebar builds its tree from that cache, so it renders no repos, for ever,
with no error anywhere. `/v0/projects/:id/repos` was returning both repos
correctly the entire time — verified by calling it from the page.

## The evidence chain, in the order it actually resolved

| step | result |
|---|---|
| daemon `view.db` | 2 rows in `repositories` — `demo`, `oracle-repo` |
| `fetch(API_BASE + '/v0/projects/:id/repos')` | `200`, both repos in the body |
| `api.fetchRepos(pid)` from the page | resolves, 2 repos — **the seed function was never broken** |
| `activeProjectId` | correct; `projectData.status === 'success'` |
| `upsertEntity('crowbar_repos', r)` then read back | **0** |
| raw `indexedDB.open('crowbar')` + `put` | **succeeded**, `keyPath: 'id'`, `version: 9` ← the tell |
| `getDB()` | `VersionError` |
| delete the database, reload | `version: 7`, no error, cache writes land, **tree renders** |

The decisive contrast is the last two rows of the middle block: a raw put
succeeded while the app's own put silently did nothing. Same store, same key
path, same data — so the fault had to be in how the app *opens* the database,
not in what it writes.

## What the cache actually contained, which is worth its own line

Not empty — **`crowbar_repos: 0`, `crowbar_projects: 0`, `crowbar_workspaces:
80` across ~20 foreign `projectId`s.** Those 80 rows were written by whichever
build owned version 9. A cache that is *partly* full of another daemon's data
reads as "the sync half-worked", which is a much more misleading state than an
empty one, and it is what kept sending me back to the sync path.

## Two real defects in the React app, separate from this port

Both are outside this port's scope; recording them because they were found here
and someone should decide about them.

1. **A version-skewed cache degrades to silence, not to an error.** Every
   entity-cache entry point swallows its exception. The user-visible result is
   an empty sidebar with no message, no toast and no console error beyond what
   `entity-stream` logs for a failed seed — and the seed did not fail here, so
   not even that fired. `maybeWipeOnVersionChange` guards the *DTO* version but
   nothing guards the *schema* version going backwards.
2. **Same family as the `inline-error` finding** already in
   `four-ported-surfaces-are-dead.md`: `getAllEntities`'s bare `catch { return
   [] }` is exactly the swallow that makes `wsListData.status === 'error'`
   unreachable. That file argued the guarded error state can never render. This
   is the same swallow causing a *different* invisible failure, which
   strengthens the case that the swallow itself is the defect.

## Consequence for the ledger

`repo-avatar`, `workspace-branch-icon`, `repo-icon-popover` and
`repo-import-dialog` are **no longer blocked**. `workspace-branch-icon` and
`repo-icon-popover-trigger` are now in the live DOM; the other two need their
overlay driven open, which is ordinary drive work, not a blocker.

**The environment is fragile.** The recovery is: delete the `crowbar`
IndexedDB, reload, and confirm `getDB()` resolves at version 7 before trusting
any capture. A build sharing this origin can put it back at version 9 at any
time. `project_dev_apps_shared_one_origin`'s per-worktree derived port is the
real fix; this instance is not running it.

## Method note

I had previously recorded that "the cache is empty and every store read path
reads the cache" and stopped there, treating it as a dead end for several
iterations. That observation was true and useless: it described the symptom in
the same words as the diagnosis. **What broke it open was asking why a write
did not land, then running the same write through a lower-level API** — the
first thing all session that distinguished "the app declines to write" from
"the write fails".
