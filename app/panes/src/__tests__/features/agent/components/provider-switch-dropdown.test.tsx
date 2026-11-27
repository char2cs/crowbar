import { createElement, forwardRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

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
    AnimatePresence: ({ children }: { children?: React.ReactNode }) => children ?? null,
    LazyMotion: ({ children }: { children?: React.ReactNode }) => children ?? null,
    domAnimation: {},
  }
})

const providers: AgentProvider[] = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg data-p="claude"><path/></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    hasTerminal: true,
    hotswap: false,
    terminalStartHere: false,
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg data-p="codex"><path/></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    hasTerminal: true,
    hotswap: false,
    terminalStartHere: false,
  },
  {
    id: 'gemini',
    displayName: 'Gemini',
    icon: '<svg data-p="gemini"><path/></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    hasTerminal: true,
    hotswap: false,
    terminalStartHere: false,
  },
]

describe('ProviderSwitchDropdown', () => {
  it('shows the current provider on the trigger, with its icon inlined', () => {
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: /claude/i })
    expect(trigger).toBeTruthy()
    expect(trigger.querySelector('svg[data-p="claude"]')).toBeTruthy()
  })

  it('lists ONLY the other providers when opened, not the current one', () => {
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    const items = screen.getAllByRole('menuitem')
    expect(items).toHaveLength(2)
    expect(items.map((el) => el.textContent)).toEqual(
      expect.arrayContaining([expect.stringContaining('Codex'), expect.stringContaining('Gemini')]),
    )
    expect(screen.queryByRole('menuitem', { name: /claude/i })).toBeNull()

    expect(
      screen.getByRole('menuitem', { name: /codex/i }).querySelector('svg[data-p="codex"]'),
    ).toBeTruthy()
  })

  it('invokes onSwitch with the clicked provider id and closes the menu', () => {
    const onSwitch = vi.fn()
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={onSwitch}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /codex/i }))

    expect(onSwitch).toHaveBeenCalledTimes(1)
    expect(onSwitch).toHaveBeenCalledWith('codex')
    expect(screen.queryAllByRole('menuitem')).toHaveLength(0)
  })

  it('cannot open or switch while prompt delivery is pending', () => {
    const onSwitch = vi.fn()
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={onSwitch}
        disabled
      />,
    )

    const trigger = screen.getByRole('button', { name: /claude/i })
    expect(trigger).toBeDisabled()
    fireEvent.click(trigger)
    expect(screen.queryAllByRole('menuitem')).toHaveLength(0)
    expect(onSwitch).not.toHaveBeenCalled()
  })

  it('toggles closed when the trigger is clicked again', () => {
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: /claude/i })
    fireEvent.click(trigger)
    expect(screen.getAllByRole('menuitem')).toHaveLength(2)

    fireEvent.click(trigger)
    expect(screen.queryAllByRole('menuitem')).toHaveLength(0)
  })

  it('renders an empty, non-crashing menu when there are no other providers to switch to', () => {
    const single: AgentProvider[] = [providers[0]]
    render(
      <ProviderSwitchDropdown providers={single} currentProviderId="claude" onSwitch={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    expect(screen.queryAllByRole('menuitem')).toHaveLength(0)
  })

  it('does not offer a DISABLED provider as a switch target', () => {
    const withDisabled = providers.map((p) => (p.id === 'gemini' ? { ...p, enabled: false } : p))
    render(
      <ProviderSwitchDropdown
        providers={withDisabled}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    expect(screen.queryByRole('menuitem', { name: /gemini/i })).toBeNull()
    expect(screen.getAllByRole('menuitem')).toHaveLength(1)
    expect(screen.getByRole('menuitem', { name: /codex/i })).toBeTruthy()
  })

  it('still names a disabled provider on the trigger when the chat is running on it', () => {
    const withDisabled = providers.map((p) => (p.id === 'gemini' ? { ...p, enabled: false } : p))
    render(
      <ProviderSwitchDropdown
        providers={withDisabled}
        currentProviderId="gemini"
        onSwitch={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: /gemini/i })
    expect(trigger).toBeTruthy()

    fireEvent.click(trigger)
    expect(screen.getAllByRole('menuitem')).toHaveLength(2)
  })

  it('explains what switching buys you when no other provider is enabled', () => {
    const onlyClaude = providers.map((p) => (p.id === 'claude' ? p : { ...p, enabled: false }))
    render(
      <ProviderSwitchDropdown
        providers={onlyClaude}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    expect(screen.queryAllByRole('menuitem')).toHaveLength(0)
    expect(screen.getByText(/no other provider is enabled/i)).toBeTruthy()
    expect(screen.getByText(/context/i)).toBeTruthy()
  })

  it('shows no such hint when there ARE providers to switch to', () => {
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    expect(screen.queryByText(/no other provider is enabled/i)).toBeNull()
    expect(screen.getAllByRole('menuitem')).toHaveLength(2)
  })

  it('falls back gracefully when currentProviderId matches no provider', () => {
    render(
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId="unknown"
        onSwitch={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: 'Provider' })
    expect(trigger).toBeTruthy()

    fireEvent.click(trigger)
    expect(screen.getAllByRole('menuitem')).toHaveLength(3)
  })
})
