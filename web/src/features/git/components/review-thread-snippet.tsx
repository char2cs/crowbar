import { useEffect, useMemo, useState } from 'react'
import type { FileDiffOptions } from '@pierre/diffs/react'
import { PatchDiff } from '@pierre/diffs/react'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { fetchFilePatch, sliceThreadPatch } from '@/features/git/lib/review-thread-snippet'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

const OPTIONS: FileDiffOptions<undefined, undefined> = {
  diffStyle: 'unified',
  disableFileHeader: true,
  overflow: 'scroll',
  hunkSeparators: 'simple',
}

/**
 * The lines a thread is anchored to, read from the file's patch through the
 * same per-file request the diff view makes. Renders nothing while loading or
 * when the anchor is no longer in the patch.
 */
export function ReviewThreadSnippet({ wsId, thread }: { wsId: string; thread: ReviewThread }) {
  const { filePath, side, lineNumber, startLine } = thread
  const [patch, setPatch] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetchFilePatch(wsId, filePath)
      .then((full) => {
        if (!cancelled) setPatch(sliceThreadPatch(full, { filePath, side, lineNumber, startLine }))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [wsId, filePath, side, lineNumber, startLine])

  const selected = useMemo(
    () => ({
      start: Math.min(startLine || lineNumber, lineNumber),
      end: lineNumber,
      side: side === 'old' ? ('deletions' as const) : ('additions' as const),
    }),
    [side, lineNumber, startLine],
  )

  // The library's 'system' default follows the OS, not the app's own toggle.
  const themeVersion = useThemeVersion()
  const options = useMemo(
    () => ({ ...OPTIONS, themeType: isDarkMode() ? ('dark' as const) : ('light' as const) }),
    // themeVersion is a pure invalidation signal; the mode is read from the DOM.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- themeVersion only triggers the re-read
    [themeVersion],
  )

  if (patch === null) return null
  return (
    <div className="border-b border-border" data-testid="review-thread-snippet">
      <PatchDiff patch={patch} options={options} selectedLines={selected} />
    </div>
  )
}
