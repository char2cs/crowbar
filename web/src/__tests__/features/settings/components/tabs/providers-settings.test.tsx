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
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const {
  updateProviderPreferencesFn,
  listProvidersFn,
  toastErrorFn,
  getDefaultPermissionLevelFn,
  updateDefaultPermissionLevelFn,
} = vi.hoisted(() => ({
  updateProviderPreferencesFn: vi.fn(),
  listProvidersFn: vi.fn(),
  toastErrorFn: vi.fn(),
  getDefaultPermissionLevelFn: vi.fn(),
  updateDefaultPermissionLevelFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  updateProviderPreferences: (...a: unknown[]) => updateProviderPreferencesFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
  getDefaultPermissionLevel: (...a: unknown[]) => getDefaultPermissionLevelFn(...a),
  updateDefaultPermissionLevel: (...a: unknown[]) => updateDefaultPermissionLevelFn(...a),
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
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
import { PROVIDER_COLUMN_CELL } from '@/features/settings/components/tabs/provider-columns'
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
  getDefaultPermissionLevelFn.mockReset().mockResolvedValue('guarded')
  updateDefaultPermissionLevelFn.mockReset()
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

  it('titles the group Agents and explains what an agent is', () => {
    seedProviders([provider('codex', 'Codex', true, true)])
    render(<ProvidersSettings />)

    // A Section's own header is hidden when it is the first one in a tab
    // (settings-section.tsx), so this tab renders its heading + intro in the
    // BODY. Without them the group is a bare list of names that never says what
    // an agent actually is. The name is Agents (spec §11) — user-facing only;
    // the tab id, the store and the wire all still say provider.
    expect(screen.getByRole('heading', { name: 'Agents' })).toBeInTheDocument()
    expect(screen.getByText(/coding CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  it('shows an empty state when the daemon genuinely reports no providers', () => {
    seedProviders([])
    render(<ProvidersSettings />)
    expect(screen.getByText('No agents available.')).toBeInTheDocument()
    expect(screen.queryByTestId('provider-toggle-codex')).toBeNull()
    // The heading and intro explain the group even when nothing is installed.
    expect(screen.getByRole('heading', { name: 'Agents' })).toBeInTheDocument()
    expect(screen.getByText(/coding CLIs Crowbar runs your chats on/i)).toBeInTheDocument()
  })

  // ── The column header ───────────────────────────────────────────────
  // The row ends in a dot and two switches. Two unlabelled switches side by side
  // is a guessing game, and the row could only ever afford one inline word
  // ("Tools") — repeated on every row, with the other switch still anonymous.
  // The columns are titled once, above the list, like a table header.
  describe('the column header', () => {
    /** The three trailing control cells of a row, in render order. */
    const rowColumns = (id: string) => [
      screen.getByTestId(`provider-connected-${id}`).parentElement,
      screen.getByTestId(`provider-tools-toggle-${id}`).parentElement,
      screen.getByTestId(`provider-toggle-${id}`).parentElement,
    ]

    /** The `w-*` token an element carries — the thing that has to match. */
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

    // THE POINT OF THE HEADER: the word "Tools" used to be printed on every row
    // because that was the only legend the row could carry. With a titled column
    // it is noise — and two rows meant two copies of it.
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

    // ALIGNMENT, STRUCTURALLY. `Switch` is `w-[calc(var(--thumb-size)*2-2px)]`
    // and `--thumb-size` drops from `--spacing(5)` to `--spacing(4)` at `sm:`, so
    // the switch is ~38px on a narrow settings pane and ~30px on a wide one. A
    // header label with its own hardcoded width would sit off-centre at one of
    // those. Header and row must therefore render the SAME column box — asserted
    // here against each other, not against a copy of the expected value, so
    // hardcoding a width into either side fails this test.
    it('lays the header labels out in the same column box as the row controls', () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      render(<ProvidersSettings />)

      const header = screen.getByTestId('provider-columns-header')
      const labels = ['Installed', 'Tools', 'Enabled'].map((t) => within(header).getByText(t))
      const cells = rowColumns('codex')

      labels.forEach((label, i) => {
        expect(widthClass(label)).toBeDefined()
        expect(widthClass(label)).toBe(widthClass(cells[i]))
        // …and both come from the one shared definition, rather than two
        // literals that happen to agree today.
        expect(label.className).toContain(PROVIDER_COLUMN_CELL)
        expect(cells[i]?.className).toContain(PROVIDER_COLUMN_CELL)
      })
    })

    // Presentational only. Every control already carries its own aria-label
    // ("Let X use Crowbar's tools", "Enable X", "Installed"/"Not installed"); a
    // header in the accessibility tree would announce each column twice.
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

    // A header titles columns that exist. None of these three states has a
    // single row under it, and a legend floating over a spinner or an error is
    // just furniture.
    it('does not appear over the empty state', () => {
      seedProviders([])
      render(<ProvidersSettings />)

      expect(screen.getByText('No agents available.')).toBeInTheDocument()
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })

    it('does not appear while the first fetch is in flight', () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      listProvidersFn.mockReturnValue(deferred<AgentProvider[]>().promise)

      render(<ProvidersSettings />)

      expect(screen.getByTestId('providers-loading')).toBeInTheDocument()
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })

    it('does not appear when the list could not be loaded', async () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())
      expect(screen.queryByTestId('provider-columns-header')).toBeNull()
    })
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
      expect(screen.queryByText('No agents available.')).toBeNull()
    })

    it('says it could not load them rather than claiming there are none', () => {
      withoutActiveWorkspace() // nothing known, and no scope to ask through
      render(<ProvidersSettings />)

      expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument()
      expect(screen.queryByText('No agents available.')).toBeNull()
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
      expect(screen.queryByText('No agents available.')).toBeNull()
    })

    it('shows the unavailable state — not the empty one — when the fetch fails', async () => {
      act(() => {
        setActiveWorkspaceStoreRef(store())
        setActiveWorkspaceId('w1')
      })
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))

      render(<ProvidersSettings />)

      await waitFor(() => expect(screen.getByTestId('providers-unavailable')).toBeInTheDocument())
      expect(screen.queryByText('No agents available.')).toBeNull()
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
    //
    // BOTH CLICKS ARE ON THE SAME PROVIDER, deliberately. The pair that has to
    // survive is "turn this provider's tools off, then turn this provider off":
    // handleToggle carries mcpDisabled through from the row it is flipping, so
    // aiming the second click at a DIFFERENT provider tested nothing — claude's
    // mcpDisabled was already false, and dropping the flag from handleToggle's
    // payload left the assertion passing.
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
      // And the tools switch is still off afterwards, which is what the user
      // would actually see go wrong.
      await waitFor(() =>
        expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
          'aria-checked',
          'false',
        ),
      )
    })
  })

  // ── A READ THAT OVERTAKES A WRITE ───────────────────────────────────
  // The rows are interactive from the workspace seed, so the user can toggle
  // while the mount GET is still in flight. That GET is a snapshot of the server
  // BEFORE the PUT, and publishing it re-installs the pre-write list: the switch
  // visibly flips back, and — because every write PUTs the complete set built
  // from whatever the store now says — the next drag or toggle writes the stale
  // flag back to the daemon. Sequencing writes against writes cannot see this;
  // the fence has to cover reads.
  describe('a stale read landing after a write', () => {
    /** Both copies seeded, with the mount GET deliberately left in flight. */
    function seedWithGetInFlight(providers: AgentProvider[]) {
      const inflight = deferred<AgentProvider[]>()
      listProvidersFn.mockReturnValue(inflight.promise)
      act(() => {
        store().getState().setAgentProviders(providers)
        setActiveWorkspaceStoreRef(store())
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

      // The mount GET, issued before the PUT, now answers with the pre-PUT list.
      await act(async () => {
        mountGet.resolve(seeded)
      })

      expect(screen.getByTestId('provider-tools-toggle-codex')).toHaveAttribute(
        'aria-checked',
        'false',
      )
      expect(useAgentProvidersStore.getState().providers[0].mcpEnabled).toBe(false)
      // The workspace copy — what the chat surfaces read — must not be reinstated
      // either, or the next write is built from a list that still says tools-on.
      expect(store().getState().agentChats.providers[0].mcpEnabled).toBe(false)
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
      // The fence must not simply refuse every read: this is the ordinary mount
      // refresh, and it has to keep repairing a stale list.
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

  // ── Default permission level ────────────────────────────────────────
  // A backend-synced setting (not the localStorage `updateSetting` path the
  // rest of this tab's siblings use for chat presentation): the daemon reads
  // it the moment a chat starts, so this row talks straight to
  // GET/PUT /v0/settings/chat/permission-level.
  //
  // The trigger shows the raw value ('guarded', not 'Guarded') until the
  // popup has been opened at least once — same base-ui quirk documented in
  // appearance-settings.test.tsx's Color Theme picker — so assertions on the
  // closed trigger check the raw PermissionLevel string.
  //
  // base-ui's Select popup keeps every item mounted (for measurement) but
  // marks the whole popup `inert` — `pointer-events: none` — until `open`
  // has actually propagated, one tick behind the trigger's own click. A
  // click landing inside that window is a real no-op in the browser too, so
  // the test waits for the popup to become interactive rather than papering
  // over it with a raw DOM event that would pass even against a dead menu.
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
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('trusted'),
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
          'full-auto',
        ),
      )

      await selectPermissionLevelOption(user, 'guarded')

      expect(updateDefaultPermissionLevelFn).toHaveBeenCalledWith('guarded')
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('guarded'),
      )
    })

    it('rolls back and toasts when the write fails', async () => {
      seedProviders([provider('codex', 'Codex', true, true)])
      getDefaultPermissionLevelFn.mockResolvedValue('guarded')
      updateDefaultPermissionLevelFn.mockRejectedValue(new Error('daemon is down'))
      const user = userEvent.setup()

      render(<ProvidersSettings />)
      await waitFor(() =>
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('guarded'),
      )

      await selectPermissionLevelOption(user, 'trusted')

      await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
      expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('guarded')
    })

    // THE RACE: a user who changes their mind before the first PUT settles.
    // The first write's eventual rejection must not roll back a SECOND,
    // already-successful selection — and must not toast for a write that is
    // no longer current.
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
        expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('guarded'),
      )

      // Two picks made before either round trip has answered.
      await selectPermissionLevelOption(user, 'trusted')
      await selectPermissionLevelOption(user, 'full-auto')

      // The SECOND, newer write resolves first…
      await act(async () => {
        second.resolve('full-auto')
      })
      // …and the FIRST, now-stale write rejects after it.
      await act(async () => {
        first.reject(new Error('daemon is down'))
      })

      expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('full-auto')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })
  })
})
