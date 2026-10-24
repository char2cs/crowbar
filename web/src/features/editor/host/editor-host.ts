import { DocumentRegistry } from '@/features/editor/documents/document-registry'
import { EditorManager } from '@/features/editor/lib/editor-manager'

/** The window's one Monaco environment: every pane, whatever workspace its
 *  buffer belongs to, mounts on this manager, and every open document's model
 *  lives in this registry. */
export interface EditorHost {
  readonly manager: EditorManager
  readonly documents: DocumentRegistry
}

let host: EditorHost | null = null
let loading: Promise<EditorHost> | null = null
const listeners = new Set<() => void>()

export function getEditorHost(): EditorHost | null {
  return host
}

/** Notified once, when the host has loaded. */
export function subscribeEditorHost(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/**
 * Loads Monaco and builds the host on the first editor need, so cold launch
 * never pays for it. Every caller shares one load; a failed chunk load is
 * forgotten so the next caller retries.
 */
export function loadEditorHost(): Promise<EditorHost> {
  // buffer-save loads here too: through the pane store it imports this module.
  loading ??= Promise.all([
    import('@/features/editor/lib/monaco-adapters'),
    import('@/features/editor/lib/buffer-save'),
  ]).then(
    ([adapters, save]) => {
      const documents = new DocumentRegistry(adapters.realModelApi(), {
        dirtyChanged: save.setDocumentDirty,
        contentChanged: save.noteDocumentEdited,
        opened: () => {},
        released: save.cancelPendingWrites,
      })
      const editors = adapters.realEditorApi(adapters.EDITOR_CREATE_OPTIONS)
      host = { manager: new EditorManager(editors, documents), documents }
      for (const listener of [...listeners]) listener()
      return host
    },
    (err: unknown) => {
      loading = null
      throw err
    },
  )
  return loading
}
