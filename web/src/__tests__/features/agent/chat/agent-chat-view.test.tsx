import { createElement, type ComponentProps } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { ledger, seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import * as commands from '@/features/agent/api/agent-chat-commands'
import * as reads from '@/features/agent/api/agent-chat-reads'
import { AgentChatView } from '@/features/agent/chat/agent-chat-view'
import { loadPromptStashes } from '@/features/agent/composer/lib/prompt-stash-persistence'
import { applyFeedFrame, chatStore } from '@/features/agent/stores/chat-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// jsdom never delivers a keydown to a Slate editable, so the box is a textarea
// with the same contract: text in, markdown out, keys through.
vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: ({
    initialValue,
    ariaLabel,
    onChange,
    onKeyDown,
  }: {
    initialValue: string
    ariaLabel: string
    onChange: (value: string) => void
    onKeyDown: (event: unknown, read: () => string, caret: object) => void
  }) =>
    createElement('textarea', {
      'aria-label': ariaLabel,
      defaultValue: initialValue,
      onChange: (event: { target: { value: string } }) => onChange(event.target.value),
      onKeyDown: (event: { currentTarget: { value: string } }) =>
        onKeyDown(event, () => event.currentTarget.value, { atStart: true, atEnd: true }),
    }),
}))

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
    <WorkspaceStoreContext.Provider value={createWorkspaceStore('w1')}>
      <AgentChatView {...props} />
    </WorkspaceStoreContext.Provider>,
  )
}

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

    const box = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(box, { target: { value: 'what changed?' } })
    fireEvent.keyDown(box, { key: 'Enter' })
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

    const box = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(box, { target: { value: 'rename snake\\_case\\_file.py' } })
    fireEvent.keyDown(box, { key: 'Enter' })
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    expect(submit.mock.calls[0]?.[2]).toBe('rename snake_case_file.py')
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

    const box = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(box, { target: { value: 'next one' } })
    fireEvent.keyDown(box, { key: 'Enter' })

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

    const box = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(box, { target: { value: '/compact' } })
    fireEvent.keyDown(box, { key: 'Enter' })

    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith('w1', 'c1', '/compact', expect.any(String), '', '', ''),
    )
  })

  it('takes a long first prompt in a fresh chat without an update loop', async () => {
    const submit = vi
      .spyOn(commands, 'submitAgentPrompt')
      .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
    const consoleError = vi.spyOn(console, 'error')
    const long = Array.from({ length: 60 }, (_, line) => `line ${line + 1} of the count`).join('\n')
    const l = ledger('c1')
    seedFeed('c1', [])
    draw('c1')

    const box = screen.getByRole('textbox', { name: /describe the change/i })
    fireEvent.change(box, { target: { value: long } })
    fireEvent.keyDown(box, { key: 'Enter' })
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    const requestId = String(submit.mock.calls[0]?.[3])
    act(() => seedFeed('c1', [l.prompt(requestId, long), l.start('turn-1', requestId)]))
    act(() => seedFeed('c1', [l.text('turn-1', 'm1', 0, '1\n2\n3', true), l.end('turn-1')]))

    expect(submit.mock.calls[0]?.[2]).toBe(long)
    expect(await screen.findByText(/line 60 of the count/)).toBeInTheDocument()
    expect(String(consoleError.mock.calls)).not.toMatch(/Maximum update depth/)
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

    const box = () => screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(box(), { target: { value: 'never mind' } })
    fireEvent.keyDown(box(), { key: 'Enter' })
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
    const requestId = String(submit.mock.calls[0]?.[3])
    act(() => seedFeed('c1', [l.prompt(requestId, 'never mind')]))
    await waitFor(() => expect(box()).toHaveValue(''))

    fireEvent.keyDown(box(), { key: 'Escape' })

    expect(retract).toHaveBeenCalledWith('w1', 'c1', requestId)
    await waitFor(() => expect(box()).toHaveValue('never mind'))
  })

  it('puts the taken-back text after what was typed while the retract was out', async () => {
    let answer: (value: { state: 'retracted'; text: string }) => void = () => {}
    vi.spyOn(commands, 'retractPrompt').mockReturnValue(
      new Promise((resolve) => (answer = resolve)) as never,
    )
    const l = ledger('c1', 10)
    seedFeed('c1', [...spokenIn('c1'), l.prompt('r1', 'take back')])
    draw('c1')
    const box = () => screen.getByRole('textbox', { name: /message the agent/i })

    fireEvent.keyDown(box(), { key: 'Escape' })
    fireEvent.change(box(), { target: { value: 'new thought' } })
    await act(async () => answer({ state: 'retracted', text: 'take back' }))

    expect(box()).toHaveValue('new thought\n\ntake back')
  })

  it('keeps taken-back text as a stash when the composer closed before the answer', async () => {
    let answer: (value: { state: 'retracted'; text: string }) => void = () => {}
    vi.spyOn(commands, 'retractPrompt').mockReturnValue(
      new Promise((resolve) => (answer = resolve)) as never,
    )
    const l = ledger('c1', 10)
    seedFeed('c1', [...spokenIn('c1'), l.prompt('r1', 'keep me')])
    const view = draw('c1')

    fireEvent.keyDown(screen.getByRole('textbox', { name: /message the agent/i }), {
      key: 'Escape',
    })
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
      const box = screen.getByRole('textbox', { name: /message the agent/i })
      fireEvent.change(box, { target: { value: 'held' } })
      fireEvent.keyDown(box, { key: 'Enter' })
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

  const type = (...keys: string[]) => {
    for (const key of keys) fireEvent.keyDown(window, { key })
  }
  const box = () => screen.getByRole('textbox', { name: /message the agent/i })

  it('does not redirect on three keys or fewer', () => {
    draw('c1')
    type('h', 'e', 'l')
    expect(box()).toHaveValue('')
  })

  it('ignores a modifier-held key and leaves the buffer for the plain keys after it', async () => {
    draw('c1')
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    type('h', 'e', 'l', 'l')
    await waitFor(() => expect(box()).toHaveValue('hell'))
  })
})
