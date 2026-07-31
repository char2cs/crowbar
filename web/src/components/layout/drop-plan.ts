import { resolvesToFirstChild, type DragSubject } from './drop-rules'
import { buildSidebarTree, type SidebarTreeNode } from './workspace-tree-utils'
import type { ResolvedDrop } from './drop-target-dom'
import type {
  FolderPlacementWrite,
  Repo,
  SidebarPlacement,
  WorkspacePlacementWrite,
} from '@/lib/store/sidebar'

/**
 * What a drop means, worked out before anything is written or sent.
 *
 * Pure: it takes the tree as it stands and the resolved target, and returns the
 * calls to fire plus the placement to paint. Keeping it here rather than inside
 * the pointer handler is what makes the whole matrix — including the multi-row
 * moves and the snap-back — testable without a synthetic drag.
 */

/** The tree as a drop needs to read it. */
export interface PlacementSnapshot {
  repos: Repo[]
  /** Project ids in sidebar order. */
  projectIds: string[]
}

/**
 * One request. A drop fires these IN ORDER: each `order` is an index into the
 * list as it stands after the previous call landed, so firing them concurrently
 * would land a multi-row move in an arbitrary arrangement.
 */
export type PlacementCall =
  | { kind: 'reparent'; projectId: string; repoId: string; id: string; parentId: string }
  | {
      kind: 'workspace'
      projectId: string
      repoId: string
      id: string
      folderId?: string
      order: number
    }
  | {
      kind: 'folder'
      projectId: string
      repoId: string
      id: string
      parentId: string
      order: number
    }
  /** `fromProjectId` addresses the repo; `projectId` is where it is going. The
   *  route is registered under the project the repo is in TODAY. */
  | { kind: 'repo'; id: string; fromProjectId: string; projectId: string; order: number }
  | { kind: 'project'; id: string; order: number }

export interface DropPlan {
  calls: PlacementCall[]
  /** What to paint immediately, and — via capturePlacement — what to undo. */
  writes: SidebarPlacement
  /** Project ids in their new order, when the drop reorders projects. */
  projectOrder?: string[]
}

/** Where a row lands among `rest` relative to `targetId`. */
function insertIndex(rest: string[], targetId: string, mode: 'before' | 'after'): number {
  const at = rest.indexOf(targetId)
  if (at < 0) return rest.length
  return mode === 'after' ? at + 1 : at
}

/** Splice `moving` into `rest` at `at`. */
function spliced(rest: string[], moving: string[], at: number): string[] {
  return [...rest.slice(0, at), ...moving, ...rest.slice(at)]
}

function findNode(nodes: SidebarTreeNode[], id: string): SidebarTreeNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node
    const hit = findNode(node.children, id)
    if (hit) return hit
  }
  return undefined
}

/**
 * Which sibling space a container owns. '' is the repo root; otherwise it is
 * whichever row carries that id.
 */
function membersOf(roots: SidebarTreeNode[], containerId: string): SidebarTreeNode[] {
  if (containerId === '') return roots
  return findNode(roots, containerId)?.children ?? []
}

export function planDrop(
  subjects: readonly DragSubject[],
  target: ResolvedDrop,
  snapshot: PlacementSnapshot,
): DropPlan | null {
  if (subjects.length === 0) return null
  switch (subjects[0].kind) {
    case 'project':
      return planProjectDrop(subjects, target, snapshot)
    case 'repo':
      return planRepoDrop(subjects, target, snapshot)
    default:
      return planRowDrop(subjects, target, snapshot)
  }
}

function planProjectDrop(
  subjects: readonly DragSubject[],
  target: ResolvedDrop,
  snapshot: PlacementSnapshot,
): DropPlan | null {
  if (target.kind !== 'project' || target.mode === 'into') return null
  const known = new Set(snapshot.projectIds)
  const moving: string[] = []
  for (const subject of subjects) {
    if (known.has(subject.id)) moving.push(subject.id)
  }
  if (moving.length === 0) return null

  const lifted = new Set(moving)
  const rest = snapshot.projectIds.filter((id) => !lifted.has(id))
  const at = insertIndex(rest, target.id, target.mode)
  return {
    calls: moving.map((id, i) => ({ kind: 'project' as const, id, order: at + i })),
    writes: {},
    projectOrder: spliced(rest, moving, at),
  }
}

