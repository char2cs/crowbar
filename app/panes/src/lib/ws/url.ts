import { isTauri } from '@/lib/crowbar-bridge'
import { apiBaseFor } from '@/lib/api/fetch'
import { inShell } from '@/lib/ws/in-shell'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__

export function wsUrl(path: string, hostId = 'local'): string {
  if (/^wss?:\/\//i.test(path)) return path

  const base =
    hostId === 'local'
      ? crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
      : apiBaseFor(hostId)
  const resolved = new URL(path, base)
  if (resolved.protocol === 'https:') resolved.protocol = 'wss:'
  else if (resolved.protocol === 'http:') resolved.protocol = 'ws:'
  return resolved.toString()
}

export function isWebSocketCapable(): boolean {
  if (isTauri() || inShell()) return true
  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  return /^(https?|wss?):\/\//i.test(base)
}
