import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ChatLoadFailure } from '@/features/agent/transcript/chat-load-failure'
import { ApiError, proxyMarkerError } from '@/lib/api/errors'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

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
})
