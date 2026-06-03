# Crowbar Backend — Language Server Protocol (LSP)

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`,
> `03-realtime-websockets.md`
> **Scope:** The backend as an LSP host/proxy between the browser's Monaco editor
> and real language-server processes. Covers UX spec §26 and the LSP parts of §7.

---

## 1. The Backend as an LSP Host

Monaco in the frontend has a full LSP client wired (UX §26). The backend's job is
to be an **LSP host/proxy**: spawn the right language server for the workspace's
language(s), speak LSP (JSON-RPC over stdio) to it, and translate between the
frontend's requests and the server's responses.

The backend is essentially a **bridge between the browser's Monaco and a real
`gopls` / `typescript-language-server` / `pyright` / … process** running on the
user's machine.

```
internal/engine/lsp/
  lsp.go              LSPEngine interface
  internal/
    registry/         language → server command (+ defaults, config-extensible)
    server/           one running LSP server process: stdio JSON-RPC client
    manager/          per-workspace pool of servers (lazy spawn, lifecycle)
    convert/          LSP types ↔ Crowbar DTO (Position, Diagnostic, …)
```

---

## 2. Two Interaction Modes

### A) Request/response (REST) — synchronous features

The frontend asks, the backend forwards to the LSP server, waits, returns.

| Feature | LSP method | Endpoint |
|---------|-----------|----------|
| Completion | `textDocument/completion` | `POST /v0/workspaces/:wsId/lsp/completion` |
| Hover | `textDocument/hover` | `POST /v0/workspaces/:wsId/lsp/hover` |
| Definition | `textDocument/definition` | `POST /v0/workspaces/:wsId/lsp/definition` |
| References | `textDocument/references` | `POST /v0/workspaces/:wsId/lsp/references` |
| Rename | `textDocument/rename` | `POST /v0/workspaces/:wsId/lsp/rename` |
| Code actions | `textDocument/codeAction` | `POST /v0/workspaces/:wsId/lsp/codeAction` |
| Document symbols | `textDocument/documentSymbol` | `POST /v0/workspaces/:wsId/lsp/documentSymbol` |
| Signature help | `textDocument/signatureHelp` | (part of the completion flow) |

"Go to symbol in current file" (UX §16) is served by `documentSymbol`. ("Go to
line" is client-side.)

### B) Push (WS) — asynchronous diagnostics

Diagnostics arrive **unprompted** from the server
(`textDocument/publishDiagnostics`), not in response to a request. They push over
the LSP WS topic, namespaced by `wsId` (`03-realtime-websockets.md`).

```
WS /v0/ws/lsp?wsId=        →  Diagnostic[]
GET /v0/workspaces/:wsId/lsp/diagnostics   (on-demand snapshot)
```

```
Diagnostic { filePath, range: { start: Position, end: Position },
             severity: error|warning|info|hint, message, source?, code? }
Position   { line, character }
```

---

## 3. Document Sync — Frontend-Driven (Q1 = A)

LSP servers are **stateful**: they answer queries against the content of "open"
documents, told to them via `didOpen` / `didChange` / `didClose`. In Crowbar the
**source of truth for file content is the editor buffer in the browser**, which
may contain **unsaved edits**.

Therefore document sync is **frontend-driven**:

- The frontend issues `didOpen` / `didChange` / `didClose` (the backend forwards
  them to the server), and/or includes the current buffer text with LSP requests.
- The backend stays **stateless** about buffer content — the browser's buffer is
  authoritative.
- Consequence: completions, diagnostics, hover, etc. reflect **unsaved edits**,
  matching VS Code's in-browser model and the UX's "live diagnostics while
  typing" expectation.

> This is deliberately **not** disk-based sync. The file watcher
> (`05-filesystem-and-watcher.md`) drives the file tree and git status, but
> **not** LSP document content — otherwise the server would lag behind unsaved
> edits.

---

## 4. Server Registry & Defaults (Q2)

`registry/` maps a file's language (by extension) to a language-server command.
It ships with a **default set** and is **config-extensible** (users add or
override servers). The capability is expected to grow over time.

**Default servers shipped at launch:**

| Language | Server |
|----------|--------|
| TypeScript / JavaScript | `typescript-language-server` |
| Go | `gopls` |
| Python | `pyright` |
| Java | `jdtls` (Eclipse JDT Language Server) |
| C / C++ | `clangd` |

Users may add servers for other languages or override the defaults via config.
The backend does not bundle the servers themselves — it expects them installed
(like the git binary and provider CLIs).

---

## 5. Server Lifecycle

Per-workspace, **lazy** (matching the watcher / LSP lifecycle in
`03-realtime-websockets.md` §6):

- The first LSP request or WS subscription for a workspace spawns the needed
  server(s).
- Servers shut down when the workspace's LSP subscription drops (ref-counted with
  the Files/Git subscriptions for that `wsId`).
- A workspace may run **multiple** servers concurrently (e.g. a repo with both Go
  and TS code) — one per language, managed by `manager/`.

### Graceful absence (UX §20)

If no server is configured or installed for a file's language, the feature is
**gracefully absent**: no completions/diagnostics, and the editor's status
indicator shows the inactive state. This is not an error — it is the documented
"LSP server not running" state.

---

## 6. Translation Layer

`convert/` maps between LSP wire types and Crowbar DTOs:

- LSP `Position` (0-based line/char) ↔ Crowbar `Position`.
- LSP `Diagnostic` ↔ Crowbar `Diagnostic` (severity enum, source, code).
- LSP `Location` / `WorkspaceEdit` ↔ definition/references/rename results.

Rename (`textDocument/rename`) returns a `WorkspaceEdit` spanning multiple files;
the frontend applies it atomically (UX §26 "all usages updated atomically").

---

## 7. Out of Scope

- Bundling/installing the language servers (expected pre-installed).
- Formatter execution (separate from LSP in this design; client/editor concern).
- Disk-based document sync (rejected — §3).
