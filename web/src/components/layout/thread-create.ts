import { createChat } from '@/features/agent/api/agent-chat-creates'
import { createSurfaceFor, type AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { runOperation } from '@/core/entities/run-operation'
import type { OperationItem } from '@/core/entities/operations'
import { toast } from '@/features/window/stores/toast-store'

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
  const provider = enabledProvider()
  if (!provider) return
  if (createInFlight.has(spec.inFlightKey)) return
  createInFlight.add(spec.inFlightKey)
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

/** The provider a new chat is started with, or null — having SAID SO — when
 *  there is none: a silent return here is indistinguishable from a dead button,
 *  and the sidebar may be the only thing on screen to say it. */
export function enabledProvider(): AgentProvider | null {
  const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
  if (provider) return provider
  toast.error(
    'No agent provider is enabled',
    'Enable one in Settings → Providers to start a chat or fork a workspace.',
  )
  return null
}
