# Chat Attachments — Design

## Motivation

The native chat surface (Plate-based composer + transcript) has rich Markdown but no attachment support. Users can't paste large blocks of text without it flooding the input as plain prose, can't attach images or files, and can't hand agents an Excalidraw sketch. This design adds four attachment kinds — pasted text blocks, images, files (including CSV-as-table), and Excalidraw diagrams — as **positional, movable, block-level** elements in the composer, insertable via a plus button, drag-and-drop, or paste.

## Non-goals

- No backend wire-format change. The chat message stays a flat markdown string end to end (`{text, clientRequestId}` in, `LedgerTurn.Text string` stored) — attachments are encoded entirely within that string.
- No provider-specific handling. Attachment delivery to the agent is a plain absolute file path in markdown text; how a given provider CLI's own permission mode handles reading it is out of scope here (see "Agent delivery").
- No re-implementation of Plate's block drag/reorder mechanism — attachments reuse whatever the editor already has for block movement.
- No editing of Excalidraw scenes rendered in a *read-only* transcript message (only in the composer, before send) — out of scope for v1.

## Attachment kinds

Every kind is a block-level void Plate node with a **resolver**: kind-specific logic for (a) how it's encoded in the message's markdown text and (b) what, if anything, needs to exist as a file for the agent to read.

| Kind | Trigger | Stored as | Markdown encoding | Agent sees |
|---|---|---|---|---|
| Text block | Paste over threshold (Cmd/Ctrl+V only; not inside an existing code block) | Inline in message text (or file if oversized — see Thresholds) | ` ```text-attachment\n<raw text>\n``` ` fenced block | The raw pasted text, verbatim |
| Image | Paste / drop / "Attach File" picker | File in `chats/{chatId}/attachments/` | `![alt](path)` | Reads the image file |
| CSV (small) | Drop / picker, under a size cutoff | Inline in message text | Standard markdown table | Reads the table as text — no file |
| File (PDF, oversized CSV, other) | Drop / picker, or CSV over the cutoff | File in `chats/{chatId}/attachments/` | `[filename](path)` | Reads the file directly |
| Excalidraw | "Attach File" dropdown → embedded editor | Inline JSON (or file if oversized); PNG persisted as a normal image file once, when the drawing is finalized | ` ```excalidraw\n{json}\n``` ` fenced block, plus `![diagram](path)` pointing at the persisted PNG | Reads the JSON scene as data, and a rendered PNG for a visual read |

Two kinds (text, Excalidraw) never require a stored file under normal size; two (image, generic file) always do. CSV straddles both, resolved by size.

**Why fenced code blocks with special language tags, for text and Excalidraw:** the composer already has a working `CodeBlockPlugin`. Reusing it for `text-attachment` and `excalidraw` tags means no new parser — just a language-tag → custom-renderer mapping (pill+modal for text, diagram-preview for Excalidraw), the same mechanism already needed for Excalidraw generalized to a second case.

**Why plain `![]()` / `[]()` for images and files:** `MarkdownImageKit` already renders `![alt](src)` today (it just has no UI path to create one yet). Reusing it means the agent — and anyone reading a raw transcript — sees ordinary markdown, not a custom container syntax.

## Editor UX

