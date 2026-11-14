type AgentChatPaneComponent =
  typeof import('@/features/agent/components/agent-chat-pane-gate').AgentChatPaneGate

let loaded: AgentChatPaneComponent | null = null

/** The chat surface once its chunk loaded, else null. Rendered directly it mounts in the first
 *  commit; a lazy one suspends first and its interruptible retry restarts under launch's burst
 *  of store updates. */
export function loadedAgentChatPane(): AgentChatPaneComponent | null {
  return loaded
}

export function preloadAgentChatPane(): Promise<AgentChatPaneComponent> {
  return import('@/features/agent/components/agent-chat-pane-gate').then(
    (m) => (loaded = m.AgentChatPaneGate),
  )
}
