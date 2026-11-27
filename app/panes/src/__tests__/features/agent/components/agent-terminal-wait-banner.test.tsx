import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  AgentReturnToChatNotice,
  AgentTerminalWaitBanner,
} from '@/features/agent/components/agent-terminal-wait-banner'
import { TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-chat-reads'

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

  it('says only that input is wanted when it cannot identify the prompt', () => {
    render(<AgentTerminalWaitBanner kind="" providerLabel="Codex" onOpenTerminal={vi.fn()} />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Codex is waiting for input in its terminal',
    )
    expect(screen.getByRole('alert')).not.toHaveTextContent('trust')
  })

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

  it('offers only the terminal when the prompt declares no answers', () => {
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

  it('puts each declared answer beside the terminal button', () => {
    const onAnswer = vi.fn()
    const onOpenTerminal = vi.fn()
    render(
      <AgentTerminalWaitBanner
        kind={TERMINAL_WAIT_TRUST}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
        answers={[{ id: 'accept', label: 'Trust workspace' }]}
        onAnswer={onAnswer}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Trust workspace' }))
    expect(onAnswer).toHaveBeenCalledWith('accept')
    expect(onOpenTerminal).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Open Terminal/ })).toBeInTheDocument()
  })

  it('keeps its message to one truncated line rather than letting it wrap', () => {
    render(
      <AgentTerminalWaitBanner
        kind={TERMINAL_WAIT_TRUST}
        providerLabel="Claude"
        onOpenTerminal={vi.fn()}
      />,
    )

    expect(screen.getByRole('alert').querySelector('p')).toHaveClass('truncate')
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
