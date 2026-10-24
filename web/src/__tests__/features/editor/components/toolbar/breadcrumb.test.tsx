import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import Breadcrumb from '@/features/editor/components/toolbar/breadcrumb'
import { fetchFileTree } from '@/features/files/lib/file-tree-api'
import { openWorkspaceFile } from '@/features/files/lib/file-tree-handlers'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

vi.mock('@/features/files/lib/file-tree-handlers', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  openWorkspaceFile: vi.fn(() => Promise.resolve()),
}))
vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  fetchFileTree: vi.fn(() => Promise.resolve([])),
}))

// The pane shows ws-b's file while ws-a is the active workspace.
function renderForeignBuffer(path: string) {
  const bufferId = windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path,
    name: path.split('/').pop()!,
    content: '# readme',
    workspaceId: 'ws-b',
  })
  setActiveWorkspaceId('ws-a')
  render(<Breadcrumb bufferId={bufferId} />)
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
      expect(openWorkspaceFile).toHaveBeenCalledWith('ws-b', 'docs/README.md', { preview: true }),
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
    expect(preview?.workspaceId).toBe('ws-b')
  })
})
