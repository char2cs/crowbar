// A chat pane is its own web view in the native shell: it may mount before its workspace's host
// resolves, and must still load the list its composer pickers need.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'

const { listProvidersFn, listGlobalProvidersFn } = vi.hoisted(() => ({
  listProvidersFn: vi.fn(),
  listGlobalProvidersFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-providers-api', () => ({
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
  listGlobalProviders: (...a: unknown[]) => listGlobalProvidersFn(...a),
}))

import { usePaneProviders } from '@/features/agent/hooks/use-pane-providers'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const claude: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: false,
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

beforeEach(() => {
  listProvidersFn.mockReset()
  listGlobalProvidersFn.mockReset().mockResolvedValue([claude])
  useAgentProvidersStore.setState({ status: 'idle', providers: [], remote: {} })
})

afterEach(() => {
  useAgentProvidersStore.setState({ status: 'idle', providers: [], remote: {} })
  resetHostWorld()
})

describe('usePaneProviders', () => {
  it('loads the local list for a local pane before any workspace host resolves', async () => {
    pairRemoteHost()
    const { result } = renderHook(() => usePaneProviders(LOCAL_HOST.id))
    await waitFor(() => expect(result.current).toEqual([claude]))
  })
})
