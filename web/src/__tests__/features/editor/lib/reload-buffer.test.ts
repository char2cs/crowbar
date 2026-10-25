import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'

const { readDisk } = vi.hoisted(() => ({ readDisk: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument: async (ref: DocumentRef) => ({ ref, text: await readDisk(ref) }),
}))

import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import {
  documentText,
  replaceDocumentText,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import {
  findDocumentBuffer,
  setDocumentUnavailable,
} from '@/features/editor/documents/open-document'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { reloadBufferFromDisk } from '@/features/editor/lib/reload-buffer'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { toast } from '@/features/window/stores/toast-store'
import { ApiError } from '@/lib/api'

const A = workspaceRef('ws-1', 'src/a.ts')

const openBuffer = (text: string, doc: DocumentRef = A) => openTestDocument(doc, text)

function state(ref: DocumentRef = A) {
  return {
    text: documentText(ref),
    saved: savedDocumentText(ref),
    dirty: findDocumentBuffer(ref)?.isDirty,
  }
}

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
})

describe('reloadBufferFromDisk (P0-9)', () => {
  it('reads a tab whose document failed to load again, as a reopen does', async () => {
    const error = vi.spyOn(toast, 'error')
    const id = windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'editor', doc: A, name: 'a.ts' })
    setDocumentUnavailable(A, { title: 'Failed to open file', message: 'a.ts' })
    readDisk.mockRejectedValue(new ApiError('big', 413))

    expect(await reloadBufferFromDisk(id, () => true)).toBe('failed')
    const tooLarge = { title: 'File too large to open', message: 'a.ts' }
    expect(findDocumentBuffer(A)?.unavailable).toEqual(tooLarge)
    expect(error).toHaveBeenCalledWith(tooLarge.title, tooLarge.message)

    readDisk.mockResolvedValue('disk')
    expect(await reloadBufferFromDisk(id, () => true)).toBe('reloaded')
    expect(state()).toEqual({ text: 'disk', saved: 'disk', dirty: false })
    expect(findDocumentBuffer(A)?.unavailable).toBeUndefined()
    error.mockRestore()
  })

  it('reloads a clean buffer from disk without asking', async () => {
    const id = await openBuffer('old')
    readDisk.mockResolvedValue('disk')
    const confirm = vi.fn(() => true)

    expect(await reloadBufferFromDisk(id, confirm)).toBe('reloaded')

    expect(confirm).not.toHaveBeenCalled()
    expect(readDisk).toHaveBeenCalledWith(workspaceRef('ws-1', 'src/a.ts'))
    expect(state()).toEqual({ text: 'disk', saved: 'disk', dirty: false })
  })

  it('reloads a document outside every workspace from its own path', async () => {
    const id = await openBuffer('old', { authority: 'host', path: '/tmp/a.ts' })
    readDisk.mockResolvedValue('disk')

    expect(await reloadBufferFromDisk(id, () => true)).toBe('reloaded')

    expect(readDisk).toHaveBeenCalledWith({ authority: 'host', path: '/tmp/a.ts' })
  })

  it('never marks unsaved edits as saved: a declined prompt keeps them dirty', async () => {
    const id = await openBuffer('old')
    replaceDocumentText(A, 'unsaved edit')
    readDisk.mockResolvedValue('disk')

    expect(await reloadBufferFromDisk(id, () => false)).toBe('kept')

    expect(readDisk).not.toHaveBeenCalled()
    expect(state()).toEqual({ text: 'unsaved edit', saved: 'old', dirty: true })
  })

  it('a confirmed reload of a dirty buffer takes the disk content', async () => {
    const id = await openBuffer('old')
    replaceDocumentText(A, 'unsaved edit')
    readDisk.mockResolvedValue('disk')

    expect(await reloadBufferFromDisk(id, () => true)).toBe('reloaded')

    expect(state()).toEqual({ text: 'disk', saved: 'disk', dirty: false })
  })

  it('does not overwrite edits typed while the disk read was in flight', async () => {
    const id = await openBuffer('old')
    let finish: (text: string) => void = () => {}
    readDisk.mockImplementation(() => new Promise<string>((r) => (finish = r)))

    const pending = reloadBufferFromDisk(id, () => true)
    replaceDocumentText(A, 'typed meanwhile')
    finish('disk')

    expect(await pending).toBe('kept')
    expect(state()).toEqual({ text: 'typed meanwhile', saved: 'old', dirty: true })
  })
})
