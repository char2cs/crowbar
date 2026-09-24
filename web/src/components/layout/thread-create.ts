import { createChat, createSurfaceFor, type AgentProvider } from '@/features/agent/api/agent-api'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { toast } from '@/features/window/stores/toast-store'
import { renderedProjectRows, visibleRepos } from '@/components/sidebar/lib/drop-actions'

// The create machinery every sidebar create shares — the in-flight guard, the
// pending row's slot, the bounded wait for the real row — and THE thread
// create, used by repo rows, project-home rows and the home header alike.

// One request in flight per key (kind + parent): a burst of clicks on one "+"
// mints one chat, not one per click — concurrent forks off one parent lose the
// runner's startup race and leave chats that can never be resumed.
export const createInFlight = new Set<string>()

/** The panel's rows as drawn at click time: the slot a create appends at is
 *  the daemon's NextSlot — every kind under `parentId`, repo headers included
 *  at the home root — and the ids `hideRowsForInFlightCreates` keeps. */
export function panelRowsAtClick(
  projectId: string,
  parentId: string,
): { order: number; rowIdsAtClick: string[] } {
  const rows = renderedProjectRows(visibleRepos(), projectId)
  return {
    order: rows.filter((r) => (r.parentId ?? '') === parentId).length,
    rowIdsAtClick: rows.map((r) => r.id),
  }
}

/** Fails the pending row with the daemon's own reason, toasted and logged so it is diagnosable. */
export function failCreate(tempId: string, err: unknown, fallback: string): void {
  const reason = err instanceof Error ? err.message : fallback
  usePendingCreatesStore.getState().setError(tempId, reason)
  toast.error(fallback, reason)
  console.error(`${fallback}:`, err)
}

/** How long a created row may take to arrive before the create is reported
 *  failed — the same bound `awaitEntity` puts on an entity after its 202. */
const ROW_ARRIVAL_TIMEOUT_MS = 30_000

/**
 * Resolves once `landed()` holds, re-checked on every write `subscribe`
 * reports; rejects if it has not within {@link ROW_ARRIVAL_TIMEOUT_MS}. The
 * row normally arrives through the ordinary reseed/WS path — the bound is for
 * the daemon failing after it answered, which used to leave the pending row
 * spinning (and this subscription alive) forever.
 */
export function untilLanded(
  subscribe: (onChange: () => void) => () => void,
  landed: () => boolean,
): Promise<void> {
  return new Promise((resolve, reject) => {
    if (landed()) {
      resolve()
      return
    }
    const timer = setTimeout(() => {
      unsubscribe()
      reject(new Error('The new row never arrived from the daemon'))
    }, ROW_ARRIVAL_TIMEOUT_MS)
    const unsubscribe = subscribe(() => {
      if (!landed()) return
      clearTimeout(timer)
      unsubscribe()
      resolve()
    })
  })
}

/** One thread create: where it runs, where it lands, and what follows. */
export interface ThreadCreate {
  /** One request in flight per key, so a double-click mints one chat. */
  inFlightKey: string
  projectId: string
  workspaceId: string
  /** Placement parent, in chat-id space; '' is the panel root. */
  parentId: string
  presentation?: LandingChatPresentation
  /** Settles once the new chat's row is drawn where it was placed. */
  landed: (chatId: string) => Promise<void>
  /** Runs once the chat exists — opens it. */
  onCreated?: (chatId: string) => void | Promise<void>
}

/**
 * THE thread create, for a repo workspace and a project home alike: draws the
 * pending row at the slot the daemon appends at, mints the chat, hides the real
 * row until `landed` confirms its placement, and fails the row with the
 * daemon's reason on a refused create or a row that never arrives.
 */
export async function startThread(spec: ThreadCreate): Promise<void> {
  const provider = enabledProvider()
  if (!provider) return
  if (createInFlight.has(spec.inFlightKey)) return
  createInFlight.add(spec.inFlightKey)
  const { projectId, workspaceId, parentId } = spec
  const { order, rowIdsAtClick } = panelRowsAtClick(projectId, parentId)
  const tempId = `pending-${crypto.randomUUID()}`
  usePendingCreatesStore.getState().addCreating({
    tempId,
    kind: 'chat',
    projectId,
    parentId,
    order,
    workspaceId,
    ownsWorktree: false,
    rowIdsAtClick,
  })
  // The guard covers the REQUEST only: `landed` can take up to
  // ROW_ARRIVAL_TIMEOUT_MS, and holding it that long would make "+" inert.
  const surface = createSurfaceFor(provider, spec.presentation)
  let chatId: string
  try {
    chatId = await createChat(workspaceId, provider.id, parentId, surface)
  } catch (err) {
    failCreate(tempId, err, 'Failed to start chat')
    return
  } finally {
    createInFlight.delete(spec.inFlightKey)
  }
  // Before anything opens a pane on it: the surface actually created.
  if (surface) presetChatLandingPresentation(chatId, surface)
  usePendingCreatesStore.getState().attachRealId(tempId, chatId)
  void spec.landed(chatId).then(
    () => usePendingCreatesStore.getState().clear(tempId),
    (err: unknown) => failCreate(tempId, err, 'Failed to start chat'),
  )
  await spec.onCreated?.(chatId)
}

/**
 * The provider a new chat is started with, or null — having SAID SO — when
 * there is none.
 *
 * Both create paths used to return silently here. A silent return is
 * indistinguishable from a dead button, and it is the shape both halves of "the
 * fork and thread buttons do nothing" took: one because it genuinely had no
 * providers to find (see the thread branch's own note), the other because a
 * real outage empties this list (`use-workspace-agent-chats-stream.ts` toasts
 * once for that, but only for a MOUNTED workspace — the sidebar can be the only
 * thing on screen). A precondition that stops a click has to be visible.
 */
export function enabledProvider(): AgentProvider | null {
  const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
  if (provider) return provider
  toast.error(
    'No agent provider is enabled',
    'Enable one in Settings → Providers to start a chat or fork a workspace.',
  )
  return null
}
