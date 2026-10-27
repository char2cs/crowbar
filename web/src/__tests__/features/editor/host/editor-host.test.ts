import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Monaco from 'monaco-editor'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import {
  getEditorHost,
  loadEditorHost,
  releaseWithDocument,
  reopenDocument,
  type EditorHost,
} from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { documentUri } from '@/features/editor/documents/document-ref'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import { applyWorkspaceEdit } from '@/features/editor/lsp/workspace-edit'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef } from '@/lib/api/documents'

// Real Monaco, no mocks: the host's whole job is the one model table every
// pane shares.
const mounted: string[] = []

const open = (workspaceId: string, path: string, text: string) =>
  openTestDocument(workspaceRef(workspaceId, path), text)

function show(host: EditorHost, paneId: string, ws: string, path: string) {
  if (!mounted.includes(paneId)) {
    host.manager.mountPane(paneId, document.body.appendChild(document.createElement('div')))
    mounted.push(paneId)
  }
  applyActiveBuffer({ manager: host.manager, registry: activeEditors }, paneId, {
    doc: workspaceRef(ws, path),
  })
}

function shownModel(host: EditorHost, paneId: string): Monaco.editor.ITextModel | null {
  return (host.manager.getRawEditor(paneId) as Monaco.editor.ICodeEditor).getModel()
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
})

afterEach(() => {
  for (const paneId of mounted.splice(0)) getEditorHost()?.manager.unmountPane(paneId)
  document.body.replaceChildren()
})

