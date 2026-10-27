/**
 * The language-server lifecycle of one open document, whatever number of panes
 * show it: didOpen on an idle tick (never ahead of the first paint), debounced
 * didChange from the model's edits, didClose with the document, and the
 * diagnostics the daemon pushes for it as model markers. Everything is
 * addressed to the document's own workspace, never the active one.
 */
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import type { DocumentRef } from '@/lib/api/documents'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { scheduleIdleTask, type IdleTaskHandle } from '../lib/idle-task'
import { LSP_MARKER_OWNER, LspClient } from '../lsp/lsp-client'
import { pathsMatch, toMonacoMarker } from '../monaco/editor-conversions'
import { getLanguageIdFromPath } from '../utils/language-id'
import { documentRefOf, isUntitledRef, sameDocument } from './document-ref'

const NONE = () => {}

/** The server's language for `ref`: the buffer's chosen one, else its path's. */
function languageOf(ref: DocumentRef): string {
  const buffer = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && sameDocument(documentRefOf(b), ref))
  const override = buffer && isEditorContent(buffer) ? buffer.languageOverride : undefined
  return override ?? getLanguageIdFromPath(ref.path) ?? 'plaintext'
}

/**
 * Attaches `model` to its workspace's language server; the returned disposer
 * detaches it, and runs by itself if the model goes first (a reopen in place
 * makes a new model, which attaches anew). Host documents, untitled buffers
 * and the home workspace have no server.
 */
export function attachDocumentLsp(ref: DocumentRef, model: Monaco.editor.ITextModel): () => void {
  if (ref.authority === 'host' || isUntitledRef(ref)) return NONE
  const { workspaceId, path } = ref
  if (isHomeWorkspace(workspaceId)) return NONE
  const client = LspClient.getInstance()

  const unsubscribe = client.onDiagnosticsUpdate(workspaceId, (filePath, diagnostics) => {
    if (!pathsMatch(filePath, path) || model.isDisposed()) return
    monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, diagnostics.map(toMonacoMarker))
  })

  let opened = false
  let pending: IdleTaskHandle | null = null
  const open = () => {
    pending = scheduleIdleTask(() => {
      pending = null
      opened = true
      void client.documentOpen(workspaceId, path, model.getValue(), languageOf(ref))
    })
  }
  const close = () => {
    pending?.cancel()
    pending = null
    if (opened) void client.documentClose(workspaceId, path)
    opened = false
  }
  open()

  const changes = model.onDidChangeContent(() => {
    if (!opened) return
    client.scheduleChange(workspaceId, path, () => (model.isDisposed() ? null : model.getValue()))
  })
  const language = model.onDidChangeLanguage(() => {
    close()
    open()
  })

  let detached = false
  const detach = () => {
    if (detached) return
    detached = true
    close()
    changes.dispose()
    language.dispose()
    gone.dispose()
    unsubscribe()
    if (!model.isDisposed()) monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, [])
  }
  const gone = model.onWillDispose(detach)
  return detach
}
