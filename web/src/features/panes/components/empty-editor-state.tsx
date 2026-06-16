import { TerminalWindow as Terminal } from '@phosphor-icons/react'
import { useCallback } from 'react'
import { createPortal } from 'react-dom'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { Button } from '@/components/ui/button'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'

const newTabRowClassName =
  'h-auto w-full justify-start gap-3 rounded-md px-3 py-1.5 text-left hover:bg-muted'

export function EmptyEditorState() {
  const bufferActions = useBufferActions()
  const contextMenu = useContextMenu()

  const handleOpenTerminal = useCallback(() => {
    bufferActions.openContent({ type: 'terminal' })
  }, [bufferActions])

  const getContextMenuItems = useCallback((): ContextMenuItem[] => {
    return [
      {
        id: 'new-terminal',
        label: 'New Terminal',
        icon: <Terminal />,
        onClick: handleOpenTerminal,
      },
    ]
  }, [handleOpenTerminal])

  return (
    <div className="flex h-full min-h-0 w-full overflow-auto" onContextMenu={contextMenu.open}>
      <div className="m-auto flex w-48 min-w-0 flex-col gap-0.5 px-2 py-3">
        <Button
          type="button"
          onClick={handleOpenTerminal}
          variant="ghost"
          className={newTabRowClassName}
          compact
        >
          <span className="shrink-0">
            <Terminal className="text-muted-foreground" />
          </span>
          <span className="text-foreground ui-text-xs">New Terminal</span>
        </Button>
      </div>

      {createPortal(
        <ContextMenu
          isOpen={contextMenu.isOpen}
          position={contextMenu.position}
          items={getContextMenuItems()}
          onClose={contextMenu.close}
        />,
        document.body,
      )}
    </div>
  )
}
