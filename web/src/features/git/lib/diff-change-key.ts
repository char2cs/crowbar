/**
 * Utilities for mapping between react-diff-view change objects and thread anchors.
 *
 * getChangeKey() key format (from react-diff-view):
 *   Insert → "I{lineNumber}"
 *   Delete → "D{lineNumber}"
 *   Normal → "N{oldLineNumber}"
 */
import { getChangeKey, isInsert, isDelete, isNormal } from 'react-diff-view'
import type { ChangeData, HunkData } from 'react-diff-view'

export interface ThreadAnchor {
  side: 'old' | 'new'
  line: number
}

/**
 * Maps a ChangeData to a {side, line} anchor for widget lookup.
 *
 * - Insert: side='new', line=lineNumber
 * - Delete: side='old', line=lineNumber
 * - Normal, side='old': line=oldLineNumber
 * - Normal, side='new': line=newLineNumber
 *
 * For widget mapping we need a single canonical anchor per change. Since Normal
 * changes appear on both sides, we return side='old' as the canonical form.
 * Callers that need both sides should compute the change key directly.
 */
export function changeKeyToAnchor(change: ChangeData): ThreadAnchor {
  if (isInsert(change)) {
    return { side: 'new', line: change.lineNumber }
  }
  if (isDelete(change)) {
    return { side: 'old', line: change.lineNumber }
  }
  // Normal change — canonical side is 'old'
  if (isNormal(change)) {
    return { side: 'old', line: change.oldLineNumber }
  }
  // Fallback (should not happen with well-formed data)
  return { side: 'old', line: 0 }
}

/**
 * Returns true when the given ChangeData matches the anchor.
 *
 * - Insert: side must be 'new', lineNumber must match
 * - Delete: side must be 'old', lineNumber must match
 * - Normal: side 'old' → oldLineNumber must match; side 'new' → newLineNumber must match
 */
function changeMatchesAnchor(change: ChangeData, anchor: ThreadAnchor): boolean {
  if (isInsert(change)) {
    return anchor.side === 'new' && change.lineNumber === anchor.line
  }
  if (isDelete(change)) {
    return anchor.side === 'old' && change.lineNumber === anchor.line
  }
  if (isNormal(change)) {
    if (anchor.side === 'old') return change.oldLineNumber === anchor.line
    return change.newLineNumber === anchor.line
  }
  return false
}

/**
 * Given a list of hunks and a thread anchor, returns the react-diff-view change key
 * (as returned by getChangeKey) that matches the anchor, or null if not found.
 *
 * This is used to build the `widgets` Record<string, ReactNode> for <Diff>.
 */
export function anchorToChangeKey(hunks: HunkData[], anchor: ThreadAnchor): string | null {
  for (const hunk of hunks) {
    for (const change of hunk.changes) {
      if (changeMatchesAnchor(change, anchor)) {
        return getChangeKey(change)
      }
    }
  }
  return null
}
