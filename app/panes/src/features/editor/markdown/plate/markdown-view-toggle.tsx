import { Code, Eye } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { documentText } from '@/features/editor/documents/document-text'
import { findEditorBuffer } from '@/features/editor/lib/buffer-save'
import { useMarkdownViewStore, selectMarkdownView, type MarkdownView } from './markdown-view-store'

export function MarkdownViewToggle({ bufferId }: { bufferId: string }) {
  const view = useMarkdownViewStore(selectMarkdownView(bufferId))
  const toggleView = useMarkdownViewStore((s) => s.toggleView)

  const onClick = () => {
    const buffer = findEditorBuffer(bufferId)
    if (buffer) documentText(documentRefOf(buffer))
    toggleView(bufferId)
  }

  return <MarkdownViewButton view={view} onClick={onClick} />
}

export function MarkdownViewButton({ view, onClick }: { view: MarkdownView; onClick: () => void }) {
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
