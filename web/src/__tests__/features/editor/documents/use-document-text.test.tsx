import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))

import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { replaceDocumentText } from '@/features/editor/documents/document-text'
import { useDocumentText } from '@/features/editor/documents/use-document-text'
import { openDocument } from '@/features/editor/documents/open-document'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

let renders = 0

function Reader({ doc }: { doc: DocumentRef | null }) {
  renders += 1
  return <output>{useDocumentText(doc) ?? 'not open'}</output>
}

async function open(path: string, text: string): Promise<DocumentRef> {
  const ref = workspaceRef('ws-1', path)
  readDocument.mockResolvedValueOnce({ ref, text })
  await openDocument(ref)
  return ref
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
  renders = 0
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useDocumentText', () => {
  it('shows the open text at once, and null for a document that is not open', async () => {
    const ref = await open('a.md', 'first')

    const { rerender } = render(<Reader doc={ref} />)
    expect(screen.getByRole('status')).toHaveTextContent('first')

    rerender(<Reader doc={workspaceRef('ws-1', 'closed.md')} />)
    expect(screen.getByRole('status')).toHaveTextContent('not open')
  })

  it('follows a replacement within 150 ms', async () => {
    const ref = await open('a.md', 'first')
    vi.useFakeTimers()
    render(<Reader doc={ref} />)

    act(() => replaceDocumentText(ref, 'second'))
    act(() => vi.advanceTimersByTime(150))

    expect(screen.getByRole('status')).toHaveTextContent('second')
  })

  it('renders at most twice for 20 replacements inside 150 ms', async () => {
    const ref = await open('a.md', 'first')
    vi.useFakeTimers()
    render(<Reader doc={ref} />)
    renders = 0

    for (let i = 1; i <= 20; i++) {
      act(() => {
        replaceDocumentText(ref, `edit ${i}`)
        vi.advanceTimersByTime(7)
      })
    }
    act(() => vi.advanceTimersByTime(300))

    expect(renders).toBeLessThanOrEqual(2)
    expect(screen.getByRole('status')).toHaveTextContent('edit 20')
  })
})
