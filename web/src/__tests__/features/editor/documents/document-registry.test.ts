import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import {
  DocumentRegistry,
  type DocumentListener,
} from '@/features/editor/documents/document-registry'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import {
  cancelPendingWrites,
  noteDocumentEdited,
  setDocumentDirty,
} from '@/features/editor/lib/buffer-save'
import { EDITOR_CREATE_OPTIONS, realModelApi } from '@/features/editor/lib/monaco-adapters'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { workspaceRef } from '@/lib/api/documents'

const DOC = workspaceRef('ws-reg', 'src/doc.ts')
const URI = documentUri(DOC)

/** Records every call, and hands each to the app's own handler as the host does. */
function recordingListener() {
  return {
    dirtyChanged: vi.fn(setDocumentDirty),
    contentChanged: vi.fn(noteDocumentEdited),
    opened: vi.fn(),
    released: vi.fn(cancelPendingWrites),
  } satisfies DocumentListener
}

let listener: ReturnType<typeof recordingListener>
let documents: DocumentRegistry

const live = () => monacoEditor.getModel(Uri.parse(URI))!

/** One undo step, as a keystroke makes. */
function edit(text: string, range = live().getFullModelRange().collapseToEnd()) {
  const model = live()
  model.pushStackElement()
  model.pushEditOperations([], [{ range, text }], () => null)
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  listener = recordingListener()
  documents = new DocumentRegistry(realModelApi(), listener)
})

afterEach(() => {
  documents.release(URI)
  document.body.replaceChildren()
})

describe('DocumentRegistry', () => {
  it('a burst of edits reports one dirty transition and no other store write', () => {
    windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'editor', doc: DOC, name: 'doc.ts' })
    documents.open(DOC, 'typescript', 'saved')
    const writes = vi.fn()
    const stop = windowPaneStore.subscribe(writes)

    for (let i = 0; i < 50; i++) edit(String(i % 10))

    stop()
    expect(listener.dirtyChanged).toHaveBeenCalledTimes(1)
    expect(listener.dirtyChanged).toHaveBeenCalledWith(URI, true)
    expect(listener.contentChanged).toHaveBeenCalledTimes(50)
    expect(writes).toHaveBeenCalledTimes(1)
  })

  it('undo back to the saved version marks the document clean', () => {
    documents.open(DOC, 'typescript', 'saved')
    edit(' one')
    edit(' two')

    live().undo()
    live().undo()

    expect(documents.isDirty(URI)).toBe(false)
    expect(listener.dirtyChanged.mock.calls).toEqual([
      [URI, true],
      [URI, false],
    ])
  })

  it('typing a character and deleting it leaves the document clean', () => {
    documents.open(DOC, 'typescript', 'saved')
    edit('x')
    const end = live().getFullModelRange()
    edit('', end.setStartPosition(end.endLineNumber, end.endColumn - 1))

    expect(live().getValue()).toBe('saved')
    expect(documents.isDirty(URI)).toBe(false)
  })

  it('a save marks clean only if the model still holds what was written', () => {
    documents.open(DOC, 'typescript', 'v0')
    documents.replace(URI, 'v1')
    documents.markSaved(URI, 'v1')
    expect(documents.isDirty(URI)).toBe(false)

    documents.replace(URI, 'v2')
    documents.markSaved(URI, 'v1')

    expect(documents.isDirty(URI)).toBe(true)
    expect(documents.savedText(URI)).toBe('v1')
    expect(documents.text(URI)).toBe('v2')
  })

  it('applyExternal keeps the caret of every editor showing the model', () => {
    documents.open(DOC, 'typescript', 'line one\nline two\n')
    const model = live()
    const [a, b] = [0, 1].map(() =>
      monacoEditor.create(document.body.appendChild(document.createElement('div')), {
        ...EDITOR_CREATE_OPTIONS,
        model,
      }),
    )
    a.setPosition({ lineNumber: 2, column: 9 })
    b.setPosition({ lineNumber: 1, column: 3 })
    const heard = vi.fn()
    documents.onDidApplyExternal(URI, heard)

    documents.applyExternal(URI, 'line one\nline 2\n', true)

    expect(a.getPosition()).toMatchObject({ lineNumber: 2, column: 7 })
    expect(b.getPosition()).toMatchObject({ lineNumber: 1, column: 3 })
    expect(heard).toHaveBeenCalledTimes(1)
    expect(documents.isDirty(URI)).toBe(false)
    expect(listener.dirtyChanged).not.toHaveBeenCalled()
    a.dispose()
    b.dispose()
  })

  it('disk text the document already holds changes nothing, so no listener hears it', () => {
    documents.open(DOC, 'typescript', 'same')
    const heard = vi.fn()
    documents.onDidApplyExternal(URI, heard)

    documents.applyExternal(URI, 'same', true)

    expect(heard).not.toHaveBeenCalled()
    expect(listener.contentChanged).not.toHaveBeenCalled()
    documents.applyExternal(URI, 'other', true)
    expect(heard).toHaveBeenCalledTimes(1)
  })

  it('the save mark moves on with every save and every open, so an older read is told apart', () => {
    expect(documents.saveMark(URI)).toBe(-1)
    documents.open(DOC, 'typescript', 'v0')
    const opened = documents.saveMark(URI)

    documents.markSaved(URI, 'v0')
    const saved = documents.saveMark(URI)
    documents.release(URI)
    documents.open(DOC, 'typescript', 'v0')

    expect(saved).not.toBe(opened)
    expect(documents.saveMark(URI)).not.toBe(saved)
    expect(documents.saveMark(URI)).not.toBe(opened)
  })

  it('tells the listener of a release while the model is still alive', () => {
    documents.open(DOC, 'plaintext', 'x')
    const model = live()
    let aliveWhenTold = false
    listener.released.mockImplementation(() => (aliveWhenTold = !model.isDisposed()))

    documents.release(URI)

    expect(aliveWhenTold).toBe(true)
    expect(model.isDisposed()).toBe(true)
  })

  it(
    'a document lives as long as its buffer, not its pane',
    async () => {
      const host = await loadEditorHost()
      const doc = workspaceRef('ws-reg', 'src/shared.ts')
      const uri = documentUri(doc)
      const { paneActions, bufferActions, activePaneId } = windowPaneStore.getState()
      const id = await openTestDocument(doc, 'shared')
      const other = paneActions.splitPane(activePaneId, 'horizontal')!
      bufferActions.openContent({ type: 'editor', doc, name: 'shared.ts' }, { paneId: other })

      paneActions.removeEditorTabFromPane(activePaneId, id)
      expect(host.documents.text(uri)).toBe('shared')

      paneActions.removeEditorTabFromPane(other, id)
      expect(host.documents.has(uri)).toBe(false)
      expect(monacoEditor.getModel(Uri.parse(uri))).toBeNull()
    },
    MONACO_LOAD_MS,
  )
})
