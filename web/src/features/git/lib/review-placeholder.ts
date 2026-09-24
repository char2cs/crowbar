import { parsePatchFiles } from '@pierre/diffs'
import type { ChangeTypes, FileDiffMetadata, Hunk } from '@pierre/diffs'
import { isImagePath } from '@/features/editor/lib/asset-data-url'
import type { FileOutline, HunkShape } from '@/features/git/api/review-window-api'
import { PATCH_LINE_CAP } from '@/features/git/lib/patch-window'
import type { GitDiff } from '@/features/git/types/git-types'

/**
 * Pure layout for the windowed Branch Review surface (review-code-view.tsx):
 * file classification, the content-free placeholder a file occupies before its
 * patch arrives, and patch parsing.
 */

// ── File classification ─────────────────────────────────────────────

/** How a changed file is rendered: as a diff, as an image, or as a bare row. */
export type ReviewFileKind = 'diff' | 'image' | 'binary'

export interface ReviewFileEntry {
  path: string
  kind: ReviewFileKind
  /** The summary row — status flags and ± counts, never line content. */
  file: GitDiff
  /** Hunk geometry for the path, absent only if the outline disagrees with the summary. */
  outline?: FileOutline
}

/**
 * Classify every changed file, preserving the summary's order.
 *
 * Binary-ness comes from the OUTLINE (git emits no `@@` headers for a binary
 * file, so the outline reports it directly) rather than from the summary, which
 * only carries counts. Image-ness is then decided by extension: the daemon does
 * not distinguish a PNG from a `.bin`, and the difference is purely about which
 * component can show it.
 */
export function partitionReviewFiles(
  files: readonly GitDiff[],
  outline: readonly FileOutline[],
): ReviewFileEntry[] {
  const byPath = new Map<string, FileOutline>()
  for (const entry of outline) {
    if (!byPath.has(entry.path)) byPath.set(entry.path, entry)
  }

  return files.map((file) => {
    const path = file.file_path
    const shape = byPath.get(path)
    const binary = shape?.isBinary ?? file.is_binary ?? false
    if (!binary) return { path, kind: 'diff', file, outline: shape }
    const image = file.is_image ?? isImagePath(path)
    return { path, kind: image ? 'image' : 'binary', file, outline: shape }
  })
}

// ── Placeholder geometry ────────────────────────────────────────────

/**
 * Build the content-free `FileDiffMetadata` a file occupies before its patch
 * arrives.
 *
 * `computeEstimatedDiffHeights` — what CodeView sizes an item with — reads only
 * each hunk's `splitLineCount` / `unifiedLineCount`, never a line of text. So a
 * hunk here carries its rendered row counts and an EMPTY `hunkContent`: the
 * virtualiser reserves the right space and the renderer draws nothing until the
 * real patch replaces it.
 *
 * The row counts need the split between context and changed lines, which a
 * `@@` header does not give — only `oldLines` and `newLines`. The summary's ±
 * counts close that gap exactly: across a whole file, context = Σ oldLines −
 * deletions, and distributing that across hunks (bounded per hunk by
 * `min(oldLines, newLines)`, the most context a hunk can hold) recovers each
 * hunk's shape. Without usable ± counts the estimate falls back to zero context
 * — the upper bound, because over-reserving costs a scrollbar that shrinks
 * while under-reserving costs a scroll that jumps.
 *
 * `isPartial` is always true on a placeholder. It is the library's flag for
 * "these lines are only what the patch showed", which switches off the
 * expand-unchanged machinery that would otherwise index into the (empty) line
 * arrays.
 */
export function buildPlaceholderFileDiff(
  file: GitDiff,
  outline: FileOutline | undefined,
): FileDiffMetadata {
  const shapes = outline?.hunks ?? []
  const additions = countOf(file.additions)
  const deletions = countOf(file.deletions)
  const hunks = trimToPatchCap(buildPlaceholderHunks(shapes, distributeContext(shapes, deletions)))

  // A capped outline stops at the server's per-file hunk limit, so its geometry
  // is a LOWER bound on the file — sizing from it alone under-reserves by
  // however much the cap cut. Every changed line renders one unified row, so
  // the summary's ± counts are a floor the outline must be topped up to.
  const room = PATCH_LINE_CAP - sumBy(hunks, (h) => h.unifiedLineCount)
  if (outline?.isPartial && room > 0) {
    const missingAdditions = Math.max(0, additions - sumBy(hunks, (h) => h.additionLines))
    const missingDeletions = Math.max(0, deletions - sumBy(hunks, (h) => h.deletionLines))
    if (missingAdditions + missingDeletions > 0) {
      // The tail carries the TRUE missing counts and reserves only the room
      // left under the cap. Reserving the full amount is the over-reservation
      // that collapses on materialisation; reserving nothing makes the
      // scrollbar lie. See reserveAtMost for why both can be satisfied.
      hunks.push(reserveAtMost(buildTailHunk(hunks, missingAdditions, missingDeletions), room))
    }
  }

  const name = outline?.path ?? file.file_path
  const prevName = outline?.oldPath ?? file.old_path
  return {
    name,
    prevName: prevName != null && prevName !== name ? prevName : undefined,
    type: changeTypeOf(file),
    hunks,
    splitLineCount: sumBy(hunks, (h) => h.splitLineCount),
    unifiedLineCount: sumBy(hunks, (h) => h.unifiedLineCount),
    isPartial: true,
    deletionLines: [],
    additionLines: [],
  }
}

