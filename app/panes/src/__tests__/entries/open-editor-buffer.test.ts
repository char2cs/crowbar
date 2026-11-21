import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

const opened = vi.hoisted(() => ({ document: vi.fn(), untitled: vi.fn(), restore: vi.fn() }))

vi.mock('@/features/editor/documents/open-document', () => ({
  openDocument: opened.document,
  openUntitledDocument: opened.untitled,
}))
vi.mock('@/features/editor/documents/restore-documents', () => ({
  restoreDocuments: opened.restore,
}))

const { openEditorBuffer } = await import('@/entries/open-editor-buffer')

beforeEach(() => {
  vi.clearAllMocks()
  const { panes } = windowPaneStore.getState()
  windowPaneStore.setState({ panes: { ...panes, [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null) } })
})
afterEach(() => windowPaneStore.setState(windowPaneStore.getInitialState(), true))

const refuse = vi.fn()

const FRESH = [
  {
    name: 'a workspace file in the workspace of the chat',
    saved: { path: 'src/a.ts', fresh: true },
    spec: { authority: 'workspace', workspaceId: 'ws1', path: 'src/a.ts' },
  },
  {
    name: 'a picked file by its absolute path',
    saved: { path: '/home/a.ts', absolute: true, fresh: true },
    spec: { absolutePath: '/home/a.ts' },
  },
]

const RESTORED = [
  {
    name: 'a workspace file',
    saved: { path: 'src/a.ts' },
    doc: { authority: 'workspace', workspaceId: 'ws1', path: 'src/a.ts' },
  },
  {
    name: 'a picked file',
    saved: { path: '/home/a.ts', absolute: true },
    doc: { authority: 'host', path: '/home/a.ts' },
  },
]

function rootTabs() {
  const { panes, buffers } = windowPaneStore.getState()
  const ids = panes[ROOT_PANE_ID]?.editorTabIds ?? []
  return buffers.filter((b) => ids.includes(b.id))
}

describe('openEditorBuffer', () => {
  it.each(FRESH)(
    'reads $name the user just opened, handing a refusal back',
    async ({ saved, spec }) => {
      await openEditorBuffer('ws1', saved, refuse)
      expect(opened.document).toHaveBeenCalledWith(spec, {
        paneId: ROOT_PANE_ID,
        onRefused: refuse,
      })
      expect(opened.restore).not.toHaveBeenCalled()
    },
  )

  it.each(RESTORED)(
    'restores $name into a tab before reading it, so a file that is gone keeps its tab',
    async ({ saved, doc }) => {
      await openEditorBuffer('ws1', saved, refuse)
      expect(rootTabs()).toEqual([expect.objectContaining({ type: 'editor', path: doc.path })])
      expect(opened.restore).toHaveBeenCalledTimes(1)
      expect(opened.document).not.toHaveBeenCalled()
    },
  )

  it('opens an untitled file', async () => {
    await openEditorBuffer('ws1', { untitled: true }, refuse)
    expect(opened.untitled).toHaveBeenCalledWith('ws1', 'root-pane')
  })

  it('refuses a pane that saved no file', async () => {
    await expect(openEditorBuffer('ws1', null, refuse)).rejects.toThrow(/no file/)
  })
})
