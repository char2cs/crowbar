type AgentChatPaneComponent =
  typeof import('@/features/agent/components/agent-chat-pane-gate').AgentChatPaneGate

let loaded: AgentChatPaneComponent | null = null

export function loadedAgentChatPane(): AgentChatPaneComponent | null {
  return loaded
}

export function preloadAgentChatPane(): Promise<AgentChatPaneComponent> {
  return import('@/features/agent/components/agent-chat-pane-gate').then(
    (m) => (loaded = m.AgentChatPaneGate),
  )
}
