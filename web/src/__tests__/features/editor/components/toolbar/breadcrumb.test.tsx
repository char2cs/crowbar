import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import Breadcrumb from '@/features/editor/components/toolbar/breadcrumb'
import { fetchFileTree } from '@/features/files/lib/file-tree-api'
import { openDocument } from '@/features/editor/documents/open-document'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

vi.mock('@/features/editor/documents/open-document', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  openDocument: vi.fn(() => Promise.resolve(null)),
}))
vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  fetchFileTree: vi.fn(() => Promise.resolve([])),
}))

function renderBuffer(doc: DocumentRef) {
  const bufferId = windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc,
    name: doc.path.split('/').pop()!,
  })
  render(<Breadcrumb bufferId={bufferId} />)
}

// The pane shows ws-b's file while ws-a is the active workspace.
function renderForeignBuffer(path: string) {
  setActiveWorkspaceId('ws-a')
  renderBuffer(workspaceRef('ws-b', path))
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  vi.clearAllMocks()
})
afterEach(() => {
  cleanup()
  setActiveWorkspaceId(null)
})

describe('Breadcrumb acts on its own buffer’s workspace', () => {
  it('the file segment opens in the buffer’s workspace, not the active one', async () => {
    renderForeignBuffer('docs/README.md')

    fireEvent.click(screen.getByRole('button', { name: 'README.md' }))

    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledWith(workspaceRef('ws-b', 'docs/README.md'), {
        preview: true,
      }),
    )
  })

  it('a directory segment lists the buffer’s workspace', async () => {
    renderForeignBuffer('docs/README.md')

    fireEvent.click(screen.getByRole('button', { name: 'docs' }))

    await waitFor(() => expect(fetchFileTree).toHaveBeenCalledWith('ws-b', 'docs'))
  })

  it('Preview opens the preview in the buffer’s workspace', () => {
    renderForeignBuffer('docs/README.md')

    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))

    const preview = windowPaneStore.getState().buffers.find((b) => b.type === 'markdownPreview')
    expect(preview).toMatchObject({
      workspaceId: 'ws-b',
      source: workspaceRef('ws-b', 'docs/README.md'),
    })
  })
})

describe('Breadcrumb of a host document', () => {
  it('shows its path as plain segments', () => {
    renderBuffer({ authority: 'host', path: '/tmp/notes/a.md' })

    expect(screen.getByText('notes')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'notes' })).toBeNull()
  })

  it('previews it as a host document', () => {
    renderBuffer({ authority: 'host', path: '/tmp/notes/a.md' })

    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))

    const preview = windowPaneStore.getState().buffers.find((b) => b.type === 'markdownPreview')
    expect(preview).toMatchObject({
      workspaceId: '',
      source: { authority: 'host', path: '/tmp/notes/a.md' },
    })
  })
})
