import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: () => true,
  unregisterRemoteHost: vi.fn(),
  forgetHostToken: vi.fn(),
  probeRemoteFingerprint: vi.fn(),
}))
// A smart, path-agnostic mock: every test drives it with mockResolvedValue /
// mockRejectedValue (never *Once*), so the DELETE call this suite asserts on
// can never race a row's own health-probe apiFetch call for the same mock's
// queued return value.
vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(), isHostIdentityChangedError: vi.fn(() => false) }))
const toastErrorFn = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

import { apiFetch } from '@/lib/api'
import { unregisterRemoteHost, forgetHostToken } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { HostsSettings } from '@/features/settings/components/tabs/hosts-settings'

describe('HostsSettings', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
    vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
    vi.mocked(unregisterRemoteHost).mockReset()
    vi.mocked(forgetHostToken).mockReset()
    toastErrorFn.mockReset()
  })

  it('lists Local plus every saved host, each with a live status dot', () => {
    useHostConfigStore.setState({
      hosts: [
        {
          id: 'h1',
          name: 'homelab',
          endpoint: '1.2.3.4:3737',
          pairedFingerprint: 'SHA256:aaaa',
          clientId: 'c1',
        },
      ],
    })
    render(<HostsSettings />)
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('homelab')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-local')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-h1')).toBeInTheDocument()
  })

  it('does not offer to revoke or remove the Local entry', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    const localRow = screen.getByText('Local').closest('[data-testid="host-row"]')
    expect(localRow?.querySelector('[aria-label*="revoke" i], [aria-label*="remove" i]')).toBeNull()
  })

  it('clicking Add host opens the add-host dialog', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    fireEvent.click(screen.getByRole('button', { name: /add.*host/i }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('clicking revoke calls the daemon DELETE first, then local cleanup, then drops the config row', async () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(useHostConfigStore.getState().hosts).toEqual([]))
    // Targets the host's OWN authed channel (the 4th positional hostId arg) —
    // never the local daemon's, which is the whole point of the amendment.
    expect(apiFetch).toHaveBeenCalledWith('/v0/clients/c1', { method: 'DELETE' }, undefined, 'h1')
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
    // No orphaned runtime-registry entry left behind for a host that no
    // longer exists.
    expect(getHostRuntime('h1')).toBeUndefined()
  })

  it('a failed server-side revoke leaves the host config and local token untouched', async () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    vi.mocked(apiFetch).mockReset().mockRejectedValue(new Error('host unreachable'))
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    expect(unregisterRemoteHost).not.toHaveBeenCalled()
    expect(forgetHostToken).not.toHaveBeenCalled()
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
  })
})
