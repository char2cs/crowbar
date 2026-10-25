import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readWorkspaceDirectory, openWorkspaceFile } = vi.hoisted(() => ({
  readWorkspaceDirectory: vi.fn(),
  openWorkspaceFile: vi.fn(async () => {}),
}))
vi.mock('@/features/file-system/controllers/platform', () => ({ readWorkspaceDirectory }))
vi.mock('@/features/files/lib/file-tree-handlers', () => ({ openWorkspaceFile }))

import { FilePathBreadcrumb } from '@/features/editor/components/toolbar/file-path-breadcrumb'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// The buffer shown may belong to a workspace other than the one in view (panes are
// window-level), so every listing and navigation must name the buffer's own.
describe('FilePathBreadcrumb', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActiveWorkspaceId('ws-in-view')
    readWorkspaceDirectory.mockResolvedValue([
      { name: 'a.ts', path: 'web/a.ts', is_dir: false, isDirectory: false, isFile: true },
    ])
  })

  it('lists a segment’s directory, and opens a file from it, in the buffer’s own workspace', async () => {
    render(<FilePathBreadcrumb workspaceId="ws-buffer" filePath="web/src/b.ts" />)

    await userEvent.click(screen.getByRole('button', { name: /web/ }))
    expect(readWorkspaceDirectory).toHaveBeenCalledExactlyOnceWith('ws-buffer', 'web')

    await userEvent.click(await screen.findByText('a.ts'))
    await waitFor(() =>
      expect(openWorkspaceFile).toHaveBeenCalledExactlyOnceWith('ws-buffer', 'web/a.ts', {
        preview: true,
      }),
    )
  })
})
