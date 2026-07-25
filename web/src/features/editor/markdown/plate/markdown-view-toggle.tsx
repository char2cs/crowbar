import { Code, Eye } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useMarkdownViewStore, selectMarkdownView } from './markdown-view-store'

/** Rich <-> Source toggle for a markdown buffer. Flushes any pending rich-editor
 *  serialize before switching so the source view opens on the latest text. */
export function MarkdownViewToggle({ bufferId }: { bufferId: string }) {
  const view = useMarkdownViewStore(selectMarkdownView(bufferId))
  const toggleView = useMarkdownViewStore((s) => s.toggleView)

  const onClick = () => {
    // Flush the rich editor's debounced write before leaving it.
    window.dispatchEvent(new Event('flush-editor-content'))
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
