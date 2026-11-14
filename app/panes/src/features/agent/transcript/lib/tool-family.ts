import type { ToolCallBody } from '@/features/agent/api/chat-rows'

export type ToolFamily =
  'read' | 'search' | 'execute' | 'edit' | 'fetch' | 'mcp' | 'subagent' | 'plan' | 'meta' | 'other'

const KINDS = new Set<string>([
  'read',
  'search',
  'execute',
  'edit',
  'fetch',
  'mcp',
  'subagent',
  'plan',
  'meta',
])

/** Crowbar's own tools speak through the chat itself, not as rows. */
const OWN_SERVER = 'crowbar'

/** A tool call's family, from the kind its descriptor mapped; decided once. */
export function toolFamily(
  body: Pick<ToolCallBody, 'kind' | 'name' | 'mcpServer' | 'addressesScope'>,
): ToolFamily {
  const name = body.name ?? ''
  if (body.addressesScope) return 'meta'
  if (body.mcpServer === OWN_SERVER || name.startsWith(`mcp__${OWN_SERVER}__`)) return 'meta'
  if (body.mcpServer || name.startsWith('mcp__')) return 'mcp'
  if (body.kind && KINDS.has(body.kind)) return body.kind as ToolFamily
  return 'other'
}
