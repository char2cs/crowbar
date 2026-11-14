import { useEffect, useMemo, useState } from 'react'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { revokeHost } from '@/features/hosts/lib/host-revoke'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { HostDotState } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'
import type { NativeHostsSettings } from '@/lib/native-chrome'

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
 * Every paired host as a native host list shows it, plus the revoke that list asks for. Revoke
 * runs here, where the IPC is, with the desktop's own flow (its questions are asked natively).
 */
export function useNativeHostsSnapshot(shownHost: string | null) {
  const hosts = useHostConfigStore((s) => s.hosts)
  const hostStreams = useConnectionStore((s) => s.hostStreams)
  const statusVersion = useHostStatusVersion(hosts.map((h) => h.id))
  const [revoking, setRevoking] = useState<ReadonlySet<string>>(() => new Set())

  const snapshot = useMemo<NativeHostsSettings>(() => {
    // hostStatusOf is a module-level read: the version is what says it changed.
    void statusVersion
    const stateOf = (id: string): HostDotState => {
      const status = hostStatusOf(id)
      return status === 'up' && hostStreams[id] === 'disconnected' ? 'no-live-stream' : status
    }
    return {
      hosts: hosts.map((h) => ({
        id: h.id,
        name: h.name,
        endpoint: h.endpoint,
        state: stateOf(h.id),
      })),
      revoking: [...revoking],
      shownHost,
    }
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

  return { snapshot, revoke }
}
