import type { ComponentProps } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import type { Value } from 'platejs'
import { PlateController, useEditorRef, type PlateEditor } from 'platejs/react'
import { ledger, seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import * as commands from '@/features/agent/api/agent-chat-commands'
import * as reads from '@/features/agent/api/agent-chat-reads'
import { AgentChatView } from '@/features/agent/chat/agent-chat-view'
import * as serialization from '@/features/agent/composer/plate/chat-composer-serialization'
import * as transcriptModule from '@/features/agent/transcript/agent-transcript'
import { loadPromptStashes } from '@/features/agent/composer/lib/prompt-stash-persistence'
import { applyFeedFrame, chatStore } from '@/features/agent/stores/chat-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// jsdom measures 0×0, and a zero-height viewport windows down to no rows.
const rect = { top: 0, left: 0, right: 768, bottom: 800, width: 768, height: 800, x: 0, y: 0 }
const originalRect = HTMLElement.prototype.getBoundingClientRect

let fetchSpy: MockInstance<typeof fetch>

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  localStorage.clear()
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
  fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'))
  vi.spyOn(reads, 'getChatTelemetry').mockRejectedValue(new Error('no telemetry'))
  vi.spyOn(reads, 'getSlashCatalog').mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalRect
  vi.restoreAllMocks()
})

function draw(chatId: string, overrides: Partial<ComponentProps<typeof AgentChatView>> = {}) {
  const props: ComponentProps<typeof AgentChatView> = {
    wsId: 'w1',
    chatId,
    providerId: 'codex',
    providers: [],
    live: true,
    canSend: true,
    active: true,
    visible: true,
    isActivePane: true,
    onOpenTerminal: () => {},
    provider: 'codex',
    model: '',
    effort: '',
    onSelectionChange: () => {},
    presentation: 'chat',
    splitEnabled: false,
    onSelectPresentation: () => {},
    ...overrides,
  }
  // An empty workspace store: no list in any store names this chat.
  return render(
    <PlateController>
      <WorkspaceStoreContext.Provider value={createWorkspaceStore('w1')}>
        <AgentChatView {...props} />
      </WorkspaceStoreContext.Provider>
      <GrabEditor />
    </PlateController>,
  )
}

// The real composer. jsdom has no `isContentEditable`, and Plate's hotkeys drop a
// key unless the box holds focus, so a key goes to a focused, editable box.
const grabbed: { editor?: PlateEditor } = {}
function GrabEditor() {
  grabbed.editor = useEditorRef()
  return null
}
const COMPOSER = /message the agent/i
const DOCUMENT = /describe the change/i
const box = (name = COMPOSER) => screen.getByRole('textbox', { name })
async function focusBox(name = COMPOSER) {
  const el = box(name)
  Object.defineProperty(el, 'isContentEditable', { value: true, configurable: true })
  await act(async () => el.focus())
  return el
}
async function type(text: string, name = COMPOSER) {
  await focusBox(name)
  await act(async () => grabbed.editor!.tf.insertText(text))
}
async function press(key: string, name = COMPOSER) {
  fireEvent.keyDown(await focusBox(name), { key })
}
const markdown = () => serialization.chatValueToMarkdown(grabbed.editor!.children as Value)

const transcriptReads = () =>
  fetchSpy.mock.calls.filter(([url]) => /\/(messages|activity)\b/.test(String(url)))

