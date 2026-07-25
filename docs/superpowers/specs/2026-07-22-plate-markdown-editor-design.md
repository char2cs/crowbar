# Plate Markdown Editor — Design

**Date:** 2026-07-22
**Status:** Approved (design); implementation plan to follow
**Area:** `web/src/features/editor`, `web/src/features/panes`

## Summary

Replace Crowbar's weak markdown experience — a raw Monaco code editor plus a
separate read-only rendered-preview tab — with a **Notion-style rich editor**
built on **Plate** as the *default* surface for `.md` / `.markdown` files. Monaco
is retained as a **raw-source escape hatch** reachable via a per-file toggle. The
markdown *text on disk* remains the single source of truth, so the file the user
edits is always plain markdown, and nothing about how files open, save, or
persist changes.

The whole separate-preview apparatus (`markdownPreview` buffer type,
`markdown-preview.tsx`, the custom `parser.ts`, and the breadcrumb "eye" preview
action for markdown) **retires**, because the rich editor *is* the live preview.

## Goals

- Opening a `.md`/`.markdown` file shows a Notion-style rich editor by default.
- A per-file **Rich ⇄ Source** toggle flips the same file between Plate and Monaco.
- The file on disk stays plain, valid markdown. Save writes markdown text through
  the *existing* save pipeline — Plate never touches the filesystem directly.
- Full-featured authoring (see Feature Scope): GFM plus math, Mermaid, callouts,
  and footnotes.
- Frontmatter is preserved byte-exact.
- Plate/Slate and the heavy plugins (Mermaid, KaTeX) are lazy-loaded, out of the
  entry chunk, exactly like Monaco is today.

## Non-goals (v1)

- **Image upload** (drag/paste a new image to disk). v1 *displays* images that
  already exist on disk; inserting new image files is a fast-follow.
- **Frontmatter properties panel** (editable Notion-style fields). v1 keeps
  frontmatter as a verbatim block; a properties UI is a fast-follow.
- **Removing CodeMirror.** CodeMirror stays exactly where it is (the git-review
  comment composer, `comment-composer.tsx`). Untouched. No migration.
- **Changing MDX handling.** `.mdx` currently maps to the `markdown` language;
  MDX-specific semantics are out of scope. (See Risks.)

## Accepted tradeoff

A rich block editor holds a document *tree*, not text, so **saving re-serializes
the whole file** into the serializer's canonical markdown dialect. Untouched
lines can come back reformatted (list markers, emphasis style, table padding,
wrapping). The user has explicitly accepted this "format on save" behavior. The
Source (Monaco) escape hatch and byte-exact frontmatter handling are the
mitigations that make it comfortable in a git-backed repo.

## Current state (what exists today)

- A `.md` file opens as a normal **`editor` buffer** (`EditorContent` in
  `web/src/features/panes/types/pane-content.ts`), carrying
  `content` / `savedContent` / `isDirty`, external-change detection, session
  persistence, and file-missing handling. **Monaco is only the render surface.**
- `web/src/features/panes/components/pane-container.tsx` → `renderActiveBuffer`
  is the routing table. The default (`editor`) case renders `EditorPane`
  (`editor-pane.tsx` → `EditorSurface` → Monaco).
- Monaco edits flow back to `buffer.content` through a throttled `ContentSink`
  (150 ms) in `web/src/features/editor/hooks/use-pane-editor-controller.ts`,
  which marks the buffer dirty. Save writes `content` to disk and sets
  `savedContent`.
- Rendered preview is a **separate `markdownPreview` buffer** opened by the
  breadcrumb eye icon (`toolbar/breadcrumb.tsx` `handlePreviewClick`), rendered by
  `web/src/features/editor/markdown/markdown-preview.tsx` via a custom
  `parseMarkdown()` (`parser.ts`) that emits DOMPurify-sanitized HTML.
- `react-markdown` (`features/panes/lib/markdown.tsx`) renders **agent-chat /
  review-comment** text — unrelated to the file editor, untouched.
- Extension → language mapping lives in
  `web/src/features/editor/utils/language-id.ts` (`md`, `mdx`, `markdown` →
  `'markdown'`).

## Decisions

1. **Library: Plate** (`platejs` / `@platejs/*`, MIT). Chosen for: fully MIT (no
   commercial-gated tier), shadcn-native components that match Crowbar's
   `@/components/ui/*` house style, and a **remark-based** markdown plugin that
   round-trips closer to the source than a pure block model.
2. **Markdown files stay `editor` buffers.** No new buffer type for the editor.
   All open/save/dirty/persist machinery is reused as-is.