/** A summary count, or 0 when it is absent or the binary sentinel (-1). */
function countOf(value: number | undefined): number {
  return typeof value === 'number' && value > 0 ? value : 0
}

function sumBy<T>(items: readonly T[], of: (item: T) => number): number {
  let total = 0
  for (const item of items) total += of(item)
  return total
}

function changeTypeOf(file: GitDiff): ChangeTypes {
  if (file.is_new) return 'new'
  if (file.is_deleted) return 'deleted'
  if (file.is_renamed) return 'rename-changed'
  return 'change'
}

/**
 * Drop whole hunks past the point where the patch request stops delivering.
 *
 * A placeholder used to reserve height from the file's FULL ± counts — 420,000
 * rows for the fixture's monster — while the patch that replaces it is capped
 * at PATCH_LINE_CAP. Materialising then shrank that item by the difference,
 * every item below it jumped up, and the viewport ended up pointing somewhere
 * unrelated. Repeated during one fast scroll it scrambled the position↔content
 * mapping badly enough that scrolling back to the top still showed a file from
 * deep in the list.
 *
 * Measured per item rather than in total: the monster published a fileDiff of
 * 0 unified rows against a 420,000-row placeholder. The surface's total scroll
 * height is a poor witness here — the renderer paginates it at
 * SCROLL_REBASE_CONTAINER_HEIGHT (12,000,000px), so any branch past that
 * ceiling reports the ceiling whatever its items do.
 *
 * Reserving what will actually be DELIVERED removes the mismatch at its source.
 * This is the same correction the window budget needed: a file costs, and
 * occupies, what it holds — not what its diff contains.
 */
function trimToPatchCap(hunks: Hunk[]): Hunk[] {
  const kept: Hunk[] = []
  let unified = 0
  for (const hunk of hunks) {
    if (unified + hunk.unifiedLineCount > PATCH_LINE_CAP) break
    kept.push(hunk)
    unified += hunk.unifiedLineCount
  }
  if (kept.length > 0) return kept
  // Dropping is not available when the FIRST hunk already exceeds the cap: a
  // whole-file rewrite is a single hunk, so there would be nothing left to
  // reserve and the file would occupy no space at all. Scale it down instead.
  const first = hunks[0]
  return first != null ? [reserveAtMost(first, PATCH_LINE_CAP)] : []
}

/**
 * Cap the SPACE a hunk reserves without changing what it reports containing.
 *
 * Height is estimated from `unifiedLineCount`/`splitLineCount`; the file
 * header's ± label sums `additionLines`/`deletionLines`. Those are separate
 * fields, which is what lets a file whose patch is capped reserve only the rows
 * that will actually arrive while still telling the reader how much the file
 * really changed. Scaling the ± counts too — an earlier version of this — made
 * the fixture's 420,000-line monster announce itself as "+20000".
 *
 * A placeholder draws nothing (its `hunkContent` is empty), so the two sets of
 * numbers describing different things costs nothing until the real patch
 * replaces the whole record.
 */
function reserveAtMost(hunk: Hunk, room: number): Hunk {
  if (hunk.unifiedLineCount <= room || room <= 0) return hunk
  const scale = room / hunk.unifiedLineCount
  return {
    ...hunk,
    unifiedLineCount: room,
    splitLineCount: Math.max(1, Math.round(hunk.splitLineCount * scale)),
  }
}

/** Per-hunk context-line estimate; see `buildPlaceholderFileDiff`. */
function distributeContext(shapes: readonly HunkShape[], deletions: number): number[] {
  const caps = shapes.map((s) => Math.min(s.oldLines, s.newLines))
  const capTotal = sumBy(caps, (c) => c)
  if (capTotal === 0 || deletions === 0) return caps.map(() => 0)

  const oldTotal = sumBy(shapes, (s) => s.oldLines)
  const contextTotal = Math.min(Math.max(oldTotal - deletions, 0), capTotal)
  return caps.map((cap) => Math.round((contextTotal * cap) / capTotal))
}

