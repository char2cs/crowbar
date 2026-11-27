import { useMemo } from 'react'
import type { CodeViewOptions } from '@pierre/diffs'
import { MAX_TURN_DIFF_PREVIEW_CHARS, parseTurnDiff } from '@/features/agent/lib/turn-diff'
import { formatAttachmentSize } from '@/features/agent/composer/plate/attachments/chat-attachment-file-card'
import { DiffCodeView } from '@/features/git/components/diff/diff-code-view'

interface TurnDiffPreviewProps {
  diff: string
  turnId: string
  target?: string
  wsId?: string
}

export function TurnDiffPreview({ diff, turnId, target }: TurnDiffPreviewProps) {
  const items = useMemo(() => parseTurnDiff(diff, turnId, target), [diff, turnId, target])
  const options = useMemo<CodeViewOptions<undefined, undefined>>(
    () => ({
      diffStyle: 'unified',
      hunkSeparators: 'simple',
      layout: { paddingTop: 0, paddingBottom: 0, gap: 8 },
      itemMetrics: { paddingBottom: 0 },
      stickyHeaders: true,
      tokenizeMaxLineLength: 2_000,
      tokenizeMaxLength: 20_000,
    }),
    [],
  )

  if (diff.length > MAX_TURN_DIFF_PREVIEW_CHARS) {
    return (
      <p className="turn-diff-unavailable">
        This full diff is {formatAttachmentSize(diff.length)}, too large to render live.
      </p>
    )
  }
  if (items.length === 0) {
    return <pre className="turn-diff-raw">{diff}</pre>
  }

  return (
    <div data-testid="turn-diff-preview" data-file-count={items.length}>
      <DiffCodeView
        viewKey={turnId}
        items={items}
        options={options}
        className="turn-diff-code-view"
      />
    </div>
  )
}
