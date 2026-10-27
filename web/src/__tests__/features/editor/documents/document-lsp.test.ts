import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { editor as monacoEditor, languages, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { attachDocumentLsp } from '@/features/editor/documents/document-lsp'
import { documentUri } from '@/features/editor/documents/document-ref'
import { loadEditorHost, reopenDocument, type EditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import type { LspDiagnostic } from '@/features/editor/lsp/lsp-client'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { __resetWorkspaceScopesForTest, setWorkspaceScope } from '@/lib/workspace-scope'

// The document, not the pane, is what the language server knows: one open per
// document, addressed to the document's own workspace, whichever is active.
type DiagnosticsHandler = (path: string, diagnostics: LspDiagnostic[]) => void
const { client, handlers } = vi.hoisted(() => {
  const handlers = new Map<string, Set<DiagnosticsHandler>>()
  const client = {
    documentOpen: vi.fn(async (..._args: unknown[]) => {}),
    documentClose: vi.fn(async (..._args: unknown[]) => {}),
    scheduleChange: vi.fn((_ws: string, _path: string, _read: () => string | null) => {}),
    onDiagnosticsUpdate: vi.fn((ws: string, handler: DiagnosticsHandler) => {
      const set = handlers.get(ws) ?? new Set()
      handlers.set(ws, set.add(handler))
      return () => set.delete(handler)
    }),
  }
  return { client, handlers }
})
vi.mock('@/features/editor/lsp/lsp-client', () => ({
  LSP_MARKER_OWNER: 'crowbar-lsp',
  LspClient: { getInstance: () => client },
}))

const A = workspaceRef('w1', 'src/a.ts')
const opens = () => client.documentOpen.mock.calls.map((c) => c.slice(0, 2))
const closes = () => client.documentClose.mock.calls.map((c) => c.slice(0, 2))
const modelOf = (ref: DocumentRef) => monacoEditor.getModel(Uri.parse(documentUri(ref)))!
const markers = (ref: DocumentRef) =>
  monacoEditor.getModelMarkers({ resource: Uri.parse(documentUri(ref)), owner: 'crowbar-lsp' })

const push = (ws: string, path: string, message: string) => {
  const range = { start: { line: 0, character: 0 }, end: { line: 0, character: 3 } }
  const diagnostic = { filePath: path, range, severity: 'error', message }
  for (const handler of [...(handlers.get(ws) ?? [])]) handler(path, [diagnostic])
}

// Runs the idle-deferred opens; a mounted editor's caret blink would keep runAll spinning.
const idle = () => vi.runOnlyPendingTimersAsync()

const mounted: string[] = []
function show(host: EditorHost, paneId: string, doc: DocumentRef) {
  host.manager.mountPane(paneId, document.body.appendChild(document.createElement('div')))
  mounted.push(paneId)
  applyActiveBuffer({ manager: host.manager, registry: activeEditors }, paneId, { doc })
}

let host: EditorHost
beforeEach(async () => {
  host = await loadEditorHost()
  resetWindowPaneStoreForTests()
  __resetWorkspaceScopesForTest()
  setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w1', owningChatId: 'chat-1' })
  setActiveWorkspaceId('w1')
  vi.useFakeTimers()
  for (const fn of Object.values(client)) fn.mockClear()
}, MONACO_LOAD_MS)

afterEach(() => {
  for (const paneId of mounted.splice(0)) host.manager.unmountPane(paneId)
  for (const uri of ['src/a.ts', 'untitled:Untitled-1'].map((p) => workspaceRef('w1', p)))
    host.documents.release(documentUri(uri))
  host.documents.release(documentUri({ authority: 'host', path: '/tmp/s5/a.ts' }))
  document.body.replaceChildren()
  vi.useRealTimers()
})

describe('document language-server sync', () => {
  it('opens a document with the language server once, however many panes show it', async () => {
    await openTestDocument(A, 'let a = 1')
    show(host, 'p1', A)
    show(host, 'p2', A)
    await idle()

    expect(opens()).toEqual([['w1', 'src/a.ts']])
    expect(client.documentOpen.mock.calls[0]?.slice(2)).toEqual(['let a = 1', 'typescript'])
    host.manager.closeBuffer('p2', documentUri(A))
    await idle()
    expect(closes()).toEqual([])
  })

  it('sends changes after the idle-deferred open and closes on release', async () => {
    await openTestDocument(A, 'let a = 1')
    modelOf(A).setValue('let a = 2 // before the open')
    expect(client.scheduleChange).not.toHaveBeenCalled()
    await idle()
    expect(client.documentOpen.mock.calls[0]?.[2]).toBe('let a = 2 // before the open')

    modelOf(A).setValue('let a = 3')
    const read = client.scheduleChange.mock.calls[0]?.[2]
    expect(client.scheduleChange.mock.calls.map((c) => c.slice(0, 2))).toEqual([['w1', 'src/a.ts']])
    expect(read?.()).toBe('let a = 3')

    host.documents.release(documentUri(A))
    expect(closes()).toEqual([['w1', 'src/a.ts']])
  })

  it('a document released before its idle open never reaches the server', async () => {
    await openTestDocument(A, 'let a = 1')
    host.documents.release(documentUri(A))
    await idle()

    expect([opens(), closes()]).toEqual([[], []])
  })

  it('reopens with the new language when the model language changes', async () => {
    languages.register({ id: 'python' })
    const id = await openTestDocument(A, 'x = 1')
    await idle()

    // What the pane's model options do when the user picks a language.
    windowPaneStore.setState((state) => ({
      buffers: state.buffers.map((b) => (b.id === id ? { ...b, languageOverride: 'python' } : b)),
    }))
    monacoEditor.setModelLanguage(modelOf(A), 'python')
    await idle()

    expect(closes()).toEqual([['w1', 'src/a.ts']])
    expect(client.documentOpen.mock.calls.map((c) => c[3])).toEqual(['typescript', 'python'])
  })

  it('a document reopened in place opens again with its new text', async () => {
    await openTestDocument(A, 'disk')
    await idle()

    reopenDocument(documentUri(A), () => host.documents.open(A, 'typescript', 'draft', 'disk'))
    await idle()

    expect(client.documentOpen.mock.calls.map((c) => c[2])).toEqual(['disk', 'draft'])
    expect(closes()).toEqual([['w1', 'src/a.ts']])
    host.documents.release(documentUri(A))
    expect(closes()).toHaveLength(2)
  })

  it('never contacts the language server for a host document', async () => {
    const doc: DocumentRef = { authority: 'host', path: '/tmp/s5/a.ts' }
    await openTestDocument(doc, 'let a = 1')
    modelOf(doc).setValue('let a = 2')
    await idle()
    host.documents.release(documentUri(doc))

    expect(Object.values(client).map((fn) => fn.mock.calls.length)).toEqual([0, 0, 0, 0])
  })

  it('never contacts the language server for an untitled buffer', async () => {
    const doc = workspaceRef('w1', 'untitled:Untitled-1')
    await openTestDocument(doc, 'let a = 1', { isVirtual: true })
    await idle()

    expect(Object.values(client).map((fn) => fn.mock.calls.length)).toEqual([0, 0, 0, 0])
  })

  it('a home workspace’s document has no language server', () => {
    setWorkspaceScope({ projectId: 'p', repoId: '', wsId: 'home' })
    const model = monacoEditor.createModel('a', 'typescript')

    attachDocumentLsp(workspaceRef('home', 'notes.ts'), model)()
    model.dispose()

    expect(client.onDiagnosticsUpdate).not.toHaveBeenCalled()
  })

  it('addresses its own workspace after another becomes active', async () => {
    await openTestDocument(A, 'let a = 1')
    setActiveWorkspaceId('w2')
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w2', owningChatId: 'chat-2' })
    await idle()

    expect(opens()).toEqual([['w1', 'src/a.ts']])
    expect(client.onDiagnosticsUpdate.mock.calls.map((c) => c[0])).toEqual(['w1'])
  })

  it('puts pushed diagnostics on the model and clears them on release', async () => {
    await openTestDocument(A, 'let a = 1')
    show(host, 'p1', A)
    show(host, 'p2', A)
    push('w1', 'src/a.ts', 'boom')
    push('w1', 'src/other.ts', 'elsewhere')

    expect(markers(A).map((m) => m.message)).toEqual(['boom'])
    expect(client.onDiagnosticsUpdate).toHaveBeenCalledTimes(1)

    host.documents.release(documentUri(A))
    expect(markers(A)).toEqual([])
    expect(handlers.get('w1')?.size ?? 0).toBe(0)
  })
})
