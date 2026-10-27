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
/** What each open document holds outside its model, let go when it is released. */
const heldWith = new Map<string, (() => void)[]>()

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

/** `dispose` runs once, when `uri`'s document is released; at once if it is not open. */
export function releaseWithDocument(uri: string, dispose: () => void): void {
  if (!host?.documents.has(uri)) return dispose()
  heldWith.set(uri, [...(heldWith.get(uri) ?? []), dispose])
}

/** Replaces `uri`'s model through `reopen`; what is released with the document stays with it. */
export function reopenDocument(uri: string, reopen: () => void): void {
  const kept = heldWith.get(uri) ?? []
  heldWith.delete(uri)
  try {
    host?.documents.release(uri)
    reopen()
  } finally {
    for (const dispose of kept) releaseWithDocument(uri, dispose)
  }
}

function released(uri: string): void {
  const held = heldWith.get(uri) ?? []
  heldWith.delete(uri)
  for (const dispose of held) dispose()
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
        released: (uri) => {
          save.cancelPendingWrites(uri)
          released(uri)
        },
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