describe('editor host', () => {
  it(
    'hands every caller the same host',
    async () => {
      const [a, b] = await Promise.all([loadEditorHost(), loadEditorHost()])
      expect(a).toBe(b)
      expect(getEditorHost()).toBe(a)
    },
    MONACO_LOAD_MS,
  )

  it(
    'keeps two workspaces’ files at the same path apart',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', 'src/x.ts', 'A')
      await open('ws-b', 'src/x.ts', 'B')

      show(host, 'p1', 'ws-a', 'src/x.ts')
      show(host, 'p2', 'ws-b', 'src/x.ts')

      expect(shownModel(host, 'p1')?.getValue()).toBe('A')
      expect(shownModel(host, 'p2')?.getValue()).toBe('B')
    },
    MONACO_LOAD_MS,
  )

  it(
    'a pane keeps its model when the buffer’s workspace store is destroyed',
    async () => {
      const host = await loadEditorHost()
      getOrCreateWorkspaceStore('ws-a')
      await open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', 'ws-a', 'src/x.ts')

      destroyWorkspaceStore('ws-a')

      expect(host.documents.text(documentUri(workspaceRef('ws-a', 'src/x.ts')))).toBe('A')
      expect(shownModel(host, 'p1')?.isDisposed()).toBe(false)
    },
    MONACO_LOAD_MS,
  )

  it(
    'a hidden dirty document takes a workspace edit and shows it beside its own edits',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', '/repo/a.ts', 'const one = 1\n')
      await open('ws-a', '/repo/b.ts', 'B')
      show(host, 'p1', 'ws-a', '/repo/a.ts')
      const typed = shownModel(host, 'p1')!
      typed.applyEdits([{ range: typed.getFullModelRange().collapseToEnd(), text: '// mine' }])
      show(host, 'p1', 'ws-a', '/repo/b.ts')

      const rename = { start: { line: 0, character: 6 }, end: { line: 0, character: 9 } }
      await applyWorkspaceEdit(
        { changes: { 'file:///repo/a.ts': [{ range: rename, newText: 'uno' }] } },
        'ws-a',
      )
      show(host, 'p1', 'ws-a', '/repo/a.ts')

      expect(shownModel(host, 'p1')?.getValue()).toBe('const uno = 1\n// mine')
    },
    MONACO_LOAD_MS,
  )

  it(
    're-showing the document a pane already shows leaves its scroll and caret alone',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', 'ws-a', 'src/x.ts')
      const restore = vi.spyOn(
        host.manager.getRawEditor('p1') as Monaco.editor.ICodeEditor,
        'restoreViewState',
      )

      show(host, 'p1', 'ws-a', 'src/x.ts')

      expect(restore).not.toHaveBeenCalled()
    },
    MONACO_LOAD_MS,
  )

  it(
    'a pane shows a document again once it reopens under the same uri',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', 'ws-a', 'src/x.ts')
      host.documents.release(documentUri(workspaceRef('ws-a', 'src/x.ts')))
      await open('ws-a', 'src/x.ts', 'reopened')

      show(host, 'p1', 'ws-a', 'src/x.ts')

      expect(shownModel(host, 'p1')?.getValue()).toBe('reopened')
    },
    MONACO_LOAD_MS,
  )

  it(
    'a pane keeps its model when another pane closes the same file',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', 'ws-a', 'src/x.ts')
      show(host, 'p2', 'ws-a', 'src/x.ts')

      host.manager.closeBuffer('p2', documentUri(workspaceRef('ws-a', 'src/x.ts')))

      const model = shownModel(host, 'p1')
      expect(model?.isDisposed()).toBe(false)
      expect(model?.getValue()).toBe('A')
    },
    MONACO_LOAD_MS,
  )

  it(
    'what is released with a document goes once, with that document alone',
    async () => {
      const host = await loadEditorHost()
      await open('ws-a', 'src/x.ts', 'X')
      await open('ws-a', 'src/y.ts', 'Y')
      const [x, y] = ['src/x.ts', 'src/y.ts'].map((p) => documentUri(workspaceRef('ws-a', p)))
      const [first, second, other] = [vi.fn(), vi.fn(), vi.fn()]
      releaseWithDocument(x, first)
      releaseWithDocument(x, second)
      releaseWithDocument(y, other)

      host.documents.release(x)
      host.documents.release(x)

      expect([first, second].map((f) => f.mock.calls.length)).toEqual([1, 1])
      expect(other).not.toHaveBeenCalled()
      host.documents.release(y)
      expect(other).toHaveBeenCalledTimes(1)
    },
    MONACO_LOAD_MS,
  )

  it(
    'what is held with a document that is not open goes at once',
    async () => {
      await loadEditorHost()
      const dispose = vi.fn()

      releaseWithDocument(documentUri(workspaceRef('ws-a', 'src/none.ts')), dispose)

      expect(dispose).toHaveBeenCalledTimes(1)
    },
    MONACO_LOAD_MS,
  )

  it(
    'a document reopened in place keeps what is released with it',
    async () => {
      const host = await loadEditorHost()
      const doc = workspaceRef('ws-a', 'src/x.ts')
      const uri = documentUri(doc)
      await open('ws-a', 'src/x.ts', 'disk')
      const dispose = vi.fn()
      releaseWithDocument(uri, dispose)

      reopenDocument(uri, () => host.documents.open(doc, 'plaintext', 'draft', 'disk'))

      expect(dispose).not.toHaveBeenCalled()
      expect(host.documents.text(uri)).toBe('draft')
      host.documents.release(uri)
      expect(dispose).toHaveBeenCalledTimes(1)
    },
    MONACO_LOAD_MS,
  )

  it(
    'a reopen that fails lets go of what was held with the document',
    async () => {
      const host = await loadEditorHost()
      const uri = documentUri(workspaceRef('ws-a', 'src/x.ts'))
      await open('ws-a', 'src/x.ts', 'disk')
      const dispose = vi.fn()
      releaseWithDocument(uri, dispose)

      expect(() =>
        reopenDocument(uri, () => {
          throw new Error('no model')
        }),
      ).toThrow('no model')

      expect(host.documents.has(uri)).toBe(false)
      expect(dispose).toHaveBeenCalledTimes(1)
    },
    MONACO_LOAD_MS,
  )
})
