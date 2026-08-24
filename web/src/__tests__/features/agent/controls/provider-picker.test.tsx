import { createElement, forwardRef, type ReactNode } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { AgentProviderPicker } from '@/features/agent/controls/provider-picker'

// The shared Dropdown animates through AnimatePresence, which keeps an exiting
// menu mounted until a transition jsdom never fires. Same stand-in the model
// picker's suite uses, and for the same reason.
vi.mock('framer-motion', () => {
  const MotionDiv = forwardRef<HTMLDivElement, Record<string, unknown>>(function MotionDiv(
    { initial: _initial, animate: _animate, exit: _exit, transition: _transition, ...domProps },
    ref,
  ) {
    return createElement('div', { ref, ...domProps })
  })
  return {
    motion: new Proxy({} as Record<string, unknown>, { get: () => MotionDiv }),
    m: new Proxy({} as Record<string, unknown>, { get: () => MotionDiv }),
    AnimatePresence: ({ children }: { children?: ReactNode }) => children ?? null,
    LazyMotion: ({ children }: { children?: ReactNode }) => children ?? null,
    domAnimation: {},
  }
})

function agent(id: string, overrides: Partial<AgentProvider> = {}): AgentProvider {
  return {
    id,
    displayName: id === 'claude' ? 'Claude' : 'Codex',
    icon: `<svg data-agent="${id}"/>`,
    connected: true,
    enabled: true,
    mcpEnabled: true,
    ...overrides,
  }
}

const trigger = () => screen.getByRole('button', { name: /^Agent:/ })

describe('AgentProviderPicker', () => {
  it('names the agent this chat is running, with its own mark', () => {
    render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[agent('claude'), agent('codex')]}
        onSwitch={vi.fn()}
      />,
    )

    expect(trigger()).toHaveTextContent('Claude')
    expect(trigger().querySelector('[data-provider-icon]')).not.toBeNull()
  })

  it('switches the chat to the agent that was picked', () => {
    const onSwitch = vi.fn()
    render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[agent('claude'), agent('codex')]}
        onSwitch={onSwitch}
      />,
    )

    fireEvent.click(trigger())
    fireEvent.click(screen.getByRole('menuitem', { name: /Codex/ }))

    expect(onSwitch).toHaveBeenCalledWith('codex')
  })

  it('ticks the current agent and offers no move to it', () => {
    render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[agent('claude'), agent('codex')]}
        onSwitch={vi.fn()}
      />,
    )
    fireEvent.click(trigger())

    const current = screen.getByRole('menuitem', { name: /Claude/ })
    expect(within(current).queryByTestId('provider-tick')).not.toBeNull()
    expect(current).toBeDisabled()
  })

  // The house rule: a control that cannot do anything is not drawn, rather than
  // drawn and greyed. One installed agent is not a choice.
  it('draws nothing when there is nowhere to switch to', () => {
    const { container } = render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[agent('claude')]}
        onSwitch={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('offers neither a disconnected agent nor one the user switched off', () => {
    render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[
          agent('claude'),
          agent('codex', { connected: false }),
          agent('gemini', { displayName: 'Gemini', enabled: false }),
          // One real destination, so the picker has a reason to exist and the
          // two exclusions above are observable rather than vacuous.
          agent('aider', { displayName: 'Aider' }),
        ]}
        onSwitch={vi.fn()}
      />,
    )
    fireEvent.click(trigger())

    expect(screen.getByRole('menuitem', { name: /Aider/ })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /Codex/ })).toBeNull()
    expect(screen.queryByRole('menuitem', { name: /Gemini/ })).toBeNull()
  })

  it('refuses to open a switch while a turn is in flight', () => {
    render(
      <AgentProviderPicker
        provider={agent('claude')}
        providers={[agent('claude'), agent('codex')]}
        disabled
        onSwitch={vi.fn()}
      />,
    )
    expect(trigger()).toBeDisabled()
  })
})
