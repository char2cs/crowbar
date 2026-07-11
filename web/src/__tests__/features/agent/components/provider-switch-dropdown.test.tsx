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
    AnimatePresence: ({ children }: { children?: React.ReactNode }) => children ?? null,
  }
})

const providers: AgentProvider[] = [
  { id: 'claude', displayName: 'Claude', icon: '<svg data-p="claude"><path/></svg>' },
  { id: 'codex', displayName: 'Codex', icon: '<svg data-p="codex"><path/></svg>' },
  { id: 'gemini', displayName: 'Gemini', icon: '<svg data-p="gemini"><path/></svg>' },
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
