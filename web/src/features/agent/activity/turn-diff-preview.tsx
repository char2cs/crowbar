import { useMemo } from 'react'
import type { CodeViewOptions } from '@pierre/diffs'
import { MAX_TURN_DIFF_PREVIEW_CHARS, parseTurnDiff } from '@/features/agent/lib/turn-diff'
import { DiffCodeView } from '@/features/git/components/diff/diff-code-view'

interface TurnDiffPreviewProps {
  diff: string
  turnId: string
  wsId?: string
}

/** A bounded, provider-neutral unified-diff preview for the current turn. */
export function TurnDiffPreview({ diff, turnId }: TurnDiffPreviewProps) {
  const items = useMemo(() => parseTurnDiff(diff, turnId), [diff, turnId])
  const options = useMemo<CodeViewOptions<undefined, undefined>>(
    () => ({
      diffStyle: 'unified',
      // The bordered wrapper is the frame: CodeView's own top/bottom padding
      // would float the diff away from its border.
      layout: { paddingTop: 0, paddingBottom: 0, gap: 8 },
      itemMetrics: { paddingBottom: 0 },
      stickyHeaders: true,
      tokenizeMaxLineLength: 2_000,
      tokenizeMaxLength: 20_000,
    }),
    [],
  )

  if (diff.length > MAX_TURN_DIFF_PREVIEW_CHARS) {
    return <p className="turn-diff-unavailable">Diff preview is too large to render live.</p>
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
