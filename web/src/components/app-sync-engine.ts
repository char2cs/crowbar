import { useEffect } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getVisibleProjectIds } from '@/lib/store/project-visibility'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import { subscribeHomeTree } from '@/lib/store/home-tree'
import { dataOf } from '@/lib/loadable'
import {
  fetchFolders,
  fetchRepoChats,
  fetchRepos,
  fetchWorkspaces,
  workspaceDTOFromWorktreeFrame,
} from '@/lib/api'
import { subscribeEntityStream, type EntityChange } from '@/lib/ws/entity-stream'
import { isStructuralChatFolderFrame } from '@/lib/ws/structural-chat-folder-frame'
import { getAllEntities, removeEntity, upsertEntity } from '@/lib/persistence/entity-cache'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { maybeWipeOnVersionChange } from '@/lib/persistence/idb'
import { wsManager } from '@/lib/ws/manager'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// §7 startup sequence, subscribed BY VISIBILITY rather than by existence.
//
//   /v0/projects                            always — one stream, a handful of rows
//   a project's repos                       while that project is visible
//                                           (known to /v0/projects, or the active one)
//   a repo's worktrees                      while its project is visible
//   a repo's tree rows (folders + chats)    while its project is visible
//
// Cost is then proportional to what exists instead of to how much work you
// have: a project that leaves the list costs one cached row and nothing else,
// and one returning renders instantly from the IndexedDB entity cache while
// its seed GET is still in flight. Teardown waits out a short grace period so
// a list flicker doesn't thrash the socket, and every stream is held under
// its own key so adding or dropping one never disturbs the others.
//
// On mount we
//   1. maybeWipeOnVersionChange() ONCE before any seeding (drops a stale cache
//      after a daemon/DTO version bump so we never merge incompatible frames);
//   2. seed + WS-subscribe the project list (`/v0/projects`);
//   3. reconcile the per-project / per-repo streams against the visible set, and
//      re-reconcile whenever that set moves.
//
// Wired up on mount by `AppSyncProvider` (app-sync-provider.tsx) via
// `useAppSyncEngine` below — split into its own module because this is one
// self-contained sync-loop engine with no dependency on that component's own
// props/state (it reads only stores and the network).

/**
 * How long a stream outlives the collapse that made it invisible. Long enough
 * that a collapse/expand tap is free (no unsubscribe/resubscribe/reseed round
 * trip), short enough that a project you actually closed stops costing frames.
 */
export const SUBSCRIPTION_GRACE_MS = 2000
const REBUILD_BATCH_MS = 16
const KEY_SEP = '|'
/** One project's home-workspace tree rows (chats + folders) — open for every
 *  VISIBLE project: a project's home row has to render exactly as reliably
 *  as its repos do, and repos are not restricted to the active project
 *  either. See `home-tree.ts`'s own doc. */
const homeTreeKey = (projectId: string): string => `hometree${KEY_SEP}${projectId}`
/** A project's repo list stream. */
const reposKey = (projectId: string): string => `repos${KEY_SEP}${projectId}`
/** One repo's worktrees, seeded and pushed through its chat surface. */
const workspacesKey = (projectId: string, repoId: string): string =>
  `workspaces${KEY_SEP}${projectId}${KEY_SEP}${repoId}`
/** One repo's sidebar tree rows — its folders AND its chats, on one reseed
 *  loop watching one signal (see `openRepoTreeSubscription`). */
const treeKey = (projectId: string, repoId: string): string =>
  `tree${KEY_SEP}${projectId}${KEY_SEP}${repoId}`

interface ActiveSubscription {
  dispose: () => void
  /** Set while the subscription is invisible but still inside its grace period. */
  teardown?: ReturnType<typeof setTimeout>
}

/**
 * The whole §7 sync engine described above — mounted once at the app root by
 * `AppSyncProvider`. Reads/writes only stores and the network; takes no props
 * of its own.
 */
