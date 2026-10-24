import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import {
  applyExternalText,
  documentText,
  markDocumentSaved,
  onExternalText,
  readDocumentText,
  registerPendingWriter,
  replaceDocumentText,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import { findDocumentBuffer, openDocument } from '@/features/editor/documents/open-document'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

// The documents live in Monaco models: the first load is the slow part, not the tests.
beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

async function open(path: string, text: string): Promise<DocumentRef> {
  const ref = workspaceRef('ws-1', path)
  readDocument.mockResolvedValueOnce({ ref, text })
  await openDocument(ref)
  return ref
}

const isDirty = (ref: DocumentRef) => findDocumentBuffer(ref)?.isDirty

beforeEach(() => {
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
})

describe('document text', () => {
  it('returns the text of an open document and null for one that is not open', async () => {
    const ref = await open('src/a.ts', 'on disk')

    expect(documentText(ref)).toBe('on disk')
    expect(savedDocumentText(ref)).toBe('on disk')
    expect(documentText(workspaceRef('ws-2', 'src/a.ts'))).toBeNull()
    expect(savedDocumentText(workspaceRef('ws-2', 'src/a.ts'))).toBeNull()
  })

  it('a user replace marks the buffer dirty; replacing back to the saved text marks it clean', async () => {
    const ref = await open('src/a.ts', 'saved')

    replaceDocumentText(ref, 'edited')
    expect(documentText(ref)).toBe('edited')
    expect(isDirty(ref)).toBe(true)

    replaceDocumentText(ref, 'saved')
    expect(isDirty(ref)).toBe(false)
  })

  it('external text applied as saved leaves the buffer clean and notifies only external listeners', async () => {
    const ref = await open('src/a.ts', 'saved')
    const other = await open('src/b.ts', 'other')
    const heard = vi.fn()
    const otherHeard = vi.fn()
    const stop = onExternalText(ref, heard)
    onExternalText(other, otherHeard)

    replaceDocumentText(ref, 'typed')
    expect(heard).not.toHaveBeenCalled()

    applyExternalText(ref, 'from disk', { saved: true })
    expect(heard).toHaveBeenCalledTimes(1)
    expect(otherHeard).not.toHaveBeenCalled()
    expect(documentText(ref)).toBe('from disk')
    expect(savedDocumentText(ref)).toBe('from disk')
    expect(isDirty(ref)).toBe(false)

    applyExternalText(ref, 'formatted', { saved: false })
    expect(documentText(ref)).toBe('formatted')
    expect(savedDocumentText(ref)).toBe('from disk')
    expect(isDirty(ref)).toBe(true)

    stop()
    applyExternalText(ref, 'again', { saved: true })
    expect(heard).toHaveBeenCalledTimes(2)
  })

  it('notifies nobody when no open document holds the ref', () => {
    const ref = workspaceRef('ws-1', 'closed.md')
    const heard = vi.fn()
    const stop = onExternalText(ref, heard)

    applyExternalText(ref, 'from a watch', { saved: true })

    expect(heard).not.toHaveBeenCalled()
    expect(documentText(ref)).toBeNull()
    stop()
  })

  it('markDocumentSaved cleans the buffer only if the text still matches', async () => {
    const ref = await open('src/a.ts', 'v0')

    replaceDocumentText(ref, 'v1')
    markDocumentSaved(ref, 'v1')
    expect(isDirty(ref)).toBe(false)
    expect(savedDocumentText(ref)).toBe('v1')

    replaceDocumentText(ref, 'v2')
    replaceDocumentText(ref, 'v3')
    markDocumentSaved(ref, 'v2')
    expect(isDirty(ref)).toBe(true)
    expect(savedDocumentText(ref)).toBe('v2')
    expect(documentText(ref)).toBe('v3')
  })

  it('readDocumentText includes edits a pending writer has not flushed yet', async () => {
    const ref = await open('notes.md', 'saved')
    let unflushed: string | null = 'typed, not flushed'
    const stop = registerPendingWriter(ref, () => {
      if (unflushed !== null) replaceDocumentText(ref, unflushed)
      unflushed = null
    })

    await expect(readDocumentText(ref)).resolves.toBe('typed, not flushed')
    expect(isDirty(ref)).toBe(true)

    stop()
    unflushed = 'after unregistering'
    expect(documentText(ref)).toBe('typed, not flushed')
  })

  it('readDocumentText waits until the document is open', async () => {
    const ref = workspaceRef('ws-1', 'later.md')
    const read = readDocumentText(ref)

    await open('later.md', 'opened later')

    await expect(read).resolves.toBe('opened later')
  })
})
