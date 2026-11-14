import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BrowserAgentPeer } from '@/features/browser/components/browser-agent-peer'

const peer = vi.hoisted(() => ({ attach: vi.fn(), detach: vi.fn(), tauri: true }))
vi.mock('@/features/browser/lib/agent-peer', () => ({ attachBrowserAgentPeer: peer.attach }))
vi.mock('@/lib/crowbar-bridge', () => ({ isDesktopApp: () => peer.tauri }))

beforeEach(() => {
  peer.attach.mockReset().mockReturnValue(peer.detach)
  peer.detach.mockReset()
  peer.tauri = true
})

// The peer module resolves once per file; awaiting it here lets every pending
// import in the component settle first.
const settle = () => import('@/features/browser/lib/agent-peer')

describe('BrowserAgentPeer', () => {
  it('attaches the peer in the desktop app and detaches it on unmount', async () => {
    const view = render(<BrowserAgentPeer />)
    await vi.waitFor(() => expect(peer.attach).toHaveBeenCalledOnce())
    view.unmount()
    expect(peer.detach).toHaveBeenCalledOnce()
  })

  it('never attaches when unmounted before the peer module loads', async () => {
    render(<BrowserAgentPeer />).unmount()
    await settle()
    expect(peer.attach).not.toHaveBeenCalled()
  })

  it('does not attach outside the desktop app, the phone app included', async () => {
    peer.tauri = false
    render(<BrowserAgentPeer />)
    await settle()
    expect(peer.attach).not.toHaveBeenCalled()
  })
})
