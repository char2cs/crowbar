import { memo, useMemo } from 'react'
import { useDiffData } from '../../hooks/use-git-diff-data'
import { normalizeGitDiff, normalizeMultiFileDiff } from '../../utils/normalize-diff'
import type { DiffViewerProps, MultiFileDiff } from '../../types/git-diff-types'
import GitDiffEditorStack from './git-diff-editor-stack'
import GitDiffEditorSurface from './git-diff-editor-surface'
import ImageDiffViewer from './git-diff-image'

function isMultiFileDiff(data: unknown): data is MultiFileDiff {
  return typeof data === 'object' && data !== null && 'files' in data && Array.isArray(data.files)
}

const DiffViewer = memo((props: DiffViewerProps) => {
  const { diff, rawDiffData, filePath, isLoading, error } = useDiffData()

  // Normalised at the single entry point both diff shapes pass through, rather
  // than at the dozen places that dereference `lines`. See normalize-diff.ts —
  // an opened diff tab persists its payload, so a tab from before the daemon
  // started sending `[]` for a binary file still restores with `null` and would
  // crash the pane it reopens into.
  const multiFileDiff = useMemo(() => {
    if (rawDiffData && isMultiFileDiff(rawDiffData)) {
      return normalizeMultiFileDiff(rawDiffData)
    }
    return null
  }, [rawDiffData])

  const safeDiff = useMemo(() => (diff ? normalizeGitDiff(diff) : diff), [diff])

  if (multiFileDiff) {
    return <GitDiffEditorStack multiDiff={multiFileDiff} isActivePane={props.isActivePane} />
  }

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <div className="ui-text-sm text-muted-foreground">Loading diff...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <div className="text-error ui-text-sm">{error}</div>
      </div>
    )
  }

  if (!safeDiff || !filePath) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <div className="ui-text-sm text-muted-foreground">No diff data available</div>
      </div>
    )
  }

  const fileName = filePath.split('/').pop() || filePath

  if (safeDiff.is_image) {
    return <ImageDiffViewer diff={safeDiff} fileName={fileName} onClose={() => {}} />
  }

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <GitDiffEditorSurface
        cacheKey={filePath}
        diff={safeDiff}
        breadcrumbProps={{
          filePathOverride: safeDiff.file_path || filePath,
        }}
      />
    </div>
  )
})

DiffViewer.displayName = 'DiffViewer'

export default DiffViewer
