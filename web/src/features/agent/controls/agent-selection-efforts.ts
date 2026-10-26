import type { AgentProvider } from '@/features/agent/api/agent-api'

/** Which effort levels are valid for `model` on `provider` — a property of
 *  the MODEL, never the provider alone (AgentModelPicker's own rule). */
export function effortLevelsFor(provider: AgentProvider | undefined, model: string): string[] {
  if (!provider?.effortSelect) return []
  return provider.efforts?.[model] ?? []
}

/** The effort to land on after picking `model`: keep the current one if the
 *  new model still declares it, otherwise that model's own first level. There
 *  is no "provider default" row in this picker (unlike the two single-axis
 *  dropdowns it replaces) — every row lands on a concrete, visible value. */
export function resolveEffort(
  provider: AgentProvider,
  model: string,
  currentEffort: string,
): string {
  const levels = effortLevelsFor(provider, model)
  if (currentEffort && levels.includes(currentEffort)) return currentEffort
  return levels[0] ?? ''
}
