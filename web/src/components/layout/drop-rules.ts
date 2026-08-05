/**
 * What a drag may do to a row, and where the indicator goes.
 *
 * Kept as pure functions with no DOM and no React so the whole matrix — every
 * permitted move and, more importantly, every refusal — is testable without a
 * synthetic pointer. `workspace-tree-context.tsx` hit-tests the pointer and
 * then asks these two questions; it decides nothing itself.
 */

/** The four movable classes. They do not mix. */
export type DropKind = 'workspace' | 'folder' | 'repo' | 'project'

export type DropMode = 'before' | 'after' | 'into'

export type AllowedModes = Record<DropMode, boolean>

export interface DragSubject {
  kind: DropKind
  id: string
  /** Repo scope, for the same-repo rule. Absent on repos and projects. */
  repoId?: string
  /** A protected branch: reorders among its own siblings and nothing else. */
  locked?: boolean
  /** Its current parent, for the locked same-parent rule. */
  parentId?: string
}

export interface DropTarget {
  kind: DropKind
  id: string
  repoId?: string
  parentId?: string
  /** Whether this row is currently showing its children. */
  expanded?: boolean
  hasChildren?: boolean
}

const NONE: AllowedModes = { before: false, after: false, into: false }
const REORDER_ONLY: AllowedModes = { before: true, after: true, into: false }
const INTO_ONLY: AllowedModes = { before: false, after: false, into: true }
const ANY: AllowedModes = { before: true, after: true, into: true }

/**
 * Dropping *after* a row that is showing children does not put the row after
 * that subtree — the gap under an expanded parent is the slot before its first
 * child, which is where the indicator is drawn. So "after" an expanded parent
 * is a re-parent, and anything that may not re-parent may not do it.
 */
export function resolvesToFirstChild(target: DropTarget, mode: DropMode): boolean {
  return mode === 'after' && !!target.expanded && !!target.hasChildren
}

/**
 * Which of before/after/into this drag may do to this target.
 *
 * The rule that earns its keep is the locked one. A protected branch owns a
 * worktree pinned to its parent, so it may be reordered among its own siblings
 * and nothing else — including not "after" an expanded sibling, because that
 * would nest it.
 */
export function allowedModes(subjects: DragSubject[], target: DropTarget): AllowedModes {
  if (subjects.length === 0) return NONE
  const kind = subjects[0].kind
  // A mixed selection is not a thing the sidebar can express; refuse rather
  // than guess which class wins.
  if (subjects.some((s) => s.kind !== kind)) return NONE
  // Never drop onto a row being dragged.
  if (subjects.some((s) => s.id === target.id)) return NONE

  if (kind === 'project') return target.kind === 'project' ? REORDER_ONLY : NONE

  if (kind === 'repo') {
    if (target.kind === 'repo') return REORDER_ONLY
    if (target.kind === 'project') return INTO_ONLY
    return NONE
  }

  // workspace | folder
  if (target.kind === 'project') return NONE
  if (subjects.some((s) => s.repoId !== target.repoId)) return NONE

  const hasLocked = subjects.some((s) => s.locked)

  if (target.kind === 'repo') {
    // Landing at the repo root is a re-parent, which a protected branch refuses.
    return hasLocked ? NONE : INTO_ONLY
  }

  if (hasLocked) {
    const sameParent = subjects.every((s) => s.parentId === target.parentId)
    if (!sameParent) return NONE
    return { before: true, after: !resolvesToFirstChild(target, 'after'), into: false }
  }

  return ANY
}

/**
 * The outer band of a row reorders; the middle nests.
 *
 * A folder gets a 20% band because nesting into one is cheap and common. A
 * workspace gets 30% because nesting under one re-parents a fork, which is the
 * heavier action and deserves a harder-to-hit target.
 */
export function edgeBandFor(kind: DropKind): number {
  return kind === 'folder' ? 0.2 : 0.3
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
  let mode: DropMode
  if (!allowed.into) {
    // Nothing to nest into, so the row is a straight 50/50 split.
    mode = ratio < 0.5 ? 'before' : 'after'
  } else if (!allowed.before && !allowed.after) {
    mode = 'into'
  } else {
    const edge = edgeBandFor(target.kind)
    if (ratio < edge) mode = 'before'
    else if (ratio > 1 - edge) mode = 'after'
    else mode = 'into'
  }
  return allowed[mode] ? mode : null
}
