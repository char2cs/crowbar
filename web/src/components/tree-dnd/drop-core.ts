/**
 * The half of drag and drop that is the same in every tree.
 *
 * Two trees can disagree completely about what a drag MEANS and still agree
 * about everything here: where the bands sit inside a row, which of
 * before/after/into a pointer at a given height is asking for, and that the gap
 * under an expanded parent is its first child's slot rather than its own. That
 * arithmetic is what makes two lists feel like one gesture, so it is written
 * once and imported, never re-derived per tree.
 *
 * What is NOT here is any answer to "may this drag do that". The workspace
 * sidebar must never rewrite fork lineage — a protected branch owns a worktree
 * pinned to its parent — while the chats tree's whole purpose is to rewrite
 * parentage. Both rules are correct and they contradict each other, so neither
 * lives here: a tree hands in its own {@link DropPolicy} and the core only ever
 * learns that this row went to that parent at that index.
 *
 * The temptation is to grow one matrix with a flag per tree. Resist it. The
 * moment this file can name a row kind, one tree's refusal is one edit away
 * from becoming another tree's silent data loss.
 */

export type DropMode = 'before' | 'after' | 'into'

/** Which of the three a drag may do to one target. */
export type AllowedModes = Record<DropMode, boolean>

/**
 * The four answers a matrix ever gives, shared so two trees spell a refusal the
 * same way. A matrix returns one of these directly and callers only ever read
 * them — treat them as constants, because a write here rewrites the rule for
 * every row on the page.
 */
export const NO_MODES: AllowedModes = { before: false, after: false, into: false }
export const REORDER_MODES: AllowedModes = { before: true, after: true, into: false }
export const INTO_MODES: AllowedModes = { before: false, after: false, into: true }
export const ALL_MODES: AllowedModes = { before: true, after: true, into: true }

/**
 * What the core needs to know about a row a drag is carrying: which class of
 * thing it is, which one it is, and where it currently sits.
 *
 * A tree widens this with whatever its own matrix reads — the sidebar adds a
 * repo scope and a locked flag — and the core never looks at the additions.
 */
export interface DragSubjectBase {
  kind: string
  id: string
  /**
   * The container this row sits in. '' is the tree's own root; `null` and
   * `undefined` both mean "a tree that models root as null rather than
   * absence" — drop-core never reads this field itself, so it accepts
   * whichever a tree's own row type actually carries.
   */
  parentId?: string | null
}

/**
 * A row as a drop TARGET. The two extra facts are the ones the band maths
 * needs, because they change what a position in the row means rather than
 * whether it is allowed.
 */
export interface DropTargetBase extends DragSubjectBase {
  /** Whether this row is currently showing its children. */
  expanded?: boolean
  hasChildren?: boolean
}

/**
 * The reorder band on a row whose middle is plain containment — a folder, a
 * chat group. Narrow, because dropping something inside one is the cheap and
 * common move and should be the easy target.
 */
export const EDGE_BAND_CONTAINER = 0.2

/**
 * The reorder band on a row whose middle does something heavier than
 * containment. In the sidebar that is nesting under a workspace, which
 * re-parents a fork; a wider band makes the reorder easy and leaves the
 * expensive move harder to hit by accident.
 */
export const EDGE_BAND_HEAVY = 0.3

/**
 * Dropping *after* a row that is showing children does not put the row after
 * that subtree — the gap under an expanded parent is the slot before its first
 * child, which is where the indicator is drawn. So "after" an expanded parent
 * is a re-parent, and anything that may not re-parent may not do it.
 */
export function resolvesToFirstChild(target: DropTargetBase, mode: DropMode): boolean {
  return mode === 'after' && !!target.expanded && !!target.hasChildren
}

/** Whether a matrix left this drag anything at all to do here. */
export function anyModeAllowed(allowed: AllowedModes): boolean {
  return allowed.before || allowed.after || allowed.into
}

/**
 * Turn a pointer position within a row into a drop mode, or null if this drag
 * may not do anything here.
 *
 * `ratio` is 0 at the row's top edge and 1 at its bottom. `edgeBand` is the
 * share of the row at each end that reorders rather than nests, and it only
 * matters when nesting is on the table at all — the two degenerate cases below
 * are what keep a half-refused row from offering a dead zone in the middle.
 */
export function dropModeAt(
  ratio: number,
  allowed: AllowedModes,
  edgeBand: number,
): DropMode | null {
  let mode: DropMode
  if (!allowed.into) {
    // Nothing to nest into, so the row is a straight 50/50 split.
    mode = ratio < 0.5 ? 'before' : 'after'
  } else if (!allowed.before && !allowed.after) {
    mode = 'into'
  } else {
    if (ratio < edgeBand) mode = 'before'
    else if (ratio > 1 - edgeBand) mode = 'after'
    else mode = 'into'
  }
  return allowed[mode] ? mode : null
}

/**
 * Everything one tree has to say about its own drags.
 *
 * `allowedModes` is the matrix — the whole of a tree's policy, including every
 * refusal it owes its domain. `edgeBandFor` is the feel: how much of each row
 * kind reorders rather than nests, given from {@link EDGE_BAND_CONTAINER} and
 * {@link EDGE_BAND_HEAVY} so two trees do not drift into different geometry.
 */
export interface DropPolicy<
  S extends DragSubjectBase = DragSubjectBase,
  T extends DropTargetBase = DropTargetBase,
> {
  allowedModes(subjects: readonly S[], target: T): AllowedModes
  edgeBandFor(kind: string): number
}

/** A row's box, as much of it as a hit test and an indicator need. */
export interface RowRect {
  top: number
  bottom: number
  left: number
  width: number
}

/**
 * The rows a drag carries.
 *
 * Grabbing a row that is part of the selection moves the whole selection;
 * grabbing one outside it moves that row alone and leaves the selection be —
 * pressing on an unselected row is not a way to extend a selection.
 */
export function dragSubjectsFor<S extends DragSubjectBase>(
  grabbed: S,
  selection: readonly S[],
): S[] {
  return selection.some((s) => s.id === grabbed.id) ? [...selection] : [grabbed]
}

/** Whether two resolved drops would draw and commit the same thing. */
export function sameDrop<R extends { kind: string; id: string; mode: DropMode }>(
  a: R | null,
  b: R | null,
): boolean {
  if (a === null || b === null) return a === b
  return a.id === b.id && a.kind === b.kind && a.mode === b.mode
}