3. **Surface selector, not new plumbing.** Routing gains one branch; the file's
   markdown text is the shared contract between Plate and Monaco.
4. **Rich is the default** for `.md`/`.markdown`; Source (Monaco) is the toggle.
5. **Retire the markdown preview path** (`markdownPreview` buffer, its renderer,
   `parser.ts`, and the markdown branch of the breadcrumb preview action). Keep
   `htmlPreview` and `csvPreview` — unrelated.

## Architecture

### The surface selector

In `pane-container.tsx` `renderActiveBuffer`, the default `editor` case gains one
branch:

```
if (buffer is markdown language) and (view mode for this buffer === 'rich'):
    render <MarkdownEditorPane bufferId=… />     // new, lazy, Plate
else:
    render <EditorPane … />                       // existing, Monaco
```

"is markdown" is decided by the buffer's language (`language-id.ts`,
`md`/`mdx`/`markdown` → `markdown`). `MarkdownEditorPane` is `React.lazy`, so
Plate/Slate never enter the entry chunk — same pattern as `EditorPane`,
`DiffPane`, `MarkdownPreview` today.

### The markdown text is the single source of truth

`EditorContent.content` (a markdown string) is the contract both surfaces share:

- **Plate mount:** parse `content` (markdown) → Slate value once. Re-parse on an
  external reload (`hasExternalChange` / file-watch refresh) or when toggling
  from Source back to Rich.
- **Plate edit:** on change, **debounced**, serialize Slate → markdown and write
  it back to `buffer.content` through the *same* buffer content-sync action
  Monaco's `ContentSink` uses (marking the buffer dirty). Plate reuses the seam;
  it does not invent a second dirty/save mechanism.
- **Monaco (Source view):** reads/writes the same `content` as plain text, exactly
  as today.
- **Save (Cmd+S):** unchanged. The existing pipeline writes `content` to disk and
  sets `savedContent`. Plate never writes files.

Because both surfaces read and write the one `content` string, the toggle is
essentially free: it swaps which component renders the buffer.

### Per-buffer view mode

One genuinely new piece of state: a per-buffer **view mode** `'rich' | 'source'`,
default `'rich'` for markdown files. It lives in editor/pane UI state keyed by
buffer id (not persisted to disk content). The toggle button flips it.

### Toggle UX

