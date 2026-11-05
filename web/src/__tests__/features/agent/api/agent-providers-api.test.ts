import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
// Only the scope REGISTRY is faked; the URL builders themselves run for real,
// so these assertions pin the actual paths this client dials rather than a
// stand-in shape. 'w1' carries a real repo, so chatBase builds the Task-17
// repo-scoped shape; 'ws-home' has repoId '' (a project-home workspace) and
// falls back to the still-live /home mount; anything else is unrecorded scope.
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-providers-api'
import { useSettingsStore } from '@/features/settings/store'

describe('agent-providers-api', () => {
  beforeEach(() => apiFetch.mockReset())

  it('listProviders returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValue(undefined)
    const p = await api.listProviders('w1')
    expect(p).toEqual([])
  })

  // The enriched read carries the three provider facts the FE leans on:
  // `connected` (is the CLI installed), `enabled` (is the provider offered) and
  // `mcpEnabled` (may its agent use Crowbar's own tools).
  it('listProviders maps connected + enabled + mcpEnabled', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'codex',
        displayName: 'Codex',
        icon: '<svg/>',
        connected: true,
        enabled: true,
        mcpEnabled: false,
      },
    ])
    const out = await api.listProviders('w1')
    expect(out[0]).toMatchObject({ id: 'codex', connected: true, enabled: true, mcpEnabled: false })
  })

  // A provider row that omits the flags defaults to enabled (spec §3.1: a
  // provider with no stored preference is enabled), NOT connected (install is
  // never assumed), and with its tool surface ON — so the FE never treats an
  // unknown provider as disabled, and never silently strips Crowbar's tools from
  // a daemon whose payload predates the field. The backend stores the NEGATIVE
  // (mcpDisabled), so absent means on.
  // A field-by-field mapper silently drops any key it forgets; compaction must survive it.
  it('listProviders carries compaction through', async () => {
    apiFetch.mockResolvedValueOnce([
      { id: 'claude', displayName: 'Claude', icon: '<svg/>', compaction: true },
      { id: 'codex', displayName: 'Codex', icon: '<svg/>', compaction: false },
    ])
    const out = await api.listProviders('w1')
    expect(out[0].compaction).toBe(true)
    expect(out[1].compaction).toBe(false)
  })

  it('listProviders carries hasTerminal:false and hotswap:true through unchanged', async () => {
    apiFetch.mockResolvedValueOnce([
      { id: 'claude', displayName: 'Claude', icon: '<svg/>', hasTerminal: false, hotswap: true },
    ])
    const out = await api.listProviders('w1')
    expect(out[0].hasTerminal).toBe(false)
    expect(out[0].hotswap).toBe(true)
  })

  it('listProviders carries terminalStartHere:true through unchanged', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'claude',
        displayName: 'Claude',
        icon: '<svg/>',
        hasTerminal: true,
        terminalStartHere: true,
      },
    ])
    const out = await api.listProviders('w1')
    expect(out[0].terminalStartHere).toBe(true)
  })

  // The selection catalogue: WHETHER each picker exists at all, plus the models
  // and the per-model effort levels the backend has already resolved.
  it('listProviders carries the model/effort catalogue through unchanged', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'codex',
        displayName: 'Codex',
        icon: '<svg/>',
        connected: true,
        enabled: true,
        mcpEnabled: true,
        modelSelect: true,
        effortSelect: true,
        models: ['gpt-5.6-sol', 'gpt-5.6-luna'],
        efforts: {
          '': ['low', 'medium', 'high'],
          'gpt-5.6-sol': ['low', 'medium', 'high', 'max', 'ultra'],
          'gpt-5.6-luna': ['low', 'medium', 'high', 'max'],
        },
      },
    ])
    const out = await api.listProviders('w1')
    expect(out[0].modelSelect).toBe(true)
    expect(out[0].effortSelect).toBe(true)
    // Descriptor order is the provider's own ranking — never re-sorted here.
    expect(out[0].models).toEqual(['gpt-5.6-sol', 'gpt-5.6-luna'])
    // '' is the key for the provider's OWN default model. The fallback rule is
    // applied server-side; the mapper must not add one of its own.
    expect(out[0].efforts?.['']).toEqual(['low', 'medium', 'high'])
    expect(out[0].efforts?.['gpt-5.6-luna']).toEqual(['low', 'medium', 'high', 'max'])
  })

  // The daemon omits EMPTY catalogues (omitempty); no catalogue must render as
  // no picker rather than a crash on a missing array.
  it('listProviders grounds omitted catalogues to empty', async () => {
    apiFetch.mockResolvedValueOnce([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    const out = await api.listProviders('w1')
    expect(out[0].models).toEqual([])
    expect(out[0].permissionLevels).toEqual([])
    expect(out[0].efforts).toEqual({})
  })

  // The global preferences write: PUTs the FULL ordered set (index = priority) to
  // the settings route and hands back the freshly resolved list, so the caller
  // reconciles from server truth without a second GET.
  it('updateProviderPreferences PUTs the ordered list and returns the resolved providers', async () => {
    apiFetch.mockResolvedValueOnce([
      { id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: true },
      { id: 'claude', displayName: 'Claude', icon: '', connected: true, enabled: false },
    ])
    const out = await api.updateProviderPreferences([
      { id: 'codex', disabled: false, mcpDisabled: true },
      { id: 'claude', disabled: true, mcpDisabled: false },
    ])
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/settings/chat/providers',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          providers: [
            { id: 'codex', disabled: false, mcpDisabled: true },
            { id: 'claude', disabled: true, mcpDisabled: false },
          ],
        }),
      }),
      undefined,
      undefined,
    )
    expect(out.map((p) => p.id)).toEqual(['codex', 'claude'])
    expect(out[1].enabled).toBe(false)
  })

  it('listGlobalProviders reads the local daemon’s list, or the named host’s own', async () => {
    apiFetch.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])

    await api.listGlobalProviders()
    await api.listGlobalProviders('host-1')

    expect(apiFetch.mock.calls[0]).toEqual([
      '/v0/settings/chat/providers',
      undefined,
      undefined,
      undefined,
    ])
    expect(apiFetch.mock.calls[1]).toEqual([
      '/v0/settings/chat/providers',
      undefined,
      undefined,
      'host-1',
    ])
  })

  it('updateProviderPreferences returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValueOnce(undefined)
    const out = await api.updateProviderPreferences([
      { id: 'codex', disabled: false, mcpDisabled: false },
    ])
    expect(out).toEqual([])
  })

  it('uploads descriptor bytes without a Blob-backed request body', async () => {
    apiFetch.mockResolvedValueOnce({ id: 'claude-custom', findings: [] })
    const file = new File(['id: claude-custom\n'], 'claude-custom.yaml', {
      type: 'application/yaml',
    })

    const report = await api.uploadDescriptor(file)

    expect(report.id).toBe('claude-custom')
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/settings/chat/descriptors',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/yaml' },
        body: expect.any(Uint8Array),
      }),
    )
    const body = apiFetch.mock.calls[0][1].body as Uint8Array
    expect(new TextDecoder().decode(body)).toBe('id: claude-custom\n')
  })
})

