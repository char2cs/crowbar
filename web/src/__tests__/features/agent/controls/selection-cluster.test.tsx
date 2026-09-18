import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'

const claude: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['sonnet', 'opus'],
  efforts: { sonnet: ['low', 'medium', 'high'] },
}

const noCatalogue: AgentProvider = {
  id: 'other',
  displayName: 'Other Agent',
  icon: '',
  connected: true,
  enabled: true,
  mcpEnabled: true,
}

function renderCluster(overrides: Partial<React.ComponentProps<typeof SelectionCluster>> = {}) {
  return render(
    <SelectionCluster
      provider={claude}
      providers={[claude]}
      model=""
      effort=""
      presentation="chat"
      splitEnabled={false}
      onSelectionChange={vi.fn()}
      onSelectPresentation={vi.fn()}
      {...overrides}
    />,
  )
}

// A launch already happened: nothing left to pick, so it draws plainly
// instead of the interactive picker that offers the NEXT one.
describe('SelectionCluster', () => {
  it('shows the interactive picker when not read-only', () => {
    renderCluster({ model: 'sonnet', effort: 'high' })
    expect(screen.getByTestId('agent-selection-picker')).toBeInTheDocument()
  })

  it('shows a plain launch label instead of the picker once read-only', () => {
    renderCluster({ model: 'sonnet', effort: 'high', readOnly: true })
    expect(screen.queryByTestId('agent-selection-picker')).not.toBeInTheDocument()
    expect(screen.getByText('sonnet')).toBeInTheDocument()
    expect(screen.getByText('High')).toBeInTheDocument()
  })

  it('falls back to the provider name, not blank fields, for a provider with no model/effort', () => {
    renderCluster({
      provider: noCatalogue,
      providers: [noCatalogue],
      model: '',
      effort: '',
      readOnly: true,
    })
    expect(screen.getByText('Other Agent')).toBeInTheDocument()
  })
})
