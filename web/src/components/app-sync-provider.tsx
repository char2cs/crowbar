import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getVisibleProjectIds } from '@/lib/store/project-visibility'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import { subscribeHomeWorkspace } from '@/lib/store/home-workspace'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'
import {
  fetchFolders,
  fetchRepoChats,
  fetchRepos,
  fetchWorkspaces,
  workspaceDTOFromWorktreeFrame,
} from '@/lib/api'
import { subscribeEntityStream, type EntityChange } from '@/lib/ws/entity-stream'
import { getAllEntities, removeEntity, upsertEntity } from '@/lib/persistence/entity-cache'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { maybeWipeOnVersionChange } from '@/lib/persistence/idb'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// §7 startup sequence, subscribed BY VISIBILITY rather than by existence.
//
//   /v0/projects                            always — one stream, a handful of rows
//   a project's repos                       while that project is visible
//                                           (not folded away, or the active one)
//   a repo's worktrees (via its chats)      while that repo is expanded, holds the
//                                           active workspace, or has live work to
//                                           report on its header
//   a repo's tree rows (folders + chats)    while that repo is expanded or holds
//                                           the active workspace
//
// Cost is then proportional to what is on screen instead of to how much work
// you have: a collapsed project costs one cached row and nothing else, and
// expanding one renders instantly from the IndexedDB entity cache while its
// seed GET is still in flight. Teardown waits out a short grace period so a
// collapse/expand tap doesn't thrash the socket, and every stream is held under
// its own key so adding or dropping one never disturbs the others.
//
// On mount we
//   1. maybeWipeOnVersionChange() ONCE before any seeding (drops a stale cache
//      after a daemon/DTO version bump so we never merge incompatible frames);
//   2. seed + WS-subscribe the project list (`/v0/projects`);
//   3. reconcile the per-project / per-repo streams against the visible set, and
//      re-reconcile whenever that set moves.

/**
 * How long a stream outlives the collapse that made it invisible. Long enough
 * that a collapse/expand tap is free (no unsubscribe/resubscribe/reseed round
 * trip), short enough that a project you actually closed stops costing frames.
 */
export const SUBSCRIPTION_GRACE_MS = 2000
const REBUILD_BATCH_MS = 16
/** How many times a rebuild whose read lost to a newer fetch will try again
 *  before leaving its claimed repos to the next reseed or frame. Two is enough
 *  for a real supersede — the winning read settles — and small enough that a
 *  store wedged in `loading` costs a handful of reads, not a permanent loop. */
const MAX_SUPERSEDED_RETRIES = 2

const KEY_SEP = '|'
/** The project-home workspace tracker for the ACTIVE project (see below). */
const homeKey = (projectId: string): string => `home${KEY_SEP}${projectId}`
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

