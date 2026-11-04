import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import type { NativeAgentsSettings, NativeHostsSettings } from '@/lib/native-chrome'
import { MobileSettingsPage } from '@/features/settings/components/mobile-settings-page'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'

type Shell = {
  webkit?: unknown
  __crowbarAgentsSettings?: (s: NativeAgentsSettings) => void
  __crowbarHostsSettings?: (s: NativeHostsSettings) => void
}
const shell = window as unknown as Shell
const posted = vi.fn()
const openTab = (name: string) =>
  fireEvent.click(within(screen.getByRole('tablist')).getByRole('tab', { name }))

beforeEach(() => {
  posted.mockClear()
  shell.webkit = { messageHandlers: { settings: { postMessage: posted } } }
})
afterEach(() => {
  delete shell.webkit
  useSettingsStore.setState((s) => {
    s.settings.compactGitStatusBadges = getDefaultSetting('compactGitStatusBadges')
  })
})

describe('MobileSettingsPage', () => {
  // The phone has no editor, file tree settings, browser or keyboard of its own to configure.
  it('offers only the tabs that apply to the phone, with search and Appearance first', async () => {
    render(<MobileSettingsPage />)
    const tabs = within(screen.getByRole('tablist')).getAllByRole('tab')
    expect(tabs.map((t) => t.textContent)).toEqual([
      'Appearance',
      'Terminal',
      'Agents',
      'Git',
      'Hosts',
    ])
    expect(screen.getByPlaceholderText('Search settings...')).toBeInTheDocument()
    expect(await screen.findByText('Color Theme')).toBeInTheDocument()
  })

  it('edits through the settings store the app reads', async () => {
    render(<MobileSettingsPage />)
    openTab('Git')
    const before = useSettingsStore.getState().settings.compactGitStatusBadges
    fireEvent.click((await screen.findAllByRole('switch'))[0])
    expect(useSettingsStore.getState().settings.compactGitStatusBadges).toBe(!before)
  })

  // A closed select names its choice as the list does, not by its stored value.
  it('names each chosen option by its label', async () => {
    useSettingsStore.setState((s) => {
      s.settings.themeMode = 'system'
    })
    render(<MobileSettingsPage />)
    expect(await screen.findByText('Sync with System')).toBeInTheDocument()
    expect(screen.queryByText('system')).not.toBeInTheDocument()
  })

  // No daemon here: Agents draws the main webview's snapshot and sends every edit back.
  it('draws the agents it is sent and sends edits back to the shell', async () => {
    render(<MobileSettingsPage />)
    openTab('Agents')
    await waitFor(() => expect(posted).toHaveBeenCalledWith({ type: 'agentsSettingsReady' }))
    const agent = (id: string, displayName: string): AgentProvider =>
      ({
        id,
        displayName,
        icon: id,
        connected: true,
        enabled: true,
        mcpEnabled: true,
      }) as AgentProvider
    act(() =>
      shell.__crowbarAgentsSettings!({
        hostId: 'h1',
        hostName: 'dockerlab',
        status: 'ready',
        providers: [agent('claude', 'Claude Code'), agent('codex', 'Codex')],
        permissionLevel: 'trusted',
      }),
    )
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByTestId('default-permission-level-trigger')).toHaveTextContent('Trusted')
    fireEvent.click(screen.getByRole('switch', { name: 'Enable Claude Code' }))
    expect(posted).toHaveBeenCalledWith({ type: 'toggleProvider', id: 'claude', enabled: false })
  })

  it('draws the paired hosts it is sent, and sends revoke and Add host back', async () => {
    render(<MobileSettingsPage />)
    openTab('Hosts')
    await waitFor(() => expect(posted).toHaveBeenCalledWith({ type: 'hostsSettingsReady' }))
    act(() =>
      shell.__crowbarHostsSettings!({
        hosts: [{ id: 'h1', name: 'dockerlab', endpoint: 'https://lab:7777', state: 'up' }],
        revoking: [],
      }),
    )
    expect(screen.getByText('dockerlab')).toBeInTheDocument()
    expect(screen.getByText('https://lab:7777')).toBeInTheDocument()
    expect(screen.queryByText('This computer')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Revoke dockerlab' }))
    expect(posted).toHaveBeenCalledWith({ type: 'revokeHost', id: 'h1' })
    fireEvent.click(screen.getByRole('button', { name: 'Add host' }))
    expect(posted).toHaveBeenCalledWith({ type: 'addHost' })
  })
})
