import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const {
  updateProviderPreferencesFn,
  listProvidersFn,
  listGlobalProvidersFn,
  uploadDescriptorFn,
  toastErrorFn,
  toastSuccessFn,
  getDefaultPermissionLevelFn,
  updateDefaultPermissionLevelFn,
} = vi.hoisted(() => ({
  updateProviderPreferencesFn: vi.fn(),
  listProvidersFn: vi.fn(),
  listGlobalProvidersFn: vi.fn(),
  uploadDescriptorFn: vi.fn(),
  toastErrorFn: vi.fn(),
  toastSuccessFn: vi.fn(),
  getDefaultPermissionLevelFn: vi.fn(),
  updateDefaultPermissionLevelFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-providers-api', () => ({
  updateProviderPreferences: (...a: unknown[]) => updateProviderPreferencesFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
  listGlobalProviders: (...a: unknown[]) => listGlobalProvidersFn(...a),
  uploadDescriptor: (...a: unknown[]) => uploadDescriptorFn(...a),
  getDefaultPermissionLevel: (...a: unknown[]) => getDefaultPermissionLevelFn(...a),
  updateDefaultPermissionLevel: (...a: unknown[]) => updateDefaultPermissionLevelFn(...a),
  getDescriptorReports: vi.fn().mockResolvedValue([]),
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    error: (...a: unknown[]) => toastErrorFn(...a),
    success: (...a: unknown[]) => toastSuccessFn(...a),
  },
}))

if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

import { ProvidersSettings } from '@/features/settings/components/tabs/providers-settings'
import { PROVIDER_COLUMN_CELL } from '@/features/settings/components/tabs/provider-columns'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

const provider = (
  id: string,
  displayName: string,
  connected: boolean,
  enabled: boolean,
  mcpEnabled = true,
): AgentProvider => ({
  id,
  displayName,
  icon: `<svg data-p="${id}"></svg>`,
  connected,
  enabled,
  mcpEnabled,
  modelSelect: false,
  effortSelect: false,
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
})

function store() {
  return getOrCreateWorkspaceStore('w1')
}

function seedProviders(providers: AgentProvider[]) {
  listGlobalProvidersFn.mockResolvedValue(providers)
  inWorkspace()
  act(() => useAgentProvidersStore.setState({ providers, status: 'ready' }))
}

function inWorkspace() {
  act(() => {
    store()
    setActiveWorkspaceId('w1')
  })
}

