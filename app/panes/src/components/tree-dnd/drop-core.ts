export type DropMode = 'before' | 'after' | 'into'

export type AllowedModes = Record<DropMode, boolean>

export const NO_MODES: AllowedModes = { before: false, after: false, into: false }
export const REORDER_MODES: AllowedModes = { before: true, after: true, into: false }
export const ALL_MODES: AllowedModes = { before: true, after: true, into: true }

export interface DragSubjectBase {
  kind: string
  id: string
  parentId?: string | null
}

export interface DropTargetBase extends DragSubjectBase {
  expanded?: boolean
  hasChildren?: boolean
}

export const EDGE_BAND_CONTAINER = 0.2

export const EDGE_BAND_HEAVY = 0.3

export function resolvesToFirstChild(target: DropTargetBase, mode: DropMode): boolean {
  return mode === 'after' && !!target.expanded && !!target.hasChildren
}

export function anyModeAllowed(allowed: AllowedModes): boolean {
  return allowed.before || allowed.after || allowed.into
}

export function dropModeAt(
  ratio: number,
  allowed: AllowedModes,
  edgeBand: number,
): DropMode | null {
  let mode: DropMode
  if (!allowed.into) {
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

export interface DropPolicy<
  S extends DragSubjectBase = DragSubjectBase,
  T extends DropTargetBase = DropTargetBase,
> {
  allowedModes(subjects: readonly S[], target: T): AllowedModes
  edgeBandFor(kind: string): number
}

export interface RowRect {
  top: number
  bottom: number
  left: number
  width: number
}

export function sameDrop<R extends { kind: string; id: string; mode: DropMode }>(
  a: R | null,
  b: R | null,
): boolean {
  if (a === null || b === null) return a === b
  return a.id === b.id && a.kind === b.kind && a.mode === b.mode
}
