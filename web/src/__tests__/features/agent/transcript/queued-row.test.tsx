import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { QueuedRow } from '@/features/agent/transcript/queued-row'

function item(overrides: Partial<PromptQueueItem> = {}): PromptQueueItem {
  return {
    clientRequestId: 'r1',
    text: 'a queued prompt',
    state: 'queued',
    createdAt: '2026-08-24T00:00:00Z',
    baselineSequence: 0,
    ...overrides,
  }
}

function draw(props: Partial<Parameters<typeof QueuedRow>[0]> = {}) {
  return render(
    <QueuedRow
      item={item()}
      showTerminalHint={false}
      onEdit={vi.fn()}
      onCancel={vi.fn()}
      onRetry={vi.fn()}
      onOpenTerminal={vi.fn()}
      {...props}
    />,
  )
}

describe('QueuedRow', () => {
  it('renders as the dashed pending pill by default', () => {
    const { container } = draw()
    expect(container.querySelector('.queued')).not.toBeNull()
    expect(container.querySelector('.frozen')).toBeNull()
  })

  // REGRESSION: the queued row used to show `item.text` as a raw string —
  // the one row in the transcript that didn't go through the same markdown
  // pipeline every confirmed message renders through (message-row.tsx),
  // so a prompt with **bold** or a `code span` showed its literal source
  // for the whole gap between pressing send and ledger confirmation.
  it('renders the queued prompt as markdown, not as literal source', () => {
    draw({ item: item({ text: 'a **bold** word' }) })
    expect(screen.getByText('bold').closest('strong')).not.toBeNull()
    expect(screen.queryByText(/\*\*bold\*\*/)).toBeNull()
  })

  // The chat's first-ever turn is kept in the frozen document's own hand for
  // the whole time it is pending — never the dashed pill every later prompt
  // gets — so sending it never flashes through a shape the reply won't match.
  describe("as the chat's first turn", () => {
    it('renders frozen, not as the dashed pill', () => {
      const { container } = draw({ firstTurn: true })
      expect(container.querySelector('.frozen')).not.toBeNull()
      expect(container.querySelector('.queued')).toBeNull()
    })

    it('marks itself for anything reading the DOM directly', () => {
      const { container } = draw({ firstTurn: true })
      expect(container.querySelector('[data-first-turn="true"]')).not.toBeNull()
    })

    it('still shows the prompt text', () => {
      draw({ firstTurn: true, item: item({ text: 'describe the change' }) })
      expect(screen.getByText('describe the change')).toBeInTheDocument()
    })

    // No hatch/pill background to flag trouble on sight here, so the error
    // text this variant would otherwise hide has to actually say something.
    it('still shows a delivery error, since there is no warning pill to imply one', () => {
      draw({
        firstTurn: true,
        item: item({ state: 'failed', error: 'The provider rejected this prompt.' }),
      })
      expect(screen.getByText('The provider rejected this prompt.')).toBeInTheDocument()
    })

    it('still offers retry/cancel actions', () => {
      const onRetry = vi.fn()
      draw({
        firstTurn: true,
        item: item({ state: 'failed', error: 'boom' }),
        onRetry,
      })
      screen.getByRole('button', { name: /retry/i }).click()
      expect(onRetry).toHaveBeenCalled()
    })
  })
})
