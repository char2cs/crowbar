import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ComposerHandle } from '@/features/agent/composer/composer-handle'

function draw(overrides: Partial<Parameters<typeof ComposerHandle>[0]> = {}) {
  const onSend = vi.fn()
  const onStop = vi.fn()
  const view = render(
    <ComposerHandle
      fieldHeight={20}
      hasText={false}
      working={false}
      canStop={false}
      sending={false}
      onSend={onSend}
      onStop={onStop}
      {...overrides}
    />,
  )
  return { ...view, onSend, onStop }
}

describe('ComposerHandle', () => {
  it('sends when there is text to send', () => {
    const { onSend } = draw({ hasText: true })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeEnabled()
    expect(button.className).not.toMatch(/\boff\b/)
    fireEvent.click(button)
    expect(onSend).toHaveBeenCalled()
  })

  // REGRESSION: the button used to hide the ONLY way to stop a running turn
  // the instant there was a character in the box — a person who started
  // typing a follow-up while a turn ran had no way left to interrupt it, not
  // even Escape (agent-chat-view.tsx wires no such fallback). Enter still
  // queues whatever is typed regardless of what this button shows, so nothing
  // is lost by having it read Stop here instead of Send.
  it('stops a running turn even with text in the box', () => {
    const { onStop } = draw({ hasText: true, working: true, canStop: true })

    const button = screen.getByRole('button', { name: 'Stop this turn' })
    expect(button).toBeEnabled()
    expect(button.className).toMatch(/\bhalt\b/)
    fireEvent.click(button)
    expect(onStop).toHaveBeenCalled()
  })

  it('sends text even mid-delivery, when there is no running turn to stop', () => {
    draw({ hasText: true, sending: true })

    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeEnabled()
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('goes inert with nothing to send and no turn in flight', () => {
    const { onSend, onStop } = draw()

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
    fireEvent.click(button)
    expect(onSend).not.toHaveBeenCalled()
    expect(onStop).not.toHaveBeenCalled()
  })

  it('becomes the stop control with an empty box and a stoppable turn running', () => {
    const { onStop } = draw({ working: true, canStop: true })

    const button = screen.getByRole('button', { name: 'Stop this turn' })
    expect(button).toBeEnabled()
    expect(button.className).toMatch(/\bhalt\b/)
    fireEvent.click(button)
    expect(onStop).toHaveBeenCalled()
  })

  it('stays the plain inert circle when working but nothing can stop it', () => {
    draw({ working: true, canStop: false })

    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeDisabled()
  })

  // The draft clears the instant a prompt queues, well before the server has
  // proven delivery — without this state the circle goes straight to its inert
  // "nothing to send" look for however long that proof takes.
  it('shows a sending spinner once a prompt is dispatched but not yet proven delivered', () => {
    const { container, onSend, onStop } = draw({ sending: true })

    const button = screen.getByRole('button', { name: 'Sending' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
    fireEvent.click(button)
    expect(onSend).not.toHaveBeenCalled()
    expect(onStop).not.toHaveBeenCalled()
  })

  // A real interrupt is worth more than a spinner that only says something is
  // in flight — stopping wins whenever both apply.
  it('prefers the stop control over the sending spinner when both apply', () => {
    draw({ working: true, canStop: true, sending: true })

    expect(screen.getByRole('button', { name: 'Stop this turn' })).toBeInTheDocument()
    expect(screen.queryByRole('status')).toBeNull()
  })
})
