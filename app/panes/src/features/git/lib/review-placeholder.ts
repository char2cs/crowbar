import { parsePatchFiles } from '@pierre/diffs'
import type { ChangeTypes, FileDiffMetadata, Hunk } from '@pierre/diffs'
import { isImagePath } from '@/features/editor/lib/asset-data-url'
import type { FileOutline, HunkShape } from '@/features/git/api/review-window-api'
import { PATCH_LINE_CAP } from '@/features/git/lib/patch-window'
import type { GitDiff } from '@/features/git/types/git-types'

type ReviewFileKind = 'diff' | 'image' | 'binary'

export interface ReviewFileEntry {
  path: string
  kind: ReviewFileKind
  file: GitDiff
  outline?: FileOutline
}

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

export function buildPlaceholderFileDiff(
  file: GitDiff,
  outline: FileOutline | undefined,
): FileDiffMetadata {
  const shapes = outline?.hunks ?? []
  const additions = countOf(file.additions)
  const deletions = countOf(file.deletions)
  const hunks = trimToPatchCap(
    buildPlaceholderHunks(shapes, distributeContext(shapes, file.deletions)),
  )

  const room = PATCH_LINE_CAP - sumBy(hunks, (h) => h.unifiedLineCount)
  if (outline?.isPartial && room > 0) {
    const missingAdditions = Math.max(0, additions - sumBy(hunks, (h) => h.additionLines))
    const missingDeletions = Math.max(0, deletions - sumBy(hunks, (h) => h.deletionLines))
    if (missingAdditions + missingDeletions > 0) {
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

function trimToPatchCap(hunks: Hunk[]): Hunk[] {
  const kept: Hunk[] = []
  let unified = 0
  for (const hunk of hunks) {
    if (unified + hunk.unifiedLineCount > PATCH_LINE_CAP) break
    kept.push(hunk)
    unified += hunk.unifiedLineCount
  }
  if (kept.length > 0) return kept
  const first = hunks[0]
  return first != null ? [reserveAtMost(first, PATCH_LINE_CAP)] : []
}

function reserveAtMost(hunk: Hunk, room: number): Hunk {
  if (hunk.unifiedLineCount <= room || room <= 0) return hunk
  const scale = room / hunk.unifiedLineCount
  return {
    ...hunk,
    unifiedLineCount: room,
    splitLineCount: Math.max(1, Math.round(hunk.splitLineCount * scale)),
  }
}

function distributeContext(shapes: readonly HunkShape[], deletions = -1): number[] {
  const caps = shapes.map((s) => Math.min(s.oldLines, s.newLines))
  const capTotal = sumBy(caps, (c) => c)
  if (capTotal === 0 || deletions < 0) return caps.map(() => 0)

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

export function patchCacheKey(
  wsId: string,
  commit: string | undefined,
  path: string,
  patch: string,
): string {
  return `${wsId}:${commit ?? ''}:${path}:${patch.length}`
}

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
