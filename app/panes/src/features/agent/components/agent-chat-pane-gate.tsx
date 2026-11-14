import { useEffect } from 'react'
import {
  AgentChatPaneBody,
  type AgentChatPaneProps,
} from '@/features/agent/components/agent-chat-pane'
import { AgentChatUnavailable } from '@/features/agent/components/agent-chat-unavailable'
import { DraftChatPane } from '@/features/agent/components/draft-chat-pane'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { useResolvedHost } from '@/lib/host-resolve'
import { useHostUnavailable } from '@/lib/host-unavailability'

// Mounts the chat body only once its workspace's host resolves (a paired install restores panes
// before the project list names hosts) since every read is addressed through it; unmounts if the
// host stops resolving, and meanwhile says what it waits for.
export function AgentChatPaneGate(props: AgentChatPaneProps) {
  const { paneId, wsId } = props
  const hostId = useResolvedHost({ wsId })
  const unavailable = useHostUnavailable({ wsId })
  // Passive, so it follows the wrapper's layout-effect `begin` even in one commit.
  useEffect(() => pendingChatSurfaces.getState().end(paneId), [paneId])

  if (hostId === null) return <AgentChatUnavailable reason={unavailable} />
  if (isDraftChatId(props.chatId)) {
    return (
      <DraftChatPane
        paneId={paneId}
        wsId={wsId}
        hostId={hostId}
        belowOverlayHeader={props.belowOverlayHeader ?? false}
      />
    )
  }
  return <AgentChatPaneBody {...props} hostId={hostId} />
}
