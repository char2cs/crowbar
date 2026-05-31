export type TurnRole = 'user' | 'agent'

export interface WidgetData {
  id: string
  type: string      // frontend registry key: 'excalidraw' | 'mermaid'
  payload: unknown  // opaque — backend never inspects
}

export interface ToolCallData {
  name: string
  args: Record<string, unknown>
  status: 'pending' | 'done' | 'error'
  output: string
}

export interface MarkdownTurn {
  id: string
  role: TurnRole
  content: string        // raw markdown; fenced blocks reference widget IDs
  timestamp: string      // ISO 8601
  authorName: string
  widgets: WidgetData[]
  streaming?: boolean    // true while agent is actively writing
}

// Turn boundary marker embedded in CM6 document text:
// <!-- turn:ID role:ROLE -->
// These lines are hidden by turn-boundary-ext decorations.
export const TURN_MARKER_RE = /^<!-- turn:([a-zA-Z0-9_-]+) role:(user|agent) -->$/

// Widget fenced block info string format: "excalidraw widget-id:abc123"
export const WIDGET_ID_RE = /widget-id:([a-zA-Z0-9_-]+)/

// Tool call embedded comment format:
// <!-- tool-call:{...JSON...} -->
export const TOOL_CALL_RE = /^<!-- tool-call:(.+) -->$/