- A plus button sits to the right of the send button, styled neutrally (no primary/green treatment — it's not "the" action).
- Clicking it opens a dropdown with two entries: **Excalidraw** (opens an embedded Excalidraw editor; on save, produces the JSON scene) and **Attach File** (opens a picker that supports both drag-and-drop and click-to-browse; a single entry point regardless of whether the picked file is an image, CSV, PDF, or other — kind is inferred from the file itself, not from which menu entry was clicked).
- Drag-and-drop onto the composer works the same way regardless of entry point. This repo already has drag-and-drop plumbing wired into composer-adjacent components (`pane-container.tsx`, `terminal.tsx`), routed through `extractDroppedFilePaths()` — but that function is currently a dead stub (`file-system-dropped-paths.ts` returns `[]` unconditionally, left out of scope by a prior session). Reviving/repurposing this path is in scope here, not new plumbing from zero.
- Every attachment inserts as a block-level void Plate node at the current cursor position (or drop position, when available).
- "Movable without worries" = a block drag-handle affordance, same interaction shape as `BlockMenuKit` in the full markdown file editor — but that plugin is explicitly excluded from the chat composer today (`chat-composer-plugins.ts` calls it "page-editor furniture"), so this is **new work for the composer**, not a reuse. Scope it as either adopting a composer-appropriate subset of `BlockMenuKit`, or a lighter drag-handle built just for attachment blocks (not full block-menu chrome like `/`-insert or block-type conversion, which chat deliberately doesn't have).
- Paste interception: a paste handler checks (1) is the caret inside an existing code-block node — if so, skip entirely, let it paste as normal code; (2) does the pasted content exceed the text-attachment threshold — if so, wrap it as a `text-attachment` block instead of inserting plain text. Image data in the clipboard always becomes an image attachment (subject to Shift bypass, see below).
- Shift+Cmd/Ctrl+V always bypasses all of the above and inserts plain text, per the original request.

## Storage & agent delivery

- **Location**: `{workspaceRoot}/chats/{chatId}/attachments/` — a new leaf under the existing `chats/` sibling directory (`RunnerDir` already nests state under `chats/` today per `worktreepath.go`), not a new top-level pattern. This keeps attachments off `git status` the same way all other chat/agent state already is, and ties their lifecycle to the chat's own lifecycle (deleting a chat already removes `chats/{chatId}`, so no separate cleanup path is needed).
- **Upload endpoint**: new endpoint, shaped like the existing icon-upload pattern (`api/internal/api/v0/endpoints/icons/icons.go` — multipart or base64-JSON body in, bytes written to a fixed path), not the content-addressed `state/content` store (that store is internal to tool-call payloads and isn't meant to produce a workspace-visible path).
- **Filenames**: `{shortid}-{originalName}` — the folder is per-*chat*, not per-message, so multiple turns can attach into the same directory; a generated-ID prefix avoids collisions while keeping the original name for display.
- **Size cap**: needed, exact number TBD in implementation (existing precedents: icons 2MiB, tool-call content store 8MiB; chat attachments likely want a larger ceiling than either given images/PDFs).
- **Delivery to the agent**: the resolver writes the file's absolute path directly into the markdown (`![alt](/abs/path/...)` / `[name](/abs/path/...)`). No copy-into-worktree step. Since `chats/{chatId}/attachments/` is a sibling of `worktree/`, not inside it, the provider CLI subprocess reading that path is relying on its own native permission mode (per the existing native-mode permission redesign) to approve a read outside its cwd — Crowbar does not special-case this per provider.

## Thresholds & fallback rules

- **Paste-to-text-attachment**: proposed default >400 chars or >6 lines collapses to a pill; shorter stays plain inline text. Tunable, not architectural. Does not trigger if the caret is inside an existing code block.
- **CSV table-vs-file**: small enough to render as a reasonable markdown table (rows × columns) stays inline; otherwise falls back to a file attachment. Exact cutoff TBD.
- **Inline-size cap (text, Excalidraw)**: the client-side prompt queue caps a message at 64KB (`MAX_PROMPT_TEXT_BYTES`, `prompt-queue-persistence.ts:29`). Since text-attachment and Excalidraw both default to inline encoding, either can exceed that cap. Fix: apply the same "small enough → inline, else → file" rule CSV already uses — an oversized paste becomes a `.txt` file attachment instead of an inline pill; an oversized Excalidraw scene becomes a `.excalidraw.json` file instead of a fenced block. One shared size check, reused across all three inline-capable kinds, rather than three separate ad-hoc rules.
- **Oversized file/image drop** (exceeds the size cap): rejected at the picker/drop handler with an inline error; nothing partial gets inserted.
- **Unsupported/unknown file type**: no special-casing — falls through to a generic file card (icon by extension, filename, size).
- **Missing attachment on reload** (file deleted from disk out-of-band): the image/file node renders a broken/missing placeholder rather than failing the whole message. `MarkdownImageKit` likely already needs to handle a broken relative image path today, so this is an existing concern being extended, not a new one.

## Open questions for the implementation plan

- Composer drag-handle scope: adopt a `BlockMenuKit`-equivalent subset, or build a lighter drag affordance scoped just to attachment blocks (composer deliberately lacks the rest of `BlockMenuKit`'s chrome — `/`-insert, block-type conversion — so a full adoption isn't a clean drop-in).
- Exact byte/char/line/row thresholds for: paste-to-pill, CSV table-vs-file, inline-vs-file size cap.
- Exact upload endpoint request/response contract (multipart vs base64-JSON, matching which existing precedent most closely).
- Excalidraw library choice and version; whether the embedded editor is a modal or an inline panel.
- Exact Plate node/component names and where they live among the three existing `plate/` directories (composer vs transcript vs markdown editor) given `chatComposerPlugins`/`chatComposerPluginsStatic` are shared between composer and transcript rendering.
- Whether "Attach File" needs any client-side mimetype allowlist/denylist, or accepts anything.

## Testing considerations

- Paste interception: threshold boundary behavior, codeblock-context exception, Shift-bypass, image-in-clipboard.
- Resolver correctness per kind: inline vs file fallback at size boundaries, markdown round-trip (encode → parse → re-render matches original).
- Drag-and-drop: revived `extractDroppedFilePaths()` path, drop-position insertion.
- Missing-file placeholder rendering (simulate a deleted attachment).
- Upload endpoint: size cap rejection, filename collision handling.
