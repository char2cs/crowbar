import { createChat } from '@/features/agent/api/agent-chat-creates'
import {
  createSurfaceFor,
  listGlobalProviders,
  type AgentProvider,
} from '@/features/agent/api/agent-providers-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  providersOf,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import { hostUnavailability, resolvedHostOf } from '@/lib/host-resolve'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { runOperation } from '@/core/entities/run-operation'
import type { OperationItem } from '@/core/entities/operations'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure, toastUnresolvedHost } from '@/features/hosts/lib/host-failure-notice'

export const createInFlight = new Set<string>()

export interface ThreadCreate {
  inFlightKey: string
  projectId: string
  workspaceId: string
  parentId: string
  presentation?: LandingChatPresentation
  providerId?: string
  onCreated?: (chatId: string) => void | Promise<void>
}

const THREAD_LABEL = 'New thread'

export async function startThread(spec: ThreadCreate): Promise<void> {
  if (createInFlight.has(spec.inFlightKey)) return
  createInFlight.add(spec.inFlightKey)
  const read = readUnreadHostProviders(spec.projectId)
  const provider = read && !(await read) ? null : enabledProvider(spec.projectId, spec.providerId)
  if (!provider) {
    createInFlight.delete(spec.inFlightKey)
    return
  }
  const { projectId, workspaceId, parentId } = spec
  const surface = createSurfaceFor(provider, spec.presentation)
  const chatId = crypto.randomUUID()
  const item = (state: OperationItem['state']): OperationItem => ({
    ref: { kind: 'chat', id: chatId },
    label: THREAD_LABEL,
    state,
    parentId,
  })
  let created: Promise<string> | undefined
  runOperation(
    {
      kind: 'chat.create',
      subject: { kind: 'workspace', id: workspaceId },
      projectId,
      label: THREAD_LABEL,
      items: [item('accepted')],
    },
    (opId) => {
      created = createChat(chatId, opId, workspaceId, provider.id, parentId, surface)
      return created.then(() => ({ items: [item('succeeded')] }))
    },
  )
  const ok = await created?.then(
    () => true,
    () => false,
  )
  createInFlight.delete(spec.inFlightKey)
  if (!ok) return
  if (surface) presetChatLandingPresentation(chatId, surface)
  await spec.onCreated?.(chatId)
}

export function enabledProvider(projectId: string, preferred = ''): AgentProvider | null {
  const hostId = resolvedHostOf({ projectId })
  if (hostId === null) {
    toastUnresolvedHost(hostUnavailability({ projectId }))
    return null
  }
  const provider = startingProvider(projectId, preferred)
  if (provider) return provider
  if (hostId === LOCAL_HOST.id) {
    toast.error(
      'No agent provider is enabled',
      'Enable one in Settings → Providers to start a chat or fork a workspace.',
    )
  } else {
    const name = useHostConfigStore.getState().hosts.find((h) => h.id === hostId)?.name
    const host = name ?? 'that host'
    toast.error(
      `No agent provider is enabled on ${host}`,
      `Enable one on ${host} itself: Settings → Providers sets up only this computer.`,
    )
  }
  return null
}

export function startingProvider(projectId: string, preferred = ''): AgentProvider | undefined {
  const hostId = resolvedHostOf({ projectId })
  if (hostId === null) return undefined
  const enabled = providersOf(useAgentProvidersStore.getState(), hostId).filter((p) => p.enabled)
  return enabled.find((p) => p.id === preferred) ?? enabled[0]
}

export function readUnreadHostProviders(projectId: string): Promise<boolean> | null {
  const hostId = resolvedHostOf({ projectId })
  const state = useAgentProvidersStore.getState()
  if (hostId === null || hostId === LOCAL_HOST.id || state.remote[hostId] !== undefined) return null
  const name = useHostConfigStore.getState().hosts.find((h) => h.id === hostId)?.name
  return listGlobalProviders(hostId).then(
    (providers) => {
      useAgentProvidersStore.getState().setHostProviders(hostId, providers)
      return true
    },
    (err: unknown) => {
      if (!toastHostFailure(err)) {
        toast.error(
          `Couldn’t reach ${name ?? 'that host'}`,
          'Check that it is online in Settings → Hosts.',
        )
      }
      return false
    },
  )
}
