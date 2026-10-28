import { useEffect, useEffectEvent, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { revokeHost } from '@/features/hosts/lib/host-revoke'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { HostDotState } from '@/features/hosts/types'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useConnectionStore } from '@/lib/ws/connection-store'
import {
  onNativeChromeIntent,
  setNativeHostsSettings,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

/** Re-renders when any of `hostIds` changes status. */
function useHostStatusVersion(hostIds: readonly string[]): number {
  const [version, setVersion] = useState(0)
  const key = hostIds.join('\n')
  useEffect(() => {
    const bump = () => setVersion((v) => v + 1)
    const stops = key
      .split('\n')
      .filter(Boolean)
      .map((id) => subscribeHostStatusOf(id, bump))
    return () => stops.forEach((stop) => stop())
  }, [key])
  return version
}

/**
 * Feeds the phone's Hosts settings tab (a webview with no IPC) while the native settings sheet is
 * up: every paired host, pushed whole on each change. Revoke runs here, where the IPC is, with
 * the desktop's own flow (its questions are asked natively over the sheet); a host picked to
 * show is the chat list's own `selectHost` intent.
 */
export function NativeHostsSettingsFeed({ shownHost }: { shownHost: string | null }) {
  const navigate = useNavigate()
  const hosts = useHostConfigStore((s) => s.hosts)
  const hostStreams = useConnectionStore((s) => s.hostStreams)
  const statusVersion = useHostStatusVersion(hosts.map((h) => h.id))
  const [revoking, setRevoking] = useState<ReadonlySet<string>>(() => new Set())

  useEffect(() => {
    const stateOf = (id: string): HostDotState => {
      const status = hostStatusOf(id)
      return status === 'up' && hostStreams[id] === 'disconnected' ? 'no-live-stream' : status
    }
    setNativeHostsSettings({
      hosts: hosts.map((h) => ({
        id: h.id,
        name: h.name,
        endpoint: h.endpoint,
        state: stateOf(h.id),
      })),
      revoking: [...revoking],
      shownHost,
    })
  }, [hosts, hostStreams, statusVersion, revoking, shownHost])

  const revoke = (id: string) => {
    const host = hosts.find((h) => h.id === id)
    if (!host || revoking.has(id)) return
    setRevoking((ids) => new Set(ids).add(id))
    void revokeHost(host).finally(() =>
      setRevoking((ids) => {
        const next = new Set(ids)
        next.delete(id)
        return next
      }),
    )
  }

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'revokeHost') revoke(intent.id)
    if (intent.type === 'addHost') {
      // Pairing is its own screen with its own native sheet: Settings goes first.
      useUIState.getState().setIsSettingsDialogVisible(false)
      void navigate({ to: '/add-host' })
    }
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])
  return null
}