function withoutActiveWorkspace() {
  act(() => {
    setActiveWorkspaceId('')
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const enabledById = () =>
  Object.fromEntries(useAgentProvidersStore.getState().providers.map((p) => [p.id, p.enabled]))

beforeEach(() => {
  updateProviderPreferencesFn.mockReset()
  listProvidersFn.mockReset()
  listGlobalProvidersFn.mockReset()
  uploadDescriptorFn.mockReset()
  toastErrorFn.mockReset()
  toastSuccessFn.mockReset()
  getDefaultPermissionLevelFn.mockReset().mockResolvedValue('guarded')
  updateDefaultPermissionLevelFn.mockReset()
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
})

afterEach(() => {
  cleanup()
  act(() => {
    setActiveWorkspaceId('')
  })
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
  destroyWorkspaceStore('w1')
})

describe('ProvidersSettings', () => {
  it('uploads a descriptor and shows its provider without a workspace', async () => {
    withoutActiveWorkspace()
    uploadDescriptorFn.mockResolvedValue({ id: 'customagent', findings: [] })
    listGlobalProvidersFn
      .mockResolvedValueOnce([])
      .mockResolvedValue([provider('customagent', 'Custom Agent', false, true)])
    render(<ProvidersSettings />)

    const file = new File(['id: customagent'], 'customagent.yaml', { type: 'application/yaml' })
    await userEvent.upload(screen.getByLabelText('Upload agent descriptor'), file)

    await waitFor(() => expect(screen.getByText('Custom Agent')).toBeInTheDocument())
    expect(uploadDescriptorFn).toHaveBeenCalledWith(file)
    expect(toastSuccessFn).toHaveBeenCalled()
  })
  it('renders one row per provider with connected + enabled state, in priority order', () => {
    seedProviders([
      provider('codex', 'Codex', true, true),
      provider('claude', 'Claude', false, false),
    ])
    render(<ProvidersSettings />)

    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByText('Claude')).toBeInTheDocument()

    expect(screen.getByTestId('provider-connected-codex')).toHaveAttribute('data-connected', 'true')
    expect(screen.getByTestId('provider-connected-claude')).toHaveAttribute(
      'data-connected',
      'false',
    )

    expect(screen.getByTestId('provider-toggle-codex')).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByTestId('provider-toggle-claude')).toHaveAttribute('aria-checked', 'false')
  })

  it('titles the group Agents and explains what an agent is', () => {
    seedProviders([provider('codex', 'Codex', true, true)])
    render(<ProvidersSettings />)

    expect(screen.getByRole('heading', { name: 'Agents' })).toBeInTheDocument()
    expect(screen.getByText(/coding CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  it('shows an empty state when the daemon genuinely reports no providers', () => {
    seedProviders([])
    render(<ProvidersSettings />)
    expect(screen.getByText('No agents available.')).toBeInTheDocument()
    expect(screen.queryByTestId('provider-toggle-codex')).toBeNull()
    expect(screen.getByRole('heading', { name: 'Agents' })).toBeInTheDocument()
    expect(screen.getByText(/coding CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  describe('the column header', () => {
    const rowColumns = (id: string) => [
      screen.getByTestId(`provider-connected-${id}`).parentElement,
      screen.getByTestId(`provider-tools-toggle-${id}`).parentElement,
      screen.getByTestId(`provider-toggle-${id}`).parentElement,
    ]

    const widthClass = (el: Element | null | undefined) =>
      el?.className.split(/\s+/).find((c) => /^w-/.test(c))

    it('titles each control column above the list', () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      render(<ProvidersSettings />)

      const header = screen.getByTestId('provider-columns-header')
      expect(within(header).getByText('Installed')).toBeInTheDocument()
      expect(within(header).getByText('Tools')).toBeInTheDocument()
      expect(within(header).getByText('Enabled')).toBeInTheDocument()
    })

    it('retires the per-row Tools caption now the column is titled', () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      render(<ProvidersSettings />)

      const captions = screen.getAllByText('Tools')
      expect(captions).toHaveLength(1)
      expect(captions[0]).toBe(
        within(screen.getByTestId('provider-columns-header')).getByText('Tools'),
      )
    })

    it('lays the header labels out in the same column box as the row controls', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      render(<ProvidersSettings />)

      const header = screen.getByTestId('provider-columns-header')
      const labels = ['Installed', 'Tools', 'Enabled'].map((t) => within(header).getByText(t))
      const cells = rowColumns('codex')

      labels.forEach((label, i) => {
        expect(widthClass(label)).toBeDefined()
        expect(widthClass(label)).toBe(widthClass(cells[i]))
        expect(label.className).toContain(PROVIDER_COLUMN_CELL)
        expect(cells[i]?.className).toContain(PROVIDER_COLUMN_CELL)
      })
    })

    it('stays out of the accessibility tree without weakening the per-switch labels', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      render(<ProvidersSettings />)

      expect(screen.getByTestId('provider-columns-header')).toHaveAttribute('aria-hidden', 'true')
      expect(screen.getByLabelText("Let Codex use Crowbar's tools")).toBeInTheDocument()
      expect(screen.getByLabelText('Enable Codex')).toBeInTheDocument()
      expect(screen.getByTestId('provider-connected-codex')).toHaveAttribute(
        'aria-label',
        'Installed',
      )
    })

    it('does not appear over the empty state', () => {
      seedProviders([])
      render(<ProvidersSettings />)

      expect(screen.getByText('No agents available.')).toBeInTheDocument()
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })
  })

  describe('with no workspace in view', () => {
    it('still renders the providers it knows about', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      withoutActiveWorkspace()

      render(<ProvidersSettings />)

      expect(screen.getByText('Codex')).toBeInTheDocument()
      expect(screen.queryByText('No agents available.')).toBeNull()
    })

    it('says it could not load them rather than claiming there are none', async () => {
      withoutActiveWorkspace()
      listGlobalProvidersFn.mockRejectedValue(new Error('daemon is down'))
      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())
      expect(screen.queryByText('No agents available.')).toBeNull()
    })

    it('loads providers through the global route without a workspace', async () => {
      withoutActiveWorkspace()
      listGlobalProvidersFn.mockResolvedValue([
        provider('customagent', 'Custom Agent', false, true),
      ])
      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByText('Custom Agent')).toBeInTheDocument())
      expect(listGlobalProvidersFn).toHaveBeenCalled()
    })
  })

  describe('loading the list', () => {
    it('reads the local daemon on open through the global route', async () => {
      inWorkspace()
      listGlobalProvidersFn.mockResolvedValue([provider('codex', 'Codex', true, true)])

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByText('Codex')).toBeInTheDocument())
      expect(listGlobalProvidersFn).toHaveBeenCalledWith()
      expect(listProvidersFn).not.toHaveBeenCalled()
    })

    it('shows a loading state while the first fetch is in flight', () => {
      inWorkspace()
      const inflight = deferred<AgentProvider[]>()
      listGlobalProvidersFn.mockReturnValue(inflight.promise)

      render(<ProvidersSettings />)

      expect(screen.getByTestId('providers-loading')).toBeInTheDocument()
      expect(screen.queryByText('No agents available.')).toBeNull()
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })

    it('shows the unavailable state — not the empty one — when the fetch fails', async () => {
      inWorkspace()
      listGlobalProvidersFn.mockRejectedValue(new Error('daemon is down'))

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())
      expect(screen.queryByText('No agents available.')).toBeNull()
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })

    it('loads the list again from the unavailable state when asked to retry', async () => {
      inWorkspace()
      listGlobalProvidersFn.mockRejectedValueOnce(new Error('daemon is down'))
      listGlobalProvidersFn.mockResolvedValueOnce([provider('codex', 'Codex', true, true)])
      render(<ProvidersSettings />)
      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())

      await userEvent.click(screen.getByRole('button', { name: 'Retry' }))

      await waitFor(() => expect(screen.getByText('Codex')).toBeInTheDocument())
      expect(listGlobalProvidersFn).toHaveBeenCalledTimes(2)
    })
  })

  it('toggling enable PUTs the full ordered preference list and reconciles the store', async () => {
    seedProviders([
      provider('codex', 'Codex', true, true),
      provider('claude', 'Claude', true, true),
    ])
    updateProviderPreferencesFn.mockResolvedValueOnce([
      provider('codex', 'Codex', true, false),
      provider('claude', 'Claude', true, true),
    ])

    render(<ProvidersSettings />)
    await userEvent.click(screen.getByTestId('provider-toggle-codex'))

    expect(updateProviderPreferencesFn).toHaveBeenCalledWith([
      { id: 'codex', disabled: true, mcpDisabled: false },
      { id: 'claude', disabled: false, mcpDisabled: false },
    ])

    await waitFor(() => {
      expect(useAgentProvidersStore.getState().providers[0].enabled).toBe(false)
    })
  })

  describe('the tools switch', () => {
    it('renders one per row, reflecting mcpEnabled', () => {
      seedProviders([
        provider('codex', 'Codex', true, true, true),
        provider('claude', 'Claude', true, true, false),
      ])
      render(<ProvidersSettings />)

      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'true',
      )
      expect(screen.getByTestId('provider-tools-toggle-claude')).toHaveAttribute(
        'aria-checked',
        'false',
      )
    })

    it('is labelled for what it does, not for the protocol behind it', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      render(<ProvidersSettings />)

      expect(screen.getByLabelText("Let Codex use Crowbar's tools")).toBeInTheDocument()
      expect(document.body.textContent).not.toMatch(/MCP/i)
    })

    it('turning it off PUTs mcpDisabled for that provider and leaves the others alone', async () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      updateProviderPreferencesFn.mockResolvedValueOnce([
        provider('codex', 'Codex', true, true, false),
        provider('claude', 'Claude', true, true, true),
      ])

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-tools-toggle-codex'))

      expect(updateProviderPreferencesFn).toHaveBeenCalledWith([
        { id: 'codex', disabled: false, mcpDisabled: true },
        { id: 'claude', disabled: false, mcpDisabled: false },
      ])

      await waitFor(() => {
        expect(useAgentProvidersStore.getState().providers[0].mcpEnabled).toBe(false)
      })
      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'false',
      )
      expect(screen.getByTestId('provider-toggle-codex')).toHaveAttribute('aria-checked', 'true')
    })

    it("survives a later toggle of the same provider's enable switch", async () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      updateProviderPreferencesFn
        .mockResolvedValueOnce([
          provider('codex', 'Codex', true, true, false),
          provider('claude', 'Claude', true, true, true),
        ])
        .mockResolvedValueOnce([
          provider('codex', 'Codex', true, false, false),
          provider('claude', 'Claude', true, true, true),
        ])

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-tools-toggle-codex'))
      await waitFor(() =>
        expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
          'aria-checked',
          'false',
        ),
      )
      await userEvent.click(screen.getByTestId('provider-toggle-codex'))

      expect(updateProviderPreferencesFn).toHaveBeenNthCalledWith(2, [
        { id: 'codex', disabled: true, mcpDisabled: true },
        { id: 'claude', disabled: false, mcpDisabled: false },
      ])
      await waitFor(() =>
        expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
          'aria-checked',
          'false',
        ),
      )
    })
  })

  describe('a stale read landing after a write', () => {
    function seedWithGetInFlight(providers: AgentProvider[]) {
      const inflight = deferred<AgentProvider[]>()
      listGlobalProvidersFn.mockReturnValue(inflight.promise)
      act(() => {
        store()
        setActiveWorkspaceId('w1')
        useAgentProvidersStore.setState({ providers, status: 'ready' })
      })
      return inflight
    }

    it('does not let the mount refetch undo a tools toggle taken while it was in flight', async () => {
      const seeded = [
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ]
      const mountGet = seedWithGetInFlight(seeded)
      updateProviderPreferencesFn.mockResolvedValueOnce([
        provider('codex', 'Codex', true, true, false),
        provider('claude', 'Claude', true, true, true),
      ])

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-tools-toggle-codex'))
      await waitFor(() =>
        expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
          'aria-checked',
          'false',
        ),
      )

      await act(async () => {
        mountGet.resolve(seeded)
      })

      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'false',
      )
      expect(useAgentProvidersStore.getState().providers[0].mcpEnabled).toBe(false)
    })

    it('still settles the status, so the tab never strands a spinner', async () => {
      const seeded = [provider('codex', 'Codex', true, true)]
      const mountGet = seedWithGetInFlight(seeded)
      updateProviderPreferencesFn.mockResolvedValueOnce([
        provider('codex', 'Codex', true, true, false),
      ])

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-tools-toggle-codex'))
      await act(async () => {
        mountGet.resolve(seeded)
      })

      expect(useAgentProvidersStore.getState().status).toBe('ready')
      expect(screen.queryByTestId('providers-loading')).toBeNull()
    })

    it('still adopts a read that finishes with no write racing it', async () => {
      const mountGet = seedWithGetInFlight([provider('codex', 'Codex', true, true)])

      render(<ProvidersSettings />)
      await act(async () => {
        mountGet.resolve([
          provider('codex', 'Codex', true, true, false),
          provider('claude', 'Claude', true, true),
        ])
      })

      expect(screen.getByText('Claude')).toBeInTheDocument()
      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'false',
      )
    })
  })

  describe('overlapping writes', () => {
    it('builds the second payload from the first toggle, not the pre-response snapshot', async () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      const first = deferred<ReturnType<typeof provider>[]>()
      const second = deferred<ReturnType<typeof provider>[]>()
      updateProviderPreferencesFn
        .mockReturnValueOnce(first.promise)
        .mockReturnValueOnce(second.promise)

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-toggle-codex'))
      await userEvent.click(screen.getByTestId('provider-toggle-claude'))

      expect(updateProviderPreferencesFn).toHaveBeenNthCalledWith(2, [
        { id: 'codex', disabled: true, mcpDisabled: false },
        { id: 'claude', disabled: true, mcpDisabled: false },
      ])
    })

    it('shows each toggle at once instead of waiting for the round trip', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      const inflight = deferred<ReturnType<typeof provider>[]>()
      updateProviderPreferencesFn.mockReturnValueOnce(inflight.promise)

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-toggle-codex'))

      expect(screen.getByTestId('provider-toggle-codex')).toHaveAttribute('aria-checked', 'false')
    })

    it('lets only the LATEST write reconcile the store, whatever order the responses land in', async () => {
      seedProviders([
        provider('codex', 'Codex', true, true),
        provider('claude', 'Claude', true, true),
      ])
      const first = deferred<ReturnType<typeof provider>[]>()
      const second = deferred<ReturnType<typeof provider>[]>()
      updateProviderPreferencesFn
        .mockReturnValueOnce(first.promise)
        .mockReturnValueOnce(second.promise)

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-toggle-codex'))
      await userEvent.click(screen.getByTestId('provider-toggle-claude'))

      await act(async () => {
        second.resolve([
          provider('codex', 'Codex', true, false),
          provider('claude', 'Claude', true, false),
        ])
      })
      await act(async () => {
        first.resolve([
          provider('codex', 'Codex', true, false),
          provider('claude', 'Claude', true, true),
        ])
      })

      expect(enabledById()).toEqual({ codex: false, claude: false })
    })

    it('rolls the toggle back and says so when the write fails', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      updateProviderPreferencesFn.mockRejectedValueOnce(new Error('daemon is down'))

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-toggle-codex'))

      await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
      expect(enabledById()).toEqual({ codex: true })
      expect(screen.getByTestId('provider-toggle-codex')).toHaveAttribute('aria-checked', 'true')
    })
  })

  async function selectPermissionLevelOption(
    user: ReturnType<typeof userEvent.setup>,
    value: string,
  ) {
    await user.click(screen.getByTestId('default-permission-level-trigger'))
    const option = await screen.findByTestId(`default-permission-level-option-${value}`)
    await waitFor(() => expect(getComputedStyle(option).pointerEvents).not.toBe('none'))
    await user.click(option)
  }

  describe('the default permission level', () => {
    it('offers Guarded, Trusted and Full Auto', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      const user = userEvent.setup()
      render(<ProvidersSettings />)

      await user.click(screen.getByTestId('default-permission-level-trigger'))

      const options = screen
        .getAllByTestId(/^default-permission-level-option-/)
        .map((el) => el.textContent)
      expect(options).toEqual(['Guarded', 'Trusted', 'Full Auto'])
    })

    it('reflects whatever the backend returned on mount', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      getDefaultPermissionLevelFn.mockResolvedValue('trusted')

      render(<ProvidersSettings />)

      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Trusted'),
      )
    })

    it('selecting Guarded PUTs it and updates the trigger', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      getDefaultPermissionLevelFn.mockResolvedValue('full-auto')
      updateDefaultPermissionLevelFn.mockResolvedValue('guarded')
      const user = userEvent.setup()

      render(<ProvidersSettings />)
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent(
          'Full Auto',
        ),
      )

      await selectPermissionLevelOption(user, 'guarded')

      expect(updateDefaultPermissionLevelFn).toHaveBeenCalledWith('guarded')
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Guarded'),
      )
    })

    it('rolls back and toasts when the write fails', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      getDefaultPermissionLevelFn.mockResolvedValue('guarded')
      updateDefaultPermissionLevelFn.mockRejectedValue(new Error('daemon is down'))
      const user = userEvent.setup()

      render(<ProvidersSettings />)
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Guarded'),
      )

      await selectPermissionLevelOption(user, 'trusted')

      await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
      expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Guarded')
    })

    it('keeps the later selection when an earlier, now-stale write rejects after a later one resolves', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      getDefaultPermissionLevelFn.mockResolvedValue('guarded')
      const first = deferred<string>()
      const second = deferred<string>()
      updateDefaultPermissionLevelFn
        .mockReturnValueOnce(first.promise)
        .mockReturnValueOnce(second.promise)
      const user = userEvent.setup()

      render(<ProvidersSettings />)
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Guarded'),
      )

      await selectPermissionLevelOption(user, 'trusted')
      await selectPermissionLevelOption(user, 'full-auto')

      await act(async () => {
        second.resolve('full-auto')
      })
      await act(async () => {
        first.reject(new Error('daemon is down'))
      })

      expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Full Auto')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })
  })
})

