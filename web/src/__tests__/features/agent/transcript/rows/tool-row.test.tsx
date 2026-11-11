import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow, ToolCallBody } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'

import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

const actions = noRowActions

function seed(rows: LedgerRow[]) {
  openFeed('chat-1')
  for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
}

const draw = (id: string) =>
  render(<TranscriptRow actions={actions} chatId="chat-1" id={id} providers={[]} />)
const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))
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

  // Each outcome reads differently at a glance, and a call that went well adds nothing.
  it('gives running a spinner, ok nothing, and each failure its own outcome', () => {
    const l = ledger()
    const call = (toolId: string, extra: Pick<ToolCallBody, 'phase'> & Partial<ToolCallBody>) =>
      l.row(
        'tool_call',
        { toolId, name: 'Bash', kind: 'execute', target: 'ls', ...extra },
        { turnId: 'turn-1' },
      )
    seed([
      l.start('turn-1'),
      call('run', { phase: 'pre' }),
      call('ok', { phase: 'post' }),
      call('bad', { phase: 'fail', error: 'boom' }),
      call('cut', { phase: 'fail', interrupted: true }),
      call('lost', { phase: 'pre' }),
    ])
    push(l.end('turn-1'))
    const row = (id: string) => {
      const view = render(
        <TranscriptRow actions={actions} chatId="chat-1" id={`t:${id}`} providers={[]} />,
      )
      const el = view.container
      return {
        spinner: el.querySelector('[data-flicker-spinner]'),
        glyph: el.querySelector('.tool-glyph'),
        outcome: el.querySelector('[data-outcome]')?.getAttribute('data-outcome'),
        text: el.querySelector('[data-outcome]')?.textContent,
      }
    }
    expect(row('ok')).toMatchObject({ spinner: null, glyph: null, outcome: undefined })
    expect(row('bad')).toMatchObject({ outcome: 'failed', text: 'Failed' })
    expect(row('cut')).toMatchObject({ outcome: 'stopped', text: 'Stopped' })
    expect(row('lost')).toMatchObject({ outcome: 'unavailable', text: 'Result unavailable' })
  })

  it('shows a live spinner only while the call runs', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'tool-1', phase: 'pre', name: 'Bash', kind: 'execute', target: 'ls' },
        { turnId: 'turn-1' },
      ),
    ])
    const { container } = draw('t:tool-1')
    expect(container.querySelector('[data-flicker-spinner]')).not.toBeNull()
    push(l.end('turn-1'))
    expect(container.querySelector('[data-flicker-spinner]')).toBeNull()
  })

  it('says a path outside the workspace whole, the tooltip with its line', () => {
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
    expect(screen.getByText('/a/b/c/d/e.ts L4')).toHaveAttribute('title', '/a/b/c/d/e.ts:4')
  })

  it('reads Result unavailable for a tool its turn abandoned', () => {
    const l = ledger()
    seed([l.start('turn-1'), l.tool('turn-1', 'tool-1', 'pre'), l.end('turn-1')])
    draw('t:tool-1')
    expect(status()).toBe('abandoned')
    expect(screen.getByText('Result unavailable')).toBeInTheDocument()
  })

  const edit = (diff: string) => ({
    toolId: 'tool-1',
    phase: 'post' as const,
    name: 'Edit',
    kind: 'edit',
    target: '/repo/README.md',
    diff,
  })
  const lines = (count: number) => Array.from({ length: count }, (_, i) => `+line ${i}`).join('\n')

  it('opens an edit diff by default when it changes 12 lines or fewer', () => {
    const l = ledger()
    seed([l.row('tool_call', edit(`@@ -0,0 +1,12 @@\n${lines(12)}`), { turnId: 'turn-1' })])
    draw('t:tool-1')
    expect(screen.getByTestId('agent-tool-diff')).toBeInTheDocument()
    expect(screen.getByText('+12 −0')).toBeInTheDocument()
  })

  it('keeps a larger edit diff closed until it is opened', () => {
    const l = ledger()
    seed([l.row('tool_call', edit(`@@ -0,0 +1,13 @@\n${lines(13)}`), { turnId: 'turn-1' })])
    draw('t:tool-1')
    expect(screen.queryByTestId('agent-tool-diff')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Show tool details' }))
    expect(screen.getByTestId('agent-tool-diff')).toBeInTheDocument()
  })

  it('shows a finished call’s short output without opening it, colour codes stripped', () => {
    const l = ledger()
    const call = { toolId: 'tool-1', name: 'Bash', kind: 'execute', target: 'ls' }
    const output = 'one\n\u001b[32mtwo\u001b[0m'
    seed([l.row('tool_call', { ...call, phase: 'post', output }, { turnId: 'turn-1' })])
    draw('t:tool-1')
    expect(screen.getByTestId('agent-tool-short-output').textContent).toBe('one\ntwo')
  })

  it('shows the first error line of a failed tool inline', () => {
    const l = ledger()
    const call = { toolId: 'tool-1', name: 'Bash', kind: 'execute', target: 'cat missing.txt' }
    seed([
      l.row(
        'tool_call',
        { ...call, phase: 'fail', error: 'cat: missing.txt: No such file\nexit status 1' },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    expect(status()).toBe('error')
    expect(screen.getByText('cat: missing.txt: No such file')).toBeInTheDocument()
    expect(screen.queryByText(/exit status 1/)).toBeNull()
  })

  it('shows a neutral exit chip for a non-zero exit with output', () => {
    const l = ledger()
    const call = {
      toolId: 'tool-1',
      name: 'Bash',
      kind: 'execute',
      title: 'Find x',
      target: 'grep x y',
    }
    seed([
      l.row(
        'tool_call',
        { ...call, phase: 'post', status: 'ok', exitCode: 1, resultRef: 'sha256:out' },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    expect(status()).toBe('ok')
    expect(screen.getByText('exit 1')).toBeInTheDocument()
    expect(screen.getByText('$ grep x y')).toBeInTheDocument()
  })

  it('reads Stopped for a tool an interrupt cut off', () => {
    const l = ledger()
    const call = { toolId: 'tool-1', name: 'Bash', kind: 'execute', target: 'sleep 9' }
    seed([l.row('tool_call', { ...call, phase: 'fail', interrupted: true }, { turnId: 'turn-1' })])
    draw('t:tool-1')
    expect(status()).toBe('interrupted')
    expect(screen.getByText('Stopped')).toBeInTheDocument()
  })

  it('names an MCP server as the noun of its row', () => {
    const l = ledger()
    const call = { toolId: 'tool-1', kind: 'mcp', name: 'mcp__echo__shout', mcpServer: 'echo' }
    seed([l.row('tool_call', { ...call, mcpTool: 'shout', phase: 'post' }, { turnId: 'turn-1' })])
    draw('t:tool-1')
    expect(screen.getByTestId('agent-mcp-server')).toHaveTextContent('echo')
    expect(screen.getByTestId('agent-tool-label')).toHaveTextContent('echo · shout')
  })

  // A denied permission stops the tool before it runs, and no post/fail row
  // follows: the answer to its ask is the only record of what happened.
  // `refused` is the daemon answering on the person's behalf (an auto-deny).
  const denials = {
    'denied by the person': { resolution: 'answered', optionIds: ['deny'], note: 'Denied' },
    'refused by the daemon': { resolution: 'refused', optionIds: [], note: 'No answer in time' },
  } as const
  it.each(Object.entries(denials))(
    'reads a tool %s as denied, not ran',
    (_, { note, ...answer }) => {
      const l = ledger()
      const call = { toolId: 'tool-1', name: 'Bash', kind: 'execute', target: 'echo deny' }
      seed([
        l.start('turn-1'),
        l.row('tool_call', { ...call, phase: 'pre' }, { turnId: 'turn-1' }),
        l.row(
          'ask',
          {
            askId: 'q1',
            event: 'permission',
            kind: 'tool_permission',
            toolId: 'tool-1',
            answerable: true,
            options: [
              { id: 'allow', kind: 'allow' },
              { id: 'deny', kind: 'deny' },
            ],
          },
          { turnId: 'turn-1' },
        ),
        l.row('ask_resolved', { askId: 'q1', ...answer, optionIds: [...answer.optionIds] }),
        l.end('turn-1'),
      ])
      draw('t:tool-1')
      expect(status()).toBe('declined')
      expect(screen.getByTestId('agent-tool-ask-note')).toHaveTextContent(note)
      expect(screen.getAllByText(/Denied|No answer in time/)).toHaveLength(1)
      expect(document.querySelector('[data-outcome="denied"]')).not.toBeNull()
      expect(screen.queryByText('Ran')).toBeNull()
    },
  )

  it('marks the bound tool row awaiting approval and replaces the ask with a one-line note once resolved', () => {
    const l = ledger()
    const reveal = vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(() => {})
    const started = l.start('turn-1')
    const call = l.row(
      'tool_call',
      { toolId: 'tool-1', phase: 'pre', name: 'Bash', kind: 'execute', target: 'ls' },
      { turnId: 'turn-1' },
    )
    const ask = l.row(
      'ask',
      {
        askId: 'q1',
        event: 'permission',
        kind: 'tool_permission',
        toolId: 'tool-1',
        answerable: true,
        options: [{ id: 'once', kind: 'allow', label: 'Allow once' }],
      },
      { turnId: 'turn-1' },
    )
    // The daemon binds an ask only to a call already open: the call comes first.
    seed([started, call, ask])
    expect(chatStore.getState().feeds['chat-1']?.order).toEqual(['t:tool-1'])
    draw('t:tool-1')
    expect(status()).toBe('awaiting_approval')
    expect(screen.getByText('Waiting for your approval')).toBeInTheDocument()
    expect(reveal).toHaveBeenCalled()
    act(
      () =>
        void applyFeedFrame('chat-1', {
          type: 'row',
          row: l.row('ask_resolved', { askId: 'q1', resolution: 'answered', optionIds: ['once'] }),
        }),
    )
    expect(status()).toBe('running')
    expect(screen.getByTestId('agent-tool-ask-note')).toHaveTextContent('Allowed · Allow once')
    expect(chatStore.getState().feeds['chat-1']?.order).toEqual(['t:tool-1'])
  })

  it('says a permission nobody answered in time on its call', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'tool-1', phase: 'pre', name: 'Bash', kind: 'execute', target: 'ls' },
        { turnId: 'turn-1' },
      ),
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          kind: 'tool_permission',
          toolId: 'tool-1',
          answerable: true,
        },
        { turnId: 'turn-1' },
      ),
      l.row('ask_resolved', { askId: 'q1', resolution: 'refused', optionIds: [] }),
    ])
    draw('t:tool-1')
    expect(screen.getByTestId('agent-tool-ask-note')).toHaveTextContent('No answer in time')
  })

  it('reads a stopped turn’s running call as stopped, an ended one’s as unavailable', () => {
    const l = ledger()
    const call = (toolId: string, turnId: string) =>
      l.row(
        'tool_call',
        { toolId, phase: 'pre', name: 'Bash', kind: 'execute', target: 'ls' },
        { turnId },
      )
    seed([
      l.start('turn-1'),
      call('tool-1', 'turn-1'),
      l.row('turn_ended', { status: 'interrupted', reason: 'stopped' }, { turnId: 'turn-1' }),
      l.start('turn-2'),
      call('tool-2', 'turn-2'),
      l.end('turn-2'),
    ])
    draw('t:tool-1')
    expect(status()).toBe('interrupted')
    expect(screen.getByText('Stopped')).toBeInTheDocument()
    expect(chatStore.getState().feeds['chat-1']?.items.get('t:tool-2')).toMatchObject({
      status: 'abandoned',
    })
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
    expect(await screen.findByText('command: ls')).toBeInTheDocument()
    expect(await screen.findByText('No longer available')).toBeInTheDocument()
    expect(String(fetchSpy.mock.calls[0]?.[0])).toContain('/v0/chats/chat-1/blobs/sha256%3Ain')
  })

  it('shows a result’s output as its own lines, without escapes or terminal colour codes', async () => {
    const result = {
      stdout: '\u001b[31mred\u001b[0m\n2',
      stderr: '',
      file: { lines: 2, text: 'a\n  b' },
    }
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) =>
      String(input).includes(encodeURIComponent('sha256:out'))
        ? new Response(JSON.stringify(result))
        : new Response(JSON.stringify('Exit code 3\n\u001b[1;32mgreen\u001b[0m')),
    )
    const l = ledger()
    const call = { toolId: 'tool-1', name: 'Bash', inputRef: 'sha256:in', resultRef: 'sha256:out' }
    seed([l.row('tool_call', { ...call, phase: 'post' }, { turnId: 'turn-1' })])
    draw('t:tool-1')
    fireEvent.click(screen.getByRole('button', { name: 'Show tool details' }))
    expect((await screen.findByText(/Exit code 3/)).textContent).toBe('Exit code 3\ngreen')
    expect((await screen.findByText(/stdout/)).textContent).toBe(
      'stdout: |\n  red\n  2\nstderr: ""\nfile:\n  lines: 2\n  text: |\n    a\n      b',
    )
  })

  it('adds the Result side when its ref lands while the panel is open', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) =>
      String(input).includes(encodeURIComponent('sha256:out'))
        ? new Response('done')
        : new Response('{"command":"ls"}'),
    )
    const l = ledger()
    const call = { toolId: 'tool-1', name: 'Bash', kind: 'execute', inputRef: 'sha256:in' }
    seed([l.start('turn-1'), l.row('tool_call', { ...call, phase: 'pre' }, { turnId: 'turn-1' })])
    draw('t:tool-1')
    fireEvent.click(screen.getByRole('button', { name: 'Show tool details' }))
    expect(await screen.findByText('command: ls')).toBeInTheDocument()
    expect(screen.queryByText('Result')).toBeNull()

    act(
      () =>
        void applyFeedFrame('chat-1', {
          type: 'row',
          row: l.row(
            'tool_call',
            { ...call, phase: 'post', resultRef: 'sha256:out' },
            { turnId: 'turn-1' },
          ),
        }),
    )

    expect(screen.getByText('Result')).toBeInTheDocument()
    expect(await screen.findByText('done')).toBeInTheDocument()
  })

  it('offers a retry, not "No longer available", when a read fails for another reason', async () => {
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response('', { status: 503 }))
      .mockResolvedValueOnce(new Response('second time'))
    const l = ledger()
    seed([
      l.row(
        'tool_call',
        { toolId: 'tool-1', phase: 'post', name: 'Bash', inputRef: 'sha256:in' },
        { turnId: 'turn-1' },
      ),
    ])
    draw('t:tool-1')
    fireEvent.click(screen.getByRole('button', { name: 'Show tool details' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }))

    expect(await screen.findByText('second time')).toBeInTheDocument()
    expect(screen.queryByText('No longer available')).toBeNull()
    expect(fetchSpy).toHaveBeenCalledTimes(2)
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
