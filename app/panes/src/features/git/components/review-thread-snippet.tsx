import { useEffect, useMemo, useState } from 'react'
import type { FileDiffOptions } from '@pierre/diffs/react'
import { PatchDiff } from '@pierre/diffs/react'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { fetchFilePatch, sliceThreadPatch } from '@/features/git/lib/review-thread-snippet'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { useHostResolved } from '@/lib/host-resolve'
import { useOwningChatId } from '@/lib/workspace-scope'

const OPTIONS: FileDiffOptions<undefined, undefined> = {
  diffStyle: 'unified',
  disableFileHeader: true,
  overflow: 'scroll',
  hunkSeparators: 'simple',
}

type Snippet = { patch: string | null } | { failed: true } | null

export function ReviewThreadSnippet({ wsId, thread }: { wsId: string; thread: ReviewThread }) {
  const { filePath, side, lineNumber, startLine } = thread
  const [snippet, setSnippet] = useState<Snippet>(null)
  const owningChatId = useOwningChatId(wsId)
  const hostResolved = useHostResolved({ wsId })
  const ready = owningChatId !== null && hostResolved

  useEffect(() => {
    if (!ready) return
    let cancelled = false
    fetchFilePatch(wsId, filePath)
      .then((full) => {
        if (cancelled) return
        setSnippet({ patch: sliceThreadPatch(full, { filePath, side, lineNumber, startLine }) })
      })
      .catch(() => {
        if (!cancelled) setSnippet({ failed: true })
      })
    return () => {
      cancelled = true
    }
  }, [ready, wsId, filePath, side, lineNumber, startLine])

  const selected = useMemo(
    () => ({
      start: Math.min(startLine || lineNumber, lineNumber),
      end: lineNumber,
      side: side === 'old' ? ('deletions' as const) : ('additions' as const),
    }),
    [side, lineNumber, startLine],
  )

  const themeVersion = useThemeVersion()
  const options = useMemo(
    () => ({ ...OPTIONS, themeType: isDarkMode() ? ('dark' as const) : ('light' as const) }),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- themeVersion only triggers the re-read
    [themeVersion],
  )

  if (snippet === null) return null
  if ('failed' in snippet) {
    return (
      <p className="border-b border-border px-3 py-2 text-sm text-muted-foreground">
        Couldn’t load these lines.
      </p>
    )
  }
  if (snippet.patch === null) return null
  return (
    <div className="border-b border-border" data-testid="review-thread-snippet">
      <PatchDiff patch={snippet.patch} options={options} selectedLines={selected} />
    </div>
  )
}
