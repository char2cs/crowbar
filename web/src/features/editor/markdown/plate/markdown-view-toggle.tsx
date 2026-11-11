import { Code, Eye } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { documentText } from '@/features/editor/documents/document-text'
import { findEditorBuffer } from '@/features/editor/lib/buffer-save'
import { useMarkdownViewStore, selectMarkdownView } from './markdown-view-store'

/** Rich <-> Source toggle for a markdown buffer. Lands the view's pending edits
 *  before switching so the other view opens on the latest text. */
export function MarkdownViewToggle({ bufferId }: { bufferId: string }) {
  const view = useMarkdownViewStore(selectMarkdownView(bufferId))
  const toggleView = useMarkdownViewStore((s) => s.toggleView)

  const onClick = () => {
    // Reading the text runs every pending writer of the document.
    const buffer = findEditorBuffer(bufferId)
    if (buffer) documentText(documentRefOf(buffer))
    toggleView(bufferId)
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={onClick}
      aria-label={view === 'rich' ? 'View markdown source' : 'View rich editor'}
      tooltip={view === 'rich' ? 'View markdown source' : 'View rich editor'}
      tooltipSide="bottom"
    >
      {view === 'rich' ? <Code /> : <Eye />}
    </Button>
  )
}
