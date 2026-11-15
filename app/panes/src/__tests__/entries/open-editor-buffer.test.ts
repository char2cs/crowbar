import { beforeEach, describe, expect, it, vi } from 'vitest'

const opened = vi.hoisted(() => ({ document: vi.fn(), untitled: vi.fn() }))

vi.mock('@/features/editor/documents/open-document', () => ({
  openDocument: opened.document,
  openUntitledDocument: opened.untitled,
}))

const { openEditorBuffer } = await import('@/entries/open-editor-buffer')

beforeEach(() => vi.clearAllMocks())

describe('openEditorBuffer', () => {
  it('opens a workspace file in the workspace of the chat', async () => {
    await openEditorBuffer('ws1', { path: 'src/a.ts' })
    expect(opened.document).toHaveBeenCalledWith(
      { authority: 'workspace', workspaceId: 'ws1', path: 'src/a.ts' },
      { paneId: 'root-pane' },
    )
  })

  it('opens a picked file by its absolute path', async () => {
    await openEditorBuffer('ws1', { path: '/home/a.ts', absolute: true })
    expect(opened.document).toHaveBeenCalledWith(
      { absolutePath: '/home/a.ts' },
      { paneId: 'root-pane' },
    )
  })

  it('opens an untitled file', async () => {
    await openEditorBuffer('ws1', { untitled: true })
    expect(opened.untitled).toHaveBeenCalledWith('ws1', 'root-pane')
  })

  it('refuses a pane that saved no file', async () => {
    await expect(openEditorBuffer('ws1', null)).rejects.toThrow(/no file/)
  })
})
