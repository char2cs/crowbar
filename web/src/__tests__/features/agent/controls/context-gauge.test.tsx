import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentTelemetry } from '@/features/agent/api/agent-api'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'

function telemetry(overrides: Partial<AgentTelemetry> = {}): AgentTelemetry {
  return { observedAt: '2026-08-24T12:00:00Z', source: 'callback', ...overrides }
}

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

  it('shows the percentage the provider itself reported', () => {
    render(<AgentContextGauge telemetry={telemetry({ context: { usedPercent: 61 } })} />)
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent('61% context')
  })

  it('rounds for the label but fills the bar with what it was given', () => {
    render(<AgentContextGauge telemetry={telemetry({ context: { usedPercent: 61.4 } })} />)
    const gauge = screen.getByTestId('agent-context-gauge')
    expect(gauge).toHaveTextContent('61% context')
    expect(gauge.querySelector('.gbar > span')).toHaveStyle({ width: '61.4%' })
  })

  it('warns once a compaction is imminent', () => {
    const { rerender } = render(
      <AgentContextGauge telemetry={telemetry({ context: { usedPercent: 84 } })} />,
    )
    expect(screen.getByTestId('agent-context-gauge').querySelector('.gbar')).not.toHaveClass('warn')
    rerender(<AgentContextGauge telemetry={telemetry({ context: { usedPercent: 92 } })} />)
    expect(screen.getByTestId('agent-context-gauge').querySelector('.gbar')).toHaveClass('warn')
  })

  it('never paints past full on a provider that over-reports', () => {
    render(<AgentContextGauge telemetry={telemetry({ context: { usedPercent: 140 } })} />)
    expect(screen.getByTestId('agent-context-gauge').querySelector('.gbar > span')).toHaveStyle({
      width: '100%',
    })
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
})
