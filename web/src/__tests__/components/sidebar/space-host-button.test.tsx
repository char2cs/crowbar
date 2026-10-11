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

afterEach(() => destroyHostRuntime('h1'))

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

    expect(screen.getByRole('button', { name: 'Host settings: Host not available' })).toBeVisible()
    expect(dot()).toHaveAttribute('data-host-tone', 'offline')
  })

  // Quiet while the host answers; a host that can't be reached turns the whole mark.
  it.each([
    ['down', 'offline'],
    ['needs-repair', 'needs-attention'],
  ] as const)('a host that is %s turns the button %s', (status, tone) => {
    renderButton('up')
    const button = screen.getByRole('button')
    expect(button).not.toHaveAttribute('data-host-tone')

    act(() => getOrCreateHostRuntime('h1').getState().setStatus(status))

    expect(button).toHaveAttribute('data-host-tone', tone)
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
