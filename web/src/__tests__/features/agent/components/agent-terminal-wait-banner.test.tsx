import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  AgentReturnToChatNotice,
  AgentTerminalWaitBanner,
} from '@/features/agent/components/agent-terminal-wait-banner'
import { TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-api'

describe('AgentTerminalWaitBanner', () => {
  it('names the prompt when Crowbar recognises it', () => {
    render(
      <AgentTerminalWaitBanner
        kind={TERMINAL_WAIT_TRUST}
        providerLabel="Claude"
        onOpenTerminal={vi.fn()}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Claude is asking whether you trust this folder.',
    )
  })

  // The honest fallback, and the reason the daemon carries a kind at all rather
  // than a sentence. An unidentified prompt says only what is known — that input
  // is wanted — because naming a login screen "workspace trust" would send the
  // user looking for the wrong thing.
  it('says only that input is wanted when it cannot identify the prompt', () => {
    render(<AgentTerminalWaitBanner kind="" providerLabel="Codex" onOpenTerminal={vi.fn()} />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Codex is waiting for input in its terminal',
    )
    expect(screen.getByRole('alert')).not.toHaveTextContent('trust')
  })

  // Forward compatibility: a daemon that learns a new kind must never make an
  // older client claim something specific it has no wording for.
  it('falls back to the generic wording for a kind it does not know', () => {
    render(
      <AgentTerminalWaitBanner
        kind="some_future_prompt"
        providerLabel="Claude"
        onOpenTerminal={vi.fn()}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Claude is waiting for input in its terminal',
    )
  })

  it('falls back to a neutral subject when the provider is not known yet', () => {
    render(<AgentTerminalWaitBanner kind="" providerLabel="" onOpenTerminal={vi.fn()} />)

    expect(screen.getByRole('alert')).toHaveTextContent('The agent is waiting for input')
  })

  // It never offers to ANSWER — that is the whole distinction from a choice card.
  // There is no channel to reply on, so the only control is the way to the place
  // where a reply is possible.
  it('offers the terminal and nothing else', () => {
    const onOpenTerminal = vi.fn()
    render(
      <AgentTerminalWaitBanner
        kind={TERMINAL_WAIT_TRUST}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(1)
    fireEvent.click(buttons[0]!)
    expect(onOpenTerminal).toHaveBeenCalledTimes(1)
  })
})

describe('AgentReturnToChatNotice', () => {
  it('offers the way back and takes no for an answer', () => {
    const onReturn = vi.fn()
    const onDismiss = vi.fn()
    render(<AgentReturnToChatNotice onReturn={onReturn} onDismiss={onDismiss} />)

    fireEvent.click(screen.getByRole('button', { name: 'Return to Chat' }))
    expect(onReturn).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })
})