export function AppSyncProvider({ children }: { children: ReactNode }) {
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
    /** Consecutive rebuilds whose read lost to a newer fetch, so the retry
     *  below can never become a permanent 16ms loop. Reset by any rebuild that
     *  actually opens something. */
    let supersededRetries = 0

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
      // The snapshot this rebuild starts from, so a FRESH one can be told apart
      // from it afterwards. Taken here rather than derived later because
      // nothing else can distinguish them: `success(old)` and `success(new)`
      // are the same shape, and only object identity says which one the read
      // actually produced. Safe to compare against everything that lands after
      // this line — `fetch()` bumps `latestFetch` synchronously, and
      // loadable-slice re-checks it on BOTH sides of its cache write, so every
      // older in-flight fetch is barred from publishing. Drop the check after
      // that write and this gate silently reads a pre-claim snapshot as a
      // brand-new one.
      const before = useWorkspaceListStore.getState().data
      await useWorkspaceListStore.getState().fetch()
      let opened = false
      let awaitingNewerRead = false
      if (!disposed) {
        const loaded = useWorkspaceListStore.getState().data
        const repos = dataOf(loaded)
        if (repos) useSidebarStore.getState().setRepos(repos)
        // A read that BOTH settled and moved is the other half of the claim.
        //
        // `fetch` returns early whenever a newer caller supersedes it
        // (loadable-slice's `latestFetch` guard — hydration-gate and
        // events/connect's `workspace:updated` both call it), and it can do so
        // at two different points. Give up after its own read and it has
        // already published `loading`, whose `dataOf` is the previous snapshot.
        // But give up in the earlier check, inside `loadCache`, and it
        // publishes NOTHING — the store still holds the `success(old)` it held
        // before we started waiting, which predates the write this claim was
        // taken for. Status alone cannot see that; identity can.
        //
        // `loaded !== before` therefore means some read that began after the
        // claim published this, and `success` means it finished. Neither is
        // sufficient alone.
        if (loaded !== before && loaded.status === 'success' && repos) {
          const seeded = useFolderSignalStore.getState().markTreeSeeded
          for (const repoId of claimed) {
            seeded(repoId)
            seededPendingRebuild.delete(repoId)
          }
          opened = true
        } else {
          // Both ways a supersede leaves the store, and only these: a newer
          // read is on its way and has yet to publish anything of its own.
          awaitingNewerRead =
            loaded.status === 'loading' || (loaded === before && loaded.status === 'success')
        }
      }
      rebuildInFlight = false
      // Nothing else republishes what a losing read dropped: the fetch that
      // superseded ours writes the store's data but never calls `setRepos`, and
      // a claim taken BEFORE this rebuild started has no follow-up of its own
      // armed (one taken during it does — its own `scheduleRebuild` set
      // `rebuildQueued`). Without this, those repos would sit unopened, drawing
      // no rows, until some unrelated frame happened to rebuild.
      //
      // Only a supersede, and only a few times. A newer read resolves itself,
      // so trying again reaches it; `error` and `idle` can persist, and
      // retrying into either is a 16ms spin. Those ids stay pending instead
      // and ride the next reseed or frame out.
      if (!opened && claimed.length > 0) {
        if (awaitingNewerRead) {
          if (supersededRetries < MAX_SUPERSEDED_RETRIES) {
            supersededRetries++
            rebuildQueued = true
          }
        }
      } else if (opened) {
        supersededRetries = 0
      }
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
      let generation = 0

      /**
       * Replace THIS repo's rows in one entity store, leaving every other
       * repo's alone — exactly like subscribeEntityStream's own `pruneScope`,
       * because these stores are deliberately cross-repo and pruning wholesale
       * would wipe the siblings on each reseed.
       *
       * `live` is re-checked after every await: a reseed superseded mid-flight
       * must not finish writing a snapshot the newer one has already replaced.
       */
      async function replaceRepoScope<T extends { id: string; repoId: string }>(
        store: 'crowbar_folders' | 'crowbar_chats',
        items: T[],
        live: () => boolean,
      ): Promise<void> {
        const cached = await getAllEntities<T>(store)
        if (!live()) return
        const fresh = new Set(items.map((item) => item.id))
        const stale = cached
          .filter((row) => row.repoId === repoId && !fresh.has(row.id))
          .map((row) => row.id)
        await Promise.all(stale.map((id) => removeEntity(store, id)))
        await Promise.all(items.map((item) => upsertEntity(store, item)))
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

      async function reseed(): Promise<void> {
        const gen = ++generation
        const live = () => !disposed && !closed && gen === generation
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

      void reseed()
      const unsubscribeSignal = useFolderSignalStore.subscribe(
        // Keyed by THIS repo's own generation — the cross-repo guard on the
        // read side, matching the bump side's own workspace-scoped repo id: a
        // chat or folder frame in repo A moves only A's counter, so B's
        // subscriber is never woken and B never refetches.
        (state) => state.generations[repoId] ?? 0,
        () => {
          if (!disposed && !closed) void reseed()
        },
      )

      return () => {
        closed = true
        unsubscribeSignal()
      }
    }

    // -- keyed subscription registry ---------------------------------------

    function openSubscription(key: string): () => void {
      const [kind, projectId, repoId] = key.split(KEY_SEP)
      if (kind === 'home') return subscribeHomeWorkspace(projectId)
      if (kind === 'repos') {
        return subscribeEntityStream<RepoDTO>({
          endpoint: `/v0/projects/${projectId}/repos`,
          store: 'crowbar_repos',
          seed: () => fetchRepos(projectId),
          onChange: onReposChange,
          // Authoritative over THIS project's repos only — crowbar_repos holds
          // other projects' repos too, including collapsed ones we still want
          // cached for an instant expand.
          pruneScope: (repo) => repo.projectId === projectId,
        })
      }
      if (kind === 'tree') {
        return openRepoTreeSubscription(projectId, repoId)
      }
      // A repo's worktrees, read and pushed through its CHATS. There is no
      // workspace resource left to subscribe: a worktree is held by a chat, so
      // the seed is the chat list (`fetchWorkspaces` derives the DTOs from it)
      // and the live half is the repo-wide chat lifecycle feed, whose
      // `worktree_state` frames carry the worktree nested inside them. Every
      // other kind on that socket maps to null and is ignored.
      //
      // This feed resolves no single workspace, so — exactly as the old
      // repo-level workspace LIST stream did not — it never starts the daemon's
      // provider PR-status poll. That is the per-CHAT stream's job; see
      // use-workspace-provider-stream.ts.
      return subscribeEntityStream<WorkspaceDTO>({
        endpoint: `/v0/projects/${projectId}/repos/${repoId}/chats/ws`,
        store: 'crowbar_workspaces',
        seed: () => fetchWorkspaces(projectId, repoId),
        mapFrame: (raw) => workspaceDTOFromWorktreeFrame(raw, projectId, repoId),
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
      for (const projectId of visibleProjects) keys.add(reposKey(projectId))

      // The project-home workspace rides no repo, so the per-repo workspace
      // streams can never carry it (see home-workspace.ts). It is tracked
      // separately, and only for the ACTIVE project: useHomeWorkspaceStore has
      // a single slot, so subscribing several projects' home workspaces would
      // have them overwrite each other.
      const activeProjectId = useProjectStore.getState().activeProjectId
      if (activeProjectId) keys.add(homeKey(activeProjectId))

      const { repos, collapsedRepos } = useSidebarStore.getState()
      const activeRepoId = getWorkspaceScope()?.repoId
      for (const repo of repos) {
        const projectId = repo.projectId
        if (!projectId || !visibleProjects.has(projectId)) continue
        const holdsActiveWorkspace = repo.id === activeRepoId
        // `defaultWorking` is the one live signal a COLLAPSED repo still
        // renders (the spinner on its avatar). Keeping the stream while a turn
        // is in flight is what lets that spinner stop; dropping it mid-turn
        // would freeze it on, which is worse than not showing it at all.
        const hasLiveWorkToReport = repo.defaultWorking === true
        const showsRows = !collapsedRepos.has(repo.id) || holdsActiveWorkspace
        if (showsRows || hasLiveWorkToReport) {
          keys.add(workspacesKey(projectId, repo.id))
        }
        // Folders and chat rows are pure structure — they carry no spinner, no
        // status, nothing a collapsed repo still paints — so they stop at
        // `showsRows` rather than following the workspace stream's live-work
        // exemption above. (A chat row deliberately carries no `working` either;
        // see rows-from-repo.ts.)
        if (showsRows) keys.add(treeKey(projectId, repo.id))
      }
      return keys
    }

    // Cheap guard: reconcile() is called from every project- and sidebar-store
    // mutation (so a newly seeded repo immediately gets its workspace stream),
    // and the overwhelming majority of those leave the desired set untouched.
    let lastSignature: string | null = null
    let lastDesired = new Set<string>()

    function reconcile(): void {
      if (disposed) return
      const desired = desiredKeys()
      const signature = [...desired].sort().join('\n')
      if (signature === lastSignature) return
      lastSignature = signature
      const isOpening = [...desired].some((key) => !lastDesired.has(key))
      lastDesired = desired

      // Close first, open second. The home tracker clears the shared
      // single-slot store on teardown, so closing the outgoing project's after
      // opening the incoming one would wipe the value we just fetched.
      for (const key of [...subscriptions.keys()]) {
        if (desired.has(key)) continue
        if (key.startsWith(`home${KEY_SEP}`)) closeNow(key)
        else scheduleClose(key)
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
      //    stream over a handful of rows, and it is what a collapsed project's
      //    row is drawn from.
      void useProjectDataStore.getState().fetch()
      rootUnsubscribes.push(useProjectDataStore.getState().startSync())

      // 3. Per-project / per-repo streams for whatever is visible. The provider
      //    mounts at the root BEFORE any project exists (fresh start / OOBE), so
      //    visibility usually arrives AFTER mount — reconcile now, and again on
      //    every project-, project-list- or sidebar-store change (active project
      //    switched, the project list landing, a project folded away, a repo
      //    collapsed, a repo seeded into the tree). Without this, importing the
      //    first project never populates the entity cache and the sidebar stays
      //    empty.
      //
      //    The project-LIST subscription is what makes "open by default" work:
      //    visibility is now "every known project minus the folded ones", so the
      //    set only grows when `/v0/projects` delivers. Its own `lastSignature`
      //    guard keeps the extra wake-ups free.
      scheduleRebuild()
      reconcile()
      rootUnsubscribes.push(useProjectStore.subscribe(reconcile))
      rootUnsubscribes.push(useProjectDataStore.subscribe(reconcile))
      rootUnsubscribes.push(useSidebarStore.subscribe(reconcile))
    }

    void start()
    return () => {
      disposed = true
      if (rebuildTimer !== undefined) clearTimeout(rebuildTimer)
      rootUnsubscribes.forEach((u) => u())
      for (const key of [...subscriptions.keys()]) closeNow(key)
    }
  }, [])
  return <>{children}</>
}
