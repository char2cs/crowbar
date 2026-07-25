import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-1',
}))
vi.mock('@/features/files/lib/file-tree-api', () => ({
  copyFileNode: vi.fn(),
  renameFileNode: vi.fn(),
}))

import { copyFileNode, renameFileNode } from '@/features/files/lib/file-tree-api'
import { useFileClipboardStore } from '@/features/file-explorer/file-explorer/stores/file-explorer-clipboard-store'

const mockCopy = copyFileNode as ReturnType<typeof vi.fn>
const mockRename = renameFileNode as ReturnType<typeof vi.fn>

const actions = () => useFileClipboardStore.getState().actions

beforeEach(async () => {
  vi.clearAllMocks()
  mockCopy.mockResolvedValue(undefined)
  mockRename.mockResolvedValue(undefined)
  await actions().clear()
})

// Paste used to resolve [] unconditionally (crowbar-bridge's clipboardPaste was
// a `return []` stub behind a "FUTURE:" comment). Cmd+C/Cmd+X staged the
// clipboard — Cut even greyed the node out — and then Cmd+V moved nothing, with
// no error: the user was shown a completed move that never happened.
describe('file clipboard paste', () => {
  it('copies each staged entry into the target directory', async () => {
    await actions().copy([{ path: 'src/a.ts', is_dir: false }])

    const results = await actions().paste('dest')

    expect(mockCopy).toHaveBeenCalledWith('ws-1', 'src/a.ts', 'dest/a.ts')
    expect(results).toEqual([
      {
        source_path: 'src/a.ts',
        destination_path: 'dest/a.ts',
        is_dir: false,
        success: true,
      },
    ])
  })

  it('keeps a copy clipboard staged so it can be pasted again', async () => {
    await actions().copy([{ path: 'src/a.ts', is_dir: false }])
    await actions().paste('dest')
    expect(useFileClipboardStore.getState().clipboard?.operation).toBe('copy')
  })

  it('moves a cut entry with the atomic rename verb, not copy+delete', async () => {
    await actions().cut([{ path: 'src/a.ts', is_dir: false }])

    await actions().paste('dest')

    expect(mockRename).toHaveBeenCalledWith('ws-1', 'src/a.ts', 'dest/a.ts')
    expect(mockCopy).not.toHaveBeenCalled()
  })

  it('consumes a cut entry once its move lands', async () => {
    await actions().cut([{ path: 'src/a.ts', is_dir: false }])
    await actions().paste('dest')
    expect(useFileClipboardStore.getState().clipboard).toBeNull()
  })

  it('pastes into the workspace root when the target directory is empty', async () => {
    await actions().copy([{ path: 'src/a.ts', is_dir: false }])
    await actions().paste('')
    expect(mockCopy).toHaveBeenCalledWith('ws-1', 'src/a.ts', 'a.ts')
  })

  it('reports a rejected entry as failed and leaves it staged for a retry', async () => {
    mockRename.mockRejectedValueOnce(new Error('409 destination exists'))
    await actions().cut([
      { path: 'src/a.ts', is_dir: false },
      { path: 'src/b.ts', is_dir: false },
    ])

    const results = await actions().paste('dest')

    expect(results.map((r) => r.success)).toEqual([false, true])
    expect(useFileClipboardStore.getState().clipboard?.entries).toEqual([
      { path: 'src/a.ts', is_dir: false },
    ])
  })

  it('does nothing with an empty clipboard', async () => {
    const results = await actions().paste('dest')
    expect(results).toEqual([])
    expect(mockCopy).not.toHaveBeenCalled()
    expect(mockRename).not.toHaveBeenCalled()
  })
})
