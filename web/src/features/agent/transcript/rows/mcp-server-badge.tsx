/** Chart tokens a server's badge can take; the same name always lands on the same one. */
const HUES = 5

function hueOf(name: string): number {
  let hash = 0
  for (const char of name) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return (Math.abs(hash) % HUES) + 1
}

/** An MCP server as the main noun of its row, coloured by its name. */
export function McpServerBadge({ server }: { server: string }) {
  return (
    <span className="mcp-server" data-hue={hueOf(server)} data-testid="agent-mcp-server">
      {server}
    </span>
  )
}
