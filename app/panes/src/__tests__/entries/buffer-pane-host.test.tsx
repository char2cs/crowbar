import { act, cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { bufferSession } from '@/entries/buffer-session'

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
    expectation: (_shell, open) => expect(open).toHaveBeenCalledWith('ws1', { path: 'a.ts' }),
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

async function renderHost(shell: Shell, open: () => Promise<string>) {
  await act(async () => {
    render(<BufferPaneHost client={shell} open={open} />)
  })
}