describe('ProvidersSettings with a remote workspace in view', () => {
  const remoteEnabled = () =>
    Object.fromEntries(
      (useAgentProvidersStore.getState().remote['host-1'] ?? []).map((p) => [p.id, p.enabled]),
    )

  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    act(() => {
      useAgentProvidersStore
        .getState()
        .setHostProviders('host-1', [provider('codex', 'Codex', true, true)])
      getOrCreateWorkspaceStore('w-remote')
      setActiveWorkspaceId('w-remote')
    })
    listGlobalProvidersFn.mockResolvedValue([provider('claude', 'Claude', true, true)])
  })
  afterEach(() => {
    resetHostWorld()
    useAgentProvidersStore.setState({ remote: {} })
    destroyWorkspaceStore('w-remote')
  })

  it('shows the local daemon’s list, not the remote host’s, and leaves that list alone', async () => {
    render(<ProvidersSettings />)

    await waitFor(() => expect(screen.getByText('Claude')).toBeInTheDocument())
    expect(screen.queryByText('Codex')).toBeNull()
    expect(listProvidersFn).not.toHaveBeenCalled()
    expect(listGlobalProvidersFn).toHaveBeenCalledWith()
    expect(remoteEnabled()).toEqual({ codex: true })
  })

  it('builds the preference write from the local rows and keeps it off the remote list', async () => {
    updateProviderPreferencesFn.mockResolvedValueOnce([provider('claude', 'Claude', true, false)])
    render(<ProvidersSettings />)
    await waitFor(() => expect(screen.getByText('Claude')).toBeInTheDocument())

    await userEvent.click(screen.getByTestId('provider-toggle-claude'))

    expect(updateProviderPreferencesFn).toHaveBeenCalledExactlyOnceWith([
      { id: 'claude', disabled: true, mcpDisabled: false },
    ])
    await waitFor(() => expect(enabledById()).toEqual({ claude: false }))
    expect(remoteEnabled()).toEqual({ codex: true })
  })
})
