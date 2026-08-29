import type { useNavigate } from '@tanstack/react-router'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { planRemoval } from './removal-plan'
import { postWorkspace } from '@/lib/api'
import { createChat } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { toast } from '@/features/window/stores/toast-store'
import type { DragSubject } from './drop-rules'

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * Find `id` — a workspace, a folder, or a repo's own default workspace —
 * across every visible repo, regardless of which project's space panel
 * rendered it. Extracted verbatim from sidebar-tree-panel.tsx (Task 8/29): a
 * row's owning repo is never ambiguous by project, so this needs no
 * per-project variant.
 */
export function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
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
 * Opens a row: navigates into a workspace, or toggles a folder's fold.
 *
 * `repos` is the caller's removal-tray-filtered list (what is actually
 * rendered), not the raw store — a row already hidden pending removal must
 * not be openable.
 */
export function handleOpen(id: string, repos: readonly Repo[], navigate: NavigateFn): void {
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
}

/**
 * Trashes a row via the removal tray. Reads the store's raw, current repos
 * (not a caller-supplied filtered snapshot): the true current state, not the
 * UI's already-hidden-pending-removal overlay.
 */
export function handleTrash(id: string): void {
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
}

/** Creates a workspace (fork) or a thread (chat) under `parentId`. */
export function handleCreate(parentId: string, kind: 'workspace' | 'thread'): void {
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
    // Matches the old placementFor exactly: a folder is placement only, and
    // every other row — the repo-home row included — is the FORK PARENT.
    // The repo-home id is a real, correct parentId to send: dropping it
    // (posting with no parentId at all) is what silently lost merge-back
    // eligibility (MergeEligibility keys on ws.ParentID != "").
    const placement = subject.kind === 'folder' ? { folderId: parentId } : { parentId }
    postWorkspace(projectId, repo.id, branch, placement).catch((err: unknown) => {
      toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
    })
    return
  }

  // A thread needs a real workspace to run in — a folder names none. Reachable
  // only from an empty folder's affordance dropdown (its own "+" always makes
  // a workspace, see above); the dropdown itself has no folder-vs-workspace
  // split to hide this option behind, so say why instead of swallowing the
  // click.
  if (subject.kind !== 'workspace') {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  const wsId = parentId
  const store = getOrCreateWorkspaceStore(wsId)
  const provider = store.getState().agentChats.providers.find((p) => p.enabled)
  if (!provider) return
  createChat(wsId, provider.id).catch((err: unknown) => {
    toast.error(err instanceof Error ? err.message : 'Failed to start chat')
  })
}
