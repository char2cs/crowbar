import { parsePatchFiles } from '@pierre/diffs'
import type { ChangeTypes, FileDiffMetadata, Hunk } from '@pierre/diffs'
import { isImagePath } from '@/features/editor/lib/asset-data-url'
import type { FileOutline, HunkShape } from '@/features/git/api/review-window-api'
import { PATCH_LINE_CAP } from '@/features/git/lib/patch-window'
import type { GitDiff } from '@/features/git/types/git-types'

// Pure layout for the windowed Branch Review: file classification, the content-free placeholder
// a file occupies before its patch arrives, and patch parsing.

// ── File classification ─────────────────────────────────────────────

/** How a changed file is rendered: as a diff, as an image, or as a bare row. */
type ReviewFileKind = 'diff' | 'image' | 'binary'

export interface ReviewFileEntry {
  path: string
  kind: ReviewFileKind
  /** The summary row — status flags and ± counts, never line content. */
  file: GitDiff
  /** Hunk geometry for the path, absent only if the outline disagrees with the summary. */
  outline?: FileOutline
}

/** Classifies files in summary order. Binary-ness comes from the outline (git emits no `@@` for a
 *  binary); image-ness from the extension, since the daemon doesn't tell a PNG from a `.bin`. */
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

// CodeView sizes items from hunk row counts alone, so hunks carry counts and empty content. Context
// per hunk = (Σ oldLines − deletions) spread under min(old, new); no ± counts → zero context, an
// over-reserve (jumps are worse). isPartial stops expand-unchanged indexing the empty line arrays.
export function buildPlaceholderFileDiff(
  file: GitDiff,
  outline: FileOutline | undefined,
): FileDiffMetadata {
  const shapes = outline?.hunks ?? []
  const additions = countOf(file.additions)
  const deletions = countOf(file.deletions)
  const known = typeof file.deletions === 'number' && file.deletions >= 0
  const context = distributeContext(shapes, known ? deletions : undefined)
  const hunks = trimToPatchCap(buildPlaceholderHunks(shapes, context))

  // A capped outline is a lower bound on the file; every changed line is one unified row, so
  // the summary's ± counts are the floor it is topped up to.
  const room = PATCH_LINE_CAP - sumBy(hunks, (h) => h.unifiedLineCount)
  if (outline?.isPartial && room > 0) {
    const missingAdditions = Math.max(0, additions - sumBy(hunks, (h) => h.additionLines))
    const missingDeletions = Math.max(0, deletions - sumBy(hunks, (h) => h.deletionLines))
    if (missingAdditions + missingDeletions > 0) {
      // The tail reports the true missing counts but reserves only the room under the cap
      // (reserveAtMost): more collapses on materialisation, none makes the scrollbar lie.
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

// Reserve only what the capped patch request will deliver: a placeholder taller than its patch
// shrinks on materialising and shifts everything below it, scrambling the scroll position.
function trimToPatchCap(hunks: Hunk[]): Hunk[] {
  const kept: Hunk[] = []
  let unified = 0
  for (const hunk of hunks) {
    if (unified + hunk.unifiedLineCount > PATCH_LINE_CAP) break
    kept.push(hunk)
    unified += hunk.unifiedLineCount
  }
  if (kept.length > 0) return kept
  // A first hunk over the cap (a whole-file rewrite) can't be dropped or the file occupies
  // nothing; scale it down instead.
  const first = hunks[0]
  return first != null ? [reserveAtMost(first, PATCH_LINE_CAP)] : []
}

// Caps the space a hunk reserves (row counts) but not its ± (addition/deletion lines), so a
// capped file still shows how much it really changed. A placeholder draws nothing, so the two
// disagreeing costs nothing until the real patch replaces it.
function reserveAtMost(hunk: Hunk, room: number): Hunk {
  if (hunk.unifiedLineCount <= room || room <= 0) return hunk
  const scale = room / hunk.unifiedLineCount
  return {
    ...hunk,
    unifiedLineCount: room,
    splitLineCount: Math.max(1, Math.round(hunk.splitLineCount * scale)),
  }
}

/** Per-hunk context-line estimate; see `buildPlaceholderFileDiff`. Zero deletions is a count (a
 *  pure addition, all old lines context); only an unknown count leaves the context at zero. */
function distributeContext(shapes: readonly HunkShape[], deletions: number | undefined): number[] {
  const caps = shapes.map((s) => Math.min(s.oldLines, s.newLines))
  const capTotal = sumBy(caps, (c) => c)
  if (capTotal === 0 || deletions === undefined) return caps.map(() => 0)

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
      // Empty on purpose: the row counts below reserve the space; nothing draws until the patch.
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

/** Matching cacheKeys make the renderer keep the cached height, so the key must change with the
 *  content: the length tells a "Show all" refetch from its capped patch, while identical text
 *  still shares a key for the highlighter cache. */
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
