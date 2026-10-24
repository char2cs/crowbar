/**
 * Models for files a language server points at: the open model of a
 * workspace path, and throwaway models so the references peek can preview
 * files that are not open.
 */
import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { readDocumentFromDisk, workspaceRef } from '@/lib/api/documents'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { documentUri, refFromUri } from '../documents/document-ref'
import { findDocumentBuffer } from '../documents/open-document'
import { langForUri } from '../lib/monaco-adapters'
import { toMonacoRange, type DaemonLocation } from './lsp-to-monaco'

export function isOutsideWorkspace(path: string): boolean {
  return /^([A-Za-z]:)?[\\/]/.test(path)
}

export function modelFor(wsId: string, path: string): Monaco.editor.ITextModel | null {
  return monacoEditor.getModel(Uri.parse(documentUri(workspaceRef(wsId, path))))
}

export function toMonacoLocations(
  wsId: string,
  locations: DaemonLocation[],
): Monaco.languages.Location[] {
  const inside: Monaco.languages.Location[] = []
  for (const loc of locations) {
    if (isOutsideWorkspace(loc.filePath)) continue
    const uri = Uri.parse(documentUri(workspaceRef(wsId, loc.filePath)))
    inside.push({ uri, range: toMonacoRange(loc.range) })
  }
  return inside
}

// The ModelRegistry adopts a peek model if its file is later opened; otherwise
// it is disposed when the next peek replaces it.
const peekModels = new Set<Monaco.editor.ITextModel>()

function isHeld(model: Monaco.editor.ITextModel): boolean {
  const ref = refFromUri(model.uri.toString())
  if (ref && getEditorHost()?.models.get(documentUri(ref))) return true
  return monacoEditor.getEditors().some((e) => e.getModel() === model)
}

/** Give every location a model the peek can show, reading files that are not open. */
export async function ensurePreviewModels(locations: Monaco.languages.Location[]): Promise<void> {
  for (const model of peekModels) {
    if (!model.isDisposed() && !isHeld(model)) model.dispose()
  }
  peekModels.clear()
  const uris = new Set(locations.map((l) => l.uri.toString()))
  await Promise.all(
    [...uris].map(async (raw) => {
      const ref = refFromUri(raw)
      if (!ref || monacoEditor.getModel(Uri.parse(raw))) return
      const text = findDocumentBuffer(ref)?.content ?? (await readDocumentFromDisk(ref))
      if (text === null || monacoEditor.getModel(Uri.parse(raw))) return
      const uri = documentUri(ref)
      peekModels.add(monacoEditor.createModel(text, langForUri(uri), Uri.parse(uri)))
    }),
  )
}
