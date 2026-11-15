import { useEffect } from 'react'
import { useStore } from 'zustand'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBuffersByIds } from '@/features/workspace/stores/hooks/use-buffer-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { report } from '@/lib/errors/report'
import type { PaneClient } from '@/sdk/pane-client'
import { bufferSession } from './buffer-session'

/** Opens the pane's buffer in the workspace `wsId` into the root pane, from the state the shell saved. */
export type OpenBuffer = (wsId: string, saved: unknown) => Promise<unknown>

/** A file or a review in the pane's one tab, placed in the workspace of the chat the shell named, with the tab's name sent to the shell as the pane title. */
export function BufferPaneHost({
  client,
  open,
}: {
  client: Pick<PaneClient, 'setTitle'>
  open: OpenBuffer
}) {
  const chatId = useStore(bufferSession, (s) => s.chatId)
  const saved = useStore(bufferSession, (s) => s.saved)
  const active = useStore(bufferSession, (s) => s.active)
  const wsId = useChatField(chatId ?? '', (c) => c?.workspaceId || null)
  const tabIds = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID]?.editorTabIds)
  const activeTabId = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID]?.activeEditorTabId)
  const [buffer] = useBuffersByIds(tabIds ?? [])

  useEffect(() => {
    if (!wsId) return
    setActiveWorkspaceId(wsId)
    // A remount of the effect must not open a second tab.
    if (windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds.length) return
    open(wsId, saved).catch((error: unknown) => report(error, { action: 'Open the pane' }))
  }, [wsId, saved, open])

  useEffect(() => {
    if (buffer?.name) client.setTitle(buffer.name)
  }, [client, buffer?.name])

  if (!wsId) return null
  return (
    <WorkspaceStoreContext.Provider value={getOrCreateWorkspaceStore(wsId)}>
      <div className="relative h-screen w-screen overflow-hidden">
        <PaneEditorViewContent
          paneId={ROOT_PANE_ID}
          wsId={wsId}
          editorTabIds={tabIds ?? []}
          activeEditorTabId={activeTabId}
          editorViewHidden={false}
          isActivePane={active}
          showing
        />
      </div>
      <EditorHostRegistry />
    </WorkspaceStoreContext.Provider>
  )
}
