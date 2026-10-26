import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider, AgentTelemetry } from '@/features/agent/api/agent-api'
import { ProviderBar } from '@/features/agent/controls/provider-bar'

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

const provider: AgentProvider = {
  id: 'codex',
  displayName: 'Codex',
  icon: '',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: false,
  effortSelect: false,
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

const telemetry: AgentTelemetry = {
  observedAt: '2026-09-09T00:00:00Z',
  source: 'callback',
  context: { usedPercent: 61 },
}

function renderBar(onCompact?: () => void) {
  return render(
    <ProviderBar
      provider={provider}
      providers={[provider]}
      model=""
      effort=""
      telemetry={telemetry}
      presentation="chat"
      splitEnabled={false}
      onSelectionChange={vi.fn()}
      onSelectPresentation={vi.fn()}
      onCompact={onCompact}
    />,
  )
}

// ProviderBar's own doc: "what this chat has SPENT, and the one gesture that
// spends less" — onCompact is the thread from AgentChatView's own gating
// (provider capability, live, not already compacting) down to the one
// element that offers it. This only has to prove the thread is intact; the
// gauge's own button-vs-span behavior is context-gauge.test.tsx's subject.
describe('ProviderBar', () => {
  it('passes onCompact through to the context gauge unchanged', () => {
    const onCompact = vi.fn()
    renderBar(onCompact)

    fireEvent.click(screen.getByTestId('agent-context-gauge'))
    expect(onCompact).toHaveBeenCalledTimes(1)
  })

  // The phone's meter is native, beside the IDE button; under the composer only the picker stays.
  it('draws no context gauge on the phone', () => {
    platform.mobile = true
    renderBar(vi.fn())
    expect(screen.queryByTestId('agent-context-gauge')).not.toBeInTheDocument()
  })

  // The phone's picker is drawn at the composer's touch scale (composer.css `[data-touch]`).
  it('draws the picker row at the touch scale on the phone only', () => {
    const desktop = renderBar().container.querySelector('.underbar')
    expect(desktop).not.toHaveAttribute('data-touch')
    platform.mobile = true
    const phone = renderBar().container.querySelector('.underbar')
    expect(phone).toHaveAttribute('data-touch')
  })

  it('leaves the gauge non-interactive when the caller offers no onCompact', () => {
    renderBar(undefined)
    expect(screen.getByTestId('agent-context-gauge').tagName).toBe('SPAN')
  })

  it('passes reportedModel through to the selection cluster unchanged', () => {
    const selectableProvider: AgentProvider = {
      ...provider,
      modelSelect: true,
      models: ['sonnet'],
    }
    render(
      <ProviderBar
        provider={selectableProvider}
        providers={[selectableProvider]}
        model=""
        effort=""
        reportedModel="Claude Sonnet 4.5"
        telemetry={telemetry}
        presentation="chat"
        splitEnabled={false}
        onSelectionChange={vi.fn()}
        onSelectPresentation={vi.fn()}
      />,
    )
    expect(screen.getByTestId('agent-selection-picker')).toHaveTextContent('Claude Sonnet 4.5')
  })
})
