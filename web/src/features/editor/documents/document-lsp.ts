// Per document, not per pane: one didOpen/didClose whatever panes show it,
// addressed to the document's own workspace, never the active one.
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import type { DocumentRef } from '@/lib/api/documents'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { scheduleIdleTask, type IdleTaskHandle } from '../lib/idle-task'
import { LSP_MARKER_OWNER, LspClient } from '../lsp/lsp-client'
import { pathsMatch, toMonacoMarker } from '../monaco/editor-conversions'
import { getLanguageIdFromPath } from '../utils/language-id'
import { documentRefOf, isUntitledRef, sameDocument } from './document-ref'

/** The language the user picked for `ref`'s tab; undefined while no tab holds it. */
function pickedLanguage(state: WindowPaneState, ref: DocumentRef): string | null | undefined {
  const buffer = state.buffers.find(
    (b) => isEditorContent(b) && sameDocument(documentRefOf(b), ref),
  )
  if (!buffer || !isEditorContent(buffer)) return undefined
  return buffer.languageOverride ?? null
}

/**
 * Lives exactly as long as `model`: its dispose, which every release and reopen
 * performs, is the one teardown. The buffer's picked language, not Monaco's id
 * (tsx and ts share one), is what the server is told.
 */
export function attachDocumentLsp(ref: DocumentRef, model: Monaco.editor.ITextModel): void {
  if (ref.authority === 'host' || isUntitledRef(ref)) return
  const { workspaceId, path } = ref
  if (isHomeWorkspace(workspaceId)) return
  const client = LspClient.getInstance()
  const byPath = getLanguageIdFromPath(path) ?? 'plaintext'
  let language = pickedLanguage(windowPaneStore.getState(), ref) || byPath

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
      void client.documentOpen(workspaceId, path, model.getValue(), language)
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
  const stopFollowingLanguage = windowPaneStore.subscribe((state) => {
    const picked = pickedLanguage(state, ref)
    // A closing tab leaves the document to its release.
    if (picked === undefined || (picked || byPath) === language) return
    language = picked || byPath
    close()
    open()
  })

  const gone = model.onWillDispose(() => {
    close()
    changes.dispose()
    stopFollowingLanguage()
    gone.dispose()
    unsubscribe()
    monacoEditor.setModelMarkers(model, LSP_MARKER_OWNER, [])
  })
}
