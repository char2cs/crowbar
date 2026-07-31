import type { DropRow } from './drop-target-dom'

/**
 * What a key press means to the sidebar tree.
 *
 * The rows have declared `role="treeitem"` inside a `role="tree"` since the
 * refactor that stopped them being buttons, and that role is a promise: arrow
 * keys, type-to-jump, Enter. This is the half of keeping it that can be decided
 * without touching the DOM — the rows come in as facts, an action goes out, and
 * the whole matrix is testable without focus, layout or a real keyboard.
 */

export type TreeKeyAction =
  /** Move focus (and the single tab stop) onto this row. */
  | { kind: 'move'; id: string }
  /** Fold this row away; it is showing children. */
  | { kind: 'collapse'; id: string }
  /** Open this row; it has children and is closed. */
  | { kind: 'expand'; id: string }
  /** Do what a click on this row does. */
  | { kind: 'activate'; id: string }
  /** Drop the multiselection. */
  | { kind: 'clear-selection' }
  /** Send the selection (or the focused row) to the removal tray. */
  | { kind: 'remove' }

/**
 * Whether a key is a type-to-jump character rather than a command.
 *
 * One printable character and no modifier: a chord is somebody's shortcut, and
 * the space bar is the row's own activation, not the start of a label.
 */
export function isTypeaheadKey(key: string, modified: boolean): boolean {
  return !modified && key.length === 1 && key !== ' '
}

/**
 * The row a tree row sits under.
 *
 * A row at a repo's root publishes an empty container, and what it is really
 * under is the repo header — which is a row in this same tree, so Left from a
 * root workspace lands on its repo and Left again on its project. That
 * uniformity is why the coarser rows share the navigation order rather than
 * having one of their own.
 */
function containerOf(row: DropRow): string {
  return (
    row.parentId || (row.kind === 'workspace' || row.kind === 'folder' ? (row.repoId ?? '') : '')
  )
}

/** The first row drawn under `row`, or undefined when it is drawing none. */
function firstChildOf(rows: readonly DropRow[], index: number): DropRow | undefined {
  const parentId = rows[index].id
  for (let i = index + 1; i < rows.length; i++) {
    if (containerOf(rows[i]) === parentId) return rows[i]
  }
  return undefined
}

/**
 * The next row whose label starts with `prefix`, searching from after the
 * focused one and wrapping — so holding a letter walks the rows that share it
 * rather than sticking on the first.
 */
function matchLabel(
  rows: readonly DropRow[],
  from: number,
  prefix: string,
  /** True while the user is still adding to one word, which must not advance. */
  extending: boolean,
): DropRow | undefined {
  const wanted = prefix.toLowerCase()
  const start = extending ? from : from + 1
  for (let step = 0; step < rows.length; step++) {
    const row = rows[(start + step + rows.length) % rows.length]
    if (row.label?.toLowerCase().startsWith(wanted)) return row
  }
  return undefined
}

export interface TreeKeyInput {
  key: string
  /** Any of cmd/ctrl/alt — a chord is never navigation. */
  modified: boolean
  rows: readonly DropRow[]
  /** The row that has focus, or null when the tree has not been entered yet. */
  focusedId: string | null
  /** The type-to-jump buffer INCLUDING this key, or '' for a command key. */
  prefix: string
  /** True when `prefix` continues a word already being typed. */
  extendingPrefix: boolean
}

/**
 * Resolve one key press against the rows as they are drawn.
 *
 * Returns null for anything the tree does not claim, so an unhandled key falls
 * through to whatever else wants it rather than being swallowed by the sidebar.
 */
export function resolveTreeKey({
  key,
  modified,
  rows,
  focusedId,
  prefix,
  extendingPrefix,
}: TreeKeyInput): TreeKeyAction | null {
  if (rows.length === 0) return null
  const index = focusedId === null ? -1 : rows.findIndex((r) => r.id === focusedId)
  const row = index === -1 ? undefined : rows[index]

  if (isTypeaheadKey(key, modified)) {
    const hit = matchLabel(rows, index === -1 ? -1 : index, prefix, extendingPrefix)
    return hit ? { kind: 'move', id: hit.id } : null
  }
  if (modified) return null

  switch (key) {
    case 'ArrowDown':
      return { kind: 'move', id: rows[Math.min(index + 1, rows.length - 1)].id }
    case 'ArrowUp':
      return { kind: 'move', id: rows[Math.max(index - 1, 0)].id }
    case 'Home':
      return { kind: 'move', id: rows[0].id }
    case 'End':
      return { kind: 'move', id: rows[rows.length - 1].id }
    case 'ArrowRight': {
      if (!row) return { kind: 'move', id: rows[0].id }
      if (row.hasChildren && !row.expanded) return { kind: 'expand', id: row.id }
      const child = firstChildOf(rows, index)
      return child ? { kind: 'move', id: child.id } : null
    }
    case 'ArrowLeft': {
      if (!row) return { kind: 'move', id: rows[0].id }
      if (row.hasChildren && row.expanded) return { kind: 'collapse', id: row.id }
      const parentId = containerOf(row)
      if (!parentId || parentId === row.id) return null
      return rows.some((r) => r.id === parentId) ? { kind: 'move', id: parentId } : null
    }
    case 'Enter':
      return row ? { kind: 'activate', id: row.id } : null
    case 'Escape':
      return { kind: 'clear-selection' }
    case 'Delete':
    case 'Backspace':
      return { kind: 'remove' }
    default:
      return null
  }
}
