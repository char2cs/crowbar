import { useEffect } from 'react'
import { attachBrowserAgentPeer } from '@/features/browser/lib/agent-peer'
import { isTauri } from '@/lib/crowbar-bridge'

/** Lets agents drive this window's browser tabs; the native webviews exist only in the desktop app. */
export function BrowserAgentPeer() {
  useEffect(() => (isTauri() ? attachBrowserAgentPeer() : undefined), [])
  return null
}
