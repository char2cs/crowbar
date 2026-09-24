import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'

describe('ComposerSignpost', () => {
  it('says why, with no button at all for a dormant chat', () => {
    const onOpenTerminal = vi.fn()
    render(
      <ComposerSignpost
        reason="dormant"
        message="Resume the provider before sending from Chat."
        onOpenTerminal={onOpenTerminal}
      />,
    )

    expect(screen.getByRole('status')).toHaveTextContent('Resume the provider')
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('offers the terminal for a provider that cannot take a typed prompt', () => {
    const onOpenTerminal = vi.fn()
    render(
      <ComposerSignpost
        reason="unsupported"
        message="This provider cannot accept a prompt typed here."
        onOpenTerminal={onOpenTerminal}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Terminal/ }))
    expect(onOpenTerminal).toHaveBeenCalled()
  })

  it('offers the terminal for a CLI blocked where only its terminal can reach', () => {
    const onOpenTerminal = vi.fn()
    render(
      <ComposerSignpost
        reason="terminal_wait"
        message="This provider is waiting for you to trust the workspace"
        onOpenTerminal={onOpenTerminal}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Terminal/ }))
    expect(onOpenTerminal).toHaveBeenCalled()
  })

  // Crowbar's own revive is in flight — there is nothing to click, only a
  // spinner saying so isn't stalled.
  it('shows a spinner and no button while reviving', () => {
    const { container } = render(
      <ComposerSignpost reason="reviving" message="Resuming this chat…" onOpenTerminal={vi.fn()} />,
    )

    expect(screen.getByText('Resuming this chat…')).toBeInTheDocument()
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
