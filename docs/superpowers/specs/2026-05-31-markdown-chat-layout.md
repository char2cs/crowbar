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
- Contains a `textarea` (auto-expanding, min 2 rows, max ~240px) in the same serif font as the document. Placeholder text is amber-tinted and italic.
- The **CrossUI Toolbar** (`Toolbar`, `ToolbarButton`, `ToolbarGroup`, `ToolbarSeparator` from `@/components/ui/toolbar`) lives **inside** the tinted zone, below the textarea. Toolbar buttons use a muted amber tint so they feel like part of the user space.
- A **send/pause button** (filled blue, 28×28px, rounded) sits at the far right of the toolbar row. `⌘↵` hint to its left.
- Submission: `⌘↵` sends; plain `Enter` inserts a newline. No other submission affordance needed.

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
| `markdown-chat-editor.tsx` | CM6 editor covers **only the read-only conversation history** (agent + past user turns). Input zone is a separate React `<textarea>` — not CM6. |
| `markdown-chat-toolbar.tsx` | Moved inside the input zone, below the textarea. Toolbar buttons get amber tint to match user zone. |
| CSS | Add custom scrollbar styles. Input zone background matches user turn tint. |

## Architecture Decision: Split Editor / Input

The current design uses a single CM6 instance for both history and input. The new design splits them:

- **CM6 editor** = read-only conversation history display (agent + committed user turns). No editable region.
- **React `<textarea>`** = the user's active input. Auto-expands. Submits via `⌘↵`.

This eliminates the `<!-- input -->` marker hack, the `Prec.highest` keymap workaround, and the complexity of keeping the input area editable within a read-only CM6 doc. The history is purely display; the input is purely input.

On submit: content from the textarea becomes a new user turn appended to the CM6 doc (as a read-only tinted band), and an agent turn starts streaming in below it. The textarea clears.

## Non-Goals

- Light mode layout (dark-first for now).
- Author name labels in the gutter (removed for document simplicity).
- Timestamps per turn.
