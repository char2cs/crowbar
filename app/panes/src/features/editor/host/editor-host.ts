import type { DocumentRegistry } from '@/features/editor/documents/document-registry'
import type { EditorManager } from '@/features/editor/lib/editor-manager'

export interface EditorHost {
  readonly manager: EditorManager
  readonly documents: DocumentRegistry
  readonly languageOf: (uri: string) => string
}

let host: EditorHost | null = null
let loading: Promise<EditorHost> | null = null
const listeners = new Set<() => void>()
const heldWith = new Map<string, (() => void)[]>()

export function getEditorHost(): EditorHost | null {
  return host
}

export function subscribeEditorHost(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function releaseWithDocument(uri: string, dispose: () => void): void {
  if (!host?.documents.has(uri)) return dispose()
  heldWith.set(uri, [...(heldWith.get(uri) ?? []), dispose])
}

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

export type EditorHostBuild = (released: (uri: string) => void) => Promise<EditorHost>

let build: EditorHostBuild | null = null

export function installEditorHostBuild(next: EditorHostBuild): void {
  build = next
}

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
