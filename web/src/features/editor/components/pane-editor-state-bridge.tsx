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
import { useStore } from 'zustand'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'

export interface PaneEditorStateBridgeProps {
  paneId: string
  isActiveSurface: boolean
  /** Stable content-change seam from the controller (write path for value). */
  onContentChange: (value: string) => void
  /**
   * The active-editor registry for the BUFFER'S OWN workspace — the same
   * value `EditorSurface` already resolved via its `workspaceId` prop.
   * Passed explicitly rather than re-derived here via `useWorkspaceStore()`
   * (ambient `WorkspaceStoreContext`, scoped to the PANE'S CHAT's workspace
   * — see pane-container.tsx's `chatStore`): a pane's chat and its open file
   * are not required to share a workspace, and when they didn't, this
   * bridge's registry subscription silently watched the wrong workspace's
   * registry and never fired for this pane's editor — the status bar/
   * breadcrumb kept whatever `filePath` it started with. Same root cause as
   * the font-size bug fixed in use-pane-editor-satellites.ts (8dcd26e8f).
   */
  registry: ActiveEditorRegistry
}

export function PaneEditorStateBridge({
  paneId,
  isActiveSurface,
  onContentChange,
  registry,
}: PaneEditorStateBridgeProps) {
  const { setContent, setFileInfo, setActiveEditorViewKey } = useEditorStateStore.use.actions()

  const [filePath, setFilePath] = useState(() => registry.get(paneId)?.filePath ?? '')
  useEffect(() => {
    return registry.subscribe(paneId, (ctx) => setFilePath(ctx?.filePath ?? ''))
  }, [registry, paneId])

  const activeBufferId = useStore(
    windowPaneStore,
    useCallback((state) => state.panes[paneId]?.activeEditorTabId ?? null, [paneId]),
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
