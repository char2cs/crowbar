/**
 * Providers settings tab: renders one row per provider (in priority order) with
 * its connected indicator + enable toggle, and — on a toggle — PUTs the COMPLETE
 * ordered preference set, applying it optimistically and reconciling from the
 * response (server truth).
 *
 * The list comes from the GLOBAL provider store, not the active workspace: the
 * Settings dialog is global, and reading a per-workspace copy made this tab claim
 * the daemon had no providers whenever no workspace was in view. The workspace
 * store is still REAL here, because a write must also land in it (the chat
 * surfaces read it). Only the network seam is mocked. A full @dnd-kit pointer
 * drag is NOT driven here (brittle in jsdom) — the reorder→payload mapping is
 * unit-tested in provider-preferences.test.ts.
 */
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { updateProviderPreferencesFn, listProvidersFn, toastErrorFn } = vi.hoisted(() => ({
  updateProviderPreferencesFn: vi.fn(),
  listProvidersFn: vi.fn(),
  toastErrorFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  updateProviderPreferences: (...a: unknown[]) => updateProviderPreferencesFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

// jsdom has no PointerEvent constructor; base-ui's Switch toggles by dispatching
// a PointerEvent('click') on its hidden checkbox. A MouseEvent-backed shim is
// enough — the checkbox's click activation still toggles and fires change.
if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

import { ProvidersSettings } from '@/features/settings/components/tabs/providers-settings'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { AgentProvider } from '@/features/agent/api/agent-api'

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
})

function store() {
  return getOrCreateWorkspaceStore('w1')
}

/**
 * The ordinary situation: a workspace is in view and the providers are already
 * known. Seeds BOTH copies — the global one the tab reads and the workspace one
 * the chat surfaces read — and points the mount refresh at the same answer the
 * server would give, so re-fetching is a no-op rather than a surprise.
 */
function seedProviders(providers: AgentProvider[]) {
  listProvidersFn.mockResolvedValue(providers)
  act(() => {
    store().getState().setAgentProviders(providers)
    setActiveWorkspaceStoreRef(store())
    setActiveWorkspaceId('w1')
    useAgentProvidersStore.setState({ providers, status: 'ready' })
  })
}

/** No workspace in view at all — Project Home, the projects screen, or any
 *  moment between two workspaces. Settings is still openable from all of them. */
function withoutActiveWorkspace() {
  act(() => {
    setActiveWorkspaceStoreRef(null)
    setActiveWorkspaceId('')
  })
}

/** A promise this test resolves by hand, so two writes can be held in flight and
 *  landed in whichever order the test wants. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/** What the tab itself is showing, read off the store it renders from. */
const enabledById = () =>
  Object.fromEntries(useAgentProvidersStore.getState().providers.map((p) => [p.id, p.enabled]))

/** The workspace copy the chat surfaces read — a write must reach it too. */
const workspaceEnabledById = () =>
  Object.fromEntries(
    store()
      .getState()
      .agentChats.providers.map((p) => [p.id, p.enabled]),
  )

beforeEach(() => {
  updateProviderPreferencesFn.mockReset()
  listProvidersFn.mockReset()
  toastErrorFn.mockReset()
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
})

afterEach(() => {
  cleanup()
  act(() => {
    setActiveWorkspaceStoreRef(null)
    setActiveWorkspaceId('')
  })
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
  destroyWorkspaceStore('w1')
})

describe('ProvidersSettings', () => {
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

  it('titles the group and explains what providers are', () => {
    seedProviders([provider('codex', 'Codex', true, true)])
    render(<ProvidersSettings />)

    // A Section's own header is hidden when it is the first one in a tab
    // (settings-section.tsx), so this tab renders its heading + intro in the
    // BODY. Without them the group is a bare list of names that never says what
    // a provider actually is.
    expect(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
    expect(screen.getByText(/agentic CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  it('shows an empty state when the daemon genuinely reports no providers', () => {
    seedProviders([])
    render(<ProvidersSettings />)
    expect(screen.getByText('No providers available.')).toBeInTheDocument()
    expect(screen.queryByTestId('provider-toggle-codex')).toBeNull()
    // The heading and intro explain the group even when nothing is installed.
    expect(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
    expect(screen.getByText(/agentic CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  // ── The tab is GLOBAL; the workspace store is not ───────────────────
  // Reported live: "No providers available." with both CLIs installed, both rows
  // enabled in the daemon's own sqlite, and the workspace-scoped GET returning
  // them. The list was being read out of the ACTIVE WORKSPACE store, so any
  // moment without one — Project Home, the projects screen, mid-switch — made
  // this tab assert something false about the machine.
  describe('with no workspace in view', () => {
    it('still renders the providers it knows about', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      withoutActiveWorkspace()

      render(<ProvidersSettings />)

      expect(screen.getByText('Codex')).toBeInTheDocument()
      expect(screen.queryByText('No providers available.')).toBeNull()
    })

    it('says it could not load them rather than claiming there are none', () => {
      withoutActiveWorkspace() // nothing known, and no scope to ask through
      render(<ProvidersSettings />)

      expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument()
      expect(screen.queryByText('No providers available.')).toBeNull()
    })
  })

  describe('loading the list', () => {
    it('fetches on mount through the active workspace and mirrors it into that store', async () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      listProvidersFn.mockResolvedValue([provider('codex', 'Codex', true, true)])

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByText('Codex')).toBeInTheDocument())
      expect(listProvidersFn).toHaveBeenCalledWith('w1')
      // The chat surfaces read the workspace copy — opening Settings must repair
      // it too, not just the dialog in front of the user.
      await waitFor(() => expect(workspaceEnabledById()).toEqual({ codex: true }))
    })

    it('shows a loading state while the first fetch is in flight', () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      const inflight = deferred<AgentProvider[]>()
      listProvidersFn.mockReturnValue(inflight.promise)

      render(<ProvidersSettings />)

      expect(screen.getByTestId('providers-loading')).toBeInTheDocument()
      expect(screen.queryByText('No providers available.')).toBeNull()
    })

    it('shows the unavailable state — not the empty one — when the fetch fails', async () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())
      expect(screen.queryByText('No providers available.')).toBeNull()
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

    // Store reconciled from the mocked response, not from local optimism.
    await waitFor(() => {
      expect(store().getState().agentChats.providers[0].enabled).toBe(false)
    })
  })

  // ── Crowbar's tools, per provider ───────────────────────────────────
  // A SECOND, INDEPENDENT axis: a provider with its tools off still spawns and
  // still holds a normal chat — the agent just cannot reach into Crowbar. The row
  // never says "MCP"; the transport is not what the user is deciding.
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

      // Round trip: the reconciled store, and the switch, both say off.
      await waitFor(() => {
        expect(store().getState().agentChats.providers[0].mcpEnabled).toBe(false)
      })
      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'false',
      )
      // …and the provider itself is untouched by a tools toggle.
      expect(screen.getByTestId('provider-toggle-codex')).toHaveAttribute('aria-checked', 'true')
    })

    // THE LANDMINE. The PUT replaces whole rows, so any later write that forgot
    // this flag would write its zero value back over the user's choice. Every
    // write carries both flags — here, an enable toggle taken after a tools
    // toggle still says mcpDisabled.
    it('survives a later, unrelated enable toggle', async () => {
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
          provider('codex', 'Codex', true, true, false),
          provider('claude', 'Claude', true, false, true),
        ])

      render(<ProvidersSettings />)
      await userEvent.click(screen.getByTestId('provider-tools-toggle-codex'))
      await waitFor(() =>
        expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
          'aria-checked',
          'false',
        ),
      )
      await userEvent.click(screen.getByTestId('provider-toggle-claude'))

      expect(updateProviderPreferencesFn).toHaveBeenNthCalledWith(2, [
        { id: 'codex', disabled: false, mcpDisabled: true },
        { id: 'claude', disabled: true, mcpDisabled: false },
      ])
    })
  })

  // ── Overlapping writes ─────────────────────────────────────────────
  // Every write PUTs the COMPLETE preference set, so a payload built from a
  // stale snapshot doesn't just miss an update — it actively UNDOES one. And
  // because `Switch checked={provider.enabled}` is fully controlled off the
  // store, the user never even saw their first toggle move.
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
      // Both toggles flipped off INSIDE the first round trip.
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

      // The NEWER response lands first…
      await act(async () => {
        second.resolve([
          provider('codex', 'Codex', true, false),
          provider('claude', 'Claude', true, false),
        ])
      })
      // …and the older one lands last, still describing a world where claude is on.
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
})
