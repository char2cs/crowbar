import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SpaceHostScope } from '@/components/sidebar/space-host-scope'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { resetConnectionState, useConnectionStore } from '@/lib/ws/connection-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const scope = () => screen.getByTestId('space-host-scope')

function renderRows(hostId: string, onOpen = vi.fn()) {
  render(
    <SpaceHostScope hostId={hostId}>
      <button type="button" onClick={onOpen}>
        Fix the thing
      </button>
      <button type="button">Untitled chat</button>
    </SpaceHostScope>,
  )
  return onOpen
}

function onRemote(status: HostStatus) {
  pairRemoteHost('h1', 'homelab-prox')
  getOrCreateHostRuntime('h1').getState().setStatus(status)
}

beforeEach(() => resetHostWorld())

afterEach(() => {
  destroyHostRuntime('h1')
  resetConnectionState()
})

describe('SpaceHostScope', () => {
  it.each(['down', 'needs-repair'] as const)(
    'greys out every row under a project whose host is %s',
    (status) => {
      onRemote(status)
      renderRows('h1')

      expect(scope()).toHaveAttribute('data-host-unreachable')
      expect(scope()).toContainElement(screen.getByRole('button', { name: 'Fix the thing' }))
      expect(scope()).toContainElement(screen.getByRole('button', { name: 'Untitled chat' }))
    },
  )

  it('greys out a project whose host is no longer configured', () => {
    onRemote('up')
    useHostConfigStore.getState().removeHost('h1')
    renderRows('h1')

    expect(scope()).toHaveAttribute('data-host-unreachable')
  })

  it.each(['up', 'unknown', 'probing'] as const)('leaves rows alone while the host is %s', (s) => {
    onRemote(s)
    renderRows('h1')

    expect(scope()).not.toHaveAttribute('data-host-unreachable')
  })

  it('leaves a local project alone, whatever the local daemon’s status', () => {
    useConnectionStore.setState({ status: 'disconnected' })
    renderRows('local')

    expect(scope()).not.toHaveAttribute('data-host-unreachable')
  })

  it('greys out and back as the host goes and returns, keeping the rows mounted', () => {
    onRemote('up')
    renderRows('h1')
    const row = screen.getByRole('button', { name: 'Fix the thing' })

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('down'))
    expect(scope()).toHaveAttribute('data-host-unreachable')
    act(() => getOrCreateHostRuntime('h1').getState().setStatus('up'))
    expect(scope()).not.toHaveAttribute('data-host-unreachable')
    expect(screen.getByRole('button', { name: 'Fix the thing' })).toBe(row)
  })

  // Greyed is a look, not a lock: what a row opens says why its host can't be reached.
  it('a greyed row still takes its click', async () => {
    onRemote('down')
    const onOpen = renderRows('h1')

    await userEvent.setup().click(screen.getByRole('button', { name: 'Fix the thing' }))

    expect(onOpen).toHaveBeenCalledOnce()
  })
})
