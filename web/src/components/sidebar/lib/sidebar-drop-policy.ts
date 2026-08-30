import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
  resolvesToFirstChild,
  type AllowedModes,
  type DropPolicy,
} from '@/components/tree-dnd/drop-core'
import { isWorkspaceLockedInSidebar, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * What a drag may do to a row in the unified sidebar tree, and where the
 * indicator goes — the one matrix that replaces the workspace tree's
 * `SIDEBAR_DROP_POLICY` (`components/layout/drop-rules.ts`) and the chats
 * tree's `CHAT_DROP_POLICY` (`features/agent/tree/lib/chat-drop.ts`) now that
 * both trees are one `SidebarRow` forest (Task 4/8).
 *
 * A `SidebarRow` carries no `repoId`/`projectId` of its own — Task 4 kept it
 * to only what the tree draws. Scope is resolved the same way
 * `sidebar/lib/row-actions.ts` already resolves a row id against the live
 * tree (`performCreateFolder`, `performRenameRow`): a row's id is matched
 * against a repo's three id spaces (its default/home workspace, a tree
 * workspace, or a folder). This runs from pointer-driven code (a drag, never
 * a render), so reading `getState()` here follows the store's own usage rule.
 */

export interface RowScope {
  repoId: string
  projectId: string | undefined
}

/**
 * A row's owning repo/project, or null if nothing in the live store claims
 * this id — a race, or a row kind (`chat`, `workflow`) with no producer
 * feeding `repos` yet. Every rule below refuses rather than guesses when this
 * comes back null, same posture the old policies took on an unrecognised
 * selection.
 *
 * Takes `repos` rather than reading the store itself so one drag call reads
 * `getState()` once, not once per subject. `drop-dom.ts`'s own design note
 * says this runs on every pointermove once a live drag wires it up (Task 21)
 * — the remaining per-row linear scan is still real cost that task should
 * consider caching per-gesture rather than per-frame; not fixed here, since
 * this file only owns the matrix, not the drag loop.
 *
 * Exported so `drop-actions.ts`'s `onDrop` implementation (Task 33) resolves
 * a row's repo/project the same way this matrix does, rather than a second
 * copy of the same id-space walk.
 */
export function resolveRowRepo(repos: readonly Repo[], rowId: string): RowScope | null {
  const repo = repos.find(
    (r) =>
      r.defaultWorkspaceId === rowId ||
      r.workspaces.some((w) => w.id === rowId) ||
      r.folders?.some((f) => f.id === rowId),
  )
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

/**
 * Which of before/after/into this drag may do to this target.
 *
 * Same-project rule generalizes `drop-rules.ts`'s same-repo rule (its
 * `allowedModes`, ~line 80: `subjects.some((s) => s.repoId !== target.repoId)`):
 * rows now span one project's whole forest rather than one repo's workspaces
 * (`rows-for-project.ts`), so the boundary a drag may not cross moved up one
 * level. Cross-repo, within-project drags stay legal, but only for a row that
 * owns no worktree (design spec §8.3, "cross-repo drag is legal only for a
 * chat with no worktree") — reparenting a fork across repos makes no sense,
 * but a bubble chat pinned to nothing on disk can move freely.
 *
 * Working-row refusal (spec §8.3) reads `SidebarRow.working`, refusing every
 * mode when any dragged row is working — the UI mirror of the backend plan's
 * `guardNotWorking`, duplicated here deliberately since a drag needs to
 * refuse before a network round trip, not after.
 *
 * A protected branch (`drop-rules.ts`'s old locked rule, ~line 82-93) owns a
 * worktree pinned to its parent, so it may reorder among its own siblings and
 * nothing else — including not "after" an expanded sibling, since that would
 * nest it. Lock state isn't a `SidebarRow` field; it's read the same way
 * `file-explorer-tree.tsx` already reads it, via `isWorkspaceLockedInSidebar`
 * against the row's `workspaceId`.
 */
export function allowedModes(subjects: readonly SidebarRow[], target: SidebarRow): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  if (subjects.some((s) => s.working)) return NO_MODES
  // Never drop a row onto itself.
  if (subjects.some((s) => s.id === target.id)) return NO_MODES
  // A mixed-kind selection is not a thing the sidebar can express; refuse
  // rather than guess which class wins (carried over from both old policies).
  const kind = subjects[0].kind
  if (subjects.some((s) => s.kind !== kind)) return NO_MODES

  const repos = useSidebarStore.getState().repos

  const targetScope = resolveRowRepo(repos, target.id)
  if (!targetScope) return NO_MODES

  for (const subject of subjects) {
    const subjectScope = resolveRowRepo(repos, subject.id)
    if (!subjectScope) return NO_MODES
    if (subjectScope.projectId !== targetScope.projectId) return NO_MODES
    if (subjectScope.repoId !== targetScope.repoId && subject.ownsWorktree) return NO_MODES
  }

  const hasLocked = subjects.some((s) => isWorkspaceLockedInSidebar(repos, s.workspaceId))
  if (hasLocked) {
    const sameParent = subjects.every((s) => s.parentId === target.parentId)
    if (!sameParent) return NO_MODES
    return resolvesToFirstChild(target, 'after')
      ? { before: true, after: false, into: false }
      : REORDER_MODES
  }

  return ALL_MODES
}

/**
 * The outer band of a row reorders; the middle nests. A folder gets the
 * container band — filing into one is cheap and common. Everything else gets
 * the heavy band: nesting under a branch re-parents a fork, nesting under a
 * chat makes the row one of its threads, and nesting into an as-yet-unwired
 * `workflow` row is assumed the same weight until one exists to say
 * otherwise — all heavier moves that deserve a harder-to-hit target. Matches
 * `drop-rules.ts`'s own `edgeBandFor` verbatim, generalized off `SidebarRowKind`.
 */
export function edgeBandFor(kind: string): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, SidebarRow> = {
  allowedModes,
  edgeBandFor,
}
