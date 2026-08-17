import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AgentContextGauge } from '@/features/agent/components/agent-context-gauge'

const getChatTelemetry = vi.fn()

vi.mock('@/features/agent/api/agent-api', () => ({
  getChatTelemetry: (...args: unknown[]) => getChatTelemetry(...args),
}))

beforeEach(() => {
  getChatTelemetry.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('AgentContextGauge', () => {
  // Usage is null until the first turn completes. A confident 0% there would be
  // a lie, so a fresh session draws no gauge at all.
  it('renders nothing when the provider has not reported', async () => {
    getChatTelemetry.mockResolvedValue(null)

    const { container } = render(<AgentContextGauge wsId="ws1" chatId="c1" visible />)

    await waitFor(() => expect(getChatTelemetry).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when a report carries no context usage', async () => {
    getChatTelemetry.mockResolvedValue({
      observedAt: 'x',
      source: 'callback',
      cost: { totalUsd: 1 },
    })

    const { container } = render(<AgentContextGauge wsId="ws1" chatId="c1" visible />)

    await waitFor(() => expect(getChatTelemetry).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the percentage the provider itself reported', async () => {
    getChatTelemetry.mockResolvedValue({
      observedAt: 'x',
      source: 'callback',
      context: { usedPercent: 19.4, usedTokens: 37117, capacityTokens: 200000 },
      rateLimits: [{ id: 'five_hour', label: '5-hour', usedPercent: 1 }],
      cost: { totalUsd: 0.0649 },
    })

    render(<AgentContextGauge wsId="ws1" chatId="c1" visible />)

    const gauge = await screen.findByTestId('agent-context-gauge')
    expect(gauge).toHaveTextContent('19% context')
    expect(gauge.title).toContain('37,117')
    expect(gauge.title).toContain('5-hour: 1%')
    expect(gauge.title).toContain('$0.0649')
  })

  // A hidden tab must not poll: the gauge describes a live process, and a
  // retained background tab has nobody looking at it.
  it('reads nothing while it is not visible', () => {
    render(<AgentContextGauge wsId="ws1" chatId="c1" visible={false} />)

    expect(getChatTelemetry).not.toHaveBeenCalled()
  })

  it('drops the previous chat gauge when the chat changes', async () => {
    getChatTelemetry.mockResolvedValue({
      observedAt: 'x',
      source: 'callback',
      context: { usedPercent: 42 },
    })
    const { rerender } = render(<AgentContextGauge wsId="ws1" chatId="c1" visible />)
    await screen.findByTestId('agent-context-gauge')

    getChatTelemetry.mockImplementation(() => new Promise(() => {}))
    rerender(<AgentContextGauge wsId="ws1" chatId="c2" visible />)

    await waitFor(() => expect(screen.queryByTestId('agent-context-gauge')).toBeNull())
  })

  // The gauge is an indicator, not the conversation: a failed read leaves the
  // last good number standing rather than blanking it.
  it('keeps the last good reading when a read fails', async () => {
    getChatTelemetry.mockResolvedValueOnce({
      observedAt: 'x',
      source: 'callback',
      context: { usedPercent: 19 },
    })
    render(<AgentContextGauge wsId="ws1" chatId="c1" visible />)
    await screen.findByTestId('agent-context-gauge')

    getChatTelemetry.mockRejectedValue(new Error('daemon restarting'))

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent('19% context')
  })
})
