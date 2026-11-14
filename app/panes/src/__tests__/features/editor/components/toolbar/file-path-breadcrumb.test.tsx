import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchFileTree, openDocument } = vi.hoisted(() => ({
  fetchFileTree: vi.fn(),
  openDocument: vi.fn(async () => {}),
}))
vi.mock('@/features/files/lib/file-tree-api', () => ({ fetchFileTree }))
vi.mock('@/features/editor/documents/open-document', () => ({ openDocument }))

import { FilePathBreadcrumb } from '@/features/editor/components/toolbar/file-path-breadcrumb'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef } from '@/lib/api/documents'

// The buffer shown may belong to a workspace other than the one in view (panes are
// window-level), so every listing and navigation must name the buffer's own.
describe('FilePathBreadcrumb', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActiveWorkspaceId('ws-in-view')
    fetchFileTree.mockResolvedValue([{ name: 'a.ts', path: 'web/a.ts', isDir: false }])
  })

  it('lists a segment’s directory, and opens a file from it, in the buffer’s own workspace', async () => {
    render(<FilePathBreadcrumb workspaceId="ws-buffer" filePath="web/src/b.ts" />)

    await userEvent.click(screen.getByRole('button', { name: /web/ }))
    expect(fetchFileTree).toHaveBeenCalledExactlyOnceWith('ws-buffer', 'web')

    await userEvent.click(await screen.findByText('a.ts'))
    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledExactlyOnceWith(workspaceRef('ws-buffer', 'web/a.ts'), {
        preview: true,
      }),
    )
  })
})
