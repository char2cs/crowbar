import { getReviewPatch } from '@/features/git/api/review-window-api'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

/** Lines shown above the anchored line, as on a GitHub review thread. */
const CONTEXT_LINES = 3
/** Ceiling for a multi-line range, so a long selection never becomes a diff. */
const MAX_SNIPPET_LINES = 12

interface Row {
  mark: ' ' | '+' | '-'
  text: string
  oldNo: number | null
  newNo: number | null
  /** The old line a row sits before, for a hunk header that starts on an addition. */
  oldCursor: number
  /** A following `\ No newline at end of file` marker belongs to this row. */
  trailer?: string
}

const HUNK_HEADER = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/

function rowsOf(hunkBody: readonly string[], oldStart: number, newStart: number): Row[] {
  const rows: Row[] = []
  let oldNo = oldStart
  let newNo = newStart
  for (const line of hunkBody) {
    const mark = line[0]
    if (mark === '\\') {
      const last = rows[rows.length - 1]
      if (last) last.trailer = line
      continue
    }
    if (mark !== ' ' && mark !== '+' && mark !== '-') continue
    rows.push({
      mark,
      text: line.slice(1),
      oldNo: mark === '+' ? null : oldNo,
      newNo: mark === '-' ? null : newNo,
      oldCursor: oldNo,
    })
    if (mark !== '+') oldNo++
    if (mark !== '-') newNo++
  }
  return rows
}

function indexOfLine(rows: readonly Row[], side: ReviewThread['side'], line: number): number {
  return rows.findIndex((r) => (side === 'old' ? r.oldNo : r.newNo) === line)
}

/**
 * The few lines of one file's unified patch around a thread's anchor, as a
 * patch of its own, or null when the anchor is not in the patch (outdated, or
 * past the server's per-file line cap).
 */
export function sliceThreadPatch(
  patch: string,
  thread: Pick<ReviewThread, 'filePath' | 'side' | 'lineNumber' | 'startLine'>,
): string | null {
  const lines = patch.split('\n')
  const firstHunk = lines.findIndex((l) => HUNK_HEADER.test(l))
  if (firstHunk < 0) return null
  const header = lines.slice(0, firstHunk)

  let start = firstHunk
  while (start < lines.length) {
    let end = start + 1
    while (end < lines.length && !HUNK_HEADER.test(lines[end])) end++
    const match = HUNK_HEADER.exec(lines[start])
    const rows = match
      ? rowsOf(lines.slice(start + 1, end), Number(match[1]), Number(match[2]))
      : []
    const anchor = indexOfLine(rows, thread.side, thread.lineNumber)
    if (anchor >= 0) return buildPatch(header, thread, rows, anchor)
    start = end
  }
  return null
}

function buildPatch(
  header: readonly string[],
  thread: Pick<ReviewThread, 'filePath' | 'side' | 'startLine'>,
  rows: readonly Row[],
  anchor: number,
): string {
  const startIdx = indexOfLine(rows, thread.side, thread.startLine)
  const top = startIdx >= 0 ? Math.min(startIdx, anchor) : anchor
  const from = Math.max(0, Math.max(top - CONTEXT_LINES, anchor + 1 - MAX_SNIPPET_LINES))
  const slice = rows.slice(from, anchor + 1)

  const oldRows = slice.filter((r) => r.mark !== '+')
  const newRows = slice.filter((r) => r.mark !== '-')
  const oldStart = oldRows[0]?.oldNo ?? slice[0].oldCursor - 1
  const newStart = newRows[0]?.newNo ?? 0
  const head =
    header.length > 0
      ? [...header]
      : [
          `diff --git a/${thread.filePath} b/${thread.filePath}`,
          `--- a/${thread.filePath}`,
          `+++ b/${thread.filePath}`,
        ]
  const body = slice.flatMap((r) => (r.trailer ? [r.mark + r.text, r.trailer] : [r.mark + r.text]))
  return [
    ...head,
    `@@ -${oldStart},${oldRows.length} +${newStart},${newRows.length} @@`,
    ...body,
    '',
  ].join('\n')
}

// Threads on one file share a single request while it is in flight.
const inflight = new Map<string, Promise<string>>()

/** One file's patch through the review diff's own per-file read. */
export function fetchFilePatch(wsId: string, path: string): Promise<string> {
  const key = `${wsId}\u0000${path}`
  const pending = inflight.get(key)
  if (pending) return pending
  const request = getReviewPatch({ wsId }, path)
    .then((res) => res.patch)
    .finally(() => inflight.delete(key))
  inflight.set(key, request)
  return request
}
