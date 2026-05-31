# Markdown-Chat: Shared Document Conversation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the existing bubble-style chat (`components/chat/`) with a shared markdown document where humans and AI agents write to the same canvas. The document is append-only, turn-based, and rendered with Obsidian-style live preview via CodeMirror 6. The existing `crowbarChat` pane content type is preserved — only its rendered component changes.

**Architecture:** Turns are first-class objects in a per-workspace Zustand store (`MarkdownTurn[]`). CodeMirror 6 is a windowed view over that store — it loads the last N turns into a single document and prepends older turns on scroll. CM6's built-in line virtualization handles DOM performance; no custom virtual list is needed. The backend stores widget payloads (Excalidraw scenes, Mermaid source) as opaque JSON blobs — it has no knowledge of widget types. The frontend owns a widget registry that maps type strings to React components rendered as CM6 widget decorations.

**Tech Stack:** `@codemirror/state`, `@codemirror/view`, `@codemirror/lang-markdown`, `@codemirror/commands`, `@uiw/react-codemirror` (CM6 React host); `@excalidraw/excalidraw`; `mermaid`; shadcn/ui Toolbar component; existing Zustand + WS infrastructure.

---

## UX Decisions

| Concern | Decision |
|---|---|
| Architecture | Turn store (source of truth) + windowed CM6 (view) |
| Editor | CodeMirror 6, Obsidian-style live preview — headings/bold/italic render as styled text while typing |
| Turn separation | Subtle background tint per author only — no borders, no horizontal rules |
| Tool calls | Collapsible inline pills inside the agent turn — click to expand args + output |
| Agent todo lists | Live markdown checklist (`- [ ]` / `- [x]`) inline in the turn; sticky (`position: sticky`) to top of scroll container while `turn.streaming === true`; releases to normal flow on completion |
| Streaming appearance | Blinking cursor at end of in-progress text; no other indicator |
| Toolbar | Docked above input area — formatting buttons (Bold, Italic, H1–H3, inline code) + Insert dropdown (Excalidraw, Code block with language picker, Mermaid) + `/` slash command palette |
| Slash commands | `/` in the input triggers a command palette for Claude Code skills and commands (`/tdd`, `/code-review`, `/plan`, etc.) |
| Widget ownership | Frontend registry — backend stores `{ id, type, payload }` as opaque JSON; zero backend changes needed to add new widget types |
| Submission | `Cmd+Enter` submits the user turn; `Enter` inserts a newline |

---

## Data Model

```ts
// features/markdown-chat/types.ts

export type TurnRole = 'user' | 'agent'

export interface WidgetData {
  id: string        // nanoid, referenced in fenced block header
  type: string      // 'excalidraw' | 'mermaid' — frontend-owned registry key
  payload: unknown  // Excalidraw scene JSON or Mermaid source; backend never inspects
}

export interface MarkdownTurn {
  id: string
  role: TurnRole
  content: string        // raw markdown; fenced blocks reference widget IDs
  timestamp: string      // ISO 8601
  authorName: string
  widgets: WidgetData[]  // attached widget payloads for this turn
  streaming?: boolean    // true while agent is actively writing
}

// Widget fenced block syntax in content:
// ```excalidraw widget-id:abc123
// ```
// CM6 widget extension reads `widget-id:` from the info string and
// looks up WidgetData by id in the turn's widgets array.
```

### Conversation Store

```ts
// features/markdown-chat/stores/conversation-store.ts
// Per-workspace Zustand store following existing registry pattern

interface ConversationState {
  turns: MarkdownTurn[]
  appendTurn: (turn: MarkdownTurn) => void
  updateStreamingTurn: (id: string, contentDelta: string) => void
  finalizeStreamingTurn: (id: string) => void
  updateWidgetPayload: (turnId: string, widgetId: string, payload: unknown) => void
}
```

---

## CM6 Architecture

### Document window

The CM6 document is assembled from the last N turns (default: 100) on load. Each turn serializes to its markdown content with a turn-boundary marker comment at the start:

```
<!-- turn:abc123 role:agent -->
The token refresh logic has three problems...

