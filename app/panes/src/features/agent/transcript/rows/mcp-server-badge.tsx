import { Visible } from '@/features/agent/composer/visible-text'

const HUES = 5

function hueOf(name: string): number {
  let hash = 0
  for (const char of name) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return (Math.abs(hash) % HUES) + 1
}

export function McpServerBadge({ server }: { server: string }) {
  return (
    <span className="mcp-server" data-hue={hueOf(server)} data-testid="agent-mcp-server">
      <Visible text={server} />
    </span>
  )
}
