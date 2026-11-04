import { act, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import type { NativeAgentsSettings, NativeChromeIntent } from '@/lib/native-chrome'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeAgentsSettings[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeAgentsSettings: (s: NativeAgentsSettings) => chrome.pushed.push(s),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))
const api = vi.hoisted(() => ({
  list: vi.fn(),
  put: vi.fn(),
  getLevel: vi.fn(),
  putLevel: vi.fn(),
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  listGlobalProviders: api.list,
  updateProviderPreferences: api.put,
  getDefaultPermissionLevel: api.getLevel,
  updateDefaultPermissionLevel: api.putLevel,
}))

const { NativeAgentsSettingsFeed } =
  await import('@/features/settings/components/native-agents-settings-feed')

const agent = (id: string, enabled = true): AgentProvider =>
  ({ id, displayName: id, icon: id, connected: true, enabled, mcpEnabled: true }) as AgentProvider
const last = () => chrome.pushed.at(-1)!
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  Object.values(api).forEach((fn) => fn.mockReset())
  api.list.mockResolvedValue([agent('claude'), agent('codex')])
  api.getLevel.mockResolvedValue('trusted')
  api.putLevel.mockImplementation(async (level: string) => level)
  useHostConfigStore.setState({
    hosts: [{ id: 'h1', name: 'dockerlab' } as never],
  })
  useAgentProvidersStore.setState({ remote: {} })
})

describe('NativeAgentsSettingsFeed', () => {
  it("pushes the host's agents and default level, read from that host", async () => {
    render(<NativeAgentsSettingsFeed hostId="h1" hostName="dockerlab" />)
    await waitFor(() => expect(last().status).toBe('ready'))
    expect(api.list).toHaveBeenCalledWith('h1')
    expect(api.getLevel).toHaveBeenCalledWith('h1')
    expect(last()).toMatchObject({
      hostId: 'h1',
      hostName: 'dockerlab',
      permissionLevel: 'trusted',
    })
    expect(last().providers.map((p) => p.id)).toEqual(['claude', 'codex'])
  })

  it("writes the page's edits to that host, and pushes the result", async () => {
    render(<NativeAgentsSettingsFeed hostId="h1" hostName="dockerlab" />)
    await waitFor(() => expect(last().status).toBe('ready'))
    api.put.mockResolvedValueOnce([agent('codex'), agent('claude', false)])

    send({ type: 'reorderProviders', ids: ['codex', 'claude'] })
    expect(api.put).toHaveBeenCalledWith(
      [expect.objectContaining({ id: 'codex' }), expect.objectContaining({ id: 'claude' })],
      'h1',
    )
    await waitFor(() => expect(last().providers.map((p) => p.id)).toEqual(['codex', 'claude']))

    send({ type: 'setDefaultPermission', level: 'full-auto' })
    expect(api.putLevel).toHaveBeenCalledWith('full-auto', 'h1')
    expect(last().permissionLevel).toBe('full-auto')
  })
})
