# Markdown Chat Layout Redesign

**Date:** 2026-05-31  
**Status:** Approved

## Problem

The current markdown-chat view looked like a chat app (bubbles, tinted panels, separate input box). The correct mental model is a **shared markdown document** — like an `.md` file open in Obsidian — where the agent and user both append to the same canvas. The only chat-like affordance is a pinned input zone at the bottom.

## Approved Design

### Document Area

- The full viewport is a scrollable markdown document.
- **Agent turns have no background tint** — they read as plain document text on the dark background.
- **User turns get a full-width warm tint** (`rgba(255,215,80,~5%)`) that bleeds edge-to-edge across the entire viewport width, not clipped to the text column. A hairline top and bottom border (`rgba(255,215,80,~7%)`) delineates each user band.
- Text content is constrained to a centered column (`max-width: 680px`, `padding: 0 48px`) — Google-doc style margins. The tint band ignores this constraint and spans 100% width.
- Custom scrollbar: thin (6px), rounded thumb, matches dark theme (`rgba(255,255,255,0.11)`), no track background.

### Input Zone (pinned bottom)

- The input zone is **the same warm tint** as user turns — it visually reads as the next user message being composed.
- A top border (`rgba(255,215,80,0.09)`) separates it from the document.
- Contains a **second CM6 editor instance** (not a `<textarea>`) — this is required so users can insert and preview rich content (Excalidraw drawings, Mermaid diagrams, images, fenced code blocks) inline while composing. The input CM6 shares the same widget registry, live-preview extension, and slash-command palette as the history viewer.
- The input CM6 is fully editable, auto-expands (min 2 rows, max ~240px), uses the same serif font/theme as the document area.
- The **CrossUI Toolbar** (`Toolbar`, `ToolbarButton`, `ToolbarGroup`, `ToolbarSeparator` from `@/components/ui/toolbar`) lives **inside** the tinted zone, below the input CM6. Toolbar buttons use a muted amber tint so they feel like part of the user space. Toolbar `onInsertWidget` targets the **input CM6**, not the history viewer.
- A **send/pause button** (filled blue, 28×28px, rounded) sits at the far right of the toolbar row. `⌘↵` hint to its left.
- Submission: `⌘↵` sends the full content of the input CM6 (including any embedded widget markers); plain `Enter` inserts a newline. On submit, the input CM6 is cleared.

**Rich content in the input:** When a user inserts an Excalidraw drawing or Mermaid block via the Insert dropdown, it renders as a live widget inside the input CM6 immediately. On submit, the fenced block markers (`\`\`\`excalidraw widget-id:…\`\`\``) travel with the turn content into the history viewer, where the same widget registry re-renders them read-only.

### Typography

- Document uses a serif body font (CalSansUI / system-ui fallback at 16px, 1.8 line-height).
- Headings (`##`, `###`) hide their `##` marker on non-cursor lines (live Obsidian-style preview).
- Bold/italic markers (`**`, `*`) similarly hidden on non-cursor lines.
- Inline code gets a faint background chip.
- Tool call lines render as collapsible pills (existing behavior, unchanged).

### Scrollbar

Replace the default browser scrollbar with a custom thin scrollbar using `scrollbar-width: thin` + `-webkit-scrollbar` CSS. Thumb: `rgba(255,255,255,0.11)`, rounded, no visible track.

## What Changes in Code

| Area | Change |
|------|--------|
| `turn-boundaries.ts` | Agent tint removed entirely. User tint becomes full-width via a block-level element, not a CM6 `Decoration.mark`. |
| `markdown-chat-view.tsx` | Remove separate toolbar component from below the editor. Layout becomes: `[scroll-doc] [input-zone]`. |
| `markdown-chat-editor.tsx` | Renamed to `markdown-chat-history.tsx`. CM6 editor covers **only the read-only conversation history** (agent + past user turns). No editable region, no transaction filter hacks. |
| `markdown-chat-input.tsx` | New component. A second CM6 instance: fully editable, full live-preview + widget extensions, auto-expands. Clears on submit. Shares the same widget registry as the history viewer. |
| `markdown-chat-toolbar.tsx` | Moved inside the input zone, below the textarea. Toolbar buttons get amber tint to match user zone. |
| CSS | Add custom scrollbar styles. Input zone background matches user turn tint. |

## Architecture Decision: Two CM6 Instances

The current design uses a single CM6 instance with a read-only filter and an `<!-- input -->` marker hack. The new design splits history and input into two separate CM6 instances.

**History viewer (`markdown-chat-history.tsx`)**
- Read-only CM6. No transaction filter needed — the entire document is immutable.
- Renders all turns from the `ConversationStore` via `turnsToDocument(turns)`.
- Supports: live-preview decorations, widget extensions (Excalidraw, Mermaid), turn tinting via React wrappers (not CM6 `Decoration.mark`), streaming via `appendStreamChunk`.
- Rebuilds when `turns` array changes length (new turn appended) or a turn's content changes (streaming).

**Input editor (`markdown-chat-input.tsx`)**
- Fully editable CM6. Same extensions as history: markdown, live-preview, widget registry, slash commands.
- Starts empty. User types, inserts widgets, etc.
- On `⌘↵`: extracts full doc content, calls `onSubmit(content)`, then clears by replacing doc with empty string.
- The toolbar's `onInsertWidget` targets this instance.

**Submit flow:**
1. Input CM6 content (plain markdown + any widget markers) → `onSubmit(content)`
2. `handleSubmit` in the view: appends user turn to store, appends empty streaming agent turn, calls `appendStreamChunk` into the history CM6 as chunks arrive, calls `resetInputMarker` (not needed anymore — input CM6 just clears itself).
3. History CM6 re-renders the new user turn as a warm-tinted band; agent turn streams in below.

**Why two instances is correct here:**
- Rich content (Excalidraw, Mermaid) requires CM6's widget system in the input — a `<textarea>` can't render inline widgets.
- Keeping history read-only is trivially enforced by never dispatching user edits to it.
- No `<!-- input -->` markers, no `Prec.highest` workarounds, no transaction filters.

## Non-Goals

- Light mode layout (dark-first for now).
- Author name labels in the gutter (removed for document simplicity).
- Timestamps per turn.
