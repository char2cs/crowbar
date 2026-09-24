import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readWorkspaceFile } = vi.hoisted(() => ({ readWorkspaceFile: vi.fn() }))
vi.mock('@/features/file-system/controllers/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/file-system/controllers/platform')>()),
  readWorkspaceFile,
}))

import { reloadBufferFromDisk } from '@/features/editor/lib/reload-buffer'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'

function openBuffer(content: string): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path: 'src/a.ts',
    name: 'a.ts',
    content,
    workspaceId: 'ws-1',
  })
}

function edit(id: string, content: string) {
  windowPaneStore.setState((s) => ({
    buffers: s.buffers.map((b) =>
      b.id === id && b.type === 'editor'
        ? { ...b, content, isDirty: content !== b.savedContent }
        : b,
    ),
  }))
}

function buffer(id: string): EditorContent {
  return windowPaneStore.getState().buffers.find((b) => b.id === id) as EditorContent
}

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
})

describe('reloadBufferFromDisk (P0-9)', () => {
  it('reloads a clean buffer from disk without asking', async () => {
    const id = openBuffer('old')
    readWorkspaceFile.mockResolvedValue('disk')
    const confirm = vi.fn(() => true)

    expect(await reloadBufferFromDisk(id, confirm)).toBe('reloaded')

    expect(confirm).not.toHaveBeenCalled()
    expect(readWorkspaceFile).toHaveBeenCalledWith('ws-1', 'src/a.ts')
    expect(buffer(id)).toMatchObject({ content: 'disk', savedContent: 'disk', isDirty: false })
  })

  it('never marks unsaved edits as saved: a declined prompt keeps them dirty', async () => {
    const id = openBuffer('old')
    edit(id, 'unsaved edit')
    readWorkspaceFile.mockResolvedValue('disk')

    expect(await reloadBufferFromDisk(id, () => false)).toBe('kept')

    expect(readWorkspaceFile).not.toHaveBeenCalled()
    expect(buffer(id)).toMatchObject({
      content: 'unsaved edit',
      savedContent: 'old',
      isDirty: true,
    })
  })

  it('a confirmed reload of a dirty buffer takes the disk content', async () => {
    const id = openBuffer('old')
    edit(id, 'unsaved edit')
    readWorkspaceFile.mockResolvedValue('disk')

    expect(await reloadBufferFromDisk(id, () => true)).toBe('reloaded')

    expect(buffer(id)).toMatchObject({ content: 'disk', savedContent: 'disk', isDirty: false })
  })

  it('does not overwrite edits typed while the disk read was in flight', async () => {
    const id = openBuffer('old')
    let finish: (text: string) => void = () => {}
    readWorkspaceFile.mockImplementation(() => new Promise<string>((r) => (finish = r)))

    const pending = reloadBufferFromDisk(id, () => true)
    edit(id, 'typed meanwhile')
    finish('disk')

    expect(await pending).toBe('kept')
    expect(buffer(id)).toMatchObject({ content: 'typed meanwhile', isDirty: true })
  })
})
