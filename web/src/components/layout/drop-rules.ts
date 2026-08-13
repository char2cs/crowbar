import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  INTO_MODES,
  NO_MODES,
  REORDER_MODES,
  dropModeAt,
  resolvesToFirstChild,
  type AllowedModes,
  type DragSubjectBase,
  type DropMode,
  type DropPolicy,
  type DropTargetBase,
} from '@/components/tree-dnd/drop-core'

/**
 * What a drag may do to a SIDEBAR row, and where the indicator goes.
 *
 * This is the sidebar's matrix and nothing else. The band arithmetic, the mode
 * resolver and the first-child rule are the same in any tree and live in
 * `tree-dnd/drop-core.ts`; what is here is the part no other tree may inherit —
 * four kinds that do not mix, a same-repo rule, and the locked rule that stops a
 * protected branch's worktree being re-parented.
 *
 * Kept as pure functions with no DOM and no React so the whole matrix — every
 * permitted move and, more importantly, every refusal — is testable without a
 * synthetic pointer. `workspace-tree-context.tsx` hit-tests the pointer and
 * then asks these two questions; it decides nothing itself.
 */

export { resolvesToFirstChild }
export type { AllowedModes, DropMode }

/** The four movable classes. They do not mix. */
export type DropKind = 'workspace' | 'folder' | 'repo' | 'project'

export interface DragSubject extends DragSubjectBase {
  kind: DropKind
  /** Repo scope, for the same-repo rule. Absent on repos and projects. */
  repoId?: string
  /** A protected branch: reorders among its own siblings and nothing else. */
  locked?: boolean
  /** Its current parent, for the locked same-parent rule. */
  parentId?: string
}

export interface DropTarget extends DropTargetBase {
  kind: DropKind
  repoId?: string
}

/**
 * Which of before/after/into this drag may do to this target.
 *
 * The rule that earns its keep is the locked one. A protected branch owns a
 * worktree pinned to its parent, so it may be reordered among its own siblings
 * and nothing else — including not "after" an expanded sibling, because that
 * would nest it.
 */
export function allowedModes(subjects: readonly DragSubject[], target: DropTarget): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  const kind = subjects[0].kind
  // A mixed selection is not a thing the sidebar can express; refuse rather
  // than guess which class wins.
  if (subjects.some((s) => s.kind !== kind)) return NO_MODES
  // Never drop onto a row being dragged.
  if (subjects.some((s) => s.id === target.id)) return NO_MODES

  if (kind === 'project') return target.kind === 'project' ? REORDER_MODES : NO_MODES

  if (kind === 'repo') {
    if (target.kind === 'repo') return REORDER_MODES
    if (target.kind === 'project') return INTO_MODES
    return NO_MODES
  }

  // workspace | folder
  if (target.kind === 'project') return NO_MODES
  if (subjects.some((s) => s.repoId !== target.repoId)) return NO_MODES

  const hasLocked = subjects.some((s) => s.locked)

  if (target.kind === 'repo') {
    // Landing at the repo root is a re-parent, which a protected branch refuses.
    return hasLocked ? NO_MODES : INTO_MODES
  }

  if (hasLocked) {
    const sameParent = subjects.every((s) => s.parentId === target.parentId)
    if (!sameParent) return NO_MODES
    return { before: true, after: !resolvesToFirstChild(target, 'after'), into: false }
  }

  return ALL_MODES
}

/**
 * The outer band of a row reorders; the middle nests.
 *
 * A folder gets the container band because nesting into one is cheap and
 * common. A workspace gets the heavy one because nesting under one re-parents a
 * fork, which is the heavier action and deserves a harder-to-hit target.
 */
export function edgeBandFor(kind: DropKind): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

/**
 * Turn a pointer position within a row into a drop mode, or null if this drag
 * may not do anything here.
 *
 * `ratio` is 0 at the row's top edge and 1 at its bottom.
 */
export function resolveDropMode(
  ratio: number,
  target: DropTarget,
  allowed: AllowedModes,
): DropMode | null {
  return dropModeAt(ratio, allowed, edgeBandFor(target.kind))
}

/**
 * The sidebar's half of the drag contract, as the shared hit test consumes it.
 *
 * Handing the core a policy object rather than letting it import this module is
 * what keeps the locked rule out of every other tree: the core can only ask
 * these two questions, and it asks them of whichever tree the pointer is over.
 */
export const SIDEBAR_DROP_POLICY: DropPolicy<DragSubject, DropTarget> = {
  allowedModes,
  edgeBandFor,
}