function planRepoDrop(
  subjects: readonly DragSubject[],
  target: ResolvedDrop,
  snapshot: PlacementSnapshot,
): DropPlan | null {
  const destination =
    target.kind === 'project'
      ? target.id
      : (snapshot.repos.find((r) => r.id === target.id)?.projectId ?? '')
  if (!destination) return null

  const projectById = new Map(snapshot.repos.map((r) => [r.id, r.projectId ?? '']))
  const moving: string[] = []
  for (const subject of subjects) {
    if (projectById.has(subject.id)) moving.push(subject.id)
  }
  if (moving.length === 0) return null

  const lifted = new Set(moving)
  const rest: string[] = []
  for (const repo of snapshot.repos) {
    if ((repo.projectId ?? '') === destination && !lifted.has(repo.id)) rest.push(repo.id)
  }
  const at =
    target.kind === 'project' || target.mode === 'into'
      ? rest.length
      : insertIndex(rest, target.id, target.mode)

  return {
    calls: moving.map((id, i) => ({
      kind: 'repo' as const,
      id,
      fromProjectId: projectById.get(id) ?? '',
      projectId: destination,
      order: at + i,
    })),
    writes: {
      // Every member of the destination level, renumbered — the moved repos and
      // the siblings they displaced. A splice alone would be undone the moment
      // the level is re-sorted by `order`.
      repos: spliced(rest, moving, at).map((id, order) => ({ id, projectId: destination, order })),
    },
  }
}

/**
 * A workspace or folder drop.
 *
 * The container is the one thing that has to be right: `into` a row means that
 * row, `after` an EXPANDED row with children means that row too (the gap under
 * it is the slot before its first child — see `resolvesToFirstChild`), and
 * anything else means the target's own container.
 */
function planRowDrop(
  subjects: readonly DragSubject[],
  target: ResolvedDrop,
  snapshot: PlacementSnapshot,
): DropPlan | null {
  const repoId = target.kind === 'repo' ? target.id : (target.repoId ?? '')
  const repo = snapshot.repos.find((r) => r.id === repoId)
  const projectId = repo?.projectId
  if (!repo || !projectId) return null

  const roots = buildSidebarTree(repo.workspaces, repo.folders ?? [])
  const firstChild = target.mode !== 'into' && resolvesToFirstChild(target, target.mode)
  const requested =
    target.mode === 'into' || firstChild
      ? target.kind === 'repo'
        ? ''
        : target.id
      : target.parentId || ''
  // A container that is not a row in this tree is the repo root — that is what
  // the repo header stands for, and what a stale parent id degrades to.
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested
  const containerKind = containerNode?.kind ?? 'root'

  const moving = subjects.map((s) => s.id)
  const lifted = new Set(moving)
  const rest: string[] = []
  for (const node of membersOf(roots, containerId)) {
    if (!lifted.has(node.id)) rest.push(node.id)
  }
  let at: number
  if (target.mode === 'into') at = rest.length
  else if (firstChild) at = 0
  else at = insertIndex(rest, target.id, target.mode)

  const calls: PlacementCall[] = []
  const workspaces: WorkspacePlacementWrite[] = []
  const folders: FolderPlacementWrite[] = []

  subjects.forEach((subject, i) => {
    const order = at + i
    if (subject.kind === 'folder') {
      calls.push({
        kind: 'folder',
        projectId,
        repoId,
        id: subject.id,
        parentId: containerId,
        order,
      })
      folders.push({ id: subject.id, parentId: containerId, order })
      return
    }

    const ws = repo.workspaces.find((w) => w.id === subject.id)
    const currentFork = ws?.parentId ?? ''
    // A fork parent only counts when it is a row in this tree: a root workspace
    // names the repo-home workspace, which is the header rather than a row.
    const forked = currentFork !== '' && repo.workspaces.some((w) => w.id === currentFork)
    const nextFork =
      containerKind === 'workspace'
        ? containerId
        : containerKind === 'root' && forked
          ? (repo.defaultWorkspaceId ?? '')
          : ''

    if (nextFork !== '' && nextFork !== currentFork) {
      // A fork move is a rebase behind a 202, so the slot the indicator promised
      // is a SECOND call, ordered after it. Fired together it would index into a
      // level the row has not joined yet and the daemon would simply append it —
      // the row landing at the right depth in the wrong place. The reparent's
      // own promise is what holds this back (see reparent-settle.ts); the
      // reparent also clears the folder, so this one only carries the index.
      calls.push({ kind: 'reparent', projectId, repoId, id: subject.id, parentId: nextFork })
      calls.push({ kind: 'workspace', projectId, repoId, id: subject.id, order })
      workspaces.push({ id: subject.id, parentId: nextFork, order })
      return
    }

    // Only a fork ROOT is filed in a folder; a forked child is ordered among its
    // fork siblings and its folder stays where it is (the server refuses a move
    // that would split the chain).
    const folderId =
      containerKind === 'folder' ? containerId : containerKind === 'root' ? '' : undefined
    calls.push({ kind: 'workspace', projectId, repoId, id: subject.id, folderId, order })
    workspaces.push({ id: subject.id, ...(folderId !== undefined && { folderId }), order })
  })

  // Renumber the siblings the move displaced, or the level paints in its old
  // order until the daemon's dense indices arrive.
  spliced(rest, moving, at).forEach((id, index) => {
    if (lifted.has(id)) return
    if (findNode(roots, id)?.kind === 'folder') folders.push({ id, order: index })
    else workspaces.push({ id, order: index })
  })

  return {
    calls,
    writes: {
      ...(workspaces.length > 0 && { workspaces }),
      ...(folders.length > 0 && { folders }),
    },
  }
}
