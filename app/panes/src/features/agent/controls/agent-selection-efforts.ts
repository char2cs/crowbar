import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

/** Effort levels are a property of the model, never the provider alone. */
export function effortLevelsFor(provider: AgentProvider | undefined, model: string): string[] {
  if (!provider?.effortSelect) return []
  return provider.efforts?.[model] ?? []
}

/** Keep the current effort if the new model declares it, else its first level,
 *  so every pick lands on a concrete, visible value. */
export function resolveEffort(
  provider: AgentProvider,
  model: string,
  currentEffort: string,
): string {
  const levels = effortLevelsFor(provider, model)
  if (currentEffort && levels.includes(currentEffort)) return currentEffort
  return levels[0] ?? ''
}
