import type { DocumentRegistry } from '@/features/editor/documents/document-registry'
import type { EditorManager } from '@/features/editor/lib/editor-manager'

/** The window's one Monaco environment: every pane, whatever workspace its
 *  buffer belongs to, mounts on this manager, and every open document's model
 *  lives in this registry. */
export interface EditorHost {
  readonly manager: EditorManager
  readonly documents: DocumentRegistry
  /** The Monaco language of a document's model. */
  readonly languageOf: (uri: string) => string
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

/** Builds the host: loads Monaco and opens its registry. `released` is told each document it lets go. */
export type EditorHostBuild = (released: (uri: string) => void) => Promise<EditorHost>

let build: EditorHostBuild | null = null

/** Makes `next` the way this page builds its editor host. Only the app installs one: a shell pane
 *  has no Monaco editor, so its bundle never carries Monaco. */
export function installEditorHostBuild(next: EditorHostBuild): void {
  build = next
}

/** Builds the host on first need, so cold launch never pays for it. Callers share one load; a
 *  failed chunk load is forgotten so the next caller retries. */
export function loadEditorHost(): Promise<EditorHost> {
  if (!build) return Promise.reject(new Error('This page has no code editor'))
  loading ??= build(released).then(
    (built) => {
      host = built
      for (const listener of [...listeners]) listener()
      return built
    },
    (err: unknown) => {
      loading = null
      throw err
    },
  )
  return loading
}
