import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getVisibleProjectIds } from '@/lib/store/project-visibility'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import { subscribeHomeWorkspace } from '@/lib/store/home-workspace'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'
import { fetchFolders, fetchRepos, fetchWorkspaces } from '@/lib/api'
import { subscribeEntityStream, type EntityChange } from '@/lib/ws/entity-stream'
import { maybeWipeOnVersionChange } from '@/lib/persistence/idb'
import type { FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

// §7 startup sequence, subscribed BY VISIBILITY rather than by existence.
//
//   /v0/projects                            always — one stream, a handful of rows
//   a project's repos                       while that project is visible
//                                           (not folded away, or the active one)
//   a repo's workspaces                     while that repo is expanded, holds the
//                                           active workspace, or has live work to
//                                           report on its header
//   a repo's folders                        while that repo is expanded or holds
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

const KEY_SEP = '|'
/** The project-home workspace tracker for the ACTIVE project (see below). */
const homeKey = (projectId: string): string => `home${KEY_SEP}${projectId}`
/** A project's repo list stream. */
const reposKey = (projectId: string): string => `repos${KEY_SEP}${projectId}`
/** One repo's workspace list stream. */
const workspacesKey = (projectId: string, repoId: string): string =>
  `workspaces${KEY_SEP}${projectId}${KEY_SEP}${repoId}`
/** One repo's sidebar folder list stream. */
const foldersKey = (projectId: string, repoId: string): string =>
  `folders${KEY_SEP}${projectId}${KEY_SEP}${repoId}`

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

    function rebuildSidebar(): void {
      void useWorkspaceListStore
        .getState()
        .fetch()
        .then(() => {
          if (disposed) return
          const repos = dataOf(useWorkspaceListStore.getState().data)
          if (repos) useSidebarStore.getState().setRepos(repos)
        })
    }

    function scheduleRebuild(): void {
      if (disposed || rebuildTimer !== undefined) return
      rebuildTimer = setTimeout(() => {
        rebuildTimer = undefined
        if (!disposed) rebuildSidebar()
      }, 0)
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

    function onFoldersChange(change: EntityChange): void {
      if (disposed) return
      if (change.kind === 'seed') {
        scheduleRebuild()
        return
      }
      // Same incremental path as a workspace frame, and for the same reason: a
      // folder frame carries a complete DTO, so it merges into exactly one repo
      // by id and leaves every row it does not name alone.
      useSidebarStore.getState().applyFolderDTO(change.frame as unknown as FolderDTO)
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
      if (kind === 'folders') {
        return subscribeEntityStream<FolderDTO>({
          endpoint: `/v0/projects/${projectId}/repos/${repoId}/folders`,
          store: 'crowbar_folders',
          seed: () => fetchFolders(projectId, repoId),
          onChange: onFoldersChange,
          // Authoritative over THIS repo's folders only — crowbar_folders holds
          // every other repo's rows too, so pruning the whole store would wipe
          // sibling repos' folders on each reseed.
          pruneScope: (folder) => folder.repoId === repoId,
        })
      }
      return subscribeEntityStream<WorkspaceDTO>({
        endpoint: `/v0/projects/${projectId}/repos/${repoId}/workspaces`,
        store: 'crowbar_workspaces',
        seed: () => fetchWorkspaces(projectId, repoId),
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
        // Folders are pure structure — they carry no spinner, no status, nothing
        // a collapsed repo still paints — so they stop at `showsRows` rather than
        // following the workspace stream's live-work exemption above.
        if (showsRows) keys.add(foldersKey(projectId, repo.id))
      }
      return keys
    }

    // Cheap guard: reconcile() is called from every project- and sidebar-store
    // mutation (so a newly seeded repo immediately gets its workspace stream),
    // and the overwhelming majority of those leave the desired set untouched.
    let lastSignature: string | null = null

    function reconcile(): void {
      if (disposed) return
      const desired = desiredKeys()
      const signature = [...desired].sort().join('\n')
      if (signature === lastSignature) return
      lastSignature = signature

      // Close first, open second. The home tracker clears the shared
      // single-slot store on teardown, so closing the outgoing project's after
      // opening the incoming one would wipe the value we just fetched.
      for (const key of [...subscriptions.keys()]) {
        if (desired.has(key)) continue
        if (key.startsWith(`home${KEY_SEP}`)) closeNow(key)
        else scheduleClose(key)
      }
      for (const key of desired) ensureOpen(key)
      // Visibility moved: re-scope the tree so a collapsed project's repos
      // leave it at once rather than lingering until its stream expires.
      scheduleRebuild()
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