export function useAppSyncEngine(): void {
  // react-doctor-disable-next-line effect-needs-cleanup -- cleanup exists (`disposed` flag + disposeAll() in the returned teardown); tracer can't follow it.
  useEffect(() => {
    let disposed = false
    const rootUnsubscribes: Array<() => void> = []
    const subscriptions = new Map<string, ActiveSubscription>()

    // -- full rebuild, coalesced ------------------------------------------
    // Reading the whole entity cache and replacing the whole tree is O(all
    // workspaces of every visible project), so it is reserved for the cases
    // that genuinely need it: a seed (which can also PRUNE), a repo frame
    // (which moves header fields), and a change in which projects are visible.
    // Live workspace frames take the incremental path below instead.
    //
    // Bursts are collapsed onto one timer: at boot every repo seeds at roughly
    // the same time, and rebuilding once per seed meant N full cache reads,
    // N IndexedDB cache writes and N tree replacements to reach one answer.
    let rebuildTimer: ReturnType<typeof setTimeout> | undefined
    let rebuildInFlight = false
    let rebuildQueued = false
    /**
     * Repos whose chats have been written to the cache but whose rows are not
     * in the STORE yet.
     *
     * The distance between those two is the whole reason this exists.
     * `openRepoTreeSubscription` writes IndexedDB and then only ARMS a
     * rebuild — `scheduleRebuild`'s 16ms timer, then an `await` on the
     * workspace list — so announcing "this repo's tree has been read" at the
     * moment the fetch resolved would open the gate while `repos` still held
     * the PRE-seed chats. `SidebarTreeSurface` re-rendering in that window
     * would ask `rows-from-repo.ts` to identify a branch row out of chats
     * that carry no `type` at all, which throws in render — and on a first
     * load after this ships, EVERY cached chat predates that field.
     */
    const seededPendingRebuild = new Set<string>()
    /** Per repo, its tree read in flight: what the cache will hold once it lands. */
    const treeReads = new Map<string, Promise<void>>()
    async function rebuildSidebar(): Promise<void> {
      if (rebuildInFlight) {
        rebuildQueued = true
        return
      }
      rebuildInFlight = true
      rebuildQueued = false
      // CLAIMED BEFORE THE READ, and only these are ever opened by this
      // rebuild. A repo whose chats land while the read below is in flight was
      // written AFTER that read reached the chats table, so this rebuild's
      // rows do not contain them — flushing "whatever is pending when I
      // finish" would open its gate onto pre-seed chats, which is the same
      // throw-in-render this queue exists to prevent, one await narrower. It
      // stays pending instead, for the follow-up `scheduleRebuild` its own
      // reseed already armed.
      const claimed = [...seededPendingRebuild]
      // A read that settles with a snapshot other than this one was published
      // after the claim; `success(old)` and `success(new)` differ only by identity.
      const before = useWorkspaceListStore.getState().data
      await useWorkspaceListStore.getState().fetch()
      if (!disposed) {
        const loaded = useWorkspaceListStore.getState().data
        const repos = dataOf(loaded)
        if (repos) useSidebarStore.getState().setRepos(repos)
        // fetch() settles with the newest read, so anything but a fresh success
        // (an error) leaves the claim pending for the next reseed or frame.
        if (loaded !== before && loaded.status === 'success' && repos) {
          const seeded = useFolderSignalStore.getState().markTreeSeeded
          for (const repoId of claimed) {
            seeded(repoId)
            seededPendingRebuild.delete(repoId)
          }
        }
      }
      rebuildInFlight = false
      // A seed that landed while IndexedDB was being read may not be present in
      // that snapshot. Run one debounced follow-up, never one fetch per seed.
      if (rebuildQueued && !disposed) scheduleRebuild()
    }

    function scheduleRebuild(): void {
      if (disposed) return
      rebuildQueued = true
      if (rebuildInFlight) return
      if (rebuildTimer !== undefined) clearTimeout(rebuildTimer)
      rebuildTimer = setTimeout(() => {
        rebuildTimer = undefined
        if (!disposed) void rebuildSidebar()
      }, REBUILD_BATCH_MS)
    }

    // -- incremental merge, keyed by the frame's entity id -----------------

    function onReposChange(change: EntityChange): void {
      if (disposed) return
      if (change.kind === 'seed') {
        // A seed is authoritative over the project's whole repo set (it prunes
        // ghosts), so the tree has to be rebuilt from the cache.
        scheduleRebuild()
        reconcile()
        return
      }
      // A tombstone removes a repo and everything under it — again a rebuild.
      if (change.frame.status !== 'deleted') {
        // A repo we have never seen (a fresh import) is appended straight away
        // so its row appears without waiting on an IndexedDB round trip; its
        // workspaces arrive on the per-repo stream reconcile() opens below.
        // For a repo we already hold this is a no-op, and the rebuild below
        // carries its changed header fields (name, avatar, path).
        useSidebarStore
          .getState()
          .mergeRepos([toSidebarRepo(change.frame as unknown as RepoDTO, [])])
      }
      scheduleRebuild()
      reconcile()
    }

    function onWorkspacesChange(change: EntityChange): void {
      if (disposed) return
      if (change.kind === 'seed') {
        scheduleRebuild()
        // A repo that was seeded only because it had never been (see
        // desiredKeys' neverSeededWorkspaces) may now be collapsed AND
        // already seeded — reconcile so its subscription can close on this
        // same tick rather than waiting on some unrelated store mutation.
        reconcile()
        return
      }
      // THE hot path. A workspace frame carries a complete DTO, so it can be
      // merged into (or removed from) exactly one repo by id — no cache read,
      // no tree replacement. This used to answer every frame with a full
      // rebuild, which is what made an idle agent turn cost O(all workspaces).
      useSidebarStore.getState().applyWorkspaceDTO(change.frame as unknown as WorkspaceDTO)
    }

    // -- the repo's TREE ROWS: reseed-on-signal, no push channel (Task 34/D) --
    //
    // Both halves of a repo's tree — its FOLDERS and its CHAT rows — are read
    // this way, on one loop watching one signal, because they are one tree over
    // one backend aggregate: a folder IS a domain.Chat row (design spec §3.1),
    // both are served off the same repo-scoped .../chats mount, and both are
    // invalidated by the same frames.
    //
    // The backend's dedicated folders REST+WS resource was deleted (its own
    // plan is closed), and chats never had one: their only live-update path is
    // an id-only invalidation frame on a WORKSPACE's chats WS (no snapshot, no
    // row — "the tree moved, read it again"). There is nothing repo-scoped left
    // to open a WS subscription against, so this is a plain reseed loop instead
    // of `subscribeEntityStream`: seed once on open, and again every time
    // `useFolderSignalStore`'s generation for this repo moves (bumped by
    // use-workspace-agent-chats-stream.ts on a folder_* frame, on a STRUCTURAL
    // chat frame, or on a reconnect, for a workspace of this repo).
    //
    // That only fires while some workspace of this repo is mounted — the
    // acceptable half of the tradeoff for folders, because the acting user's
    // OWN folder edits never depend on it: sidebar-placement.ts applies a
    // create/rename/move/delete's own `{folder, shifted}` response to the store
    // directly, the instant it lands. A CHAT edit has no such local apply and
    // does depend on the frame — which is fine, because a chat can only be
    // created, renamed or moved from a surface that has that workspace mounted.
    //
    // KNOWN BACKEND LIMITATION, folders half only (see `fetchFolders`'s own doc
    // comment in lib/api.ts): the daemon's ListInRepo does not actually scope by
    // repo, so a reseed here can ingest another repo's folder rows, each
    // mis-stamped with THIS repo's id. Not fixable from here. The CHATS half has
    // no such flaw — `ListChatsInRepo` resolves each row's owning repo server
    // side and serves only this repo's.
    function openRepoTreeSubscription(projectId: string, repoId: string): () => void {
      let closed = false
      /** The reseed in flight, if any — a signal arriving mid-read coalesces
       *  into it (and queues one more) instead of superseding it: discarding
       *  the first GET's result only widened the stale window by a round trip. */
      let inFlight: Promise<void> | null = null
      let rerun = false

      /**
       * Replace THIS repo's rows in one entity store, leaving every other
       * repo's alone — exactly like subscribeEntityStream's own `pruneScope`,
       * because these stores are deliberately cross-repo and pruning wholesale
       * would wipe the siblings on each reseed.
       *
       * `live` is re-checked after every await: a reseed closed mid-flight
       * must not finish writing a snapshot nobody holds any more.
       */
      async function replaceRepoScope<T extends { id: string; repoId: string }>(
        store: 'crowbar_folders' | 'crowbar_chats',
        items: T[],
        live: () => boolean,
      ): Promise<void> {
        const cached = await getAllEntities<T>(store)
        if (!live()) return
        const fresh = new Set(items.map((item) => item.id))
        const stale: string[] = []
        const known = new Map<string, string>()
        for (const row of cached) {
          if (row.repoId === repoId && !fresh.has(row.id)) stale.push(row.id)
          if (fresh.has(row.id)) known.set(row.id, JSON.stringify(row))
        }
        // Only rows that actually changed are written: a warm boot reseeds
        // every visible repo with exactly what the cache already holds.
        const changed = items.filter((item) => known.get(item.id) !== JSON.stringify(item))
        await Promise.all(stale.map((id) => removeEntity(store, id)))
        await Promise.all(changed.map((item) => upsertEntity(store, item)))
      }

      /**
       * One half of the reseed, isolated from the other's failure on purpose: a
       * transient error reading chats must not also discard folders that came
       * back fine (and vice versa), because the next signal may be a long way
       * off. Returns whether it wrote anything worth rebuilding for.
       */
      async function reseedHalf<T extends { id: string; repoId: string }>(
        label: string,
        store: 'crowbar_folders' | 'crowbar_chats',
        read: () => Promise<T[]>,
        live: () => boolean,
      ): Promise<boolean> {
        try {
          const items = await read()
          if (!live()) return false
          await replaceRepoScope(store, items, live)
          return live()
        } catch (err) {
          console.error(`app-sync-provider: ${label} reseed failed for repo ${repoId}`, err)
          return false
        }
      }

      async function readTree(): Promise<void> {
        const live = () => !disposed && !closed
        const wrote = await Promise.all([
          reseedHalf('folders', 'crowbar_folders', () => fetchFolders(projectId, repoId), live),
          reseedHalf('chats', 'crowbar_chats', () => fetchRepoChats(projectId, repoId), live),
        ])
        const [, chatsWrote] = wrote
        // QUEUED, not announced. The chat list is in the CACHE now, which is
        // not where the sidebar reads rows from — `rebuildSidebar` opens the
        // gate once these rows are actually in the store (see
        // `seededPendingRebuild`). Keyed on the CHATS half alone: a workspace's
        // row is identified by the chat that owns it, and a folders-only
        // success answers nothing about that. A failed read queues nothing and
        // is retried on the next signal, rather than publishing a list the
        // daemon never confirmed.
        if (chatsWrote && live()) seededPendingRebuild.add(repoId)
        if (wrote.some(Boolean) && live()) scheduleRebuild()
      }

      function reseed(): void {
        if (inFlight) {
          rerun = true
          return
        }
        const read = readTree().finally(() => {
          inFlight = null
          if (treeReads.get(repoId) === read) treeReads.delete(repoId)
          if (rerun && !disposed && !closed) {
            rerun = false
            reseed()
          }
        })
        inFlight = read
        treeReads.set(repoId, read)
      }

      reseed()
      const unsubscribeSignal = useFolderSignalStore.subscribe(
        // Keyed by THIS repo's own generation — the cross-repo guard on the
        // read side, matching the bump side's own workspace-scoped repo id: a
        // chat or folder frame in repo A moves only A's counter, so B's
        // subscriber is never woken and B never refetches.
        (state) => state.generations[repoId] ?? 0,
        () => {
          if (!disposed && !closed) reseed()
        },
      )

      // The bump-signal subscription above only ever fires while SOME
      // workspace of this repo is mounted (use-workspace-agent-chats-
      // stream.ts is what calls bump, and it is only mounted per open
      // workspace-view tab) — an assumption this file's own doc comment
      // states outright ("a chat can only be created, renamed or moved from
      // a surface that has that workspace mounted"). That assumption is
      // false for the sidebar tree itself: dragging a row IN THE SIDEBAR
      // reorders/reparents it with no tab open at all (caught live: a fork
      // with no open tab PATCHed 200, correct data, and the sidebar never
      // repainted without a manual reload). This subscription is mounted
      // whenever the repo's tree rows are (the same "tree" key desiredKeys
      // already gates this whole function behind), so it is the one place
      // that can hear a structural frame regardless of any open tab —
      // chatBase(wsId)/ws resolves to this SAME repo-scoped URL (see
      // agent-api.ts's chatBase → repoChatsBaseForWorkspace), so this reuses
      // the identical multiplexed connection rather than opening a second one.
      const unsubscribeFrames = wsManager.subscribe(
        `/v0/projects/${projectId}/repos/${repoId}/chats/ws`,
        (frame) => {
          if (disposed || closed) return
          if (frame && typeof frame === 'object' && 'reconnected' in frame) {
            reseed()
            return
          }
          if (isStructuralChatFolderFrame(frame)) reseed()
        },
      )

      return () => {
        closed = true
        unsubscribeSignal()
        unsubscribeFrames()
      }
    }

    /**
     * GET .../workspaces mints a workspace's owning chat on first read, and
     * its created frame can land before this repo's chat socket is open — so
     * the chat list may not hold an owner the workspace rows already name.
     * A missing owner is a real signal to re-read the tree, not a timer.
     */
    async function reseedChatsForUnlistedOwners(
      repoId: string,
      rows: readonly WorkspaceDTO[],
    ): Promise<void> {
      const owners = rows.map((ws) => ws.owningChatId).filter((id): id is string => !!id)
      if (owners.length === 0) return
      // A tree read in flight is the list the cache is about to hold (on a
      // cold boot it was sent alongside this seed); judge against that.
      await treeReads.get(repoId)
      if (disposed) return
      const cached = await getAllEntities<{ id: string; repoId: string }>('crowbar_chats')
      if (disposed) return
      const listed = new Set<string>()
      for (const c of cached) if (c.repoId === repoId) listed.add(c.id)
      if (owners.some((id) => !listed.has(id))) useFolderSignalStore.getState().bump(repoId)
    }

    // -- keyed subscription registry ---------------------------------------

    function openSubscription(key: string): () => void {
      const [kind, projectId, repoId] = key.split(KEY_SEP)
      if (kind === 'repos') {
        return subscribeEntityStream<RepoDTO>({
          endpoint: `/v0/projects/${projectId}/repos`,
          store: 'crowbar_repos',
          seed: () => fetchRepos(projectId),
          onChange: onReposChange,
          // Authoritative over THIS project's repos only — crowbar_repos holds
          // other projects' repos too, cached for an instant return.
          pruneScope: (repo) => repo.projectId === projectId,
        })
      }
      if (kind === 'tree') {
        return openRepoTreeSubscription(projectId, repoId)
      }
      if (kind === 'hometree') {
        return subscribeHomeTree(projectId)
      }
      // A repo's worktrees: the seed reads the real GET .../workspaces
      // resource directly (fetchWorkspaces, api.ts — restored after chat
      // derivation left a chatless workspace, including a repo's own default
      // checkout, unreachable), and the live half is the repo-wide chat
      // lifecycle feed, whose `worktree_state` frames carry the worktree
      // nested inside them for the workspaces a chat DOES own. Every other
      // kind on that socket maps to null and is ignored.
      //
      // This feed resolves no single workspace, so — exactly as the old
      // repo-level workspace LIST stream did not — it never starts the daemon's
      // provider PR-status poll. That is the per-CHAT stream's job; see
      // use-workspace-provider-stream.ts.
      return subscribeEntityStream<WorkspaceDTO>({
        endpoint: `/v0/projects/${projectId}/repos/${repoId}/chats/ws`,
        store: 'crowbar_workspaces',
        seed: async () => {
          const rows = await fetchWorkspaces(projectId, repoId)
          // AFTER a successful fetch, not before: desiredKeys' own
          // neverSeededWorkspaces bypass must keep applying for every
          // attempt until one actually lands.
          useFolderSignalStore.getState().markWorkspacesSeeded(repoId)
          await reseedChatsForUnlistedOwners(repoId, rows)
          return rows
        },
        mapFrame: (raw) => workspaceDTOFromWorktreeFrame(raw, projectId, repoId),
        shouldReseed: isStructuralChatFolderFrame,
        onChange: onWorkspacesChange,
        // Authoritative over THIS repo's workspaces only — crowbar_workspaces
        // also holds every other repo's rows; pruning the whole store would
        // wipe sibling repos on each reseed.
        pruneScope: (ws) => ws.repoId === repoId,
      })
    }

    function ensureOpen(key: string): void {
      const existing = subscriptions.get(key)
      if (existing) {
        // Re-expanded inside the grace period: keep the live stream, just call
        // off its teardown. No unsubscribe, no reseed, no flash.
        if (existing.teardown !== undefined) {
          clearTimeout(existing.teardown)
          existing.teardown = undefined
        }
        return
      }
      subscriptions.set(key, { dispose: openSubscription(key) })
    }

    function closeNow(key: string): void {
      const existing = subscriptions.get(key)
      if (!existing) return
      if (existing.teardown !== undefined) clearTimeout(existing.teardown)
      subscriptions.delete(key)
      existing.dispose()
    }

    function scheduleClose(key: string): void {
      const existing = subscriptions.get(key)
      if (!existing || existing.teardown !== undefined) return
      existing.teardown = setTimeout(() => closeNow(key), SUBSCRIPTION_GRACE_MS)
    }

    // -- which streams should be open right now ----------------------------

    function desiredKeys(): Set<string> {
      const visibleProjects = getVisibleProjectIds()
      const keys = new Set<string>()
      for (const projectId of visibleProjects) {
        keys.add(reposKey(projectId))
        keys.add(homeTreeKey(projectId))
      }

      // Every visible repo draws its whole tree (the restyled sidebar folds
      // rows via collapsedChatRows, which hides nothing the streams feed), so
      // each one keeps both its workspaces and its folders+chats streams open.
      const { repos } = useSidebarStore.getState()
      for (const repo of repos) {
        const projectId = repo.projectId
        if (!projectId || !visibleProjects.has(projectId)) continue
        keys.add(workspacesKey(projectId, repo.id))
        keys.add(treeKey(projectId, repo.id))
      }
      return keys
    }

    // Cheap guard: reconcile() runs whenever the inputs of desiredKeys() move
    // (so a newly seeded repo immediately gets its workspace stream), and most
    // of those leave the desired set untouched — compared as sets, with no
    // sort/join per call.
    let lastDesired: Set<string> | null = null

    function reconcile(): void {
      if (disposed) return
      const desired = desiredKeys()
      const previous = lastDesired
      let isOpening = previous === null
      if (previous) {
        for (const key of desired) {
          if (!previous.has(key)) {
            isOpening = true
            break
          }
        }
        if (!isOpening && desired.size === previous.size) return
      }
      lastDesired = desired

      for (const key of [...subscriptions.keys()]) {
        if (!desired.has(key)) scheduleClose(key)
      }
      for (const key of desired) ensureOpen(key)
      // Closing a section must be a render-only operation. Its cached rows are
      // already hidden by the tree, so rebuilding here only throws their object
      // identities away and makes the next expand wait on IndexedDB. An opening
      // does need a cache read: the section may have started collapsed at boot.
      if (isOpening) scheduleRebuild()
    }

    async function start(): Promise<void> {
      // 1. Version-gated wipe BEFORE seeding so a stale cache can't leak frames.
      await maybeWipeOnVersionChange()
      if (disposed) return

      // 2. Project list: GET seed + live WS stream. Always on — it is one
      //    stream over a handful of rows, and it is what every project's
      //    row is drawn from.
      //    Live first, so a read the root route's guard already has in flight
      //    is joined rather than repeated (frames replay onto its answer).
      rootUnsubscribes.push(useProjectDataStore.getState().startSync())
      void useProjectDataStore.getState().fetch()

      // 3. Per-project / per-repo streams for whatever is visible. The provider
      //    mounts at the root BEFORE any project exists (fresh start / OOBE), so
      //    visibility usually arrives AFTER mount — reconcile now, and again on
      //    every project-, project-list- or sidebar-store change (active project
      //    switched, the project list landing, a repo seeded into the tree).
      //    Without this, importing the first project never populates the
      //    entity cache and the sidebar stays empty.
      //
      //    The project-LIST subscription is what makes "open by default" work:
      //    visibility is "every known project plus the active one", so the set
      //    only grows when `/v0/projects` delivers. Its own `lastSignature`
      //    guard keeps the extra wake-ups free.
      scheduleRebuild()
      reconcile()
      // Narrow: only the fields desiredKeys() reads wake it — not every
      // sidebar write (selection, drag, working flags, per-frame rows).
      rootUnsubscribes.push(
        useProjectStore.subscribe((s, prev) => {
          if (s.activeProjectId !== prev.activeProjectId) reconcile()
        }),
      )
      rootUnsubscribes.push(
        useProjectDataStore.subscribe((s, prev) => {
          if (s.data !== prev.data) reconcile()
        }),
      )
      rootUnsubscribes.push(
        useSidebarStore.subscribe((s, prev) => {
          if (s.repos !== prev.repos) reconcile()
        }),
      )
    }

    void start()
    return () => {
      disposed = true
      if (rebuildTimer !== undefined) clearTimeout(rebuildTimer)
      rootUnsubscribes.forEach((u) => u())
      for (const key of [...subscriptions.keys()]) closeNow(key)
    }
  }, [])
}