describe('AgentChatView', () => {
  it('symptom 4: renders rows for a chat no workspace lists', async () => {
    seedFeed('c-unlisted', spokenIn('c-unlisted', 'an answer from the record'))
    draw('c-unlisted')

    expect(await screen.findByText('an answer from the record')).toBeInTheDocument()
  })

  it('symptom 4: renders the reply to a sent prompt with no transcript request', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r0', 'earlier'), l.start('turn-0', 'r0'), l.end('turn-0')])
    draw('c1')

    await type('what changed?')
    await press('Enter')
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    const requestId = String(submit.mock.calls[0]?.[3])

    act(() =>
      seedFeed('c1', [
        l.prompt(requestId, 'what changed?'),
        l.start('turn-1', requestId),
        l.text('turn-1', 'm1', 0, 'Two files.', true),
        l.end('turn-1'),
      ]),
    )

    expect(await screen.findByText('Two files.')).toBeInTheDocument()
    await waitFor(() => expect(screen.getAllByText('what changed?')).toHaveLength(1))
    expect(transcriptReads()).toEqual([])
  })

  it('sends the draft without the escapes its markdown does not need', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    seedFeed('c1', spokenIn('c1'))
    draw('c1')

    await type('rename snake_case_file.py')
    await press('Enter')
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    expect(submit.mock.calls[0]?.[2]).toBe('rename snake_case_file.py')
  })

  it('shows Stopping… in the same render as the Stop click', async () => {
    const stop = vi.spyOn(commands, 'stopChat').mockReturnValue(new Promise(() => {}))
    const l = ledger('c1', 4)
    seedFeed('c1', [...spokenIn('c1'), l.start('turn-2')])
    draw('c1')

    fireEvent.click(await screen.findByRole('button', { name: 'Stop this turn' }))

    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Stopping…')
    expect(stop).toHaveBeenCalledWith('w1', 'c1')
    act(() => {
      applyFeedFrame('c1', {
        type: 'row',
        row: l.row(
          'turn_ended',
          { status: 'interrupted', reason: 'stopped' },
          { turnId: 'turn-2' },
        ),
      })
    })
    expect(screen.queryByText('Stopping…')).toBeNull()
    expect(screen.getByText('Interrupted · you stopped this turn')).toBeInTheDocument()
    // The next turn is not the one Stop was pressed on.
    act(() => void applyFeedFrame('c1', { type: 'row', row: l.start('turn-3') }))
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.queryByText('Stopping…')).toBeNull()
  })

  it('streams a reply the record has not reached yet', async () => {
    seedFeed('c1', spokenIn('c1'))
    draw('c1')

    act(() => {
      applyFeedFrame('c1', {
        type: 'delta',
        delta: { id: 'm2', turnId: 'turn-2', stream: 'answer', n: 1, text: 'Streaming now' },
      })
    })

    expect(await screen.findByText('Streaming now')).toBeInTheDocument()
  })

  // The record has not reached the reply yet, but the agent is plainly busy.
  it('holds a prompt while a reply streams ahead of its rows', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    seedFeed('c1', spokenIn('c1'))
    draw('c1')
    act(() => {
      applyFeedFrame('c1', {
        type: 'delta',
        delta: { id: 'm2', turnId: 'turn-2', stream: 'answer', n: 1, text: 'Streaming now' },
      })
    })

    await type('next one')
    await press('Enter')

    expect(await screen.findByTestId('queued-prompt')).toHaveAttribute('data-state', 'queued')
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(submit).not.toHaveBeenCalled()
  })

  it('sends a slash command the catalog cannot match, rather than swallowing Enter', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    seedFeed('c1', spokenIn('c1'))
    draw('c1')

    await type('/compact')
    await press('Enter')

    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith('w1', 'c1', '/compact', expect.any(String), '', '', ''),
    )
  })

  it('takes a long first prompt in a fresh chat without an update loop', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    const consoleError = vi.spyOn(console, 'error')
    const lines = Array.from({ length: 60 }, (_, line) => `line ${line + 1} of the count`)
    const long = lines.join('\n\n')
    const l = ledger('c1')
    seedFeed('c1', [])
    draw('c1')

    await focusBox(DOCUMENT)
    await act(async () =>
      lines.forEach((line, n) => {
        if (n > 0) grabbed.editor!.tf.insertBreak()
        grabbed.editor!.tf.insertText(line)
      }),
    )
    await press('Enter', DOCUMENT)
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    const requestId = String(submit.mock.calls[0]?.[3])
    act(() => seedFeed('c1', [l.prompt(requestId, long), l.start('turn-1', requestId)]))
    act(() => seedFeed('c1', [l.text('turn-1', 'm1', 0, '1\n2\n3', true), l.end('turn-1')]))

    expect(submit.mock.calls[0]?.[2]).toBe(long)
    expect(await screen.findByText(/line 60 of the count/)).toBeInTheDocument()
    expect(String(consoleError.mock.calls)).not.toMatch(/Maximum update depth/)
  })
})

