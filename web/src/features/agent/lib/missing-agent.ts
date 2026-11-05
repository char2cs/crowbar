import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

/** Why a chat that records no agent cannot start yet, or undefined when it names one.
 *  A send or Start session there has nothing to launch, so the pane says so up front. */
export function missingAgentNote(
  providerId: string,
  providers: AgentProvider[],
): string | undefined {
  if (providerId) return undefined
  return providers.some((p) => p.connected && p.enabled)
    ? 'This chat has no agent yet. Pick one, then send a message to start it.'
    : 'No agent is installed. Add one in Settings → Agents to start this chat.'
}
