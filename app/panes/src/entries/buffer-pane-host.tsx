import { useEffect } from 'react'
import { useStore } from 'zustand'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBuffersByIds } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { saveBuffer } from '@/features/editor/lib/buffer-save'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'
import { hasUnsavedEdits, type PaneContent } from '@/features/panes/types/pane-content'
import { report } from '@/lib/errors/report'
import type { PaneClient } from '@/sdk/pane-client'
import { bufferSession } from './buffer-session'
import type { OpenBuffer } from './open-buffer'

type Shell = Pick<
  PaneClient,
  'setTitle' | 'markDirty' | 'markClean' | 'onSave' | 'setState' | 'refuseOpen'
>

const WORK = { dirty: 'dirty', clean: 'clean' } as const
type Work = (typeof WORK)[keyof typeof WORK]

function reportWork(client: Shell, work: Work): void {
  if (work === WORK.dirty) client.markDirty()
  if (work === WORK.clean) client.markClean()
}

function reportTitle(client: Shell, name: string | undefined): void {
  if (name) client.setTitle(name)
}

function useShellReporting(client: Shell, buffer: PaneContent | undefined): void {
  const dirty = buffer ? hasUnsavedEdits(buffer) : false
  const bufferId = buffer?.id

  useEffect(() => {
    reportTitle(client, buffer?.name)
  }, [client, buffer?.name])

  useEffect(() => {
    reportWork(client, dirty ? WORK.dirty : WORK.clean)
  }, [client, dirty])

  useEffect(
    () =>
      client.onSave(() => {
        if (!bufferId) return
        saveBuffer(bufferId).catch((error: unknown) => report(error, { action: 'Save the file' }))
      }),
    [client, bufferId],
  )
}

function settled(saved: unknown): unknown {
  const openedJustNow = typeof saved === 'object' && saved !== null && 'fresh' in saved
  if (!openedJustNow) return null
  const { fresh: _fresh, line: _line, ...kept } = saved as Record<string, unknown>
  return kept
}

async function openInto(client: Shell, open: OpenBuffer, wsId: string, saved: unknown) {
  let refused = false
  await open(wsId, saved, (reason) => {
    refused = true
    client.refuseOpen(reason)
  })
  const kept = settled(saved)
  if (!refused && kept) client.setState(kept)
}

function WorkspaceFeeds({ wsId }: { wsId: string }) {
  useWorkspaceEffects(wsId, true)
  return null
}

/** A file or a review in the pane's one tab, placed in the workspace of the chat the shell named, with its workspace's feeds (files, git, review threads) held, the tab's name sent to the shell as the pane title and its unsaved edits reported so the shell asks before closing it. */
export function BufferPaneHost({ client, open }: { client: Shell; open: OpenBuffer }) {
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
    if (windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds.length) return
    openInto(client, open, wsId, saved).catch((error: unknown) =>
      report(error, { action: 'Open the pane' }),
    )
  }, [client, wsId, saved, open])

  useShellReporting(client, buffer)
  useSaveKeyboard()

  if (!wsId) return null
  return (
    <WorkspaceStoreContext.Provider value={getOrCreateWorkspaceStore(wsId)}>
      <WorkspaceFeeds wsId={wsId} />
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
