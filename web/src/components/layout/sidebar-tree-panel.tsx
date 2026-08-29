import { useCallback, useMemo } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { applyPendingRemovals, planRemoval } from './removal-plan'
import { RemovalTray } from './removal-tray'
import { postWorkspace } from '@/lib/api'
import { createChat } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { toast } from '@/features/window/stores/toast-store'
import type { DragSubject } from './drop-rules'

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

/**
 * Find `id` — a workspace, a folder, or a repo's own default workspace —
 * across every visible repo.
 */
function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === id)
    if (ws) {
      return {
        repo,
        subject: {
          kind: 'workspace',
          id,
          repoId: repo.id,
          locked: ws.status === 'locked',
          parentId: ws.parentId,
        },
      }
    }
    const folder = repo.folders?.find((f) => f.id === id)
    if (folder) {
      return { repo, subject: { kind: 'folder', id, repoId: repo.id, parentId: folder.parentId } }
    }
    if (repo.defaultWorkspaceId === id) {
      // Not a real drag/removal subject — `draftFor` finds no matching row in
      // `repo.workspaces` for this id and returns null, which is what makes
      // trashing the repo-home row a safe no-op below rather than a delete.
      return { repo, subject: { kind: 'workspace', id, repoId: repo.id } }
    }
  }
  return null
}

/**
 * Bridges today's `Repo[]` into `SidebarTree` — the mount point Task 8
 * replaces `WorkspaceTree` and `AgentChatsPanel` with. One flat tree over
 * every visible repo (Part C gives each project its own space; until then
 * every repo's rows sit side by side here, exactly as the tree they replace
 * showed every project at once).
 *
 * Rename, drag-and-drop, multiselect, the right-click menu, per-row lock and
 * branch import have no home on `SidebarRow`'s four-prop surface yet (Parts
 * C/D/G/H build those back in) — this panel wires only what that surface
 * offers: opening a row, trashing one through the same removal tray the old
 * trees used, and creating a child.
 */
export function SidebarTreePanel() {
  const navigate = useNavigate()
  const allRepos = useSidebarStore((s) => s.repos)
  const hiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const repos = useMemo(() => applyPendingRemovals(allRepos, hiddenIds), [allRepos, hiddenIds])
  const rows = useMemo(() => repos.flatMap(rowsFromRepo), [repos])

  const handleOpen = useCallback(
    (id: string) => {
      const found = resolveRow(repos, id)
      if (!found) return
      // A folder has no destination — the row body toggles it, same as the
      // tree it replaces.
      if (found.subject.kind === 'folder') {
        useSidebarStore.getState().toggleChatRow(id)
        return
      }
      if (!found.repo.projectId) return
      void navigate({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: found.repo.projectId, repoId: found.repo.id, wsId: id },
      })
    },
    [repos, navigate],
  )

  const handleTrash = useCallback((id: string) => {
    const currentRepos = useSidebarStore.getState().repos
    const found = resolveRow(currentRepos, id)
    if (!found) return
    const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
    // A repo-home id resolves to a `workspace` subject that names no row in
    // `repo.workspaces` — planRemoval finds nothing to draft, so this is a
    // no-op rather than a delete. Repo deletion gets its own confirmation
    // flow in Part H; it is not reachable from a row's trash button yet.
    const drafts = planRemoval([found.subject], currentRepos, projects)
    if (drafts.length === 0) return
    useRemovalTrayStore.getState().hold(drafts)
  }, [])

  const handleCreate = useCallback((parentId: string, kind: 'workspace' | 'thread') => {
    const currentRepos = useSidebarStore.getState().repos
    const found = resolveRow(currentRepos, parentId)
    if (!found) return
    const { repo, subject } = found
    const { projectId } = repo
    if (!projectId) return

    if (kind === 'workspace') {
      // §3.4: a new workspace is minted with a generated slug, not a typed
      // name — SidebarRow's "+" has no naming step. The slug settles later
      // (an agent renames the branch); this bridge only mints it.
      const branch = `workspace-${crypto.randomUUID().slice(0, 8)}`
      const placement =
        subject.kind === 'folder'
          ? { folderId: parentId }
          : parentId === repo.defaultWorkspaceId
            ? {}
            : { parentId }
      postWorkspace(projectId, repo.id, branch, placement).catch((err: unknown) => {
        toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
      })
      return
    }

    // A thread needs a real workspace to run in — a folder names none.
    if (subject.kind !== 'workspace') return
    const wsId = parentId
    const store = getOrCreateWorkspaceStore(wsId)
    const provider = store.getState().agentChats.providers.find((p) => p.enabled)
    if (!provider) return
    createChat(wsId, provider.id).catch((err: unknown) => {
      toast.error(err instanceof Error ? err.message : 'Failed to start chat')
    })
  }, [])

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <SidebarTree
          rows={rows}
          onOpen={handleOpen}
          onTrash={handleTrash}
          onCreate={handleCreate}
        />
      </ScrollArea>
      <RemovalTray />
    </div>
  )
}
