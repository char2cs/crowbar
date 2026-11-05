// The tree-agnostic half of drag and drop: band geometry, pointer height → before/after/into,
// and "after an expanded parent is its first child's slot". Never "may this drag do that": each
// tree passes its own {@link DropPolicy}, so one tree's refusal can't become another's data loss.

export type DropMode = 'before' | 'after' | 'into'

/** Which of the three a drag may do to one target. */
export type AllowedModes = Record<DropMode, boolean>

/** The four answers a matrix gives, shared so trees spell refusals alike. Treat as constants:
 *  a write here rewrites the rule for every row on the page. */
export const NO_MODES: AllowedModes = { before: false, after: false, into: false }
export const REORDER_MODES: AllowedModes = { before: true, after: true, into: false }
export const INTO_MODES: AllowedModes = { before: false, after: false, into: true }
export const ALL_MODES: AllowedModes = { before: true, after: true, into: true }

/** A carried row's class, id and position. Trees widen it for their own matrix (the sidebar
 *  adds repo scope and a locked flag); the core never reads the additions. */
export interface DragSubjectBase {
  kind: string
  id: string
  /** The row's container: '' is the root, and `null`/`undefined` are accepted for trees that model
   *  root that way (drop-core never reads this field). */
  parentId?: string | null
}

/** A row as a drop TARGET; the extra facts change what a position in the row means. */
export interface DropTargetBase extends DragSubjectBase {
  /** Whether this row is currently showing its children. */
  expanded?: boolean
  hasChildren?: boolean
}

/** Reorder band for a plain container (folder, chat group): narrow, since nesting into one is the
 *  cheap, common move. */
export const EDGE_BAND_CONTAINER = 0.2

/** Reorder band for a row whose middle does something heavier (nesting re-parents a fork): wider,
 *  so the expensive move is harder to hit by accident. */
export const EDGE_BAND_HEAVY = 0.3

/** "After" an expanded parent lands before its first child (where the indicator draws), so it is a
 *  re-parent, and anything that may not re-parent may not do it. */
export function resolvesToFirstChild(target: DropTargetBase, mode: DropMode): boolean {
  return mode === 'after' && !!target.expanded && !!target.hasChildren
}

/** Whether a matrix left this drag anything at all to do here. */
export function anyModeAllowed(allowed: AllowedModes): boolean {
  return allowed.before || allowed.after || allowed.into
}

// Pointer position in a row → drop mode, or null. `ratio` runs 0 (top) to 1 (bottom); `edgeBand`
// is each end's reorder share and matters only when nesting is allowed. The degenerate cases
// below keep a half-refused row from having a dead middle.
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

/** A tree's whole drag policy: `allowedModes` is the matrix, `edgeBandFor` the per-kind feel
 *  (from the shared EDGE_BAND_* values so trees don't drift into different geometry). */
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

/** The rows a drag carries: grabbing a selected row moves the selection; grabbing another moves
 *  it alone and leaves the selection be. */
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