describe('AgentChatView one composer', () => {
  const submitted = () =>
    vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })

  it('symptom 7: typing re-renders neither the transcript nor serializes the draft', async () => {
    const renders = vi.spyOn(transcriptModule, 'AgentTranscript')
    seedFeed('c1', spokenIn('c1'))
    draw('c1')
    const editable = await focusBox()
    const serialize = vi.spyOn(serialization, 'chatValueToMarkdown')
    renders.mockClear()

    for (const key of 'twenty typed letters') {
      fireEvent.keyDown(editable, { key })
      const end = grabbed.editor!.api.end([])
      await act(async () => grabbed.editor!.tf.insertText(key, { at: end }))
    }

    expect(editable).toHaveTextContent('twenty typed letters')
    expect(serialize).not.toHaveBeenCalled()
    expect(renders).not.toHaveBeenCalled()
  })

  it('serializes the draft exactly once on send', async () => {
    const submit = submitted()
    seedFeed('c1', spokenIn('c1'))
    draw('c1')
    const serialize = vi.spyOn(serialization, 'chatValueToMarkdown')
    await type('ship it')

    await press('Enter')

    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    expect(submit.mock.calls[0]?.[2]).toBe('ship it')
    expect(serialize).toHaveBeenCalledTimes(1)
  })

  it('symptom 5: the first prompt is typed into the one composer and sends through the outbox', async () => {
    const submit = submitted()
    seedFeed('c1', [])
    draw('c1')
    expect(screen.getAllByRole('textbox')).toEqual([box(DOCUMENT)])

    await type('add a flag', DOCUMENT)
    await press('Enter', DOCUMENT)

    expect(await screen.findByTestId('queued-prompt')).toHaveTextContent('add a flag')
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith('w1', 'c1', 'add a flag', expect.any(String), '', '', ''),
    )
    expect(screen.getAllByRole('textbox')).toEqual([box()])
  })

  it('keeps the composer mounted when the first send moves it to the dock', async () => {
    submitted()
    seedFeed('c1', [])
    draw('c1')
    const editable = box(DOCUMENT)

    await type('first words', DOCUMENT)
    await press('Enter', DOCUMENT)

    expect(await screen.findByTestId('queued-prompt')).toBeInTheDocument()
    expect(box()).toBe(editable)
    expect(markdown()).toBe('')
  })
})

