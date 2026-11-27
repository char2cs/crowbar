import { useEffect } from 'react'
import {
  AgentChatPaneBody,
  type AgentChatPaneProps,
} from '@/features/agent/components/agent-chat-pane'
import { AgentChatUnavailable } from '@/features/agent/components/agent-chat-unavailable'
import { DraftChatPane } from '@/features/agent/components/draft-chat-pane'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { useChatPaneHost } from '@/features/hosts/lib/host-resolve-cached'
import { useHostUnavailable } from '@/lib/host-unavailability'

export function AgentChatPaneGate(props: AgentChatPaneProps) {
  const { paneId, wsId } = props
  const hostId = useChatPaneHost(wsId, props.chatId)
  const unavailable = useHostUnavailable({ wsId })
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