// THE BUG: with "native chats" off (chatIsDefaultPresentation: false — default
// landing surface 'terminal'), starting a codex chat showed "This agent has no
// terminal view attached right now". The default was consulted only at DISPLAY
// time, so the create named no `surface` and the daemon forked codex's default
// face — the api transport, which forks NO PTY — leaving the pane asking for a
// terminal view that never existed. `createSurfaceFor` is the one place that
// derivation lives.
describe('createSurfaceFor', () => {
  const CAN_START = {
    id: 'codex',
    hasTerminal: true,
    terminalStartHere: true,
  } as api.AgentProvider
  const CANNOT_START = {
    id: 'legacy',
    hasTerminal: true,
    terminalStartHere: false,
  } as api.AgentProvider

  function setLandingDefault(landing: 'chat' | 'terminal'): void {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: landing === 'chat' },
    }))
  }

  afterEach(() => setLandingDefault('chat'))

  it("names the terminal when that is the user's default and the provider may start there", () => {
    setLandingDefault('terminal')
    expect(api.createSurfaceFor(CAN_START)).toBe('terminal')
  })

  it('names nothing for a provider whose terminal is not a landing surface', () => {
    setLandingDefault('terminal')
    expect(api.createSurfaceFor(CANNOT_START)).toBeUndefined()
  })

  it("names nothing when the user's default is Chat, whichever provider it is", () => {
    setLandingDefault('chat')
    expect(api.createSurfaceFor(CAN_START)).toBeUndefined()
    expect(api.createSurfaceFor(CANNOT_START)).toBeUndefined()
  })

  // An explicitly asked-for surface is the "start THIS chat on the CLI"
  // affordance (⌥⌘N, the row menu). It answers for itself in both directions —
  // it must not be re-derived from a preference it exists to override.
  it('lets an explicit surface win over the default, both ways round', () => {
    setLandingDefault('chat')
    expect(api.createSurfaceFor(CAN_START, 'terminal')).toBe('terminal')
    setLandingDefault('terminal')
    expect(api.createSurfaceFor(CAN_START, 'chat')).toBe('chat')
  })

  // providerCanStartOnTerminal reads PERMISSIVE for an unresolved provider on
  // purpose — an affordance stays offered and leaves the refusal to click time.
  // A CREATE cannot borrow that: it forks the face it names, so "we do not know
  // yet" must never become "born on the terminal".
  it('never names the terminal for an unresolved provider, though the gate is permissive', () => {
    setLandingDefault('terminal')
    expect(api.providerCanStartOnTerminal(undefined)).toBe(true)
    expect(api.createSurfaceFor(undefined)).toBeUndefined()
  })
})
