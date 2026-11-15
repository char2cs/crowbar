import { beforeEach, describe, expect, it, vi } from 'vitest'

const opened = vi.hoisted(() => ({ document: vi.fn(), untitled: vi.fn() }))

vi.mock('@/features/editor/documents/open-document', () => ({
  openDocument: opened.document,
  openUntitledDocument: opened.untitled,
}))

const { openEditorBuffer } = await import('@/entries/open-editor-buffer')

beforeEach(() => vi.clearAllMocks())

const ROOT = { paneId: 'root-pane' }

const CASES = [
  {
    name: 'a workspace file in the workspace of the chat',
    saved: { path: 'src/a.ts' },
    spec: { authority: 'workspace', workspaceId: 'ws1', path: 'src/a.ts' },
  },
  {
    name: 'a picked file by its absolute path',
    saved: { path: '/home/a.ts', absolute: true },
    spec: { absolutePath: '/home/a.ts' },
  },
]

describe('openEditorBuffer', () => {
  it.each(CASES)('opens $name', async ({ saved, spec }) => {
    await openEditorBuffer('ws1', saved)
    expect(opened.document).toHaveBeenCalledWith(spec, ROOT)
  })

  it('opens an untitled file', async () => {
    await openEditorBuffer('ws1', { untitled: true })
    expect(opened.untitled).toHaveBeenCalledWith('ws1', 'root-pane')
  })

  it('refuses a pane that saved no file', async () => {
    await expect(openEditorBuffer('ws1', null)).rejects.toThrow(/no file/)
  })
})
