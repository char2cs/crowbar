/**
 * The LSP document lifecycle for the model a pane shows: didOpen (deferred to
 * an idle tick so it never delays the first paint), debounced didChange from
 * the model's own edits, didClose on swap/unmount, and the diagnostics
 * markers the daemon pushes for it.
 */
import { useCallback, useEffect, useSyncExternalStore } from 'react'
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { scheduleIdleTask } from '../lib/idle-task'
import { LSP_MARKER_OWNER, LspClient } from '../lsp/lsp-client'
import { pathsMatch, toMonacoMarker } from '../monaco/editor-conversions'

/**
 * Whether LspClient can address the active workspace yet. Its LSP URL needs
 * the workspace's OWNING CHAT id, which the sidebar records asynchronously
 * (often after the workspace hydrates); until then the document lifecycle
 * waits instead of throwing, and re-runs the moment the id lands.
 */
export function useLspScopeReady(): boolean {
  const wsId = getActiveWorkspaceId()
  const owningChatId = useSyncExternalStore(
    useCallback(
      (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
      [wsId],
    ),
    useCallback(() => (wsId ? getOwningChatId(wsId) : null), [wsId]),
  )
  return !wsId || isHomeWorkspace(wsId) || owningChatId !== null
}

export function useLspDocumentSync(
  model: Monaco.editor.ITextModel | null,
  filePath: string,
  languageId: string,
  workspaceId: string,
): void {
  const scopeReady = useLspScopeReady()
  useEffect(() => {
    if (!model || !filePath || !scopeReady) return
    const client = LspClient.getInstance()

    // LspClient follows the ACTIVE workspace's topic, while this pane may show
    // another workspace's file: two worktrees share relative paths, so a
    // batch counts only when it is for this pane's own workspace.
    const unsubscribe = client.onDiagnosticsUpdate((path, diagnostics, wsId) => {
      if (wsId !== workspaceId || !pathsMatch(path, filePath) || model.isDisposed()) return
      monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, diagnostics.map(toMonacoMarker))
    })

    let opened = false
    const openHandle = scheduleIdleTask(() => {
      opened = true
      void client.documentOpen(filePath, model.getValue(), languageId)
    })
    const changes = model.onDidChangeContent(() => {
      if (opened) {
        client.scheduleChange(filePath, () => (model.isDisposed() ? null : model.getValue()))
      }
    })

    return () => {
      openHandle.cancel()
      changes.dispose()
      unsubscribe()
      if (opened) void client.documentClose(filePath)
      if (!model.isDisposed()) monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, [])
    }
  }, [model, filePath, languageId, scopeReady, workspaceId])
}