A **Rich ⇄ Source** control in the editor toolbar (near the breadcrumb), shown
only when the active buffer is a markdown file. Flipping to Source mounts Monaco
on the current `content`; flipping back to Rich re-parses `content` into Slate.
Before a Rich→Source flip, Plate flushes any pending debounced serialize so
Monaco opens on the latest text (mirrors Monaco's existing flush-on-switch-away).

## Feature scope (v1 = full-featured, "Option C")

**Core (GFM):** headings; bold / italic / strikethrough; ordered, unordered, and
**task-list** (checkbox) lists; links (internal → open in editor, external → open
in browser, reusing the path-resolution logic from the retired
`markdown-preview.tsx`); blockquotes; **GFM tables**; **fenced code blocks with
syntax highlighting**; horizontal rules; inline code.

**Extended (each its own Plate plugin + markdown serialization slice):**

- **Math** — inline `$…$` and block `$$…$$`, rendered with **KaTeX** (lazy).
- **Mermaid** — fenced ```` ```mermaid ```` blocks rendered as diagrams via the
  **Mermaid** lib (lazy; the heaviest addition; themed to the app).
- **Callouts / admonitions** — a defined syntax (e.g. GitHub `> [!NOTE]`
  blockquote-based) round-tripped losslessly.
- **Footnotes** — `[^1]` references and definitions.

Each extended feature is an independently verifiable slice with round-trip
fixtures; if any one proves disproportionately costly it can ship a beat later
without blocking the GFM core.

### Code blocks

Fenced code uses Plate's code-block plugin with syntax highlighting. Highlighting
library choice (lowlight/Shiki/reuse of the app's existing highlighter) is a
plan-level decision; requirement: highlight common languages and theme to the
app's tokens.

### Local images

Display-only in v1. Relative paths (`./img.png`, `/abs/in/root.png`) resolve
against the file's directory / workspace root (reuse `resolvePath` from the
retired preview) and load via Tauri's asset protocol. Drag/paste-to-insert is a
non-goal for v1 (fast-follow).

### YAML frontmatter

The leading `--- … ---` block is preserved **byte-exact**. Plate treats it as an
opaque node holding the raw text; the serializer re-emits those exact bytes. It is
never parsed into structured fields or reformatted in v1. (remark-frontmatter to
detect it; a dedicated verbatim Plate node to carry it.) This is the top defense
against silently corrupting Astro/Hugo/Jekyll/spec metadata.

### Notion affordances

On by default: slash (`/`) command menu, drag-handle block reordering, block
selection, and the floating/formatting toolbar — Plate's standard set, styled to
the app.

## Retirement & cleanup

- Remove `markdownPreview` from `PaneContentType`, `MarkdownPreviewContent`, its
  `OpenContentSpec` variant, and the `pane-container.tsx` case.
- Delete `web/src/features/editor/markdown/markdown-preview.tsx`, `parser.ts`,
  and their now-unused styles/tests.
- Remove the **markdown branch** of `breadcrumb.tsx` `handlePreviewClick` (keep
  the html/csv branches and their preview buffers).
- Sweep `hasTextContent` and related type guards that name `markdownPreview`.

Do this as its own slice, after the Plate editor is proven, so the fallback path
isn't removed before its replacement works.

## Save / dirty semantics

- Serialize Slate → markdown on a **debounce** (align with Monaco's 150 ms sink
  cadence), writing to `buffer.content` and marking dirty via the existing action.
- Autosave vs. manual save follows whatever the app does today for `editor`
  buffers — no new save policy.
- Flush pending serialization on: blur, tab switch-away, the existing
  `flush-editor-content` window event, and Rich→Source toggle — so a save or a
  view switch never races a stale in-memory value.

## Error handling & fallback

- The Plate surface is wrapped in an **ErrorBoundary**. If parsing or rendering a
  document throws (malformed input, an unmodeled construct), it falls back to the
  **Source (Monaco)** view for that buffer and surfaces a toast — the user always
  reaches raw text and can never be stranded on a blank/broken editor.
- Serialization is defensive: an unknown/failed node preserves its original source
  text rather than dropping content.

## Non-functional

- **Bundle:** `MarkdownEditorPane` and the Mermaid/KaTeX renderers are lazy and
  code-split; verify with a build that the entry chunk gains **zero** Plate/Slate
  bytes (same check used for CodeMirror in the comment-composer chunk).
- **Theming:** Plate's shadcn components bind to the app's CSS token system
  (light/dark, UI font). Diagrams/math themed to match.
- **Perf (WKWebView):** validate typing latency and scroll on a large document
  (e.g. a long README / spec) in the real Tauri app; Slate re-render cost on big
  docs is the risk to watch.

## Testing

Follow the repo mirror convention (`web/src/__tests__/…`, `@/` imports):

- **Round-trip fixtures** (the core safety net): markdown → Slate → markdown for
  frontmatter (byte-exact), GFM tables, task lists, fenced code, math, Mermaid,
  callouts, footnotes, nested lists, links. Assert no drift beyond the accepted
  canonicalization, and **zero** change for frontmatter.
- **Surface-selector routing:** a markdown buffer in `rich` renders Plate; in
  `source` renders Monaco; a non-markdown buffer always renders Monaco.
- **Toggle:** flipping preserves `content` and dirty state; a pending edit flushes
  before Source opens.
- **Bridge/save:** a Plate edit marks the buffer dirty and writes serialized
  markdown to `content`; save persists it through the existing pipeline.
- **Fallback:** a throwing document falls back to Source + toast.
- **Live Tauri verification** (required, serialized on the single running dev
  instance — never spawn a second): open a real repo `.md`, confirm rich editing,
  each extended feature, the toggle, save produces sane git diff, frontmatter
  untouched, Mermaid/math render and theme correctly.

## Risks

- **`.mdx` files** map to `markdown` today; MDX (JSX in markdown) is not something
  a markdown serializer round-trips safely. Plan decision: either exclude `.mdx`
  from the Plate default (keep it on Monaco) or treat unknown JSX as verbatim.
- **Serializer drift on first save** of existing files (the accepted tradeoff);
  the Source toggle is the release valve.
- **Mermaid bundle/perf** is the heaviest single addition; keep it strictly lazy.
- **Large-document Slate perf** in WKWebView — measure before shipping.

## Rollout / slicing (feeds the implementation plan)

1. Serialization bridge + surface selector + per-buffer view mode + toggle, with
   the **GFM core** only. Prove open/edit/save/toggle end-to-end.
2. Frontmatter verbatim node.
3. Extended plugins, each its own slice: math, footnotes, callouts, Mermaid.
4. Local image display.
5. Retire the old preview path (last, once Plate is proven).

Subagents implement slices in parallel with headless unit tests; live Tauri
verification is serialized and run against the one existing dev instance. No PR /
no push unless explicitly requested.
