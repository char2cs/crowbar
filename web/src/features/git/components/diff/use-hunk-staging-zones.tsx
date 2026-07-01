import { useMemo } from 'react'
import type { CommentZoneSpec } from '@/features/editor/components/use-diff-comment-zones'
import type { GitDiff } from '@/features/git/types/git-types'
import {
  buildUnifiedThreadAnchorMap,
  findUnifiedModelLine,
} from '@/features/git/utils/diff-editor-content'
import { groupLinesIntoHunks } from '@/features/git/utils/git-diff-helpers'
import DiffHunkHeader from './git-diff-hunk-header'

/**
 * Per-hunk Stage/Unstage headers for the WORKING-TREE diff, rendered as Monaco
 * view zones anchored above each hunk's first line. Reuses DiffHunkHeader
 * (which calls stageHunk/unstageHunk directly), so deleting the native DOM
 * renderer doesn't lose per-hunk staging.
 *
 * Returns null when disabled (commit/stash/tag/review diffs have no staging).
 */
export function useHunkStagingZones(params: {
  enabled: boolean
  diff: GitDiff
  isStaged: boolean
}): CommentZoneSpec[] | null {
  const { enabled, diff, isStaged } = params
  const anchors = useMemo(() => buildUnifiedThreadAnchorMap(diff), [diff])
  const hunks = useMemo(() => groupLinesIntoHunks(diff.lines), [diff.lines])
  const filePath = diff.file_path

  const zones = useMemo<CommentZoneSpec[]>(() => {
    if (!enabled) return []
    const result: CommentZoneSpec[] = []
    for (const hunk of hunks) {
      const first = hunk.lines[0]
      if (!first) continue
      const side: 'old' | 'new' = first.line_type === 'removed' ? 'old' : 'new'
      const num = side === 'old' ? first.old_line_number : first.new_line_number
      if (num == null) continue
      const modelLine = findUnifiedModelLine(anchors, side, num)
      if (modelLine == null) continue
      result.push({
        key: `hunk:${hunk.id}`,
        // Anchor above the hunk's first line (afterLineNumber 0 = before line 1).
        afterModelLine: Math.max(0, modelLine - 1),
        node: (
          <DiffHunkHeader
            hunk={hunk}
            isCollapsed={false}
            onToggleCollapse={() => {}}
            isStaged={isStaged}
            filePath={filePath}
            isInMultiFileView={false}
          />
        ),
      })
    }
    return result
  }, [enabled, hunks, anchors, isStaged, filePath])

  if (!enabled) return null
  return zones
}
