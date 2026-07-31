import { buildSidebarTree, indexSidebarTree } from './workspace-tree-utils'
import {
  getPostDeleteNavigationTarget,
  EMPTY_FOLDERS,
  type Folder,
  type Repo,
} from '@/lib/store/sidebar'
import type { RemovalDraft } from '@/lib/store/sidebar-removal'
import type { DragSubject } from './drop-rules'

/**
 * What a removal means, worked out before anything is hidden.
 *
 * Pure, like `drop-plan.ts` next door and for the same reason: the whole of it —
 * which rows go, which rows go WITH them, what the tray says and what the pane's
 * overlay promises — is decided here and can be tested without a pointer.
 *
 * The one rule worth stating: a hold is not a delete. Everything below computes
 * what to HIDE, and hiding is undone by putting the ids back. The destructive
 * half is one API call per entry, and it happens only when a hold is committed.
 */

/**
 * The rows a removal hides, and the rows that go with them.
 *
 * Three kinds, three answers, and they differ because the daemon differs:
 *
 * - **A workspace** takes its whole subtree: the delete cascades server-side, so
 *   hiding only the row would promise less than is about to happen.
 * - **A folder** takes nothing. It holds no worktree, and deleting one reparents
 *   its children to the folder's own parent — which is why a folder needs no
 *   confirmation where a repo does.
 * - **A repo** takes every worktree under it, which is exactly why it waits on
 *   an answer rather than on a clock.
 */
export function planRemoval(
  subjects: readonly DragSubject[],
  repos: readonly Repo[],
): RemovalDraft[] {
  const drafts: RemovalDraft[] = []
  const claimed = new Set<string>()

  for (const subject of subjects) {
    const draft = draftFor(subject, repos)
    // A row already inside another subject's subtree is not a second removal —
    // it is part of the first one, and holding it twice would put two rows in
    // the tray for one disappearance.
    if (!draft || claimed.has(draft.id)) continue
    drafts.push(draft)
    for (const id of draft.hiddenIds) claimed.add(id)
  }

  return drafts
}

function draftFor(subject: DragSubject, repos: readonly Repo[]): RemovalDraft | null {
  if (subject.kind === 'project') return null

  if (subject.kind === 'repo') {
    const repo = repos.find((r) => r.id === subject.id)
    if (!repo?.projectId) return null
    return {
      kind: 'repo',
      id: repo.id,
      label: repo.name,
      projectId: repo.projectId,
      repoId: repo.id,
      hiddenIds: [repo.id],
      extra: repo.workspaces.length,
      fallbackWsId: null,
    }
  }

  const repo = repos.find((r) => r.id === subject.repoId)
  if (!repo?.projectId) return null

  if (subject.kind === 'folder') {
    const folder = (repo.folders ?? EMPTY_FOLDERS).find((f) => f.id === subject.id)
    if (!folder) return null
    return {
      kind: 'folder',
      id: folder.id,
      label: folder.name,
      projectId: repo.projectId,
      repoId: repo.id,
      hiddenIds: [folder.id],
      extra: 0,
      fallbackWsId: null,
    }
  }

  const workspace = repo.workspaces.find((w) => w.id === subject.id)
  // A protected branch keeps its worktree; the daemon refuses the delete, so the
  // tray must never accept one and promise otherwise.
  if (!workspace || workspace.status === 'locked') return null

  const tree = indexSidebarTree(
    buildSidebarTree(repo.workspaces, repo.folders ?? EMPTY_FOLDERS),
    repo.id,
  )
  const descendants = tree.index.descendantsOf(workspace.id)
  return {
    kind: 'workspace',
    id: workspace.id,
    label: workspace.branch,
    projectId: repo.projectId,
    repoId: repo.id,
    hiddenIds: [workspace.id, ...descendants],
    extra: descendants.length,
    // Resolved now, against a tree that still has the row in it.
    fallbackWsId: getPostDeleteNavigationTarget(repos as Repo[], workspace.id),
  }
}

/**
 * What the pane's overlay says while a removal is armed.
 *
 * It names what will go, because the pane is a large target and the sidebar
 * behind it may already be scrolled somewhere else by the time the pointer
 * arrives — "release to remove" alone leaves the user to remember which rows
 * they picked up.
 */
export function describeRemoval(drafts: readonly RemovalDraft[]): {
  title: string
  detail: string
} {
  if (drafts.length === 1) {
    const [only] = drafts
    return {
      title: `Release to remove ${only.label}`,
      detail:
        only.kind === 'repo'
          ? 'You will confirm it in the sidebar before anything is deleted'
          : 'You will have 8 seconds to undo',
    }
  }
  return {
    title: `Release to remove ${drafts.length} rows`,
    detail: 'You will have 8 seconds to undo',
  }
}

/**
 * The sidebar as it reads with the tray's rows taken out.
 *
 * A held row is hidden, never deleted, so this is a projection over the repos
 * the store already holds — which is what makes Cancel a matter of dropping an
 * id rather than putting a subtree back together.
 *
 * A held FOLDER is the one case that rewrites rather than filters: deleting a
 * folder reparents its children to the folder's own parent, so the preview has
 * to show them there. Filtering the folder alone would re-root them, which is a
 * different place and a promise the commit would not keep.
 */
export function applyPendingRemovals(
  repos: readonly Repo[],
  hiddenIds: ReadonlySet<string>,
): Repo[] {
  if (hiddenIds.size === 0) return repos as Repo[]

  const out: Repo[] = []
  for (const repo of repos) {
    if (hiddenIds.has(repo.id)) continue

    const folders = repo.folders ?? EMPTY_FOLDERS
    const heldFolders = folders.filter((f) => hiddenIds.has(f.id))
    const workspaces = repo.workspaces.filter((w) => !hiddenIds.has(w.id))
    if (heldFolders.length === 0 && workspaces.length === repo.workspaces.length) {
      out.push(repo)
      continue
    }

    if (heldFolders.length === 0) {
      out.push({ ...repo, workspaces })
      continue
    }

    // Walk each held folder's own parent up past any held ancestor, so a hold
    // that takes a folder and its parent folder still lands the survivors at the
    // outermost place that is actually still on screen.
    const byId = new Map(folders.map((f) => [f.id, f]))
    const survivingParentOf = (folderId: string): string => {
      let cursor = byId.get(folderId)?.parentId ?? ''
      const seen = new Set<string>([folderId])
      while (cursor && hiddenIds.has(cursor) && !seen.has(cursor)) {
        seen.add(cursor)
        cursor = byId.get(cursor)?.parentId ?? ''
      }
      return cursor
    }
    const rehomed = new Map(heldFolders.map((f) => [f.id, survivingParentOf(f.id)]))

    const survivors: Folder[] = []
    for (const folder of folders) {
      if (hiddenIds.has(folder.id)) continue
      survivors.push(
        folder.parentId && rehomed.has(folder.parentId)
          ? { ...folder, parentId: rehomed.get(folder.parentId) }
          : folder,
      )
    }

    out.push({
      ...repo,
      workspaces: workspaces.map((w) =>
        w.folderId && rehomed.has(w.folderId) ? { ...w, folderId: rehomed.get(w.folderId) } : w,
      ),
      folders: survivors,
    })
  }

  return out
}
