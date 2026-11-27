import { documentUri } from '@/features/editor/documents/document-ref'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { DocumentRef } from '@/lib/api/documents'
import { heldScopes, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'

export const WATCH_LIMIT = {
  code: 'watch_limit',
  message: 'watch: the OS has no file watches left',
} as const

export const WATCH_REFUSED = { code: 'unavailable', message: 'watch: refused' } as const

export const MONACO_LOAD_MS = 120_000

interface OpenOptions {
  saved?: string
  paneId?: string
  name?: string
  isVirtual?: boolean
  isPreview?: boolean
}

export async function openTestDocument(
  doc: DocumentRef,
  text: string,
  opts: OpenOptions = {},
): Promise<string> {
  const { documents } = await loadEditorHost()
  const uri = documentUri(doc)
  documents.open(doc, 'plaintext', text, opts.saved ?? text)
  const id = windowPaneStore.getState().bufferActions.openContent(
    {
      type: 'editor',
      doc,
      name: opts.name ?? doc.path,
      isVirtual: opts.isVirtual,
      isPreview: opts.isPreview,
    },
    { paneId: opts.paneId },
  )
  setDocumentDirty(uri, documents.isDirty(uri))
  return id
}

export function hostWatches(opened: (scope: string) => Promise<void> = () => Promise.resolve()) {
  const w = { calls: [] as string[], opened, held: 0 }
  streamDaemon.answer = ({ op, scope }) => {
    if (op !== 'sub') return []
    w.calls.push('subscribe')
    w.opened(scope).then(
      () => (w.calls.push('opened'), streamDaemon.deliver({ t: 'synced', scope })),
      (problem: unknown) =>
        streamDaemon.deliver({ t: 'closed', scope, reason: 'refused', problem }),
    )
    return []
  }
  return Object.defineProperty(w, 'held', { get: () => heldScopes('doc:').length })
}
