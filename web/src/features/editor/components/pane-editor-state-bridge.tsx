/**
 * PaneEditorStateBridge — mirrors the active buffer's identity into the shared
 * editor-state store (status-bar cursor view-key + the legacy `filePath`/value
 * seam) for the ACTIVE surface only.
 *
 * Mounted once per pane by {@link EditorSurface}. It reads the active file/buffer
 * through its OWN narrow subscriptions (registry for `filePath`, a narrow
 * workspace selector for `activeBufferId`) so a buffer switch re-renders only
 * this tiny bridge — not EditorSurface. It renders nothing.
 */

import { useCallback, useEffect, useState } from 'react'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { useEditorStateStore } from '@/features/editor/stores/state-store'

export interface PaneEditorStateBridgeProps {
  paneId: string
  isActiveSurface: boolean
  /** Stable content-change seam from the controller (write path for value). */
  onContentChange: (value: string) => void
}

export function PaneEditorStateBridge({
  paneId,
  isActiveSurface,
  onContentChange,
}: PaneEditorStateBridgeProps) {
  const { setContent, setFileInfo, setActiveEditorViewKey } = useEditorStateStore.use.actions()

  const registry = useWorkspaceStore().activeEditorRegistry
  const [filePath, setFilePath] = useState(() => registry.get(paneId)?.filePath ?? '')
  useEffect(() => {
    return registry.subscribe(paneId, (ctx) => setFilePath(ctx?.filePath ?? ''))
  }, [registry, paneId])

  const activeBufferId = useWorkspaceStoreContext(
    useCallback((state) => state.panes[paneId]?.activeBufferId ?? null, [paneId]),
  )
  const editorViewKey = activeBufferId ? `${paneId}:${activeBufferId}` : null

  useEffect(() => {
    if (!isActiveSurface) return
    setActiveEditorViewKey(editorViewKey)
  }, [editorViewKey, isActiveSurface, setActiveEditorViewKey])

  useEffect(() => {
    if (!isActiveSurface) return
    setContent('', onContentChange)
  }, [isActiveSurface, onContentChange, setContent])

  useEffect(() => {
    if (!isActiveSurface) return
    setFileInfo(filePath)
  }, [filePath, isActiveSurface, setFileInfo])

  return null
}

export default PaneEditorStateBridge
