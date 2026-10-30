import { wsManager } from '@/lib/ws/manager'
import { isNotFoundError } from '@/lib/errors/api-error'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { NON_STRUCTURAL_CHAT_KINDS } from '@/lib/ws/chat-frame-kinds'
import {
  chatBase,
  getChat,
  listChats,
  mapChat,
  type AgentChat,
  type AgentTelemetry,
} from '@/features/agent/api/agent-chat-reads'
import { clearExcalidrawEditRequest } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { clearPersistedPromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
import {
  applyChat,
  applyChatFrame,
  chatStore,
  removeChats,
  setChatScope,
  setChatTelemetry,
  type ChatScopeStatus,
} from '@/features/agent/stores/chat-store'
import { runnerLeft, startedWorking } from '@/features/agent/stores/lib/reduce-chat-frame'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { chatWorkspaceIn } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

/** One frame on a lifecycle endpoint (`${chatBase(wsId)}/ws`). */
interface LifecycleFrame {
  chatId: string
  kind: string
  runnerId?: string
  /** Present only on folder frames. */
  folderId?: string
  chat?: AgentChat
  version?: number
  telemetry?: AgentTelemetry
}

// A turn's live views belong to each chat's own feed (chat-feed.ts).
const FEED_KINDS = new Set([
  'message_delta',
  'plan',
  'choice',
  'compaction_started',
  'compaction_stopped',
  'prompt_settled',
  'worktree_state',
])

interface Scope {
  /** Attach count by workspace; every workspace here shares the endpoint. */
  refs: Map<string, number>
  /** Chats this endpoint's list or frames carried. */
  members: Set<string>
  status: ChatScopeStatus
  unsubscribe: () => void
}

const scopes = new Map<string, Scope>()

const workspaceOf = (scope: Scope) => scope.refs.keys().next().value ?? ''
const isLive = (endpoint: string, scope: Scope) => scopes.get(endpoint) === scope

function setStatus(scope: Scope, status: ChatScopeStatus) {
  scope.status = status
  for (const wsId of scope.refs.keys()) setChatScope(wsId, status)
}

// The sidebar's per-repo tree re-reads on this; scoped to the endpoint's repo.
function bumpTree(scope: Scope) {
  const repoId = getWorkspaceScope(workspaceOf(scope))?.repoId
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}

function forget(chatId: string) {
  const owner = chatStore.getState().chats[chatId]?.workspaceId
  removeChats([chatId])
  if (owner) clearPersistedPromptQueue(owner, chatId)
  clearScrollPosition(chatId)
  clearExcalidrawEditRequest(chatId)
  windowPaneStore.getState().paneActions.forgetChat(chatId)
}

// A chat the list omits is a suspect, not a deletion: only its own workspace
// may confirm it gone, since asking through another mount 404s a live chat.
async function forgetIfGone(endpoint: string, scope: Scope, chatId: string) {
  const owner =
    chatStore.getState().chats[chatId]?.workspaceId ||
    chatWorkspaceIn(windowPaneStore.getState().panes, chatId)
  if (!owner || !scope.refs.has(owner)) return
  try {
    applyChat(await getChat(owner, chatId))
  } catch (err) {
    if (isLive(endpoint, scope) && isNotFoundError(err)) forget(chatId)
  }
}

async function seed(endpoint: string, scope: Scope) {
  try {
    const chats = await listChats(workspaceOf(scope))
    if (!isLive(endpoint, scope)) return
    const listed = new Set<string>()
    for (const chat of chats) {
      listed.add(chat.id)
      scope.members.add(chat.id)
      applyChat(chat)
    }
    setStatus(scope, 'ready')
    for (const id of Object.keys(chatStore.getState().chats)) {
      if (!listed.has(id)) void forgetIfGone(endpoint, scope, id)
    }
  } catch {
    // Non-fatal: the socket still pushes every change.
    if (isLive(endpoint, scope) && scope.status === 'seeding') setStatus(scope, 'error')
  }
}

// A pane following a runner its chat no longer has lets go of it at once: an
// `exited` may never come.
function releaseRunner(runnerId: string, chatId: string) {
  const { panes, paneActions } = windowPaneStore.getState()
  for (const pane of Object.values(panes)) {
    if (pane.runnerId === runnerId && pane.chatId === chatId) {
      paneActions.setPaneRunner(pane.id, null)
    }
  }
}

function adoptIfViewless(scope: Scope, chatId: string) {
  const { panes, paneActions } = windowPaneStore.getState()
  if (chatPaneIndex(panes).has(chatId)) return
  const wsId = workspaceOf(scope)
  const workspaceId = chatStore.getState().chats[chatId]?.workspaceId || wsId
  const projectId = getWorkspaceScope(wsId)?.projectId || resolveChatProjectId(chatId, workspaceId)
  if (projectId) paneActions.adoptBackgroundChat(chatId, projectId, workspaceId)
}

function applyLifecycle(scope: Scope, ev: LifecycleFrame) {
  const outcome = applyChatFrame({
    chatId: ev.chatId,
    kind: ev.kind,
    runnerId: ev.runnerId,
    version: ev.version,
    chat: ev.chat ? mapChat(ev.chat) : undefined,
  })
  if (outcome.kind === 'deleted') return forget(ev.chatId)
  if (outcome.kind === 'applied') scope.members.add(ev.chatId)
  const left = runnerLeft(outcome)
  if (left) releaseRunner(left, ev.chatId)
  // A first sight through the list is not an edge; only a live one adopts.
  if (startedWorking(outcome) && scope.status === 'ready') adoptIfViewless(scope, ev.chatId)
}

function onFrame(endpoint: string, scope: Scope, data: unknown) {
  if (!isLive(endpoint, scope) || typeof data !== 'object' || data === null) return
  if ('reconnected' in data) {
    void seed(endpoint, scope)
    bumpTree(scope)
    void useAgentProvidersStore.getState().load(workspaceOf(scope))
    return
  }
  const ev = data as LifecycleFrame
  if (ev.folderId) return bumpTree(scope)
  if (!ev.chatId) return
  if (!NON_STRUCTURAL_CHAT_KINDS.has(ev.kind)) bumpTree(scope)
  if (FEED_KINDS.has(ev.kind)) return
  if (ev.kind === 'telemetry') return setChatTelemetry(ev.chatId, ev.telemetry ?? null)
  applyLifecycle(scope, ev)
}

function release(endpoint: string, scope: Scope) {
  scope.unsubscribe()
  scopes.delete(endpoint)
  const held = new Set([...scopes.values()].flatMap((other) => [...other.members]))
  removeChats([...scope.members].filter((id) => !held.has(id)))
}

/** Holds `wsId`'s lifecycle endpoint open: one subscription and one seed per
 *  endpoint however many workspaces share it. */
export function attachScope(wsId: string): () => void {
  const endpoint = `${chatBase(wsId)}/ws`
  let scope = scopes.get(endpoint)
  const fresh = !scope
  if (!scope) {
    const created: Scope = {
      refs: new Map(),
      members: new Set(),
      status: 'seeding',
      unsubscribe: () => {},
    }
    scopes.set(endpoint, created)
    scope = created
  }
  const held = scope
  held.refs.set(wsId, (held.refs.get(wsId) ?? 0) + 1)
  setChatScope(wsId, held.status)
  if (fresh) {
    held.unsubscribe = wsManager.subscribe(endpoint, (data) => onFrame(endpoint, held, data))
    void seed(endpoint, held)
  }
  if (useAgentProvidersStore.getState().status === 'idle') {
    void useAgentProvidersStore.getState().load(wsId)
  }
  let detached = false
  return () => {
    if (detached) return
    detached = true
    const refs = (held.refs.get(wsId) ?? 1) - 1
    if (refs > 0) {
      held.refs.set(wsId, refs)
      return
    }
    held.refs.delete(wsId)
    setChatScope(wsId, null)
    if (held.refs.size === 0 && isLive(endpoint, held)) release(endpoint, held)
  }
}
