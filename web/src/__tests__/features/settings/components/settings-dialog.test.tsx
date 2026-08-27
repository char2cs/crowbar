/**
 * Settings dialog registration for the Agents tab: the three touchpoints
 * (SettingsTab union, SETTINGS_TAB_ITEMS, the dialog's switch) are wired so that
 * opening Settings on the `providers` tab renders the real ProvidersSettings.
 *
 * The tab READS "Agents" and is KEYED `providers` (spec §11): the rename is
 * user-facing only, so the id, the component and everything on the wire keep the
 * domain word. Both halves of that are asserted below, because a rename that
 * reached the id would be a breaking change wearing a copy edit.
 *
 * Only the agent-api network seam is mocked; the dialog, vertical tabs and the
 * tab itself are all real. The list reads the GLOBAL provider store (the dialog
 * is global — a per-workspace read is the defect this replaced), so that is what
 * gets seeded here.
 */
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  updateProviderPreferences: vi.fn(),
  listProviders: vi.fn().mockResolvedValue([]),
  getDefaultPermissionLevel: vi.fn().mockResolvedValue('guarded'),
  updateDefaultPermissionLevel: vi.fn(),
}))

import SettingsDialog from '@/features/settings/components/settings-dialog'
import { SETTINGS_TAB_ITEMS } from '@/features/settings/components/settings-tab-items'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { AgentProvider } from '@/features/agent/api/agent-api'

const CLAUDE: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
}

beforeEach(() => {
  const st = getOrCreateWorkspaceStore('w1')
  act(() => {
    st.getState().setAgentProviders([CLAUDE])
    setActiveWorkspaceStoreRef(st)
    useAgentProvidersStore.setState({ providers: [CLAUDE], status: 'ready' })
    useUIState.getState().setSettingsInitialTab('providers')
  })
})

afterEach(() => {
  cleanup()
  act(() => {
    setActiveWorkspaceStoreRef(null)
    useUIState.getState().setSettingsInitialTab('appearance')
  })
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
  destroyWorkspaceStore('w1')
})

describe('Settings dialog — Agents tab registration', () => {
  it('lists the tab under its `providers` id', () => {
    expect(SETTINGS_TAB_ITEMS.map((t) => t.id)).toContain('providers')
  })

  // The rename, both halves of it: the rail reads Agents, the id under it does
  // not move, and no tab is keyed by the new word.
  it('reads "Agents" in the rail while staying keyed `providers`', () => {
    const item = SETTINGS_TAB_ITEMS.find((t) => t.id === 'providers')
    expect(item?.label).toBe('Agents')
    expect(SETTINGS_TAB_ITEMS.map((t) => t.id)).not.toContain('agents')
  })

  it('renders the real ProvidersSettings when opened on the providers tab', () => {
    render(<SettingsDialog isOpen onClose={() => {}} />)
    // The connected indicator testid is unique to ProvidersSettings, so its
    // presence proves the dialog switch routed `providers` → ProvidersSettings.
    expect(screen.getByTestId('provider-connected-claude')).toBeInTheDocument()
    expect(screen.getByTestId('provider-toggle-claude')).toBeInTheDocument()
  })
})
