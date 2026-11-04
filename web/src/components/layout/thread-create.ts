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
import { resolvedHostOf } from '@/lib/host-resolve'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { runOperation } from '@/core/entities/run-operation'
import type { OperationItem } from '@/core/entities/operations'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure, toastUnresolvedHost } from '@/features/hosts/lib/host-failure-notice'

// The create machinery every sidebar create shares — the in-flight guard and
// THE thread create, used by repo rows, project-home rows and the home header alike.

// One request in flight per key (kind + parent): a burst of clicks on one "+"
// mints one chat, not one per click — concurrent forks off one parent lose the
// runner's startup race and leave chats that can never be resumed.
export const createInFlight = new Set<string>()

/** One thread create: where it runs, where it lands, and what follows. */
export interface ThreadCreate {
  /** One request in flight per key, so a double-click mints one chat. */
  inFlightKey: string
  projectId: string
  workspaceId: string
  /** Placement parent, in chat-id space; '' is the panel root. */
  parentId: string
  presentation?: LandingChatPresentation
  /** Runs once the daemon created the chat — opens it. */
  onCreated?: (chatId: string) => void | Promise<void>
}

const THREAD_LABEL = 'New thread'

/**
 * THE thread create, for a repo workspace and a project home alike: a local
 * operation under a minted chat id, drawn until the chat arrives (or with the
 * daemon's reason if refused); the chat opens as soon as the daemon answers.
 */
export async function startThread(spec: ThreadCreate): Promise<void> {
  if (createInFlight.has(spec.inFlightKey)) return
  createInFlight.add(spec.inFlightKey)
  const read = readUnreadHostProviders(spec.projectId)
  const provider = read && !(await read) ? null : enabledProvider(spec.projectId)
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
  // A refusal is the operation's to show: its row keeps the reason, the watcher toasts it.
  const ok = await created?.then(
    () => true,
    () => false,
  )
  createInFlight.delete(spec.inFlightKey)
  if (!ok) return
  // Before anything opens a pane on it: the surface actually created.
  if (surface) presetChatLandingPresentation(chatId, surface)
  await spec.onCreated?.(chatId)
}

/**
 * The provider a new chat in `projectId` starts with, or null having said why — a
 * click that is refused must be visible. It comes from the project's own host's
 * list, so a remote host's list must have been read: see {@link readUnreadHostProviders}.
 */
export function enabledProvider(projectId: string): AgentProvider | null {
  const hostId = resolvedHostOf({ projectId })
  if (hostId === null) {
    toastUnresolvedHost(projectId)
    return null
  }
  const provider = providersOf(useAgentProvidersStore.getState(), hostId).find((p) => p.enabled)
  if (provider) return provider
  if (hostId === LOCAL_HOST.id) {
    toast.error(
      'No agent provider is enabled',
      'Enable one in Settings → Providers to start a chat or fork a workspace.',
    )
  } else {
    // Settings → Providers sets up this computer only; a host's providers are set on it.
    const name = useHostConfigStore.getState().hosts.find((h) => h.id === hostId)?.name
    const host = name ?? 'that host'
    toast.error(
      `No agent provider is enabled on ${host}`,
      `Enable one on ${host} itself: Settings → Providers sets up only this computer.`,
    )
  }
  return null
}

/**
 * Reads the provider list of a remote host nobody has asked yet; null when there is
 * nothing to wait for, so a common click stays synchronous. False — the host named in
 * a toast — when it did not answer.
 */
export function readUnreadHostProviders(projectId: string): Promise<boolean> | null {
  const hostId = resolvedHostOf({ projectId })
  const state = useAgentProvidersStore.getState()
  if (hostId === null || hostId === LOCAL_HOST.id || state.remote[hostId] !== undefined) return null
  // Named before the wait: the host may be removed while it is asked.
  const name = useHostConfigStore.getState().hosts.find((h) => h.id === hostId)?.name
  // Read here rather than through the store's loadHost, which keeps no failure: a changed
  // identity must not read as merely unreachable.
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
