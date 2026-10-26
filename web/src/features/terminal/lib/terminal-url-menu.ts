import type { ContextMenuItem } from '@/components/ui/context-menu'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { toast } from '@/features/window/stores/toast-store'
import { isDesktopApp, showNativeContextMenu } from '@/lib/crowbar-bridge'
import { openWorkspaceLink, refuseRemoteLoopback } from '@/lib/workspace-link'

/** The view the terminal itself lives in; a browser tab opens there, never in "the active" one.
 *  Its workspace's host decides whether a loopback link can open on this computer. */
export interface TerminalView {
  workspaceId: string
  paneId?: string
}

export async function showTerminalUrlMenu(
  url: string,
  position: { x: number; y: number },
  view: TerminalView,
): Promise<void> {
  const { workspaceId, paneId } = view
  const openInDefaultBrowser = () => {
    openWorkspaceLink(url, workspaceId).catch((error: unknown) => {
      console.error('Failed to open link:', error)
    })
  }
  // No OS menu (nor a Crowbar browser) outside the desktop app: the link just opens.
  if (!isDesktopApp()) {
    openInDefaultBrowser()
    return
  }
  const items: ContextMenuItem[] = [
    {
      id: 'open-default-browser',
      label: 'Open in Default Browser',
      onClick: openInDefaultBrowser,
    },
    {
      id: 'open-crowbar-browser',
      label: 'Open in Crowbar Browser',
      disabled: paneId === undefined,
      onClick: () => {
        if (paneId === undefined || refuseRemoteLoopback(url, workspaceId)) return
        windowPaneStore
          .getState()
          .bufferActions.openContent({ type: 'browser', url, workspaceId }, { paneId })
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
