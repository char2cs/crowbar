type AgentChatPaneComponent =
  typeof import('@/features/agent/components/agent-chat-pane').AgentChatPane

let loaded: AgentChatPaneComponent | null = null

/** The chat surface once its chunk has loaded, else null. Rendered directly it
 *  mounts in the first commit; a lazy component always suspends on its first
 *  render, and the retry that follows is interruptible, so a burst of store
 *  updates at launch kept restarting it. */
export function loadedAgentChatPane(): AgentChatPaneComponent | null {
  return loaded
}

export function preloadAgentChatPane(): Promise<AgentChatPaneComponent> {
  return import('@/features/agent/components/agent-chat-pane').then(
    (m) => (loaded = m.AgentChatPane),
  )
}
