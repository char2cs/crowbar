import { render, screen } from '@testing-library/react'
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
})
