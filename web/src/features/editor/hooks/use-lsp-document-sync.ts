/**
 * The LSP document lifecycle for the model a pane shows: didOpen (deferred to
 * an idle tick so it never delays the first paint), debounced didChange from
 * the model's own edits, didClose on swap/unmount, and the diagnostics
 * markers the daemon pushes for it. All of it is addressed to the pane's own
 * workspace, so which workspace is active never touches a hidden pane.
 */
import { useEffect } from 'react'
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { scheduleIdleTask } from '../lib/idle-task'
import { LSP_MARKER_OWNER, LspClient } from '../lsp/lsp-client'
import { pathsMatch, toMonacoMarker } from '../monaco/editor-conversions'

export function useLspDocumentSync(
  model: Monaco.editor.ITextModel | null,
  filePath: string,
  languageId: string,
  workspaceId: string,
): void {
  useEffect(() => {
    if (!model || !filePath) return
    const client = LspClient.getInstance()

    const unsubscribe = client.onDiagnosticsUpdate(workspaceId, (path, diagnostics) => {
      if (!pathsMatch(path, filePath) || model.isDisposed()) return
      monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, diagnostics.map(toMonacoMarker))
    })

    let opened = false
    const openHandle = scheduleIdleTask(() => {
      opened = true
      void client.documentOpen(workspaceId, filePath, model.getValue(), languageId)
    })
    const changes = model.onDidChangeContent(() => {
      if (!opened) return
      client.scheduleChange(workspaceId, filePath, () =>
        model.isDisposed() ? null : model.getValue(),
      )
    })

    return () => {
      openHandle.cancel()
      changes.dispose()
      unsubscribe()
      if (opened) void client.documentClose(workspaceId, filePath)
      if (!model.isDisposed()) monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, [])
    }
  }, [model, filePath, languageId, workspaceId])
}
