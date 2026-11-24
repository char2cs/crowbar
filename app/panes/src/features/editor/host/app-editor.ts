import { installPreviewOpener } from '@/features/editor/documents/open-preview'
import type { DocumentRegistry } from '@/features/editor/documents/document-registry'
import type { DocumentRef } from '@/lib/api/documents'
import { installEditorHostBuild, type EditorHost } from './editor-host'

const load = () =>
  Promise.all([
    import('@/features/editor/lib/monaco-adapters'),
    import('@/features/editor/lib/buffer-save'),
    import('@/features/editor/documents/document-lsp'),
    import('monaco-editor/esm/vs/editor/editor.api.js'),
    import('@/features/editor/documents/document-registry'),
    import('@/features/editor/lib/editor-manager'),
  ])

type Loaded = Awaited<ReturnType<typeof load>>
type Released = (uri: string) => void

function attachLsp([, , lsp, monaco]: Loaded) {
  return (uri: string, ref: DocumentRef) => {
    const model = monaco.editor.getModel(monaco.Uri.parse(uri))
    if (model) lsp.attachDocumentLsp(ref, model)
  }
}

function releaseWith([, save]: Loaded, released: Released) {
  return (uri: string) => {
    save.cancelPendingWrites(uri)
    released(uri)
  }
}

function registryOver(loaded: Loaded, released: Released): DocumentRegistry {
  const [adapters, save, , , registry] = loaded
  return new registry.DocumentRegistry(adapters.realModelApi(), {
    dirtyChanged: save.setDocumentDirty,
    contentChanged: save.noteDocumentEdited,
    opened: attachLsp(loaded),
    released: releaseWith(loaded, released),
  })
}

async function buildHost(released: Released): Promise<EditorHost> {
  const loaded = await load()
  const [adapters, , , , , manager] = loaded
  const documents = registryOver(loaded, released)
  const editors = adapters.realEditorApi(adapters.EDITOR_CREATE_OPTIONS)
  return {
    manager: new manager.EditorManager(editors, documents),
    documents,
    languageOf: adapters.langForUri,
  }
}

async function openAsPreview(doc: DocumentRef): Promise<void> {
  const { openDocument } = await import('@/features/editor/documents/open-document')
  await openDocument(doc, { preview: true })
}

/** Gives the app (not a shell pane) its Monaco editor: the host it builds on first need, and the
 *  preview tab it opens itself. Both load lazily, so a page that never edits never pays for it. */
export function installAppEditor(): void {
  installEditorHostBuild(buildHost)
  installPreviewOpener(openAsPreview)
}
