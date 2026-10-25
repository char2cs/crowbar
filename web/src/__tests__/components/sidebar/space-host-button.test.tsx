import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { SpaceHostButton } from '@/components/sidebar/space-host-button'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { reportChannelState, resetConnectionState } from '@/lib/ws/connection-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

function renderButton(status: HostStatus) {
  pairRemoteHost('h1', 'homelab-prox')
  getOrCreateHostRuntime('h1').getState().setStatus(status)
  render(<SpaceHostButton hostId="h1" />)
}

const dot = () => screen.getByTestId('host-status-h1')

beforeEach(() => {
  resetHostWorld()
  useUIState.setState({ isSettingsOpen: false, settingsInitialTab: 'appearance' })
})

afterEach(() => {
  destroyHostRuntime('h1')
  resetConnectionState()
})

describe('SpaceHostButton', () => {
  it.each([
    ['up', 'Host settings: homelab-prox, Online'],
    ['down', 'Host settings: homelab-prox, Offline'],
    ['needs-repair', 'Host settings: homelab-prox, Needs pairing again'],
  ] as const)('a host that is %s is named %j, its dot in that state', (status, name) => {
    renderButton(status)

    expect(screen.getByRole('button', { name })).toContainElement(dot())
    expect(dot()).toHaveAttribute('data-host-state', status)
    expect(dot()).not.toHaveAttribute('title')
  })

  it('a removed host is named for what it is now', () => {
    pairRemoteHost('h1', 'homelab-prox')
    useHostConfigStore.getState().removeHost('h1')
    render(<SpaceHostButton hostId="h1" />)

    const button = screen.getByRole('button', { name: 'Host settings: Host not available' })
    expect(button).toHaveTextContent('Host not available')
    expect(dot()).toHaveAttribute('data-host-tone', 'offline')
  })

  // Quiet while the host is online; short of that, the whole mark turns and says why in words.
  it.each([
    ['down', 'offline', 'Offline'],
    ['needs-repair', 'needs-attention', 'Needs pairing again'],
    ['probing', 'unsettled', 'Checking…'],
  ] as const)('a host that is %s turns the button %s and says %j', (status, tone, word) => {
    renderButton('up')
    const button = screen.getByRole('button')
    expect(button).not.toHaveAttribute('data-host-tone')
    expect(button.textContent).toBe('')

    act(() => getOrCreateHostRuntime('h1').getState().setStatus(status))

    expect(button).toHaveAttribute('data-host-tone', tone)
    expect(button).toHaveTextContent(word)
  })

  it('stays quiet for a host that answers while its live streams are closed', () => {
    renderButton('up')

    act(() => reportChannelState('crowbar://h1/v0/ws/git', false, 'h1'))

    const button = screen.getByRole('button', {
      name: 'Host settings: homelab-prox, Online — no live stream',
    })
    expect(button).not.toHaveAttribute('data-host-tone')
    expect(button.textContent).toBe('')
  })

  it('opens Settings on the Hosts tab', async () => {
    renderButton('down')

    await userEvent.setup().click(screen.getByRole('button'))

    expect(useUIState.getState()).toMatchObject({
      isSettingsOpen: true,
      settingsInitialTab: 'hosts',
    })
  })

  it('says the same on hover as it reads out', async () => {
    renderButton('down')

    await userEvent.setup().hover(screen.getByRole('button'))

    expect(await screen.findByText('Host settings: homelab-prox, Offline')).toBeVisible()
  })
})
