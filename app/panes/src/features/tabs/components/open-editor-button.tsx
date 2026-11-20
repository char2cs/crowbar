import { CodeIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

/** The chat-only header's way into the editor area; that header exists only while the editor is
 *  closed, so no gate is needed. Absent without a workspace: new files have nowhere to go. */
export function OpenEditorButton({ wsId, onOpen }: { wsId: string | null; onOpen: () => void }) {
  if (!wsId) return null
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      data-testid="open-editor"
      aria-label="Open editor"
      tooltip="Open editor"
      tooltipSide="bottom"
      onClick={onOpen}
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
    >
      <CodeIcon />
    </Button>
  )
}