describe('AgentChatView Esc', () => {
  it('symptom 5: Esc in an empty composer puts the retracted text back', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    const retract = vi
      .spyOn(commands, 'retractPrompt')
      .mockResolvedValue({ state: 'retracted', text: 'never mind' })
    const l = ledger('c1', 10)
    seedFeed('c1', spokenIn('c1'))
    draw('c1')

    await type('never mind')
    await press('Enter')
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    const requestId = String(submit.mock.calls[0]?.[3])
    act(() => seedFeed('c1', [l.prompt(requestId, 'never mind')]))
    expect(markdown()).toBe('')

    await press('Escape')

    expect(retract).toHaveBeenCalledWith('w1', 'c1', requestId)
    await waitFor(() => expect(markdown()).toBe('never mind'))
  })

  it('puts the taken-back text after what was typed while the retract was out', async () => {
    let answer: (value: { state: 'retracted'; text: string }) => void = () => {}
    vi.spyOn(commands, 'retractPrompt').mockReturnValue(
      new Promise((resolve) => (answer = resolve)) as never,
    )
    const l = ledger('c1', 10)
    seedFeed('c1', [...spokenIn('c1'), l.prompt('r1', 'take back')])
    draw('c1')

    await press('Escape')
    await type('new thought')
    const caret = { path: [0, 0], offset: 4 }
    act(() => grabbed.editor!.tf.select(caret))
    await act(async () => answer({ state: 'retracted', text: 'take back' }))

    expect(markdown()).toBe('new thought\n\ntake back')
    expect(grabbed.editor!.selection).toEqual({ anchor: caret, focus: caret })
  })

  // A drop or a menu paste edits the box with no key; the recalled-and-edited
  // prompt must still come back from the walk it abandoned.
  it('abandons a history walk on an edit that came with no key', async () => {
    const l = ledger('c1', 10)
    seedFeed('c1', [...spokenIn('c1'), l.prompt('r1', 'first'), l.prompt('r2', 'second')])
    draw('c1')
    const atEdge = (edge: 'start' | 'end') =>
      act(() => grabbed.editor!.tf.select(grabbed.editor!.api[edge]([])!))

    await press('ArrowUp')
    expect(markdown()).toBe('second')
    await act(async () =>
      grabbed.editor!.tf.insertText(' plus', { at: grabbed.editor!.api.end([]) }),
    )
    atEdge('start')
    await press('ArrowUp')
    atEdge('end')
    await press('ArrowDown')

    expect(markdown()).toBe('second plus')
  })

  it('keeps taken-back text as a stash when the composer closed before the answer', async () => {
    let answer: (value: { state: 'retracted'; text: string }) => void = () => {}
    vi.spyOn(commands, 'retractPrompt').mockReturnValue(
      new Promise((resolve) => (answer = resolve)) as never,
    )
    const l = ledger('c1', 10)
    seedFeed('c1', [...spokenIn('c1'), l.prompt('r1', 'keep me')])
    const view = draw('c1')

    await press('Escape')
    view.unmount()
    await act(async () => answer({ state: 'retracted', text: 'keep me' }))

    expect(loadPromptStashes('w1', 'c1').map((stash) => stash.markdown)).toEqual(['keep me'])
  })

  it('shows the terminal-wait banner only from the chat terminalWait', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const submit = vi
        .spyOn(commands, 'submitAgentPrompt')
        .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
      const l = ledger('c1', 10)
      seedFeed('c1', spokenIn('c1'))
      const first = draw('c1')
      await type('held')
      await press('Enter')
      await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
      act(() => seedFeed('c1', [l.prompt(String(submit.mock.calls[0]?.[3]), 'held')]))

      await act(async () => vi.advanceTimersByTime(60_000))
      expect(screen.queryByRole('button', { name: /open terminal/i })).not.toBeInTheDocument()
      expect(screen.queryByText(/only its terminal can give/i)).not.toBeInTheDocument()

      draw('c1', { terminalWaiting: true, terminalWaitKind: '' })
      first.unmount()
      expect(screen.getByText(/only its terminal can give/i)).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('AgentChatView type-to-focus', () => {
  beforeEach(() => {
    setActiveWorkspaceId('w1')
    seedFeed('c1', spokenIn('c1'))
  })

  // The box takes focus on mount; these keys land while something else has it.
  const keys = (...pressed: string[]) => {
    act(() => (document.activeElement as HTMLElement | null)?.blur())
    for (const key of pressed) fireEvent.keyDown(window, { key })
  }

  it('does not redirect on three keys or fewer', () => {
    draw('c1')
    keys('h', 'e', 'l')
    expect(markdown()).toBe('')
  })

  it('ignores a modifier-held key and leaves the buffer for the plain keys after it', async () => {
    draw('c1')
    keys()
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    keys('h', 'e', 'l', 'l')
    await waitFor(() => expect(markdown()).toBe('hell'))
  })
})
