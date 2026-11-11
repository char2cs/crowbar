import { PERMISSION_LEVEL_OPTIONS } from '@/features/agent/api/agent-providers-api'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
beforeEach(() => {
  resetChatStore()
  writeChat({ id: 'c1', permissionLevel: 'trusted' })
})
afterEach(() => {
  platform.mobile = false
})

const provider: AgentProvider = {
  id: 'provider-b',
  displayName: 'Provider B',
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

function renderBar(props: Partial<React.ComponentProps<typeof ProviderBar>> = {}) {
  return render(
    <ProviderBar
      provider={provider}
      providers={[provider]}
      model=""
      effort=""
      telemetry={telemetry}
      onSelectionChange={vi.fn()}
      {...props}
    />,
  )
}

const permissionLevel = {
  options: PERMISSION_LEVEL_OPTIONS.filter((opt) => opt.value !== 'full-auto'),
  level: 'trusted' as const,
  setLevel: vi.fn(),
  cycle: vi.fn(() => true),
}
const permission = { wsId: 'w1', chatId: 'c1', levels: ['guarded', 'trusted'] as const }

// The footer line: model chip, permission chip (desktop only), and at the right the context pill.
describe('ProviderBar', () => {
  it('passes onCompact through to the context pill’s Compact item', () => {
    const onCompact = vi.fn()
    renderBar({ onCompact })

    fireEvent.click(screen.getByTestId('agent-context-gauge'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Compact conversation' }))
    expect(onCompact).toHaveBeenCalledTimes(1)
  })

  it('leaves the pill a plain report when the caller offers no onCompact', () => {
    renderBar()
    expect(screen.getByTestId('agent-context-gauge').tagName).toBe('SPAN')
  })

  it('draws the same context pill on the phone, short', () => {
    platform.mobile = true
    renderBar({ onCompact: vi.fn() })
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent(/^61%$/)
  })

  // The phone's level choice lives in the model picker sheet instead.
  it('draws the permission chip on the desktop only', () => {
    renderBar({
      permission: { ...permission, levels: [...permission.levels] },
      permissionLevel,
    })
    expect(screen.getByTestId('chat-permission-level-chip')).toHaveTextContent('Trusted')
    platform.mobile = true
    document.body.innerHTML = ''
    renderBar({
      permission: { ...permission, levels: [...permission.levels] },
      permissionLevel,
    })
    expect(screen.queryByTestId('chat-permission-level-chip')).not.toBeInTheDocument()
  })

  it('names compaction at the right of the line only while it runs', () => {
    const { rerender } = renderBar({ compacting: true })
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
    rerender(
      <ProviderBar
        provider={provider}
        providers={[provider]}
        model=""
        effort=""
        telemetry={telemetry}
        onSelectionChange={vi.fn()}
      />,
    )
    expect(screen.queryByText('Compacting…')).not.toBeInTheDocument()
  })

  // The board's footer never loses the pill: not while compacting, not under a failure, not
  // before the provider's first report.
  it('keeps the context pill, unreported and warning, while compacting with no telemetry', () => {
    renderBar({ telemetry: null, compacting: true })
    const pill = screen.getByTestId('agent-context-gauge')
    expect(pill).toHaveTextContent(/^context$/)
    expect(pill).toHaveAttribute('data-level', 'warn')
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
  })

  it('draws the pill for any provider, unreported until it reports', () => {
    renderBar({ telemetry: null })
    expect(screen.getByTestId('agent-context-gauge')).toHaveTextContent(/^context$/)
  })

  // The phone's controls are drawn at the composer's touch scale (composer-pill.css `[data-touch]`).
  it('draws the row at the touch scale on the phone only', () => {
    const desktop = renderBar().container.querySelector('.underbar')
    expect(desktop).not.toHaveAttribute('data-touch')
    platform.mobile = true
    const phone = renderBar().container.querySelector('.underbar')
    expect(phone).toHaveAttribute('data-touch')
  })

  it('passes reportedModel through to the selection cluster unchanged', () => {
    const selectableProvider: AgentProvider = {
      ...provider,
      modelSelect: true,
      models: ['model-a'],
    }
    renderBar({
      provider: selectableProvider,
      providers: [selectableProvider],
      reportedModel: 'Model X 4.5',
    })
    expect(screen.getByTestId('agent-selection-picker')).toHaveTextContent('Model X 4.5')
  })
})
