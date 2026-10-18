# Upgrade notes: an install from before the stabilization audit

What the first run of this version does to a home written by the pre-audit
daemon (70ec430 and earlier), and why going back afterwards is not supported.

## What the first run does

All of it runs synchronously at boot, before anything is served, through the
migration engine (`api/internal/core/migrate`). The daemon answers
`/v0/health` with `migrating` while it works. Over rows it cannot replay a
step is best-effort: the rows are counted (`skipped_rows`, in the ledger and
`crowbar migrate --plan`) and logged, the step completes, and the daemon
starts. A step that fails outright (an I/O error, a corrupt ledger, or its own
postcondition not holding) no longer lets the daemon carry on: see "When a
step fails" below.

Every boot:

- **Workspace provisioning.** Rows written before `provisioning` existed get it
  recorded as an event: a home or default checkout is `shared`, an empty path a
  `placeholder`, anything else `provisioned` (including pre-leaf
  `<slug>/<branch>` rows).
- **Owning chats.** A live workspace with no recorded owner gets the owner the
  old daemon would have picked by heuristic (a legacy branch row, else the
  earliest untitled row that is not a thread) recorded. A fresh chat is minted
  only when there is no candidate.
- **Interrupted deletes** are resumed and tombstones purged, as before.

Once per install (recorded in the ledger at `<home>/migrate/ledger.jsonl`; a
pre-audit home's existing markers under `<home>/state/upgrades/` are adopted
once: the step is recorded as done, the marker deleted, nothing re-run):

- `chat-types`: chat rows with no `type` (minted before the field) become `chat`.
- `chat-providers-from-history`: each chat's stored provider is restated to
  the newest provider its runner history names (conversations, switch markers,
  placements) — the answer the old daemon gave.
- `project-homes`: a project saved without a home workspace gets one, under
  the deterministic `ProjectHomeID`; the old daemon minted it on the first
  `/home` read, which no longer writes.
- `hook-deliveries`: the retired `<chatsDir>/.hook-deliveries/` journal is
  deleted from each workspace's Crowbar-managed chats directory, and nothing
  else.
- `workspace-paths-table`: the retired `workspace_paths` table is dropped from
  `view.db`.

Also changed for old data, without a migration:

- **Pre-leaf workspaces** (`<slug>/<branch>`, no `worktree` leaf) keep their
  shared `<slug>/chats` tree. Deleting one removes only its git worktree; the
  purger removes a directory only when it proves it is a single workspace's
  own leaf root, so siblings' chats, attachments and checkouts — including
  branches named `chats`, `threads` or `storages` — are never touched. A
  checkout that holds another workspace's files is never `--force` removed.
- **Descriptor overrides** in `~/.crowbar/descriptors/` that today's rules
  refuse (typically copied from an older shipped descriptor) no longer block
  the provider: the shipped descriptor runs, the boot log names the refused
  override and its findings, and Settings → Providers shows "Override refused
  — using shipped" with the reasons.

## When a step fails

Run `crowbar migrate --plan` (same `CROWBAR_HOME`) to see the layout, the
pending steps, whether `state/` would be backed up, and the last run.

A failed step, a layout newer than this binary knows, an unreadable `LAYOUT`
file or a corrupt ledger halts the boot. The daemon stays up holding the home,
opens no database, answers `/v0/health` with `status: "halted"` and the reason
in `detail.message`, and answers 503 `daemon_halted` to everything else. The
desktop app shows the reason in place of its window and does not restart the
daemon. Fix the cause and relaunch; the retry resumes from the checkpoints. A
`LAYOUT` newer than the binary means a newer Crowbar has been here: run that
one. A ctrl-C or SIGTERM mid-migration is not a failure: the run is recorded
`interrupted` and the next boot resumes.

Before a run that rewrites `state/`, the daemon copies it to
`<home>/migrate/backups/<run-id>/state/` (needing the copy's size plus 64 MiB
free, else it halts before touching anything). A halt names the backup; Crowbar
never prunes or restores one, so restoring is a manual copy back.

## Downgrading is not supported

After this version has run, going back to a pre-audit build is not supported.
The old daemon found a workspace's worktree through the `workspace_paths`
table, written once at creation; a placeholder this version provisions in
place gets its path only in the event log, and the table itself is now
dropped, so the old daemon's delete and boot sweep would forget such
workspaces without removing their worktrees. The event log does not carry
the old daemon back either: asynx stores each event as a JSON patch against
the previous state, and a warm read starts from the aggregate's one latest
snapshot decoded into the running binary's struct. This version snapshots
every workspace it backfills or provisions in place; the old binary decodes
that snapshot into a struct with no `provisioning` (or `createdBranch`)
field, so the fields vanish from the state it replays onto, any later patch
that replaces one of them fails to apply, and any snapshot the old binary
writes back persists the loss. Keep a copy of `~/.crowbar/state` before
upgrading if a rollback might be needed.

The automatic backup does not make a rollback safe across a halt. A retry
reuses the backup taken before the failed attempt, because it is the image the
migration started from. So a halt, then reinstalling an older release, running
it for days, then upgrading again leaves the retry with that same pre-halt
image: the days in between are in no backup, and the ledger cannot see them,
since it records migrations, not normal use. Copy `~/.crowbar/state` yourself
before upgrading again.

A halt's `layout` field is 0 for `layout_unreadable`: the `LAYOUT` file could
not be read, so no layout is known.
