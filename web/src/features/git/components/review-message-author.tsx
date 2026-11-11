import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { useChatListReady } from '@/features/agent/hooks/use-chat-providers'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import type { IdentityDTO } from '@/features/git/api/identity-api'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import type { ReviewMessage } from '@/features/workspace/stores/slices/branch-review-slice'

interface AuthorDisplay {
  name: string
  login: string | null
  avatarUrl: string | null
  isAgent: boolean
  /** Raw SVG for the agent provider that wrote this, when it is one we know. */
  providerIcon: string | null
}

/** Every field is '' when the message carries no attribution, names an unknown
 *  provider, or names a deleted chat: all three mean "we know nothing". */
interface AgentAttribution {
  providerName: string
  providerIcon: string
  /** '' when there is no chat id at all, or the id resolves to no chat. */
  chatTitle: string
  /** The message names a chat that no longer exists — a link would open nothing. */
  chatDeleted: boolean
}

const NO_ATTRIBUTION: AgentAttribution = {
  providerName: '',
  providerIcon: '',
  chatTitle: '',
  chatDeleted: false,
}

// An agent is named by its provider, resolved by id and never by branching on
// the id; a message written before attribution existed falls through to "Agent".
export function resolveAuthorDisplay(
  message: ReviewMessage,
  currentIdentity: IdentityDTO | null | undefined,
  agent: AgentAttribution,
): AuthorDisplay {
  if (message.isAgent) {
    return {
      name: agent.providerName || 'Agent',
      login: null,
      avatarUrl: null,
      isAgent: true,
      providerIcon: agent.providerIcon || null,
    }
  }
  const login = message.author ?? null
  if (login && currentIdentity && currentIdentity.login === login) {
    return {
      name: currentIdentity.displayName || login,
      login,
      avatarUrl: currentIdentity.avatarUrl || `https://github.com/${login}.png?size=48`,
      isAgent: false,
      providerIcon: null,
    }
  }
  return {
    name: login ?? 'Unknown',
    login,
    avatarUrl: login ? `https://github.com/${login}.png?size=48` : null,
    isAgent: false,
    providerIcon: null,
  }
}

/** Narrow selectors, one per field: an object would re-render the thread on
 *  every unrelated chat write. */
export function useAgentAttribution(message: ReviewMessage): AgentAttribution {
  const providerId = message.providerId ?? ''
  const chatId = message.chatId ?? ''

  const providerName = useAgentProvidersStore((s) =>
    providerId ? (s.providers.find((p) => p.id === providerId)?.displayName ?? '') : '',
  )
  const providerIcon = useAgentProvidersStore((s) =>
    providerId ? (s.providers.find((p) => p.id === providerId)?.icon ?? '') : '',
  )
  const chatTitle = useChatField(chatId, (chat) => (chat ? chat.title || UNTITLED_CHAT_LABEL : ''))
  // "Deleted" is a claim only a landed list can back.
  const listReady = useChatListReady()

  if (!message.isAgent) return NO_ATTRIBUTION
  return {
    providerName,
    providerIcon,
    chatTitle,
    chatDeleted: chatId !== '' && chatTitle === '' && listReady,
  }
}

export function MessageAvatar({ display }: { display: AuthorDisplay }) {
  // The same glyph the chat's tab and sidebar row carry.
  if (display.providerIcon) {
    return (
      <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <ProviderIcon svg={display.providerIcon} className="size-3.5" />
      </span>
    )
  }
  const initials = (display.name || 'U').slice(0, 2).toUpperCase()
  return (
    <Avatar className="mt-0.5 size-6 shrink-0 text-xs font-semibold">
      {display.avatarUrl && <AvatarImage src={display.avatarUrl} alt={display.name} />}
      <AvatarFallback className="text-xs font-semibold text-muted-foreground">
        {initials}
      </AvatarFallback>
    </Avatar>
  )
}

/** The chat an agent message came out of: a link while it exists, inert text
 *  once deleted, since the id outlives the chat. */
export function ChatOrigin({
  wsId,
  chatId,
  title,
  deleted,
}: {
  wsId: string
  chatId: string
  title: string
  deleted: boolean
}) {
  if (deleted) {
    return (
      <span
        data-testid="review-message-chat-deleted"
        title="The chat this came from has been deleted"
        className="min-w-0 truncate text-xs text-muted-foreground/60 italic"
      >
        Deleted chat
      </span>
    )
  }
  return (
    <button
      type="button"
      data-testid="review-message-chat-link"
      title={`Open ${title}`}
      onClick={() =>
        windowPaneStore.getState().paneActions.openChat(chatId, {
          projectId: resolveChatProjectId(chatId, wsId) ?? undefined,
          workspaceId: wsId,
        })
      }
      className="min-w-0 max-w-40 truncate text-xs text-muted-foreground underline-offset-2 transition-colors hover:text-foreground hover:underline focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {title}
    </button>
  )
}
