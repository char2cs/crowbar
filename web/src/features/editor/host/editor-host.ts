import { EditorManager, type BufferMeta } from '@/features/editor/lib/editor-manager'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { ModelRegistry } from '@/features/editor/lib/model-registry'
import { isEditorContent } from '@/features/panes/types/pane-content'

/** The window's one Monaco environment: every pane, whatever workspace its
 *  buffer belongs to, mounts on this manager and shares this model table. */
export interface EditorHost {
  readonly manager: EditorManager
  readonly models: ModelRegistry
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
  // window-pane-store loads here too: its pane actions import this module.
  loading ??= Promise.all([
    import('@/features/editor/lib/monaco-adapters'),
    import('@/features/panes/stores/window-pane-store'),
  ]).then(
    ([adapters, { windowPaneStore }]) => {
      // The uri names the whole document, so the lookup spans every buffer in the window.
      const meta: BufferMeta = {
        lang: adapters.langForUri,
        text: (uri) => {
          for (const b of windowPaneStore.getState().buffers) {
            if (isEditorContent(b) && b.path && documentUri(documentRefOf(b)) === uri) {
              return b.content
            }
          }
          return ''
        },
      }
      const models = new ModelRegistry(adapters.realModelApi())
      const editors = adapters.realEditorApi(adapters.EDITOR_CREATE_OPTIONS)
      host = { manager: new EditorManager(editors, models, meta), models }
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
