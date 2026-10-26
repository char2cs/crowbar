import { useEffect } from 'react'
import { isDesktopApp } from '@/lib/crowbar-bridge'

/** Lets agents drive this window's browser tabs; the native webviews exist only in the desktop app. */
export function BrowserAgentPeer() {
  useEffect(() => {
    if (!isDesktopApp()) return
    // Loaded on mount, not with the entry: the peer carries the page-agent
    // scripts, which the boot bundle budget has no room for.
    let detach: (() => void) | undefined
    let unmounted = false
    void import('@/features/browser/lib/agent-peer').then(({ attachBrowserAgentPeer }) => {
      if (!unmounted) detach = attachBrowserAgentPeer()
    })
    return () => {
      unmounted = true
      detach?.()
    }
  }, [])
  return null
}