<!-- turn:def456 role:user -->
Good points. What about the Redis layer?
```

Boundary markers are invisible — CM6 line decorations hide them — but used by the turn-boundary extension to track which ranges belong to which turn.

When the user scrolls to the top of the CM6 document, the scroll handler prepends the next batch of older turns via a transaction, updating all range positions accordingly.

### Extension stack

| Extension | Responsibility |
|---|---|
| `lang-markdown` | Syntax tree — required by all other extensions |
| `live-preview-ext` | Hides markdown syntax tokens and renders styled decorations in their place (headings, bold, italic, inline code). Active outside the cursor line, like Obsidian. |
| `turn-boundary-ext` | Parses boundary markers; applies role-based tint (`Decoration.mark` with CSS class); enforces read-only on all turns except the last one via transaction filter |
| `widget-ext` | Detects fenced blocks with `widget-id:` in the info string; replaces with a `WidgetDecoration` mounting the appropriate React component from the widget registry |
| `streaming-ext` | Exposes `appendStreamChunk(view, text)` — dispatches insert transactions at the document end during agent streaming; manages the blinking cursor decoration |
| `todo-sticky-ext` | Detects `- [ ]` / `- [x]` list blocks inside the current streaming turn; applies `sticky-active` CSS class (`position: sticky; top: 8px`) while `streaming === true`; removes on finalization |
| `slash-command-ext` | Intercepts `/` at the start of a line or after a space; opens the command palette overlay |

### Read-only enforcement

The transaction filter in `turn-boundary-ext` rejects any transaction that modifies character ranges belonging to a completed (non-streaming) turn. The user's editable region is always the final turn in the document. Agent streaming appends at the absolute end via `streaming-ext`, which bypasses the filter using a transaction annotation.

---

## Toolbar

The toolbar docks above the CM6 input area and contains two groups:

**Formatting group** (applies markdown syntax around selection or at cursor):
- Bold (`**`), Italic (`*`), Heading 1 (`#`), Heading 2 (`##`), Heading 3 (`###`), Inline code (`` ` ``)

**Insert dropdown** (inserts a block at cursor):
- **Excalidraw** — generates a `nanoid`, creates a blank `WidgetData` entry in the pending turn, inserts the fenced block ` ```excalidraw widget-id:<id> ``` `
- **Code block** → sub-menu with languages: TypeScript, JavaScript, Python, Go, Shell, JSON, Other (plain)
- **Mermaid diagram** — inserts a ` ```mermaid ``` ` block with a starter template

**Slash commands**: typing `/` anywhere in the input triggers a floating command palette listing available Claude Code skills and commands. The palette filters as the user types and dispatches the selected command to the agent on selection.

---

## Agent Streaming

1. Agent chunk arrives via the existing WS channel (`lib/ws/`)
2. `streaming-ext.appendStreamChunk(view, chunk)` dispatches an insert transaction at the document end
3. CM6 re-renders the affected lines; the blinking cursor decoration stays at the end
4. `todo-sticky-ext` watches for checklist syntax in the streaming range — if found, applies `sticky-active`
5. Checklist items tick from `- [ ]` → `- [x]` as the agent streams updated lines
6. On stream complete: `finalizeStreamingTurn(id)` is called on the store; `streaming-ext` removes the cursor decoration; `todo-sticky-ext` removes `sticky-active`; `turn-boundary-ext` marks the turn's range as read-only

Tool call pills are written as HTML comments in the markdown source that `widget-ext` parses and replaces with collapsed pill widget decorations (same extension responsible for Excalidraw/Mermaid):

```
<!-- tool-call:{"name":"read_file","args":{"path":"src/auth/token.ts"},"status":"done","output":"..."} -->
```

---

## Widget Registry

```ts
// features/markdown-chat/extensions/widget-registry.ts

export interface WidgetComponent {
  render: (payload: unknown, onChange: (payload: unknown) => void) => ReactNode
}

export const WIDGET_REGISTRY: Record<string, WidgetComponent> = {
  excalidraw: ExcalidrawWidget,
  mermaid: MermaidWidget,
}
```

`ExcalidrawWidget` renders the `@excalidraw/excalidraw` React component. On every scene change, it calls `onChange` which dispatches `updateWidgetPayload` on the conversation store. The updated payload is synced to the backend via the existing mutation layer.

`MermaidWidget` renders a read-only Mermaid SVG. It is not interactive — the user edits the source in the fenced block; the widget re-renders on content change.