function buildPlaceholderHunks(shapes: readonly HunkShape[], context: readonly number[]): Hunk[] {
  const hunks: Hunk[] = []
  let unifiedLineStart = 0
  let splitLineStart = 0
  let additionLineIndex = 0
  let deletionLineIndex = 0
  let previousOldEnd = 1

  for (let i = 0; i < shapes.length; i++) {
    const shape = shapes[i]
    const shared = Math.min(context[i] ?? 0, shape.oldLines, shape.newLines)
    const additionLines = shape.newLines - shared
    const deletionLines = shape.oldLines - shared

    hunks.push({
      collapsedBefore: Math.max(0, shape.oldStart - previousOldEnd),
      additionStart: shape.newStart,
      additionCount: shape.newLines,
      additionLines,
      additionLineIndex,
      deletionStart: shape.oldStart,
      deletionCount: shape.oldLines,
      deletionLines,
      deletionLineIndex,
      // Empty on purpose: this is the whole reason a placeholder is cheap. The
      // row counts below reserve the space; nothing draws until the patch lands.
      hunkContent: [],
      hunkSpecs: `@@ -${shape.oldStart},${shape.oldLines} +${shape.newStart},${shape.newLines} @@`,
      splitLineStart,
      splitLineCount: shared + Math.max(additionLines, deletionLines),
      unifiedLineStart,
      unifiedLineCount: shared + additionLines + deletionLines,
      noEOFCRDeletions: false,
      noEOFCRAdditions: false,
    })

    const last = hunks[hunks.length - 1]
    unifiedLineStart += last.unifiedLineCount
    splitLineStart += last.splitLineCount
    additionLineIndex += shape.newLines
    deletionLineIndex += shape.oldLines
    previousOldEnd = shape.oldStart + shape.oldLines
  }

  return hunks
}

/** The synthetic hunk standing in for everything a capped outline omitted. */
function buildTailHunk(hunks: readonly Hunk[], additions: number, deletions: number): Hunk {
  const previous = hunks[hunks.length - 1]
  return {
    collapsedBefore: 0,
    additionStart: previous != null ? previous.additionStart + previous.additionCount : 1,
    additionCount: additions,
    additionLines: additions,
    additionLineIndex: previous != null ? previous.additionLineIndex + previous.additionCount : 0,
    deletionStart: previous != null ? previous.deletionStart + previous.deletionCount : 1,
    deletionCount: deletions,
    deletionLines: deletions,
    deletionLineIndex: previous != null ? previous.deletionLineIndex + previous.deletionCount : 0,
    hunkContent: [],
    splitLineStart: previous != null ? previous.splitLineStart + previous.splitLineCount : 0,
    splitLineCount: Math.max(additions, deletions),
    unifiedLineStart: previous != null ? previous.unifiedLineStart + previous.unifiedLineCount : 0,
    unifiedLineCount: additions + deletions,
    noEOFCRDeletions: false,
    noEOFCRAdditions: false,
  }
}

// ── Patch parsing ───────────────────────────────────────────────────

/**
 * Identity for one parsed patch, as the renderer understands identity.
 *
 * `areDiffTargetsEqual` treats two different FileDiffMetadata objects as the
 * SAME target when their cacheKeys match, and a same-target update keeps the
 * cached estimated height rather than recomputing it. So the key has to change
 * whenever the CONTENT does, not merely whenever the file does.
 *
 * Keying on `wsId:path` alone broke "Show all" completely. Expanding a
 * truncated file refetches the same path uncapped, so the key was identical,
 * so the library kept the capped item's height: the 420,000 lines were
 * fetched, parsed and published — `getItem` reported all of them — and the
 * file still occupied exactly the 20,000 rows it had before, showing the same
 * truncated content. The one affordance for reading a large file whole did
 * nothing, silently, after a 13MB download.
 *
 * The patch length distinguishes them and costs nothing: identical text still
 * shares a key, which is what the highlighter cache wants.
 */
export function patchCacheKey(
  wsId: string,
  commit: string | undefined,
  path: string,
  patch: string,
): string {
  return `${wsId}:${commit ?? ''}:${path}:${patch.length}`
}

/** Parse one file's unified patch. A header-only body yields a hunkless file
 *  rather than nothing, which is what lets the truncation banner still render. */
export function parseSingleFilePatch(
  patch: string,
  cacheKey: string,
): FileDiffMetadata | undefined {
  try {
    for (const parsed of parsePatchFiles(patch, cacheKey)) {
      const file = parsed.files[0]
      if (file != null) return file
    }
  } catch {
    return undefined
  }
  return undefined
}

/** A short, stable identity for an ordered path list (FNV-1a). */
export function signatureOf(paths: readonly string[]): string {
  let hash = 0x811c9dc5
  for (const path of paths) {
    for (let i = 0; i < path.length; i++) {
      hash ^= path.charCodeAt(i)
      hash = Math.imul(hash, 0x01000193)
    }
    hash ^= 0x2f
    hash = Math.imul(hash, 0x01000193)
  }
  return `${paths.length}:${(hash >>> 0).toString(36)}`
}
