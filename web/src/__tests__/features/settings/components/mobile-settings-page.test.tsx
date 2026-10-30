import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import type { NativeAgentsSettings } from '@/lib/native-chrome'
import { MobileSettingsPage } from '@/features/settings/components/mobile-settings-page'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'

type Shell = {
  webkit?: unknown
  __crowbarAgentsSettings?: (s: NativeAgentsSettings) => void
  __crowbarSettingsTab?: (tab: string | null) => void
}
const shell = window as unknown as Shell
const posted = vi.fn()

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

const showTab = (tab: string | null) => act(() => shell.__crowbarSettingsTab!(tab))

describe('MobileSettingsPage', () => {
  // The title, back button and list are native: the page is only the tab it is told to draw.
  it('draws nothing until the shell names a tab, then just that tab', async () => {
    render(<MobileSettingsPage />)
    await waitFor(() => expect(posted).toHaveBeenCalledWith({ type: 'settingsTabReady' }))
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('Search settings...')).not.toBeInTheDocument()
    showTab('appearance')
    // The first load of a tab's chunk can outlast the default wait on a busy machine.
    expect(await screen.findByText('Color Theme', {}, { timeout: 8000 })).toBeInTheDocument()
    showTab(null)
    expect(screen.queryByText('Color Theme')).not.toBeInTheDocument()
  })

  it('edits through the settings store the app reads', async () => {
    render(<MobileSettingsPage />)
    showTab('git')
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
    showTab('appearance')
    expect(await screen.findByText('Sync with System')).toBeInTheDocument()
    expect(screen.queryByText('system')).not.toBeInTheDocument()
  })

  // No daemon here: Agents draws the main webview's snapshot and sends every edit back.
  it('draws the agents it is sent and sends edits back to the shell', async () => {
    render(<MobileSettingsPage />)
    showTab('providers')
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
})
