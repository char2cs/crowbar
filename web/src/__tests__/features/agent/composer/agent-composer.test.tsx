import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function draw(overrides: Partial<Parameters<typeof AgentComposer>[0]> = {}) {
  return render(
    <AgentComposer
      wsId="w1"
      chatId="c1"
      activity={NO_ACTIVITY}
      providerLabel="Claude"
      live
      working={false}
      compacting={false}
      sending={false}
      submitUnavailable={false}
      canStop={false}
      draft=""
      fieldHeight={20}
      slashOpen={false}
      onDraftChange={vi.fn()}
      onHeightChange={vi.fn()}
      onKeyDown={vi.fn()}
      onSend={vi.fn()}
      onStop={vi.fn()}
      onOpenTerminal={vi.fn()}
      draftSeed={0}
      seedText=""
      {...overrides}
    />,
  )
}

// The bar delegates its own dispatched-but-unproven visual to the handle — this
// only has to prove the wiring reaches it, not re-litigate the handle's own
// precedence rules (covered in composer-handle.test.tsx).
describe('AgentComposer', () => {
  it('passes sending through to the handle as an input', () => {
    const { container } = draw({ sending: true })

    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('shows the plain send affordance when nothing is in flight', () => {
    const { container } = draw({ sending: false })

    expect(container.querySelector('[data-flicker-spinner]')).toBeNull()
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeInTheDocument()
  })

  // Task 24 gives the composer its own modal state, opened via the handle's
  // plus button. Tasks 29/34 replace the placeholder `null` branches with the
  // real modals — until then, opening either must be a true no-op: no dialog
  // appears and the rest of the bar keeps working exactly as before.
  it('opens the excalidraw modal slot from the plus button without a visible modal yet', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))

    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  it('opens the attach-file modal slot from the plus button without a visible modal yet', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))

    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  // Pre-existing branch, unrelated to the plus button: proves the switch still
  // reaches `case 'choice'` (and thus never the input/handle branch) once a
  // pending choice is waiting.
  it('renders the choice card instead of the field when a choice is pending', () => {
    draw({
      activity: {
        ...NO_ACTIVITY,
        choices: [
          {
            id: 'k1',
            turnId: 't1',
            seq: 1,
            kind: 'tool_permission',
            toolName: 'Bash',
            options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
            pending: true,
            answerable: true,
            at: '2026-08-18T12:00:00Z',
          },
        ],
      },
    })

    expect(screen.getByRole('group')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Send prompt' })).toBeNull()
  })

  // Pre-existing branches, unrelated to the plus button: compaction shares the
  // 'input' render path (it queues rather than blocking), and the pill grows a
  // `multi` class once the field itself reports more than one line.
  it('keeps the field mounted while compacting, queuing behind the busy provider', () => {
    draw({ compacting: true })

    expect(screen.getByRole('textbox', { name: 'Message the agent' })).toBeInTheDocument()
  })

  it('marks the pill multiline once the field grows past one line', () => {
    const { container } = draw({ fieldHeight: 40 })

    expect(container.querySelector('.pill')).toHaveClass('multi')
  })
})
