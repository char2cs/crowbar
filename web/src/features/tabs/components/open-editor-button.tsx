import { CodeIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { togglePaneEditor } from '@/features/panes/utils/pane-command-actions'

/** The chat-only header's way into the editor area; that header exists only while the editor is
 *  closed, so no gate is needed. Absent without a workspace: a new file would have nowhere to go. */
export function OpenEditorButton({ paneId, wsId }: { paneId: string; wsId: string | null }) {
  if (!wsId) return null
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      data-testid="open-editor"
      aria-label="Open editor"
      tooltip="Open editor"
      tooltipSide="bottom"
      onClick={() => togglePaneEditor(paneId)}
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
    >
      <CodeIcon />
    </Button>
  )
}