---

## Pane Integration

The existing `crowbarChat` pane content type in `features/panes/types/pane-content.ts` is unchanged. Only `pane-container.tsx` changes — the rendered component switches from `ChatView` to `MarkdownChatView`.

The entire `components/chat/` directory (`ChatView`, `MessageBubble`, `ChatInput`, `ChatEmptyState`, `ToolCallSeparator`) is deleted. `MarkdownContent.tsx` is kept if used outside of chat.

---

## MSW Mock

Following the existing mock pattern (`lib/mock/conversations.ts`), a new MSW handler provides mock `MarkdownTurn[]` arrays keyed by `wsId`. The mock simulates streaming by sending chunks over the existing mock WS channel at 30ms intervals.

---

## File Map

### New (`web/src/`)

| Action | Path | Responsibility |
|---|---|---|
| Create | `features/markdown-chat/types.ts` | `MarkdownTurn`, `WidgetData`, `TurnRole` |
| Create | `features/markdown-chat/stores/conversation-store.ts` | Zustand per-workspace turn store |
| Create | `features/markdown-chat/extensions/live-preview.ts` | CM6 Obsidian-style live preview extension |
| Create | `features/markdown-chat/extensions/turn-boundaries.ts` | Tinting, read-only enforcement, boundary markers |
| Create | `features/markdown-chat/extensions/widget-ext.ts` | Fenced block → React widget decorations |
| Create | `features/markdown-chat/extensions/streaming-ext.ts` | Agent chunk dispatch, blinking cursor |
| Create | `features/markdown-chat/extensions/todo-sticky.ts` | Sticky checklist while streaming |
| Create | `features/markdown-chat/extensions/slash-command-ext.ts` | `/` command palette trigger |
| Create | `features/markdown-chat/extensions/widget-registry.ts` | Type → component registry |
| Create | `features/markdown-chat/components/markdown-chat-view.tsx` | Root component, replaces ChatView in pane |
| Create | `features/markdown-chat/components/markdown-chat-editor.tsx` | CM6 host — wires all extensions |
| Create | `features/markdown-chat/components/markdown-chat-toolbar.tsx` | Docked toolbar with formatting + insert dropdown |
| Create | `features/markdown-chat/components/slash-command-palette.tsx` | Floating command palette overlay |
| Create | `features/markdown-chat/components/excalidraw-widget.tsx` | Excalidraw React widget |
| Create | `features/markdown-chat/components/mermaid-widget.tsx` | Mermaid SVG renderer |
| Create | `features/markdown-chat/components/tool-call-pill.tsx` | Collapsible tool call pill |
| Create | `lib/mock/markdown-chat.ts` | Mock `MarkdownTurn[]` + streaming WS simulation |
| Create | `mocks/handlers/markdown-chat.ts` | MSW handlers for chat REST + WS endpoints |

### Modified (`web/src/`)

| Action | Path | Change |
|---|---|---|
| Modify | `features/panes/components/pane-container.tsx` | Render `MarkdownChatView` for `crowbarChat` content type |
| Delete | `components/chat/chat-view.tsx` | Replaced by `MarkdownChatView` |
| Delete | `components/chat/message-bubble.tsx` | Replaced by CM6 turn rendering |
| Delete | `components/chat/chat-input.tsx` | Replaced by CM6 editor + toolbar |
| Delete | `components/chat/chat-empty-state.tsx` | Replaced by empty-state in `MarkdownChatView` |
| Delete | `components/chat/tool-call-separator.tsx` | Replaced by `ToolCallPill` widget |
| Delete | `lib/mock/conversations.ts` | Fully replaced by `lib/mock/markdown-chat.ts` |

### New dependencies (`web/package.json`)

| Package | Purpose |
|---|---|
| `@codemirror/state` | CM6 state management |
| `@codemirror/view` | CM6 editor view + decorations |
| `@codemirror/lang-markdown` | Markdown language support |
| `@codemirror/commands` | Standard keybindings |
| `@uiw/react-codemirror` | React wrapper for CM6 |
| `@excalidraw/excalidraw` | Embedded drawing canvas |
| `mermaid` | Diagram renderer |
| `nanoid` | Widget ID generation |
