import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

export function effortLevelsFor(provider: AgentProvider | undefined, model: string): string[] {
  if (!provider?.effortSelect) return []
  return provider.efforts?.[model] ?? []
}

export function resolveEffort(
  provider: AgentProvider,
  model: string,
  currentEffort: string,
): string {
  const levels = effortLevelsFor(provider, model)
  if (currentEffort && levels.includes(currentEffort)) return currentEffort
  return levels[0] ?? ''
}
