import { revealInFolder } from '@/features/files/lib/file-tree-handlers'
import { reloadBufferFromDisk } from '@/features/editor/lib/reload-buffer'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import { useWorkspaceIsLocal } from '@/lib/host-resolve'
import { getRelativePath } from '@/utils/path-helpers'
import TabContextMenu from './tab-context-menu'

export interface TabMenuState {
  isOpen: boolean
  position: { x: number; y: number }
  buffer: PaneContent | null
}

const writeText = (text: string) => navigator.clipboard.writeText(text)

// Read live at call time, so a fast second action sees the latest editorTabIds.
function paneBuffersInOrder(paneId: string | undefined): PaneContent[] {
  if (!paneId) return []
  const state = windowPaneStore.getState()
  const byId = new Map(state.buffers.map((b) => [b.id, b]))
  return (state.panes[paneId]?.editorTabIds ?? []).flatMap((id) => byId.get(id) ?? [])
}

/** One tab's menu in `paneId`'s strip. "Close Others/All/to the Right" touch only this pane's
 *  tabs, never a pinned one or the sole tab a pane holds (isUncloseable). */
export function TabBarContextMenu({
  menu,
  paneId,
  wsId,
  onClose,
  onPin,
  onCloseTab,
}: {
  menu: TabMenuState
  paneId?: string
  wsId?: string | null
  onClose: () => void
  onPin: (bufferId: string) => void
  onCloseTab: (bufferId: string) => void
}) {
  const { removeEditorTabFromPane } = usePaneActions()
  const { closeBuffer } = useBufferActions()
  // The file lives in the workspace its tab was opened in, which the pane need not be.
  const menuWorkspaceId = menu.buffer?.workspaceId
  const menuOnLocalHost = useWorkspaceIsLocal(menuWorkspaceId ?? '')

  const closeAll = (buffers: PaneContent[]) => {
    for (const b of buffers) {
      if (b.isPinned || b.isUncloseable) continue
      if (paneId) removeEditorTabFromPane(paneId, b.id)
      closeBuffer(b.id)
    }
  }
  const closeToRight = (bufferId: string) => {
    const buffers = paneBuffersInOrder(paneId)
    const idx = buffers.findIndex((b) => b.id === bufferId)
    if (idx !== -1) closeAll(buffers.slice(idx + 1))
  }

  return (
    <TabContextMenu
      isOpen={menu.isOpen}
      position={menu.position}
      buffer={menu.buffer}
      onClose={onClose}
      onPin={onPin}
      onCloseTab={onCloseTab}
      onCloseOthers={(keep) => closeAll(paneBuffersInOrder(paneId).filter((b) => b.id !== keep))}
      onCloseAll={() => closeAll(paneBuffersInOrder(paneId))}
      onCloseToRight={closeToRight}
      onCopyPath={(path) => void writeText(path)}
      onCopyRelativePath={(path) => void writeText(wsId ? getRelativePath(path, wsId) : path)}
      onReload={(bufferId) =>
        void reloadBufferFromDisk(bufferId, (buffer) =>
          window.confirm(`Discard unsaved changes to ${buffer.name} and reload it from disk?`),
        )
      }
      onRevealInFinder={
        menuWorkspaceId && menuOnLocalHost
          ? (path) => revealInFolder(menuWorkspaceId, path)
          : undefined
      }
    />
  )
}
