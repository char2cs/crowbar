import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { dataOf } from '@/lib/loadable'
import { fetchRepos, fetchWorkspaces } from '@/lib/api'
import { subscribeEntityStream } from '@/lib/ws/entity-stream'
import { getAllEntities } from '@/lib/persistence/entity-cache'
import { maybeWipeOnVersionChange } from '@/lib/persistence/idb'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// §7 startup sequence. There is no flat cross-project workspace GET anymore:
// the live sidebar tree is built from the WS-driven entity cache. On mount we
//   1. maybeWipeOnVersionChange() ONCE before any seeding (drops a stale cache
//      after a daemon/DTO version bump so we never merge incompatible frames);
//   2. seed + WS-subscribe the project list (`/v0/projects`);
//   3. for the active project, seed + WS-subscribe its repos
//      (`/v0/projects/:p/repos`) and, per repo, its workspaces
//      (`/v0/projects/:p/repos/:r/workspaces`) — each via subscribeEntityStream,
//      which seeds (GET) before the first push so no frame is dropped.
// Every entity-cache mutation triggers a sidebar rebuild from the cache.
export function AppSyncProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    let disposed = false
    const unsubscribes: Array<() => void> = []

    // Rebuild the sidebar repos tree from the current entity-cache contents.
    // useWorkspaceListStore.fetch() reads crowbar_repos + crowbar_workspaces and
    // groups them; we then push the result into the sidebar the tree renders.
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

    // Subscriptions for the CURRENT active project's repos + per-repo workspace
    // streams. Re-created whenever the active project changes (see below), so the
    // previous project's streams are torn down first.
    let projectUnsubscribes: Array<() => void> = []
    let currentProjectId: string | null = null

    function subscribeActiveProject(projectId: string): void {
      projectUnsubscribes.forEach((u) => u())
      projectUnsubscribes = []
      if (!projectId) {
        rebuildSidebar()
        return
      }

      const subscribedRepos = new Set<string>()
      const subscribeReposWorkspaces = async (): Promise<void> => {
        const repos = await getAllEntities<RepoDTO>('crowbar_repos')
        if (disposed) return
        for (const repo of repos) {
          if (repo.projectId !== projectId || subscribedRepos.has(repo.id)) continue
          subscribedRepos.add(repo.id)
          projectUnsubscribes.push(
            subscribeEntityStream<WorkspaceDTO>({
              endpoint: `/v0/projects/${projectId}/repos/${repo.id}/workspaces`,
              store: 'crowbar_workspaces',
              seed: () => fetchWorkspaces(projectId, repo.id),
              onChange: rebuildSidebar,
              // Authoritative over THIS repo's workspaces only — crowbar_workspaces
              // also holds every other repo's rows; pruning the whole store would
              // wipe sibling repos on each reseed.
              pruneScope: (ws) => ws.repoId === repo.id,
            }),
          )
        }
      }

      projectUnsubscribes.push(
        subscribeEntityStream<RepoDTO>({
          endpoint: `/v0/projects/${projectId}/repos`,
          store: 'crowbar_repos',
          seed: () => fetchRepos(projectId),
          onChange: () => {
            void subscribeReposWorkspaces()
            rebuildSidebar()
          },
          // Authoritative over THIS project's repos only — crowbar_repos holds
          // other projects' repos too.
          pruneScope: (repo) => repo.projectId === projectId,
        }),
      )
    }

    async function start(): Promise<void> {
      // 1. Version-gated wipe BEFORE seeding so a stale cache can't leak frames.
      await maybeWipeOnVersionChange()
      if (disposed) return

      // 2. Project list: GET seed + live WS stream.
      void useProjectDataStore.getState().fetch()
      unsubscribes.push(useProjectDataStore.getState().startSync())

      // 3. Active project's repos + per-repo workspace streams. The provider
      //    mounts at the root BEFORE any project exists (fresh start / OOBE), so
      //    the active project usually becomes set AFTER mount — subscribe now if
      //    one is already active, and re-subscribe whenever it changes. Without
      //    this, importing the first project never populates the entity cache and
      //    the sidebar stays empty.
      currentProjectId = useProjectStore.getState().activeProjectId || null
      subscribeActiveProject(currentProjectId ?? '')
      unsubscribes.push(
        useProjectStore.subscribe((state) => {
          if (disposed) return
          const next = state.activeProjectId || null
          if (next === currentProjectId) return
          currentProjectId = next
          subscribeActiveProject(next ?? '')
        }),
      )
    }

    void start()
    return () => {
      disposed = true
      unsubscribes.forEach((u) => u())
      projectUnsubscribes.forEach((u) => u())
    }
  }, [])
  return <>{children}</>
}
