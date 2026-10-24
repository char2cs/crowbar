import { CodeIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { togglePaneEditor } from '@/features/panes/utils/pane-command-actions'

/**
 * The chat-only header's way into the editor area. That header exists exactly
 * while the editor area is closed (chatFillsPane), so it needs no open/closed
 * gate of its own; SplitToggleButton, on the editor's row, closes it again.
 * Absent for a chat with no workspace: a new file or terminal would have
 * nowhere to live.
 */
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
