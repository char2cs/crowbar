import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

function seed(rows: LedgerRow[]) {
  openFeed('chat-1')
  for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
}

const draw = (id: string) => render(<TranscriptRow chatId="chat-1" id={id} providers={[]} />)
const status = () => document.querySelector('[data-status]')?.getAttribute('data-status')

beforeEach(() => chatStore.setState({ feeds: {} }))
afterEach(() => vi.restoreAllMocks())

describe('ToolRow', () => {
  it('reads as running with its live output, then as done', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'tool-1', phase: 'pre', name: 'Bash', kind: 'execute', target: 'make test' },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    expect(status()).toBe('running')
    expect(screen.getByText('Running')).toBeInTheDocument()
    expect(screen.getByText('make test')).toBeInTheDocument()
    act(
      () =>
        void applyFeedFrame('chat-1', {
          type: 'delta',
          delta: delta('tool-1', 1, 'ok  pkg/one\n**PASS**', { stream: 'tool_output' }),
        }),
    )
    expect(screen.getByTestId('agent-tool-output')).toHaveTextContent('ok pkg/one PASS')
    act(
      () =>
        void applyFeedFrame('chat-1', {
          type: 'row',
          row: l.row(
            'tool_call',
            { toolId: 'tool-1', phase: 'post', durationMs: 1500 },
            { turnId: 'turn-1' },
          ),
        }),
    )
    expect(status()).toBe('ok')
    expect(screen.getByText('Ran')).toBeInTheDocument()
    expect(screen.getByText('1.5s')).toBeInTheDocument()
    expect(screen.queryByTestId('agent-tool-output')).toBeNull()
  })

  it('shows the last three segments of a path and keeps the whole in the tooltip', () => {
    const l = ledger()
    seed([
      l.row(
        'tool_call',
        {
          toolId: 'tool-1',
          phase: 'post',
          kind: 'read',
          locations: [{ path: '/a/b/c/d/e.ts', line: 4 }],
        },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    expect(screen.getByText('c/d/e.ts:4')).toHaveAttribute('title', '/a/b/c/d/e.ts:4')
  })

  it('reads a tool its turn closed on as abandoned', () => {
    const l = ledger()
    seed([l.start('turn-1'), l.tool('turn-1', 'tool-1', 'pre'), l.end('turn-1', 'interrupted')])
    draw('t:tool-1')
    expect(status()).toBe('abandoned')
  })

  it('reads its request and result by blob ref only once opened', async () => {
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async (input) =>
        String(input).includes(encodeURIComponent('sha256:in'))
          ? new Response('{"command":"ls"}')
          : new Response('gone', { status: 404 }),
      )
    const l = ledger()
    seed([
      l.row(
        'tool_call',
        {
          toolId: 'tool-1',
          phase: 'post',
          name: 'Bash',
          inputRef: 'sha256:in',
          resultRef: 'sha256:out',
        },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    expect(fetchSpy).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Show tool details' }))
    expect(await screen.findByText(/"command": "ls"/)).toBeInTheDocument()
    expect(await screen.findByText('No longer available')).toBeInTheDocument()
    expect(String(fetchSpy.mock.calls[0]?.[0])).toContain('/v0/chats/chat-1/blobs/sha256%3Ain')
  })
})

describe('AskRow', () => {
  it('records a resolved ask and draws nothing while it is pending', () => {
    const l = ledger()
    seed([
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          kind: 'tool_permission',
          toolName: 'Bash',
          answerable: true,
          options: [{ id: 'y', kind: 'allow' }],
        },
        { turnId: 'turn-1' },
      ),
    ])
    const { container } = draw('a:q1')
    expect(container).toBeEmptyDOMElement()
    act(
      () =>
        void applyFeedFrame('chat-1', {
          type: 'row',
          row: l.row('ask_resolved', { askId: 'q1', resolution: 'answered', optionIds: ['y'] }),
        }),
    )
    expect(screen.getByTestId('agent-turn-choices')).toHaveTextContent('Bash · Allow')
  })
})
