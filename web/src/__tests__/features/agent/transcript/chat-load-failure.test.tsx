import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ChatLoadFailure } from '@/features/agent/transcript/chat-load-failure'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { ApiError, proxyMarkerError } from '@/lib/api/errors'
import { HostNotResolvedError } from '@/lib/host-path'
import {
  loadProjects,
  markHostsRead,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

function renderFailure(error: Error) {
  const onRetry = vi.fn()
  const onOpenTerminal = vi.fn()
  const view = render(
    <ChatLoadFailure error={error} onRetry={onRetry} onOpenTerminal={onOpenTerminal} />,
  )
  return { onRetry, onOpenTerminal, ...view }
}

beforeEach(() => resetHostWorld())

describe('ChatLoadFailure', () => {
  it.each([
    [
      'the local daemon’s error',
      new ApiError('internal error', 500, undefined, undefined, 'local'),
    ],
    ['a dropped connection', new TypeError('Failed to fetch')],
  ])('%s reads as it always did: the failure, Retry and Terminal', (_, error) => {
    const { container, onRetry, onOpenTerminal } = renderFailure(error)

    expect(container).toHaveTextContent(/^Couldn’t load this chat’s messages\.Retry Terminal$/)
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    fireEvent.click(screen.getByRole('button', { name: 'Terminal' }))
    expect(onRetry).toHaveBeenCalledOnce()
    expect(onOpenTerminal).toHaveBeenCalledOnce()
  })

  // A remote chat's failure is its host's: the line under it says which host and what it did.
  it.each([
    ['cannot be reached', proxyMarkerError('host-unreachable', 'h1')!, 'Couldn’t reach homelab'],
    [
      'refuses this computer',
      new ApiError('client is not paired', 401, undefined, undefined, 'h1'),
      'homelab needs to be paired again',
    ],
  ])('a host that %s is named under the failure', (_, error, notice) => {
    pairRemoteHost('h1', 'homelab')

    renderFailure(error)

    expect(screen.getByText('Couldn’t load this chat’s messages.')).toBeVisible()
    expect(screen.getByText(notice)).toBeVisible()
  })

  it('follows the project list: "still loading" becomes "gone" once the list lands', () => {
    pairRemoteHost('h1', 'homelab')
    renderFailure(new HostNotResolvedError('project', 'p-vanished', 'the list is not loaded'))
    expect(screen.getByText('Still loading your projects')).toBeVisible()

    // Loaded, but homelab has not said what it holds yet: the project may still be there.
    act(() => loadProjects(projectOn('p-local', 'local')))
    expect(screen.getByText('Still loading your projects')).toBeVisible()

    act(() => markHostsRead('h1'))

    expect(screen.queryByText('Still loading your projects')).toBeNull()
    expect(screen.getByText('This project is no longer in your list')).toBeVisible()
  })

  it('follows the host list: a removed host stops being named', () => {
    pairRemoteHost('h1', 'homelab')
    renderFailure(proxyMarkerError('host-unreachable', 'h1')!)
    expect(screen.getByText('Couldn’t reach homelab')).toBeVisible()

    act(() => useHostConfigStore.getState().removeHost('h1'))

    expect(screen.getByText('That host is no longer paired with this computer')).toBeVisible()
  })
})
