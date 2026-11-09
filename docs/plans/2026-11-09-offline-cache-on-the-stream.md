# Offline Cold Start on the Stream — Plan

**Status:** Proposed · **Date:** 2026-11-09

## 1. What develop shipped, and why the merge did not keep it

Develop (#242) made a cold start with the host unreachable draw something. It
did this by keeping a second copy of server truth in IndexedDB:

- `chat-history`: each chat's newest transcript page, keyed by chat, indexed by
  `fetchedAt` and `hostId`. It was written on every merge and read before the
  first fetch.
- The entity cache (`crowbar_projects`, `crowbar_repos`, `crowbar_workspaces`,
  `crowbar_chats`), plus `home-tree` and `folder-signal`. These seeded the
  sidebar before any host answered.
- `home-workspace:<projectId>` in `workspaces-data`. A restored home pane read it
  as a `stale` home workspace.

On this branch the client has one owner per piece of state. The entity store is
filled from the stream, and a chat's feed is its ledger. Each cache above is a
second copy that can disagree with its owner, and each has its own
invalidation. This is the shape CLAUDE.md asks to stop and plan. The merge
therefore kept:

- the offline behaviour that needs no copy: a host that does not answer makes
  the pane read-only, the composer says `Offline — read-only`, and Start session
  is refused;
- the IndexedDB upgrade path (v12 retirements, v14 `editor-drafts`), so a
  database a develop build left still opens.

It dropped the caches. The `chat-history` store is still declared, so the open
stays additive; nothing reads or writes it.

## 2. What is lost today

A cold start with every host down draws an empty sidebar and no transcripts.
A session that loses its host keeps everything it already holds, because the
entity store and the feeds stay in memory.

## 3. Proposal: persist the owner, not a copy

Persist what the stream already delivers, at the store that owns it, as one
snapshot per scope:

1. **Entity snapshot.** On each applied batch, the entity store writes
   `{scope, v, byKind}` for the `projects` scope to one IndexedDB record,
   debounced. At boot, before the stream connects, the store loads that record
   as a snapshot marked `restored`. The first live snapshot of the scope
   replaces it wholesale, as any resync does. The store keeps no merge and no
   second map.
2. **Feed tail.** A chat feed persists its last N ledger rows with their `seq`.
   At mount with the host down, the feed starts from them with status
   `restored`. When the host answers, the feed resumes from the stored `seq`
   through the normal catch-up, so the stored rows are the feed's own prefix,
   not a parallel transcript.
3. **Removal.** Removing a host drops its records by `hostId`, from the one
   place that already handles host removal.

## 4. Acceptance

- Kill the daemon, reload, and the sidebar and the last chat draw from the
  restored snapshot, read-only.
- Restart the daemon, and the first snapshot replaces the restored state
  without a flash or a duplicate row.
- No component reads IndexedDB for entities or transcripts. The stores are the
  only readers.
- `chat-history` and the `crowbar_*` stores are retired in the same change
  (a version bump that deletes them).
