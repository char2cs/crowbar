import { act, cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { bufferSession } from '@/entries/buffer-session'
import { holding } from '@/core/stream/stream'
import { setWorkspaceScope } from '@/lib/workspace-scope'

vi.mock('@/features/agent/hooks/use-chat-entity', () => ({
  useChatField: (_chatId: string, pick: (chat: unknown) => unknown) => pick({ workspaceId: 'ws1' }),
}))
vi.mock('@/features/panes/components/pane-editor-view-content', () => ({
  PaneEditorViewContent: () => null,
}))
const { saveActiveBuffer } = vi.hoisted(() => ({ saveActiveBuffer: vi.fn() }))
vi.mock('@/features/editor/lib/buffer-save', () => ({
  saveActiveBuffer,
  saveAllDirtyBuffers: vi.fn(),
  saveBuffer: vi.fn(),
}))

import type { OpenBuffer } from '@/entries/open-buffer'

const { BufferPaneHost } = await import('@/entries/buffer-pane-host')

afterEach(() => {
  cleanup()
  bufferSession.setState({ chatId: null, saved: null, active: false })
  windowPaneStore.setState(windowPaneStore.getInitialState(), true)
})

function withOpenTab(s: ReturnType<typeof windowPaneStore.getState>) {
  const buffer: PaneContent = { id: 'b1', type: 'newTabPage', name: 'a.ts', workspaceId: 'ws1' }
  const pane = makePane(ROOT_PANE_ID, null, { editorTabIds: ['b1'] })
  return { buffers: [...s.buffers, buffer], panes: { ...s.panes, [ROOT_PANE_ID]: pane } }
}

function withDirtyTab(s: ReturnType<typeof windowPaneStore.getState>) {
  const next = withOpenTab(s)
  const dirty = { ...next.buffers[0]!, type: 'editor', isDirty: true } as PaneContent
  return { ...next, buffers: [dirty] }
}

function client() {
  return {
    setTitle: vi.fn(),
    markDirty: vi.fn(),
    markClean: vi.fn(),
    onSave: vi.fn(() => () => undefined),
    setState: vi.fn(),
    refuseOpen: vi.fn(),
  }
}

type Shell = ReturnType<typeof client>
type Case = {
  behaviour: string
  tab: typeof withOpenTab
  act?: () => void
  expectation: (shell: Shell, open: ReturnType<typeof vi.fn>) => void
}

const cases: Case[] = [
  {
    behaviour: 'opens the saved buffer in the chat workspace',
    tab: withOpenTab,
    expectation: (_shell, open) =>
      expect(open).toHaveBeenCalledWith('ws1', { path: 'a.ts' }, expect.any(Function)),
  },
  {
    behaviour: 'titles the pane with the tab',
    tab: withOpenTab,
    expectation: (shell) => expect(shell.setTitle).toHaveBeenCalledWith('a.ts'),
  },
  {
    behaviour: 'reports unsaved edits to the shell',
    tab: withDirtyTab,
    expectation: (shell) => expect(shell.markDirty).toHaveBeenCalled(),
  },
  {
    behaviour: 'saves the pane buffer on mod+s, which no shell key reaches inside a pane web view',
    tab: withDirtyTab,
    act: () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true })),
    expectation: () => expect(saveActiveBuffer).toHaveBeenCalledTimes(1),
  },
]

describe('BufferPaneHost', () => {
  it.each(cases)('$behaviour', async ({ tab, act, expectation }) => {
    const shell = client()
    const open = vi.fn(() => {
      windowPaneStore.setState(tab)
      return Promise.resolve('b1')
    })
    bufferSession.setState({ chatId: 'c1', saved: { path: 'a.ts' }, active: false })
    await renderHost(shell, open)
    act?.()
    expectation(shell, open)
  })
})

describe('BufferPaneHost feeds', () => {
  it('holds the workspace ws: scope, which streams the review threads in', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1', owningChatId: 'c1' })
    bufferSession.setState({ chatId: 'c1', saved: null, active: true })
    await renderHost(
      client(),
      vi.fn(() => Promise.resolve('b1')),
    )
    await waitFor(() => expect(holding('local', 'ws:ws1')).toBe(true))
  })
})

describe('BufferPaneHost opening a file the user just opened', () => {
  const FRESH = { path: 'a.ts', fresh: true }
  const NOT_FOUND = { title: 'File not found', message: 'a.ts' }

  it('hands a refused read to the shell, which closes the pane', async () => {
    const shell = client()
    const open = vi.fn((_ws: string, _saved: unknown, refuse: (r: typeof NOT_FOUND) => void) => {
      refuse(NOT_FOUND)
      return Promise.resolve(null)
    })
    bufferSession.setState({ chatId: 'c1', saved: FRESH, active: false })
    await renderHost(shell, open)
    await waitFor(() => expect(shell.refuseOpen).toHaveBeenCalledWith(NOT_FOUND))
    expect(shell.setState).not.toHaveBeenCalled()
  })

  it('once open, saves the file as one to restore with the layout, without its line', async () => {
    const shell = client()
    bufferSession.setState({ chatId: 'c1', saved: { ...FRESH, line: 3 }, active: false })
    await renderHost(
      shell,
      vi.fn(() => Promise.resolve('b1')),
    )
    await waitFor(() => expect(shell.setState).toHaveBeenCalledWith({ path: 'a.ts' }))
    expect(shell.refuseOpen).not.toHaveBeenCalled()
  })
})

async function renderHost(shell: Shell, open: OpenBuffer) {
  await act(async () => {
    render(<BufferPaneHost client={shell} open={open} />)
  })
}
