import { useEffect } from 'react'
import {
  AgentChatPaneBody,
  type AgentChatPaneProps,
} from '@/features/agent/components/agent-chat-pane'
import { AgentChatUnavailable } from '@/features/agent/components/agent-chat-unavailable'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { useResolvedHost } from '@/lib/host-resolve'
import { useHostUnavailable } from '@/lib/host-unavailability'

/**
 * The chat pane once its workspace's host is known. A paired install restores the
 * pane before the project list names that host, and every read the pane makes is
 * addressed through it: the body mounts, with all its hooks and effects, only then,
 * and unmounts if the host stops resolving. Until then the pane says what it waits for.
 */
export function AgentChatPaneGate(props: AgentChatPaneProps) {
  const { paneId, wsId } = props
  const hostId = useResolvedHost({ wsId })
  const unavailable = useHostUnavailable({ wsId })
  // Passive, so it follows the wrapper's layout-effect `begin` even in one commit.
  useEffect(() => pendingChatSurfaces.getState().end(paneId), [paneId])

  if (hostId === null) return <AgentChatUnavailable reason={unavailable} />
  return <AgentChatPaneBody {...props} hostId={hostId} />
}
