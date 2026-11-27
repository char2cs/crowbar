import { useCallback, useSyncExternalStore } from 'react'
import { entityStore } from '@/core/entities/entity-store'
import { hostScope } from '@/core/stream/host-scope'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  chatHostOf,
  subscribeHostResolution,
  useChatHost,
  useResolvedHost,
} from '@/lib/host-resolve'
import { persistedHostOf } from '@/features/hosts/lib/host-placements'

function cachedHost(wsId: string, chatId: string): string | null {
  const hostId =
    (wsId ? persistedHostOf('workspace', wsId) : null) ?? persistedHostOf('chat', chatId)
  if (hostId === null) return null
  if (entityStore.getState().loaded[hostScope(hostId, 'projects')]) return null
  if (hostId === LOCAL_HOST.id) return hasLocalDaemon() ? hostId : null
  return useHostConfigStore.getState().hosts.some((h) => h.id === hostId) ? hostId : null
}

export function useChatPaneHost(wsId: string, chatId: string): string | null {
  const resolved = useResolvedHost({ wsId })
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostResolution(wsId, onChange),
    [wsId],
  )
  const cached = useSyncExternalStore(subscribe, () => cachedHost(wsId, chatId))
  return resolved ?? cached
}

export function isChatRecalled(chatId: string): boolean {
  return cachedHost('', chatId) !== null
}

export function feedHostOf(chatId: string): string | null {
  return chatHostOf(chatId) ?? cachedHost('', chatId)
}

export function useFeedHost(chatId: string): string | null {
  const resolved = useChatHost(chatId)
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostResolution(undefined, onChange),
    [],
  )
  const cached = useSyncExternalStore(subscribe, () => cachedHost('', chatId))
  return resolved ?? cached
}
