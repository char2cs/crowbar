import type { ChangeData, HunkData } from 'react-diff-view'
import type { GitDiff } from '@/features/git/types/git-types'

/**
 * Maps a GitDiff (backend model) to the HunkData[] format expected by react-diff-view.
 *
 * Content-marker decision: GitDiffLine.content is already stripped of the leading
 * +/-/space marker by the parser (git-diff-parser.ts line.substring(1)), matching
 * what gitdiff-parser itself does (line.slice(1)). We pass content through as-is.
 */
export function gitDiffToHunks(diff: GitDiff): HunkData[] {
  const hunks: HunkData[] = []
  let currentChanges: ChangeData[] = []
  let hunkContent: string | null = null

  const flushHunk = () => {
    if (hunkContent === null && currentChanges.length === 0) return

    const changes = currentChanges
    const content = hunkContent ?? synthesizeHunkHeader(changes)

    // Compute hunk stats from changes
    let oldStart = 0
    let newStart = 0
    let oldLines = 0
    let newLines = 0

    for (const change of changes) {
      if (change.type === 'delete') {
        if (oldStart === 0) oldStart = change.lineNumber
        oldLines++
      } else if (change.type === 'insert') {
        if (newStart === 0) newStart = change.lineNumber
        newLines++
      } else {
        // normal
        if (oldStart === 0) oldStart = change.oldLineNumber
        if (newStart === 0) newStart = change.newLineNumber
        oldLines++
        newLines++
      }
    }

    // Fall back to 1 if we had no lines on a side (e.g. pure add/delete)
    if (oldStart === 0) oldStart = 1
    if (newStart === 0) newStart = 1

    hunks.push({ content, oldStart, newStart, oldLines, newLines, changes })
    currentChanges = []
    hunkContent = null
  }

  for (const line of diff.lines) {
    if (line.line_type === 'header') {
      flushHunk()
      hunkContent = line.content
      continue
    }

    if (line.line_type === 'added') {
      const change: ChangeData = {
        type: 'insert',
        content: line.content,
        lineNumber: line.new_line_number ?? 1,
        isInsert: true,
      }
      currentChanges.push(change)
    } else if (line.line_type === 'removed') {
      const change: ChangeData = {
        type: 'delete',
        content: line.content,
        lineNumber: line.old_line_number ?? 1,
        isDelete: true,
      }
      currentChanges.push(change)
    } else if (line.line_type === 'context') {
      const change: ChangeData = {
        type: 'normal',
        content: line.content,
        oldLineNumber: line.old_line_number ?? 1,
        newLineNumber: line.new_line_number ?? 1,
        isNormal: true,
      }
      currentChanges.push(change)
    }
    // 'header' is handled above; any unknown type is silently skipped
  }

  // Flush the last hunk (or synthesize a single hunk for files with no header lines)
  if (currentChanges.length > 0 || (hunkContent !== null && currentChanges.length === 0)) {
    flushHunk()
  }

  return hunks
}

function synthesizeHunkHeader(changes: ChangeData[]): string {
  let oldStart = 0
  let newStart = 0
  let oldLines = 0
  let newLines = 0

  for (const change of changes) {
    if (change.type === 'delete') {
      if (oldStart === 0) oldStart = change.lineNumber
      oldLines++
    } else if (change.type === 'insert') {
      if (newStart === 0) newStart = change.lineNumber
      newLines++
    } else {
      if (oldStart === 0) oldStart = change.oldLineNumber
      if (newStart === 0) newStart = change.newLineNumber
      oldLines++
      newLines++
    }
  }

  if (oldStart === 0) oldStart = 1
  if (newStart === 0) newStart = 1

  return `@@ -${oldStart},${oldLines} +${newStart},${newLines} @@`
}
