import type { ContextMenuItem } from '@/components/ui/context-menu'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { toast } from '@/features/window/stores/toast-store'
import { showNativeContextMenu } from '@/lib/crowbar-bridge'
import { openExternalUrl } from '@/lib/external-open'

/** The view the terminal itself lives in; a browser tab opens there, never in "the active" one. */
export interface TerminalViewTarget {
  workspaceId: string
  paneId: string
}

export async function showTerminalUrlMenu(
  url: string,
  position: { x: number; y: number },
  target: TerminalViewTarget | null,
): Promise<void> {
  const items: ContextMenuItem[] = [
    {
      id: 'open-default-browser',
      label: 'Open in Default Browser',
      onClick: () => {
        openExternalUrl(url).catch((error: unknown) => {
          console.error('Failed to open link:', error)
        })
      },
    },
    {
      id: 'open-crowbar-browser',
      label: 'Open in Crowbar Browser',
      disabled: target === null,
      onClick: () => {
        if (!target) return
        windowPaneStore
          .getState()
          .bufferActions.openContent(
            { type: 'browser', url, workspaceId: target.workspaceId },
            { paneId: target.paneId },
          )
      },
    },
    {
      id: 'copy-link',
      label: 'Copy Link',
      onClick: () => {
        navigator.clipboard.writeText(url).catch(() => toast.error('Could not copy link', url))
      },
    },
  ]
  try {
    await showNativeContextMenu(items, position)
  } catch (error) {
    console.error('Failed to show link menu:', error)
  }
}
