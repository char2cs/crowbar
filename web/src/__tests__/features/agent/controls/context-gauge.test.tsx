import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
afterEach(() => {
  platform.mobile = false
})

function telemetry(overrides: Partial<AgentTelemetry> = {}): AgentTelemetry {
  return { observedAt: '2026-08-24T12:00:00Z', source: 'callback', ...overrides }
}

const reported = (usedPercent: number) => telemetry({ context: { usedPercent } })

describe('AgentContextGauge', () => {
  // "Not reported" and "zero" are different facts, and a confident 0% over the
  // first is a lie a user would act on.
  it('renders nothing when the provider has not reported', () => {
    const { container } = render(<AgentContextGauge telemetry={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when a report carries no context usage', () => {
    const { container } = render(<AgentContextGauge telemetry={telemetry()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says "N% context" on the desktop and just "N%" on the phone', () => {
    const { unmount } = render(<AgentContextGauge telemetry={reported(38)} />)
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent(/^38%context$/)
    unmount()
    platform.mobile = true
    render(<AgentContextGauge telemetry={reported(38)} />)
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent(/^38%$/)
  })

  it('fills the ring with what it was given and rounds only the label', () => {
    render(<AgentContextGauge telemetry={reported(61.4)} />)
    const pill = screen.getByTestId('agent-context-gauge')
    expect(pill).toHaveTextContent('61%')
    expect(pill.querySelector('.ctx-ring')).toHaveStyle({ '--p': '61.4' })
  })

  it('never paints past full on a provider that over-reports', () => {
    render(<AgentContextGauge telemetry={reported(140)} />)
    expect(screen.getByTestId('agent-context-gauge').querySelector('.ctx-ring')).toHaveStyle({
      '--p': '100',
    })
  })

  it('warns once a compaction is imminent', () => {
    const { rerender } = render(<AgentContextGauge telemetry={reported(84)} />)
    expect(screen.getByTestId('agent-context-gauge')).not.toHaveAttribute('data-level', 'warn')
    rerender(<AgentContextGauge telemetry={reported(85)} />)
    expect(screen.getByTestId('agent-context-gauge')).toHaveAttribute('data-level', 'warn')
  })

  it('carries the tokens, the limit windows and the cost in its title', () => {
    render(
      <AgentContextGauge
        telemetry={telemetry({
          context: { usedPercent: 20, usedTokens: 40_000, capacityTokens: 200_000 },
          rateLimits: [{ id: 'five_hour', label: '5-hour', usedPercent: 12 }],
          cost: { totalUsd: 0.4213 },
        })}
      />,
    )
    const title = screen.getByTestId('agent-context-gauge').getAttribute('title') ?? ''
    expect(title).toContain('40,000 of 200,000 tokens')
    expect(title).toContain('5-hour: 12%')
    expect(title).toContain('$0.4213')
  })

  describe('compaction', () => {
    it('is a plain report when compaction is not offered', () => {
      render(<AgentContextGauge telemetry={reported(61)} />)
      expect(screen.getByTestId('agent-context-gauge').tagName).toBe('SPAN')
    })

    it('is a menu button once offered, whose Compact item runs it and closes the menu', async () => {
      const onCompact = vi.fn()
      render(<AgentContextGauge telemetry={reported(61)} onCompact={onCompact} />)
      const pill = screen.getByTestId('agent-context-gauge')
      expect(pill.tagName).toBe('BUTTON')
      expect(onCompact).not.toHaveBeenCalled()

      fireEvent.click(pill)
      fireEvent.click(screen.getByRole('menuitem', { name: 'Compact conversation' }))
      expect(onCompact).toHaveBeenCalledTimes(1)
      await waitFor(() => expect(screen.queryByRole('menuitem')).not.toBeInTheDocument())
    })

    // Compaction is a provider capability, not telemetry: a wiped usage report must not take it down.
    it('still offers Compact, and no invented 0% ring, when there is no usage report', () => {
      const onCompact = vi.fn()
      render(<AgentContextGauge telemetry={null} onCompact={onCompact} />)
      const pill = screen.getByTestId('agent-context-gauge')
      expect(pill).toHaveTextContent('Compact')
      expect(pill.querySelector('.ctx-ring')).toBeNull()
      fireEvent.click(pill)
      expect(onCompact).toHaveBeenCalledTimes(1)
    })
  })
})
