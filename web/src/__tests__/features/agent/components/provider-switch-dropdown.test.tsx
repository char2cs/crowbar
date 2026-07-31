import { createElement, forwardRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import type { AgentProvider } from '@/features/agent/api/agent-api'

// jsdom has no ResizeObserver; the shared Dropdown only needs
// construct/observe/disconnect to reposition its menu on open.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// The shared Dropdown animates open/close via framer-motion's AnimatePresence,
// which keeps the exiting menu mounted until its exit transition resolves —
// jsdom never fires that, so closing would only ever be observable through a
// timing wait. Replace it with a synchronous, no-animation stand-in so a
// closed menu is immediately absent from the DOM (no timers, no polling).
vi.mock('framer-motion', () => {
  const MotionDiv = forwardRef<HTMLDivElement, Record<string, unknown>>(function MotionDiv(
    { initial: _initial, animate: _animate, exit: _exit, transition: _transition, ...domProps },
    ref,
  ) {
    return createElement('div', { ref, ...domProps })
  })
  return {
    motion: new Proxy({} as Record<string, unknown>, { get: () => MotionDiv }),
    // `m` is the LazyMotion-driven sibling of `motion` (see dropdown.tsx) — same
    // no-animation stand-in works for it.
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
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg data-p="codex"><path/></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
  {
    id: 'gemini',
    displayName: 'Gemini',
    icon: '<svg data-p="gemini"><path/></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
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

    // Each menu row inlines that provider's own icon (not the current one's).
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
    // "Disabled" means hidden entirely from every surface that OFFERS a provider
    // — the New-chat rows and this switcher alike. Switching to a disabled
    // provider would spawn the very CLI the user turned off.
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
    // Disabling never touches chats already running on that provider, so the
    // trigger must keep naming what this chat is actually talking to — and the
    // user must still be able to switch AWAY to an enabled one.
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
    // An empty menu is a dead end. Say why it is empty AND what enabling a second
    // provider would give you — including that the conversation carries over,
    // which is the non-obvious part users would never guess.
    const onlyClaude = providers.map((p) => (p.id === 'claude' ? p : { ...p, enabled: false }))
    render(
      <ProviderSwitchDropdown
        providers={onlyClaude}
        currentProviderId="claude"
        onSwitch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /claude/i }))

    // Still nothing selectable — the hint must not masquerade as a switch target.
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
    // All three providers are "other" relative to an unmatched current id.
    expect(screen.getAllByRole('menuitem')).toHaveLength(3)
  })
})
