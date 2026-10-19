# S5 — Editor host and documents: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One Monaco environment per window whose models are the only owner of buffer text, one document API that can open any file on the host over the local socket, and a slim grammar bundle — measured against open-file and keystroke budgets in the Tauri app.

**Architecture:** The daemon gains one document read/write path keyed by a `DocumentRef` (workspace + relative path, or an absolute host path); host paths are served only to requests that arrived on the unix socket. The client keeps one window-level `EditorHost` (manager + document registry) instead of one per workspace store; the buffer store keeps identity, dirty flag and view state, and every reader of text goes through one seam that resolves to the Monaco model. Shiki loads through `shiki/core` with the JavaScript regex engine and per-language grammars, shared by Monaco and Plate.

**Tech stack:** Go (gin, net/http), React 19, zustand+immer, Monaco 0.55.1, Plate, shiki 4.4.3, vitest + fake-indexeddb, Tauri MCP for live checks.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` — §3 "Monaco owns buffer text", §7 rows 1–2, §11 row S5. Constraints: `REFACTOR-CONSTRAINTS` (carried below) and the repo `CLAUDE.md`.

**Baseline commit:** written against `9641256b5`. S1–S4 land before this plan runs and will have moved some of the code named here (entity store, per-workspace store slices, error envelope). Before each task, `grep` the symbols its **Files** block names. When a symbol moved, follow the responsibility, not the path. When S2–S4 already deleted something this plan deletes, say so in the commit message and skip that step.

---

## Global constraints

- No feature flags, no parallel old/new paths, no compatibility shims between stages; each task deletes what it replaces in the same commit.
- The app keeps working after every task: targeted tests green, `bun tsc --noEmit` clean, the touched surface verified live where the task says so.
- A fix starts with a failing test. Never fix with a timer, poll, retry, flag or resync effect. Every backend bug gets a black-box `TestRegression_*`.
- Web tests live in `web/src/__tests__/` mirroring `web/src/`, use `@/` imports, have at most 3 `vi.mock` each, never assert Tailwind classes or read `src/` text, and never sleep (fake timers, `waitFor`).
- Extend the unit's existing test file. Do not create `<unit>-<scenario>.test.ts`.
- Run only the targeted tests named in each task: `cd web && node_modules/.bin/vitest run <files>`. Never run the whole vitest suite. Typecheck with `bun tsc --noEmit`. `bunx tsc` is a different npm package that always exits 0.
- Go: build and test with `-tags noEmbed`; integration with `-tags 'integration noEmbed'`; `make lint` for new code; `make deadcode`; funlen 100/50, gocyclo 15, nestif 2, early returns, wrapped errors (`fmt.Errorf("op: ctx: %w", err)`), required deps as constructor args, no `...ForTest` in non-test files.
- Files ≤ 500 lines; a task that substantially edits an oversize file brings it under 500 lines.
- Delete code you make unused in the same change. knip, eslint and deadcode baselines only shrink: when a task fixes an entry in `web/knip.json` `ignoreIssues`, `web/eslint.config.js` `*_BASELINE` or `api/deadcode-baseline.txt`, delete the entry.
- Comments explain why in ≤ 3 lines. No narrative comments.
- Web: kebab-case files, narrow store selectors, `getState()` only in handlers and effects, stores never import components, `@/components/ui/*` + CSS-var tokens, phosphor icons, base-ui, Plate is the only markdown renderer.
- Preserve every existing animation. The editor area's are listed in the feature map below.
- Safety: never touch `~/.crowbar/crowbar.sock` or `~/.crowbar/state`; never read or write `~/.claude` or `~/.codex`; live runs use `make dev-desktop` / `make dev-bundle` (isolated `CROWBAR_HOME=<worktree>/.crowbar`). Run `df -h /System/Volumes/Data` before any build and stop if under 8 GB free. Never message other sessions, never bare `git stash`, never push, no PRs.
- Live verification uses only the Tauri MCP (`mcp__tauri__*`). Do not use `osascript`, `screencapture` or other OS automation. Synthetic key events never reach Monaco's input in WKWebView, so keystroke measurements need the user at the keyboard. Ask them; do not substitute.

## Review focus

The five failure modes a person will hit that no single task's happy-path test covers. Each line names the task whose tests pin it.

1. **One file reached two ways** (Files tree vs. a terminal link with the absolute path; `/var/...` vs `/private/var/...`) must be one buffer and one model, never two copies that overwrite each other on save. Pinned by T1 `TestRegression_AbsolutePathInsideWorktreeIsWorkspaceDocument` and T3 `the same file through an absolute path reuses the workspace buffer`.
2. **Typing while a save is in flight**, and Plate edits not yet flushed when Cmd+S lands, must stay dirty and reach disk on the next save. Pinned by T5 `readDocumentText includes edits a pending writer has not flushed yet` and T6 `a save marks clean only if the model still holds what was written`.
3. **Reload or crash with unsaved text**: the text must come back dirty after a reload, including for untitled buffers. Pinned by T6 `restores a dirty draft as a dirty document` and `pagehide flushes pending drafts`.
4. **Host file on a non-local transport** (browser dev mode over `make dev-api`, Docker): a visible reason, never a blank tab or a silent no-op. Pinned by T1 `TestRegression_HostDocumentRefusedOverTCP` and T3 `a refused host read says why and opens nothing`.
5. **The same document in two split panes** after model ownership: live sync, one undo stack, and closing one pane must not dispose the model the other shows. Pinned by T2 `a pane keeps its model when another pane closes the same file`, and by T6 `a document lives as long as its buffer, not its pane`.

---

## Rulings (decisions the spec leaves open)

1. **Document identity.** `DocumentRef = { authority: 'workspace', workspaceId, path (relative) } | { authority: 'host', path (absolute) }`. The daemon canonicalises every absolute path: if it lies inside a known worktree or project home (longest canonical prefix, symlinks resolved), it becomes that workspace's document. A file therefore has exactly one identity, and LSP, blame and the watcher apply whenever they can. Buffers keep workspace-relative paths, so S8's worktree move does not invalidate persisted tabs.
2. **Authority = transport.** Host documents are served only when `http.LocalAddrContextKey`'s network is `unix`. Loopback TCP is refused too, because any local process and DNS-rebinding web pages can reach it. The daemon cannot verify that a path came from a dialog, a drop or a terminal link, and a unix-socket client already has a shell through the PTY routes. So the transport check is the security boundary. The "grant" is enforced on the client by construction: only `openDocumentFromDialog`, `openDocumentsAtPaths` (OS drop) and terminal links build a path-only `DocumentTarget`.
3. **One document API.** `GET/PUT /v0/documents/content` replaces the chat-scoped and project-home `/files/content` routes for every client read and write: editor, assets, uploads, templates, `.gitignore`. Tree and mutation routes stay on `/files`.
4. **Host documents get text editing only:** open, edit, save, reload, rich markdown, previews, shiki highlighting. They get no LSP, blame, git status, explorer reveal or external-change watch, because no watcher runs outside worktrees. Open an issue for the external-change watch in T3 (step 7); do not add a poll.
5. **Monaco models own all document text, including rich markdown.** Plate reads the model at mount and writes back into it, debounced 150 ms as today, with a registered pending writer so saves see unflushed edits. The first rich-markdown open therefore loads the editor host; the cold-open budget covers it.
6. **Dirty state** = the model's alternative version id differs from the saved one, AND (when lengths match) the text differs from the saved text. This keeps today's behaviour that typing a character and deleting it is clean. The store is written only on transitions.
7. **Unsaved text persists as drafts** in an `editor-drafts` IndexedDB store, written after a 1 s quiet period per document and on `pagehide`. They no longer live in the layout record, which keeps identity only. The legacy layout's buffer content converts into drafts once (T6), as this stage's piece of the legacy → layout-1 conversion.
8. **Restored documents open eagerly at window hydrate.** Today `EditorHostRegistry` already loads Monaco at boot for any pane with an editor tab, and hydrate already reads every restored file from disk.
9. **Grammar engine:** `shiki/core` + `createJavaScriptRegexEngine({ forgiving: true })` + the `shiki/langs` lazy index. This drops the 622 KB inlined oniguruma wasm from the open path. If the live check in T4 shows the JS engine misses the colored-viewport budget where oniguruma meets it, swap that one line for `createOnigurumaEngine(import('shiki/wasm'))`. There is one engine either way.
10. **LSP document sync is per document, not per pane** (T7): one didOpen/didClose per document, whatever number of panes show it.
11. **New UI required by §7 row 1:** "Open File…" (Cmd+O, `tabs.openFile`) in the keymap and in the tab "+" menu. It is desktop-only and hidden in browser mode (no stub).

## Spec statements the code contradicts

- *"full-bundle grammar load on first open"* (§7 row 2): `shiki/bundle/full` is a 35 KB lazy index, and grammars already load per language. What the first open actually pays is the 622 KB inlined oniguruma wasm plus each grammar's first compile (60–78 ms for tsx). "Slim" therefore means the engine and the index, not the grammars.
- *"One Monaco environment per workspace store"*: only the manager and model registry are per workspace. The widget host (`EditorHostRegistry`, `editor-portal-registry.ts`) is already window-level.
- `hydrateWorkspace` reads the `editor-state` IndexedDB store on every workspace activation, but nothing ever writes it (dead read).
- A Finder drop onto a pane passes absolute paths into the workspace-relative open, so the daemon's `safepath` rejects every one ("Failed to open file"). The pane's DOM drop handler is wired to `extractDroppedFilePaths`, which always returns `[]`, a stub that ships.
- `buffer-save.ts`'s `settings://user-settings.json` branch is dead (nothing opens that buffer). `EditorManager.layoutAll` has no callers. `EditorContent.tokens` is never read.
- The CSV preview offered by the breadcrumb renders "CSV preview not available" (`extensions/viewers/csv`). This is a stub outside S5; it is deleted with `extensions` in S9.
- Markdown assets, the breadcrumb dropdown and jump navigation resolve the workspace from the *ambient* active workspace, which is the ambient-fallback bug class. T3 moves them onto the buffer's `DocumentRef`.
- `make dev` runs `dev-api` on `tcp://127.0.0.1:3737`. Under Ruling 2, host documents are refused there by design; only the desktop sidecar (unix socket) serves them.

---

## Budgets

All budgets are measured live, on the real gesture, in the Tauri app with the isolated home. The procedure and instrument are in Appendix A. A metric passes when it is within its budget **and** no more than 10 % worse than its T0 baseline.

| Id | Metric (gesture → milestone) | Build | Budget |
|---|---|---|---|
| O1 | Single click on a Files-tree row → the file's first line painted in the pane (10 distinct 2,000-line `.ts` files, editor host and TS grammar warm) | `make dev-desktop` | p50 ≤ 80 ms, p95 ≤ 120 ms |
| O2 | Same click → the probe identifier on line 1 re-classed by grammar tokens (shiki semantic colouring) | `make dev-desktop` | p95 ≤ 400 ms |
| O3 | Cmd+O, pick a file outside every workspace → painted (from the window's `focus` when the dialog closes), 5 samples | `make dev-desktop` | p95 ≤ 150 ms |
| O4 | First file open after app launch (editor host cold) → painted / grammar-coloured | `make dev-bundle` | ≤ 500 ms / ≤ 1,000 ms |
| K1 | Trusted keydown in Monaco → next frame (keydown → rAF → MessageChannel), ≥ 200 keystrokes typed by the user at natural speed into a 5,000-line `.ts` file shown in two split panes, autosave off | `make dev-desktop` | p95 ≤ 16 ms, max ≤ 50 ms (first keystroke after focus excluded) |
| K2 | IndexedDB writes during one continuous burst (gaps < 1 s), then the 2 s after it | `make dev-desktop` | `workspace-layout` ≤ 1 (the clean→dirty flip), `editor-drafts` 0 during, exactly 1 per dirty document after |
| K3 | Frames longer than 33 ms during the K1 burst | `make dev-desktop` | 0 |

O4 runs on `make dev-bundle` because Vite dev serves Monaco unbundled, which makes a cold load unrepresentative. That target also exports the isolated `CROWBAR_HOME`.

---

## Feature map: everything the editor does today, and where it lives after S5

| Feature | Today | After |
|---|---|---|
| Retained per-pane Monaco widget; tab switch = model swap; native view state per (pane, file) | `editor-manager.ts` (one per workspace store), `editor-surface.tsx`, `use-pane-editor-controller.ts` | Same files; one manager per window (T2) |
| Same file in two panes = one model (live sync, shared undo) | `model-registry.ts`, held per pane | `documents/document-registry.ts`; model lives as long as the buffer (T6) |
| Widget survives split, fullscreen, tab switch (portal host) | `editor-host-registry.tsx`, `editor-portal-registry.ts` | Unchanged |
| Monaco loads only when an editor first renders | `workspace-store.ts` `armEditor` | `host/editor-host.ts` `loadEditorHost` (T2) |
| Pane-resize layer promotion, layout suppressed mid-drag, `data-editor-layout` GPU frame, `pane-resize-end` flush | `editor-surface.tsx` `mountPane`, `styles/editor-theme.css` | Unchanged |
| Selection-drag cursor-sync throttle (100 ms) | `editor-surface.tsx`, `selection-drag.ts`, `raf-coalesce.ts` | Unchanged |
| Status bar: line:col chip, toggles (breadcrumbs, minimap, line numbers, word wrap, parameter hints, autocompletion), language picker (`languageOverride`), LSP status menu | `editor-status-actions.tsx`, `lsp-status-menu.tsx`, `state-store.ts` | Unchanged; LSP restart reads text via the seam (T5) |
| Settings → editor/model options (font, line height, tab size, whitespace, indent guides, semantic highlighting, occurrences) | `use-editor-appearance.ts` | Unchanged |
| Theme from CSS vars, zoom levels | `define-theme.ts`, `resolve-css-color.ts`, `use-theme-version.ts`, `zoom-store` | Unchanged |
| Breadcrumb with directory dropdown and find button; md/html/csv preview buttons | `breadcrumb.tsx`, `file-path-breadcrumb.tsx`, `breadcrumb-directory.ts` | Driven by the buffer's `DocumentRef`: dropdown lists the buffer's own workspace; host docs show plain segments (T3) |
| Preview tabs (italic, promoted on first edit), pinned tabs, per-workspace tab cap with eviction, reopen closed tab | `buffer-slice.ts`, `tabs/*`, `editor-surface.tsx` `writeContent` | Promotion on the document's first dirty transition (T6); reopen through `openDocument` (T3) |
| Dirty dot, unsaved aria labels, close-dirty confirmation | `tab-bar-item.tsx`, `tab-bar.tsx`, `use-pane-keyboard.ts`, `use-tab-keyboard-nav.ts` | Unchanged readers of `isDirty`; writer is the registry (T6) |
| Save, Save All, autosave (1 s per document), format-on-save, Save As for untitled, locked-workspace error toast | `buffer-save.ts` | Text read through the seam (T5) from the model (T6); autosave keyed by document uri |
| Reload from disk with dirty confirmation | `reload-buffer.ts` | `applyExternalText` (T5) |
| External change: clean buffer reloads silently, dirty buffer flagged + toast; warm-reactivation reconcile | `external-buffer-sync.ts`, `hydrate.ts`, `use-workspace-file-tree.ts` | `savedDocumentText` + `applyExternalText` (T5); reconcile moves into `external-buffer-sync.ts` (T6) |
| "File not found" placeholder | `editor-pane.tsx` + hydrate | `restore-documents.ts` sets `fileMissing` (T6) |
| Unsaved edits survive reload | Full buffer text in the persisted layout | `editor-drafts` store (T6) |
| Rich markdown (Plate): rich/source toggle, frontmatter banner, images/assets, mermaid, HTML nodes, callouts, code blocks | `markdown/plate/*` | Plate reads and writes the model (T5/T6); assets resolve from the `DocumentRef` (T3) |
| Markdown preview tab, HTML preview tab with inlined assets, live update | `markdown-preview.tsx`, `html-preview.tsx` | `useDocumentText` (T5) |
| Monarch grammars on demand; custom diff/gitignore/gitattributes/toml/… languages | `language-contributions.ts` | Unchanged |
| Shiki semantic tokens in Monaco; Plate code-block highlighting (chat + markdown) | `shiki-tokens.ts`, `shiki-lowlight.ts` (`shiki/bundle/full`) | `lib/highlight/shiki-grammars.ts` (T4) |
| LSP: completion, hover, signature help, definition, references peek, rename, code actions, code lens, formatting, document symbols, semantic tokens, diagnostics, workspace edits | `lsp/*`, `use-lsp-document-sync.ts` | Providers unchanged; sync per document (T7); host docs skipped (T3) |
| Inline git blame | `use-inline-blame.ts` | Unchanged; skipped for host docs (T3) |
| Jump list back/forward; go-to-definition across files | `jump-list-store.ts`, `jump-navigation.ts`, `reveal.ts`, `registerEditorOpener` | Entries and targets carry a `DocumentRef` (T3) |
| Cmd/Ctrl+A select-all override, focus on swap, Monaco find widget and go-to-line | `use-pane-editor-satellites.ts`, Monaco | Unchanged |
| Explorer click/preview open, drag a file onto a pane, active file reveal | `file-tree-handlers.ts`, `use-file-explorer-drag-drop.ts`, `file-explorer-tree.tsx` | `openDocument` (T3) |
| Git history open file; breadcrumb path click; markdown preview link click | `git-history-list.tsx`, `file-path-breadcrumb.tsx`, `markdown-preview.tsx` | `openDocument` (T3) |
| Terminal file links (open at line) | `use-xterm-instance.ts` (refuses paths outside the workspace) | `revealInEditor({ doc: { absolutePath } })` (T3) |
| Finder drop onto a pane | `use-pane-drop-handlers.ts` (broken) | `openDocumentsAtPaths` (T3) |
| Reveal in Finder | `revealInFolder` | Unchanged |
| Upload files into the tree; new-file templates; `.gitignore` reading | `file-upload.ts`, `use-file-explorer-context-menu.tsx`, `use-file-explorer-gitignore.ts` | `writeDocument` / `readDocument` (T1) |
| Open File… | — | Cmd+O + tab "+" menu, desktop only (T3) |
| Animations: tab close-button fade (`transition-opacity`, `tab-bar-item.tsx`), status-bar button colours (`transition-colors`, `editor-status-actions.tsx`), markdown link transition (`markdown/styles.css:143`), Monaco caret blink (`cursorBlinking: 'blink'`), Monaco find-widget slide | — | Files untouched by S5. T8 re-checks each live |

## Deletion table

| Spec "Deletes" item | Exactly what goes | Task |
|---|---|---|
| Open benchmark | `web/src/features/editor/utils/file-open-benchmark.ts`; its calls in `use-file-explorer-visible-rows.ts:3,77-83`, `file-explorer-tree.tsx:5,246-249`, `use-tree-container-events.ts:4,133-134` | T0 |
| Per-workspace editor managers | `WorkspaceEditorHandles` + `armEditor` (`workspace-store.ts:20-153`); `canEvictWorkspace` + its veto (`workspace-store-registry.ts:270-280`, `workspace-host.tsx:208`); model disposal in `destroyWorkspaceStore` (`:283-299`); `EditorPane`'s arming and ambient fallback (`editor-pane.tsx:54-121`); `managerKey` (`use-pane-editor-controller.ts`); `EditorManager.hasMountedPanes/layoutAll/disposeAll`; `ModelRegistry.disposeAll`; per-workspace `activeEditorRegistry`; tests `workspace-store-editor-arm.test.ts`, `editor-pane-workspace-scope.test.tsx`, `editor-pane-surface-select.test.tsx` (moved) | T2 |
| Content sink | `lib/content-sink.ts` + test; the sink in `use-pane-editor-controller.ts:143-181`; `writeContent`/`onControllerContentChange` (`editor-surface.tsx:252-287`); `setBufferContent`; the `flush-editor-content` window event (centralised in T5, deleted in T6) | T6 |
| Apply latch | `externalApplyRef` (`editor-surface.tsx:261`, satellites deps); `EditorManager.applyExternalEdit` | T6 |
| Store → model reconcile | Satellites effect (`use-pane-editor-satellites.ts:123-146`); `shouldReconcileModelFromStore`; `BufferMeta.text`; `ModelRegistry` adopt-with-buffer-text; `EditorContent.content/savedContent/tokens`; `TokenEntry` | T6 |
| Same commits, replaced paths | `/files/content` GET/PUT (chat + home) handlers, routes and tests; `readFile`, `writeFile`, `readWorkspaceFile`, `writeWorkspaceFile` (`platform.ts`); `writeFileContent` (`file-tree-api.ts`) | T1 |
|  | `open-file-content.ts`; `openWorkspaceFile`; `workspaceRelativePath`; `extractDroppedFilePaths` + its test; `editor-uri.ts` + test (folded into `document-ref.ts`) | T3 |
|  | Both `shiki/bundle/full` imports and their per-module highlighter singletons | T4 |
|  | `editor-state` IndexedDB store and its read; `settings://` save branch; `ModelRegistry` (becomes `DocumentRegistry`) | T6 |
|  | `hooks/use-lsp-document-sync.ts` + test | T7 |

---

## Task 0: Budget instruments, baseline, and the open benchmark goes

**Goal:** Record the baseline for every budget with the injected instruments, then delete the in-app benchmark they replace.

**Files:**
- Delete: `web/src/features/editor/utils/file-open-benchmark.ts`
- Modify: `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-visible-rows.ts` (drop import line 3 and the `fileOpenBenchmark` blocks at 77-83)
- Modify: `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx` (line 5 and the effect at 246-249)
- Modify: `web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts` (line 4 and 133-134)
- Modify: `docs/plans/2026-10-17-s5-editor-host-plan.md` (Appendix B, "baseline" column)

**Interfaces:** Produces the baseline numbers T8 compares against. `utils/frontend-trace.ts` stays (git-slice and use-filtered-file-tree still use it).

- [ ] **Step 1:** `df -h /System/Volumes/Data` (stop under 8 GB). Then `make dev-desktop` and `make seed`. Note the `worktree` path printed by the seed.
- [ ] **Step 2:** Create the fixtures with Appendix A §A.1 (10 open files, the typing file, the host file).
- [ ] **Step 3:** Measure O1, O2, O3, K1, K2 and K3 with Appendix A §A.2–A.4. K1–K3 and O3 need the user at the keyboard; ask them. Then quit, `make dev-bundle`, and measure O4 the same way. Write the numbers into Appendix B's "baseline" column.
- [ ] **Step 4:** Delete the benchmark and its three call sites. `grep -rn "fileOpenBenchmark\|file-open-benchmark\|VITE_FILE_OPEN_BENCHMARK" web/src` must return nothing.
- [ ] **Step 5:** Verify: `cd web && bun tsc --noEmit && bunx eslint src/features/file-explorer --max-warnings 0 && bun run knip`. Then `node_modules/.bin/vitest run src/__tests__/features/file-explorer`.
- [ ] **Step 6:** Commit.

```bash
git add -A web/src/features docs/plans/2026-10-17-s5-editor-host-plan.md
git commit -m "perf(editor): measure S5 budgets live and drop the in-app open benchmark"
```

**Acceptance:** Appendix B has a baseline for O1–O4 and K1–K3; the benchmark is gone; knip and tsc are clean.

---

## Task 1: One document API, with host files on the local socket

**Goal:** `GET/PUT /v0/documents/content` reads and writes any file by `DocumentRef` or by absolute path. Host paths are served only over the unix socket. Every client read and write moves to it, and both `/files/content` routes are deleted.

**Files (Go):**
- Create: `api/internal/domain/document.go`, which holds `DocumentAuthority`, `DocumentRef`, `Document` and `Transport`.
- Create: `api/internal/app/usecases/file/document.go`, which holds `ReadDocument`, `WriteDocument` and `resolveDocument`.
- Create: `api/internal/api/v0/endpoints/documents/routes.go`, `handlers.go`, `handlers_test.go`.
- Modify: `api/internal/app/apperr/apperr.go`: add `ErrHostDocumentNeedsLocalTransport`.
- Modify: `api/internal/engine/fs/safepath/safepath.go` + `safepath_test.go`: add `ErrNotAbsolute` and `Canonical`.
- Modify: `api/internal/engine/fs/internal/content/content.go` + `content_test.go`: add `ReadAbs` and `WriteAbs`, sharing one `readFile(full, cap)` and `writeAtomic`.
- Modify: `api/internal/engine/fs/fs.go` + `fs_test.go`: the `Engine` gains `ReadHostContent` and `WriteHostContent`.
- Modify: `api/internal/app/usecases/file/file.go` + `file_test.go`: `FsEngine` gains the two host methods; `WorkspaceSyncer` gains `List`; `Usecase` replaces `ReadContent`/`WriteContent` with `ReadDocument`/`WriteDocument`.
- Modify: `api/internal/app/usecases/mocks/mocks.go`: add the matching mock funcs.
- Modify: `api/internal/api/libs/status.go` + `status_test.go`: map the new errors.
- Modify: `api/internal/api/v0/router.go`: mount the endpoint on the `/v0` group.
- Modify (deletions): `api/internal/api/v0/endpoints/files/routes.go`, `files/handlers/{files.go,handlers.go,handlers_test.go,error_test.go,chat_scope_test.go}`, `files/routes_test.go`, `home/routes.go`, `home/handlers/{files.go,handlers.go,files_test.go,handlers_test.go}`, `api/internal/api/v0/route_audit_test.go`.
- Modify (tests moved to the new route): `api/tests/{files,git,review,home,regressions,fixtures}_test.go`, `api/tests/integration/files/{files_test.go,home_files_test.go}`, `api/tests/integration/lifecycle/lifecycle_test.go`, `api/tests/integration/agent/agent_mcp_context_test.go`.

**Files (web):**
- Create: `web/src/lib/api/documents.ts`; test `web/src/__tests__/lib/api/documents.test.ts`.
- Modify: `features/workspace/lib/open-file-content.ts`, `features/workspace/lib/external-buffer-sync.ts`, `features/panes/stores/slices/buffer-slice.ts` (reopen refill), `features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`, `features/file-explorer/file-explorer/hooks/use-file-explorer-gitignore.ts`, `features/editor/lsp/monaco-lsp-providers.ts`, `features/editor/markdown/plate/markdown-asset.ts`, `features/editor/lsp/workspace-edit.ts`, `features/editor/components/html/html-preview.tsx`, `features/editor/lib/buffer-save.ts`, `features/editor/lib/reload-buffer.ts`, `features/editor/lib/reveal.ts`, `lib/persistence/hydrate.ts`, `features/files/lib/file-upload.ts`.
- Modify: `features/file-system/controllers/platform.ts` (delete `readFile`, `writeFile`, `readWorkspaceFile`, `writeWorkspaceFile`); `features/files/lib/file-tree-api.ts` (delete `writeFileContent`).
- Modify tests: replace their mock targets `@/features/file-system/controllers/platform` / `writeFileContent` with `@/lib/api/documents` in `platform.test.ts`, `file-tree-api.test.ts`, `file-upload.test.ts`, `open-file-content.test.ts`, `external-buffer-sync.test.ts`, `markdown-asset.test.ts`, `jump-navigation.test.ts`, `reload-buffer.test.ts`, `reveal.test.ts`, `buffer-save.test.ts`, `hydrate.test.ts`.

**Interfaces — Produces:**

```go
// domain/document.go
type DocumentAuthority string
const (
	DocumentAuthorityWorkspace DocumentAuthority = "workspace"
	DocumentAuthorityHost      DocumentAuthority = "host"
)
type DocumentRef struct {
	Authority   DocumentAuthority `json:"authority"`             // "" = unresolved absolute path
	WorkspaceID string            `json:"workspaceId,omitempty"`
	Path        string            `json:"path"`
}
type Document struct {
	DocumentRef
	FileContent
}
type Transport int
const (
	TransportRemote Transport = iota
	TransportLocal
)

// usecases/file
ReadDocument(ctx context.Context, target domain.DocumentRef, transport domain.Transport) (domain.Document, error)
WriteDocument(ctx context.Context, target domain.DocumentRef, content, encoding string, transport domain.Transport, now time.Time) (domain.DocumentRef, error)

// engine/fs
ReadHostContent(absPath string) (domain.FileContent, error)
WriteHostContent(absPath, content, encoding string) error
// safepath
var ErrNotAbsolute = errors.New("path is not absolute")
func Canonical(path string) string // EvalSymlinks of the deepest existing ancestor + the rest
```

HTTP contract:
- `GET /v0/documents/content?workspace=<id>&path=<rel>` or `GET /v0/documents/content?path=<abs>` returns `{authority, workspaceId?, path, content, encoding?}` (the resolved ref plus content).
- `PUT /v0/documents/content` with body `{workspaceId?, path, content, encoding?}` returns the resolved `{authority, workspaceId?, path}`.
- A host ref on a non-unix transport gets 403 with error code `host_document_requires_local_transport`. When S3's closed code enum exists, the code is added there; otherwise use `libs.WriteErrCode`.

```ts
// web/src/lib/api/documents.ts
export type DocumentRef =
  | { authority: 'workspace'; workspaceId: string; path: string }
  | { authority: 'host'; path: string }
export type DocumentTarget = DocumentRef | { absolutePath: string }
export interface DocumentRead { ref: DocumentRef; text: string }
export const HOST_DOCUMENT_TRANSPORT_CODE = 'host_document_requires_local_transport'
export function readDocument(target: DocumentTarget): Promise<DocumentRead>
export function writeDocument(ref: DocumentRef, content: string, opts?: { encoding: 'base64' }): Promise<DocumentRef>
```

- [ ] **Step 1: Write the failing black-box tests.**

In `api/tests/integration/files/files_test.go` (kit, unix socket), add suite methods:

```go
func (s *FilesSuite) TestRegression_OpenFileOutsideWorkspace() {
	// abs := filepath.Join(<os.MkdirTemp outside every worktree>, "outside.md"), content "outside\n"
	// GET /v0/documents/content?path=<url-escaped abs> → 200
	//   authority == "host", path == safepath.Canonical(abs), content == "outside\n"
	// PUT /v0/documents/content {"path": abs, "content": "edited\n"} → 200
	//   os.ReadFile(abs) == "edited\n"
}
func (s *FilesSuite) TestRegression_AbsolutePathInsideWorktreeIsWorkspaceDocument() {
	// wt := s.Env.WorktreePath(t, s.wsID)
	// GET ?path=<wt>/hello.txt → authority "workspace", workspaceId s.wsID, path "hello.txt"
	// link := <tmp>/wt-link symlinked to wt; GET ?path=<link>/hello.txt → the same three fields
}
```

In `api/tests/files_test.go` (httptest, TCP), add:

```go
func TestRegression_HostDocumentRefusedOverTCP(t *testing.T) {
	// h := newHarness(t); abs := file in t.TempDir() with "secret\n"
	// h.raw(GET ?path=abs, 403): envelope code == "host_document_requires_local_transport"
	// h.raw(PUT {"path": abs, "content": "x"}, 403): os.ReadFile(abs) still "secret\n"
}
```

Then rewrite `TestFiles_TreeSaveReadBack` and `TestFiles_LockedWorkspaceRejectsWrite` onto `docURL(imported.workspaceID, path)`. Add this helper to `fixtures_test.go`: `func docURL(wsID, path string) string`, returning `/v0/documents/content?workspace=…&path=…` with query escaping. Workspace documents must work over TCP; a locked workspace's PUT must still return 409.

- [ ] **Step 2: Run and see them fail.**
`cd api && go test -tags 'integration noEmbed' -run 'TestFilesSuite/TestRegression_|TestRegression_HostDocumentRefusedOverTCP|TestFiles_' ./tests/...`. Expected: FAIL with 404 (route missing).

- [ ] **Step 3: Engine.**
In `safepath`:
- Add `ErrNotAbsolute`.
- Add `Canonical(path)`, reusing `existingAncestor`: canonicalise the deepest existing ancestor with `filepath.EvalSymlinks`, then append the non-existent remainder.

In `content`:
- Extract `readFile(full string, cap int64)` from `readWithCap`.
- `ReadAbs(abs)` and `WriteAbs(abs, content, encoding)` return `ErrNotAbsolute` unless `filepath.IsAbs(abs) && filepath.Clean(abs) == abs`. Otherwise they call `readFile` / `decodeContent` + `MkdirAll` + `writeAtomic`. Same 25 MiB cap.
- Unit tests: relative and unclean paths are rejected, the round trip works, an oversize file returns `ErrFileTooLarge`, and `Canonical` resolves a symlinked parent of a not-yet-existing file.

- [ ] **Step 4: Usecase.**
`resolveDocument(ctx, target)` resolves as follows:
- **Workspace authority:** returned as is (`safepath` guards the relative path at read and write).
- **Empty `WorkspaceID` with a relative path:** wrap `apperr.ErrInvalidArgument`.
- **Absolute path (authority host or ""):** `canon := safepath.Canonical(path)`. Then `u.syncer.List(ctx)`, skip rows with an empty `WorktreePath` (placeholders), and pick the row whose `safepath.Canonical(WorktreePath)` is the longest prefix of `canon` on a path-separator boundary. If one matches, return `{workspace, row.ID, rel}`; otherwise `{host, canon}`.

`ReadDocument`:
- Workspace: use the existing `repoPath` + `fs.ReadContent`.
- Host: return `ErrHostDocumentNeedsLocalTransport` unless the transport is `TransportLocal`, then `fs.ReadHostContent`.

`WriteDocument` mirrors `ReadDocument`:
- Workspace: `writePath`, which keeps the locked-workspace 409, then `resync`.
- Host: transport check, then `fs.WriteHostContent`, with no resync.

Unit tests in `file_test.go` use fake syncer/fs and cover:
- longest prefix wins (a project home that contains a worktree);
- placeholders are skipped;
- a home workspace row matches;
- a relative path without a workspace is a bad request;
- host with remote transport is refused and host with local transport reads.

- [ ] **Step 5: HTTP.**
`documents.Register(rg *gin.RouterGroup, files Documents)`, where `Documents` is the two-method consumer interface handler packages use today. It mounts `GET /documents/content → h.Read` and `PUT /documents/content → h.Write`.

`transportOf(r *http.Request) domain.Transport` returns `TransportLocal` iff `r.Context().Value(http.LocalAddrContextKey).(net.Addr).Network() == "unix"`.

Map errors in `libs/status.go`:
- `ErrHostDocumentNeedsLocalTransport` → 403, with the handler writing the code via `WriteErrCode`.
- `safepath.ErrNotAbsolute` → 400 (`isBadRequest`).

`handlers_test.go` covers `transportOf` under `httptest.NewServer` (remote) and under a `net.Listen("unix", <os.MkdirTemp>/s)` server (local), plus query parsing (`workspace` + `path` vs `path` alone).

Mount in `router.go` beside `files.Register`.

- [ ] **Step 6: Delete the old routes.**
Remove `ReadContent`/`SaveContent` (chat-scoped) and `FileContent`/`SaveFileContent` (home): their routes, `Files` interface methods and handler tests. Remove the six `/files/content` lines from `route_audit_test.go` and add the two documents routes. Move every listed test caller to `docURL`; the kit suites use `/v0/documents/content?workspace=<s.wsID>&path=…`.

- [ ] **Step 7: Web client, test first.**
`documents.test.ts` mocks `@/lib/api` (`apiFetch` only, 1 mock) and asserts:

```ts
it('reads a workspace document by workspace and relative path')  // URL '/v0/documents/content?workspace=w1&path=src%2Fa.ts'; resolves { ref: {authority:'workspace',workspaceId:'w1',path:'src/a.ts'}, text }
it('reads an absolute path and returns the document the daemon resolved')  // URL '?path=%2Ftmp%2Fx.md'; ref from the response
it('decodes a base64 payload byte-faithfully')  // same decode as decodeFileContent
it('writes base64 bytes when asked')  // PUT body { workspaceId:'w1', path:'a.bin', content, encoding:'base64' }
```

Implement `documents.ts`, reusing `decodeFileContent`. Then switch every listed call site. Map each removed function to its replacement:
- `readWorkspaceFile(ws, p)` → `(await readDocument({authority:'workspace', workspaceId: ws, path: p})).text`.
- `writeWorkspaceFile(ws, p, c)` → `writeDocument({...}, c)`.
- `writeFileContent(ws, p, c, 'base64')` → `writeDocument(ref, c, {encoding:'base64'})`.
- `readFile`/`writeFile` (active workspace, used by the gitignore and template hooks) → the same, with the `wsId` those hooks already hold.

Delete the four platform functions and `writeFileContent`, and update the listed tests' mock targets.

- [ ] **Step 8: Verify.**
Go:
- `cd api && go test -tags noEmbed ./internal/engine/fs/... ./internal/app/usecases/file/... ./internal/api/...`
- `go test -tags 'integration noEmbed' -run 'TestFilesSuite|TestFiles_|TestRegression_HostDocument|TestGit|TestReview|TestHome' ./tests/... ./tests/integration/files/... ./tests/integration/lifecycle/...`
- `make lint`, `make deadcode`.

Web:
- `bun tsc --noEmit`
- `node_modules/.bin/vitest run` on the 12 test files listed above
- `bun run knip`

Live (`make dev-desktop`): open, edit, save and reload a workspace file; upload a file through the explorer.

- [ ] **Step 9: Commit.**

```bash
git commit -m "feat(api): one document API, host files only over the local socket

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- Over unix, a host file reads and writes.
- Over TCP the same request is a 403 with the code, and the file is untouched.
- An absolute path inside a worktree, including through a symlink, resolves to the workspace document.
- No client or Go test references `/files/content`.

---

## Task 2: One Monaco environment per window

**Goal:** A single lazily-loaded `EditorHost` (manager + model registry) serves every pane, whatever workspace a buffer belongs to. The per-workspace handles, the eviction veto and the ambient-workspace fallback go.

**Files:**
- Create: `web/src/features/editor/host/editor-host.ts`, `web/src/features/editor/host/use-editor-host.ts`
- Create tests: `web/src/__tests__/features/editor/host/editor-host.test.ts`, `web/src/__tests__/features/panes/components/editor-pane.test.tsx`. The latter absorbs the surface-select cases from `editor-pane-surface-select.test.tsx`.
- Modify:
  - `features/workspace/stores/workspace-store.ts`: `WorkspaceStore = StoreApi<WorkspaceState>`.
  - `features/workspace/stores/workspace-store-registry.ts`: delete `canEvictWorkspace` and the editor-model disposal in `destroyWorkspaceStore`.
  - `features/workspace/components/workspace-host.tsx`: delete the veto.
  - `features/panes/components/editor-pane.tsx`.
  - `features/editor/components/editor-surface.tsx`: a `host: EditorHost` prop replaces `workspaceId`.
  - `features/editor/hooks/use-pane-editor-controller.ts`: drop `managerKey`.
  - `features/editor/hooks/use-pane-editor-satellites.ts`: derive `workspaceId` from the bound context's uri.
  - `features/editor/lib/editor-manager.ts`: delete `hasMountedPanes`, `layoutAll`, `disposeAll`.
  - `features/editor/lib/model-registry.ts`: delete `disposeAll`.
  - `features/editor/lib/active-editor-context.ts`: export the window singleton `activeEditors`.
  - `features/panes/stores/slices/pane-actions/editor-tab-actions.ts`.
  - `features/editor/lsp/monaco-lsp-providers.ts` (`isHeld`).
  - `features/editor/lib/reveal.ts` (`registriesFor` → `activeEditors`).
  - `features/panes/hooks/use-chat-workspace-id.ts`: doc comment only; the editorManager rationale is gone.
- Delete: `web/src/__tests__/features/workspace/stores/workspace-store-editor-arm.test.ts`, `web/src/__tests__/features/panes/components/editor-pane-workspace-scope.test.tsx`, `web/src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx`.
- Update tests: `workspace-store-registry.test.ts`, `workspace-host.test.tsx`, `editor-tab-actions.test.ts`, `use-chat-workspace-id.test.ts`, `reveal.test.ts`, `jump-navigation.test.ts`, `editor-manager.test.ts`, `model-registry.test.ts`, `open-file-real-monaco-flow.test.ts` (rewritten onto the host; it stays the real-Monaco guard).
- `web/knip.json`: drop the `workspace-store.ts` `["types"]` entry if knip no longer reports it.

**Interfaces — Produces:**

```ts
// features/editor/host/editor-host.ts
export interface EditorHost { readonly manager: EditorManager; readonly models: ModelRegistry }
export function getEditorHost(): EditorHost | null
export function loadEditorHost(): Promise<EditorHost>   // idempotent; a failed chunk load clears the cached promise
export function subscribeEditorHost(listener: () => void): () => void  // fires once, when the host loads
// features/editor/host/use-editor-host.ts
export function useEditorHost(): EditorHost | null      // triggers loadEditorHost; reports a load failure
// features/editor/lib/active-editor-context.ts
export const activeEditors: ActiveEditorRegistry
```

`BufferMeta.text(uri)` now lives in `editor-host.ts`. It finds the editor buffer whose `fileUri(b.workspaceId, b.path) === uri` across the whole window store. The uri already carries the workspace.

- [ ] **Step 1: Failing tests.** In `editor-host.test.ts`, use real Monaco (`setup.ts` already shims `matchMedia`):

```ts
it('hands every caller the same host', async () => {
  const [a, b] = await Promise.all([loadEditorHost(), loadEditorHost()])
  expect(a).toBe(b); expect(getEditorHost()).toBe(a)
})
it('keeps two workspaces’ files at the same path apart', async () => {
  // open buffers {ws-a, src/x.ts, 'A'} and {ws-b, src/x.ts, 'B'}; mount panes p1, p2 on the host manager;
  // applyActiveBuffer for each → p1 model value 'A', p2 model value 'B'
})
it('a pane keeps its model when the buffer’s workspace store is destroyed', async () => {
  // getOrCreateWorkspaceStore('ws-a'); show ws-a buffer in p1; destroyWorkspaceStore('ws-a')
  // → host.models.get(fileUri('ws-a', 'src/x.ts')) still returns the model and its value
})
it('a pane keeps its model when another pane closes the same file', async () => {
  // same buffer shown in p1 and p2; manager.closeBuffer('p2', uri) → p1's editor model is not disposed and keeps its text
})
```

In `editor-pane.test.tsx`:

```ts
it('renders Monaco for a buffer whose workspace has no store yet')  // ambient WorkspaceStoreContext = other workspace; the surface mounts and host.models holds the buffer's uri with its text
it('routes rich markdown to Plate')        // moved from editor-pane-surface-select.test.tsx
it('routes source-view markdown to Monaco')  // moved
it('shows "File not found" for a missing buffer')
```

- [ ] **Step 2:** Run: `node_modules/.bin/vitest run src/__tests__/features/editor/host src/__tests__/features/panes/components/editor-pane.test.tsx`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement `editor-host.ts`.** Dynamic-import `@/features/editor/lib/monaco-adapters` once (that import keeps `language-contributions`' eager `edcore.main` ahead of any widget). Build `ModelRegistry(realModelApi())` and `new EditorManager(realEditorApi(EDITOR_CREATE_OPTIONS), models, meta)`, store the host, notify subscribers. On a rejected import, reset the cached promise.
  `useEditorHost` = `useSyncExternalStore(subscribeEditorHost, getEditorHost)` plus an effect that calls `loadEditorHost()` while the host is null. A rejection surfaces through the app's error reporter (S3's `report()`), or `toast.error('Editor failed to load', message)` where `report()` does not exist.
- [ ] **Step 4: Rewire.**
  - `EditorPane`: `const host = useEditorHost()`; render `<EditorSurface host={host} …>` when `host && buffer`. Delete `useWorkspaceStoreContext`, `getWorkspaceStore`, `armed`, `armTick` and the long fallback comments. Keep the rich-markdown branch ahead of the host gate.
  - `EditorSurface`: use `host.manager`. Replace `registry` with `activeEditors`. `selectActiveBuffer` returns `{ bufferId, filePath, workspaceId: buffer.workspaceId }`.
  - Satellites: take `workspaceId` from `uriToWorkspaceId(bound uri)`.
  - `editor-tab-actions` uses `getEditorHost()?.manager.closeBuffer(paneId, fileUri(buf.workspaceId, buf.path))`.
  - `monaco-lsp-providers` `isHeld` uses `getEditorHost()?.models.get(uri)`.
  - `reveal.ts` subscribes to `activeEditors` only.
  - Delete everything in the deletion-table row "Per-workspace editor managers".
  - The per-pane `held` set in `EditorManager` stays until T6. Do not release a model on tab switch: a→b→a would recreate it and lose undo.
- [ ] **Step 5:** Run the new tests plus every updated test file listed above. `bun tsc --noEmit`, `bunx eslint src/features/editor src/features/panes src/features/workspace --max-warnings 0`, `bun run knip`, `bun run lint:doctor`.
- [ ] **Step 6: Live (`make dev-desktop`).** Run `caffeinate -u -t 900` and have the window foregrounded. Check that:
  - Files of two different workspaces show in a split.
  - Switch the active workspace away and back: neither pane blanks.
  - A tab switch keeps cursor and scroll.
  - Resize a pane while typing: no snap, layer promotion still active (`data-pane-resizing` set mid-drag).
- [ ] **Step 7:** Commit with `refactor(editor): one Monaco host per window instead of per workspace store`.

**Acceptance:** `grep -rn "armEditor\|editorManager\|modelRegistry\|activeEditorRegistry\|canEvictWorkspace\|hasMountedPanes" web/src` finds only the host module. The four host tests and four EditorPane tests pass.

---

## Task 3: Host documents in the editor

**Goal:** A buffer's identity is a `DocumentRef`. One `openDocument` serves the explorer, git panel, breadcrumbs, previews, jump list, LSP opener, terminal links, Finder drops and a new Open File… dialog. Files outside every workspace open, edit and save.

**Files:**
- Create: `web/src/features/editor/documents/document-ref.ts`, `web/src/features/editor/documents/open-document.ts`
- Create tests: `web/src/__tests__/features/editor/documents/document-ref.test.ts` (absorbs `editor-uri.test.ts`), `web/src/__tests__/features/editor/documents/open-document.test.ts` (absorbs `open-file-content.test.ts`)
- Delete: `features/editor/lib/editor-uri.ts` + its test; `features/workspace/lib/open-file-content.ts` + `__tests__/features/workspace/open-file-content.test.ts`; `features/file-system/utils/file-system-dropped-paths.ts` + its test.
- Modify:
  - `features/panes/types/pane-content.ts`: `EditorContent.authority`; the editor variant of `OpenEditorTabSpec` takes `doc: DocumentRef`; `ClosedBuffer.authority`.
  - `features/panes/stores/slices/buffer-slice.ts`: dedup via `sameDocument`; `workspaceId = doc.workspaceId ?? ''`; no ambient fallback for editors; reopen through `openDocument`.
  - `features/panes/utils/persisted-layout.ts` + test: legacy editor buffers and closed entries without `authority` become `'workspace'`.
  - `features/files/lib/file-tree-handlers.ts`: delete `openWorkspaceFile`.
  - `features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts`
  - `features/git/components/git-history-list.tsx`
  - `features/editor/markdown/markdown-preview.tsx`
  - `features/editor/components/toolbar/{breadcrumb.tsx,file-path-breadcrumb.tsx,breadcrumb-directory.ts}`: list via `fetchFileTree(doc.workspaceId, path)`; host docs show plain segments.
  - `features/panes/hooks/use-pane-drop-handlers.ts`: Tauri drop → `openDocumentsAtPaths`; drop the DOM file-drop branch.
  - `features/terminal/hooks/use-xterm-instance.ts`
  - `features/terminal/lib/terminal-file-links.ts` + test: delete `workspaceRelativePath`.
  - `features/editor/lib/reveal.ts`: `RevealTarget.doc`.
  - `features/editor/stores/jump-list-store.ts`, `features/editor/utils/jump-navigation.ts`: entries carry `doc`; delete the "recorded in another workspace" refusal.
  - `features/editor/lsp/lsp-query.ts`, `features/editor/lsp/monaco-lsp-providers.ts`: `refFromUri`; extract the peek-model helpers into `features/editor/lsp/peek-models.ts`, which brings the provider file under 500 lines.
  - `features/editor/hooks/use-pane-editor-controller.ts`, `features/editor/lib/pane-editor-controller.ts`, `features/editor/host/editor-host.ts`, `features/panes/stores/slices/pane-actions/editor-tab-actions.ts`: `documentUri`.
  - `features/editor/hooks/use-inline-blame.ts`, `features/editor/hooks/use-lsp-document-sync.ts`: skip host docs.
  - `features/editor/markdown/plate/{markdown-asset.ts,markdown-editor-pane.tsx}`: `MarkdownAssetInfo = { doc: DocumentRef }`; resolve via `siblingRef`; no ambient `WorkspaceStoreContext`.
  - `features/editor/components/html/html-preview.tsx`
  - `features/file-explorer/file-explorer/components/file-explorer-tree.tsx`, `features/file-explorer/file-explorer/hooks/use-file-explorer-visible-rows.ts`: ignore host buffers for active-path reveal.
  - `features/keymaps/registry.ts`: `TAB_OPEN_FILE`.
  - `features/panes/hooks/use-pane-keyboard.ts`
  - `features/tabs/components/tab-bar.tsx`: untitled via `openUntitledDocument`; "Open File…" item in `TabAddButton`, desktop only.
  - Callers of the old untitled spec in `use-pane-keyboard.ts`.

**Interfaces — Consumes:** `readDocument`, `DocumentRef`, `DocumentTarget`, `HOST_DOCUMENT_TRANSPORT_CODE` (T1); `getEditorHost`, `activeEditors` (T2). **Produces:**

```ts
// documents/document-ref.ts
export function documentUri(ref: DocumentRef): string   // workspace: 'crowbar://editor/<enc ws>/<enc path>' (unchanged); host: 'crowbar://host/<enc abs>'
export function refFromUri(uri: string): DocumentRef | null
export function documentRefOf(buffer: Pick<EditorContent, 'authority' | 'workspaceId' | 'path'>): DocumentRef
export function siblingRef(ref: DocumentRef, relativePath: string): DocumentRef  // resolves ./ and ../ against ref's directory
export function sameDocument(a: DocumentRef, b: DocumentRef): boolean
export function documentName(ref: DocumentRef): string
// documents/open-document.ts
export interface OpenDocumentOptions { preview?: boolean; paneId?: string }
export function findDocumentBuffer(ref: DocumentRef): EditorContent | null
export function openDocument(target: DocumentTarget, opts?: OpenDocumentOptions): Promise<string | null>  // buffer id; null after reporting a failure
export function openDocumentsAtPaths(paths: readonly string[], paneId: string): Promise<void>  // sequential, drop order
export function openDocumentFromDialog(paneId: string): Promise<void>  // openNativeDialog({ multiple: true, directory: false })
export function openUntitledDocument(workspaceId: string, paneId: string): Promise<string>  // async so T6 can open its model first
// reveal.ts
export interface RevealTarget { doc: DocumentTarget; position?: { line: number; character: number }; beforeShow?: (bufferId: string) => void }
// keymaps/registry.ts
export const TAB_OPEN_FILE = 'tabs.openFile'  // label 'Open file…', category 'Tabs', defaultChord 'mod+o', liveEditable true
```

Failure copy for `openDocument`, routed through `report()`/toast:

| Failure | Title | Message |
|---|---|---|
| 403 + `HOST_DOCUMENT_TRANSPORT_CODE` | "Can't open this file here" | "Files outside your workspaces open only in the Crowbar desktop app." |
| 404 | "File not found" | `<path>` |
| 413 | "File too large to open" | `<name>` |
| anything else | "Failed to open file" | `<name>` |

- [ ] **Step 1: Failing tests.** `open-document.test.ts` mocks `@/lib/api/documents`, `@/lib/native-dialog` and `@/lib/crowbar-bridge` (`isTauri` → true): 3 mocks.

```ts
it('opens a file outside every workspace as a host document')     // readDocument → {ref:{authority:'host',path:'/tmp/s5/notes.md'},text:'hi'}; buffer authority 'host', path '/tmp/s5/notes.md', workspaceId ''
it('the same file through an absolute path reuses the workspace buffer')  // open {authority:'workspace',workspaceId:'w1',path:'src/a.ts'}, then {absolutePath:'/wt/src/a.ts'} resolving to the same ref → same buffer id, one buffer
it('a refused host read says why and opens nothing')               // readDocument rejects new ApiError('…',403,HOST_DOCUMENT_TRANSPORT_CODE) → no buffer; toast title "Can't open this file here"
it('opens every dropped path, in drop order')                      // openDocumentsAtPaths(['/a/1.md','/a/2.md'],'p1') → pane p1 editorTabIds in that order
it('Open File opens each path the dialog returns')                 // openNativeDialog → ['/x/a.ts','/x/b.ts'] → two buffers; null → none
```

In `document-ref.test.ts`:
- `documentUri`/`refFromUri` round-trip for both authorities, including spaces and unicode.
- Workspace URIs equal the old `fileUri` output for the same input.
- `siblingRef({authority:'host',path:'/a/b/c.md'}, '../img/x.png')` returns `{authority:'host',path:'/a/img/x.png'}`; the workspace form stays relative.

`persisted-layout.test.ts`: a legacy editor buffer without `authority` loads as `'workspace'`.

- [ ] **Step 2:** Run these three files. Expected: FAIL.
- [ ] **Step 3: Implement the documents modules.**
  - `document-ref.ts`.
  - `openDocument`:
    1. A `DocumentRef` target with an existing buffer just shows it, through the pane that holds it, or else is added to `opts.paneId`.
    2. Otherwise `readDocument`. Re-check `findDocumentBuffer(read.ref)`, because a concurrent open may have created it.
    3. `openContent({ type:'editor', doc: read.ref, name: documentName(read.ref), content: read.text, isPreview: opts.preview }, { paneId })`.
  - `revealInEditor` = `openDocument` + the existing `editorReadyFor` (keyed by the document uri) + `placeCursor`.
- [ ] **Step 4: Switch every caller** listed under Files.
  - Terminal: `openFile: (abs, line) => void revealInEditor({ doc: { absolutePath: abs }, position: line ? { line: line - 1, character: 0 } : undefined })`. The "outside the current workspace" toast and `getActiveWorkspaceId` go.
  - Pane drop: the Tauri handler → `openDocumentsAtPaths(paths, paneId)`. The DOM `handleDrop` keeps only the tab-data early return. Delete `extractDroppedFilePaths`.
  - Explorer, git history, breadcrumb and preview links use `openDocument({ authority:'workspace', workspaceId, path }, …)` with the workspace they already hold. The breadcrumb's is `doc.workspaceId`, not `getActiveWorkspaceId()`.
- [ ] **Step 5: Open File command.**
  - Registry entry `TAB_OPEN_FILE`.
  - `use-pane-keyboard` handles it next to `TAB_NEW_FILE`, only when `isTauri()`: `void openDocumentFromDialog(activePaneId)`.
  - The `TabAddButton` menu gains "Open File…" (phosphor `FolderOpen`) when `isTauri()`.
  - Registry tests that assert chords and categories cover it; extend `use-pane-keyboard.test.ts` with "mod+o opens the dialog on desktop and does nothing in the browser".
- [ ] **Step 6: Host-doc behaviour.**
  - `lsp-query` and every provider return null for a non-workspace ref.
  - `useLspDocumentSync` and `useInlineBlame` no-op for host refs.
  - The breadcrumb renders host paths as non-interactive segments.
  - Explorer reveal ignores host buffers.
- [ ] **Step 7:** Open a GitHub-style issue note in the commit body ("host documents have no external-change watch") and ask the user to file it. No code for it.
- [ ] **Step 8: Verify.** Run the new tests plus `terminal-file-links.test.ts`, `persisted-layout.test.ts`, `reveal.test.ts`, `jump-navigation.test.ts`, `monaco-lsp-providers.test.ts`, `markdown-asset.test.ts`, `file-tree-handlers.test.ts`, `use-file-explorer-drag-drop.test.ts`, `use-pane-keyboard.test.ts`, `editor-tab-actions.test.ts`. Then `bun tsc --noEmit`, eslint on touched dirs, `bun run knip`, `bun run lint:doctor`.
  Live (`make dev-desktop`, user at the keyboard for the drop and the link):
  - Cmd+O picks `/tmp/…/host-probe.ts`: opens, edits, Cmd+S, `cat` shows the edit.
  - Drag the same file from Finder onto a pane.
  - In a terminal `echo /tmp/…/host-probe.ts`, click the link.
  - Open a file of the active workspace via its absolute path from the terminal: the existing tab activates, no duplicate.
  - Markdown with a relative image outside a workspace renders the image.
- [ ] **Step 9:** Commit with `feat(editor): open any host file through the document API`.

**Acceptance:** §7 row 1 works through dialog, drop and terminal link; no `getActiveWorkspaceId()` remains in any editor open path; a refused host read is visible.

---

## Task 4: Slim grammar bundle

**Goal:** Monaco's semantic tokens and Plate's code blocks share one highlighter built from `shiki/core` + the JavaScript regex engine + per-language grammar imports. Neither module imports `shiki/bundle/full`.

**Files:**
- Create: `web/src/lib/highlight/shiki-grammars.ts`; test `web/src/__tests__/lib/highlight/shiki-grammars.test.ts`
- Modify: `web/src/features/editor/monaco/shiki-tokens.ts`. Delete `loadBundle`, `getHighlighter`, `warmLanguage`, `languageStates`, `SHIKI_LANG_BY_LANGUAGE_ID` (it moves), and the bundle reset in `__resetShikiTokensForTests`. Model rule-stack state stays.
- Modify: `web/src/components/editor/plugins/shiki-lowlight.ts`. Delete its own `languages` map, `highlighter` and `loadLanguage`.
- Modify tests: `shiki-tokens.test.ts` (mock target `shiki/bundle/full` → `@/lib/highlight/shiki-grammars`), `shiki-lowlight.test.ts`, `semantic-tokens-provider.test.ts` if it reaches the bundle.
- `web/knip.json`: shrink the `shiki-tokens.ts` entry if fixed.

**Interfaces — Produces:**

```ts
export interface ShikiGrammar {
  tokenizeLine(line: string, prev: StateStack | null, timeLimit?: number): {
    readonly tokens: readonly { readonly startIndex: number; readonly endIndex: number; readonly scopes: string[] }[]
    readonly ruleStack: StateStack
  }
}
export type GrammarState = 'loading' | 'ready' | 'failed'
export function grammarFor(languageId: string): ShikiGrammar | null  // ready grammar or null; starts the load on first ask
export function grammarStateOf(languageId: string): GrammarState | undefined
export function onGrammarReady(listener: (languageId: string) => void): () => void
```

`languageId` is a Crowbar language id. The module maps `javascriptreact→jsx`, `typescriptreact→tsx`, `embedded_template→erb`, `angular→angular-html` and `restructuredtext→rst`, then shiki's `bundledLanguagesAlias` (`sh`, `ts`, …).

- [ ] **Step 1: Failing test.**

```ts
it('loads a grammar on first ask and announces it')  // grammarFor('typescript') null; onGrammarReady fires 'typescript'; tokenizeLine('function probe() {}', null) has a scope starting 'entity.name.function'
it('maps crowbar ids and shiki aliases')  // 'typescriptreact' and 'sh' reach ready
it('stays failed for a language shiki has no grammar for')  // 'gitignore' → 'failed', grammarFor null, never retried
it('tokenizes the editor languages like the oniguruma engine')
// for [typescript, tsx, javascript, python, go, rust, json, yaml, markdown, shellscript, css, html] × a 10-line fixture each:
// the test builds its own createHighlighterCore({ engine: createOnigurumaEngine(import('shiki/wasm')), langs:[same import], themes:[github-dark] })
// and asserts per-token scopes are equal line by line
```

- [ ] **Step 2:** Run it. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - `createHighlighterCore({ themes: [import('shiki/themes/github-dark.mjs')], langs: [], engine: createJavaScriptRegexEngine({ forgiving: true }) })` from `shiki/core` and `shiki/engine/javascript`.
  - Language loaders come from `shiki/langs` (`bundledLanguages`, `bundledLanguagesAlias`): a 25 KB index of per-language dynamic imports.
  - Each language moves `loading → ready | failed` exactly once.
  - `shiki-tokens` asks `grammarFor`/`onGrammarReady` instead of warming its own. `shiki-lowlight` does the same, and `code-block-node` subscribes to `onGrammarReady` directly.
  - The one-off compile deferral in `shikiTokensInRange` (`compiledLanguages`) stays where it is.
- [ ] **Step 4:** Run the new test, `shiki-tokens.test.ts`, `shiki-lowlight.test.ts` and `semantic-tokens-provider.test.ts`; then `bun tsc --noEmit`, eslint and knip. Check `grep -rn "bundle/full" web/src` is empty.
- [ ] **Step 5: Live.**
  - `make dev-desktop`: measure O2 on 5 files.
  - `make dev-bundle`: measure O4.
  - Chat transcript code fences in an exotic language (e.g. `haskell`) still colour.
  - If O2 or O4-colored is over budget while the oniguruma engine meets it, swap the engine line (Ruling 9), re-measure, and say which engine shipped in the commit body.
- [ ] **Step 6:** Commit with `perf(editor): one slim shiki highlighter for Monaco and Plate`.

**Acceptance:** One highlighter instance; parity test green; no `bundle/full` import; O2 within budget.

---

## Task 5: Document text seam

**Goal:** Every reader and writer of document text outside the Monaco surface goes through one module whose contract is the final one. T6 then changes only its backing, from store content to Monaco models, without touching consumers.

**Files:**
- Create: `web/src/features/editor/documents/document-text.ts`, `web/src/features/editor/documents/use-document-text.ts`
- Create tests: `web/src/__tests__/features/editor/documents/document-text.test.ts`, `web/src/__tests__/features/editor/documents/use-document-text.test.tsx`
- Modify consumers:
  - `features/editor/lib/buffer-save.ts`: read text, format via `applyExternalText`, `markDocumentSaved`.
  - `features/editor/lsp/format-buffer.ts`: `formatDocumentWithLsp(ref, text): Promise<string | null>`.
  - `features/editor/lib/reload-buffer.ts`
  - `features/workspace/lib/external-buffer-sync.ts`
  - `lib/persistence/hydrate.ts` (reconcile)
  - `features/editor/lsp/workspace-edit.ts`: open buffer → `replaceDocumentText`.
  - `features/editor/markdown/markdown-preview.tsx`, `features/editor/components/html/html-preview.tsx`: `useDocumentText(sourceRef) ?? snapshot`.
  - `features/editor/components/toolbar/breadcrumb.tsx`: preview snapshot via `documentText`.
  - `features/editor/components/toolbar/lsp-status-menu.tsx`
  - `features/editor/lsp/peek-models.ts`
  - `features/editor/markdown/plate/markdown-editor-pane.tsx`: initial text, flush, external resync, pending writer.
- Modify tests: `buffer-save.test.ts`, `reload-buffer.test.ts`, `external-buffer-sync.test.ts`, `markdown-editor-pane.test.tsx`, `markdown-editor-pane-scroll.test.tsx`, `hydrate.test.ts` (only where they poke `content` directly).

**Interfaces — Produces** (the final contract; T6 keeps it):

```ts
export function documentText(ref: DocumentRef): string | null                 // includes edits a pending writer holds
export function readDocumentText(ref: DocumentRef): Promise<string>           // waits until the document is open, then documentText
export function replaceDocumentText(ref: DocumentRef, text: string): void     // a user edit: undoable, may mark dirty
export function applyExternalText(ref: DocumentRef, text: string, opts: { saved: boolean }): void  // reload, external change, format result
export function markDocumentSaved(ref: DocumentRef, text: string): void       // clean iff the document still holds exactly `text`
export function savedDocumentText(ref: DocumentRef): string | null            // what disk held at the last load/save
export function onExternalText(ref: DocumentRef, listener: () => void): () => void  // applyExternalText only
export function registerPendingWriter(ref: DocumentRef, flush: () => void): () => void
// use-document-text.ts
export function useDocumentText(ref: DocumentRef | null): string | null       // re-renders at most every 150 ms while text changes
```

T5 backing:
- Text and saved text come from the buffer's `content`/`savedContent`.
- `documentText` first dispatches the existing `flush-editor-content` event (so the Monaco sink lands) and runs the ref's pending writers.
- `replaceDocumentText` calls `setBufferContent`.
- `applyExternalText` patches `content` (and `savedContent`, `isDirty:false`, `hasExternalChange:false`, `fileMissing:false` when `saved`) and notifies `onExternalText` listeners.
- `useDocumentText` selects the buffer's content.

- [ ] **Step 1: Failing tests.** `document-text.test.ts` opens documents with `openDocument`, mocking `@/lib/api/documents`. Write it backing-agnostic, because it must pass unchanged in T6.

```ts
it('returns the text of an open document and null for one that is not open')
it('a user replace marks the buffer dirty; replacing back to the saved text marks it clean')
it('external text applied as saved leaves the buffer clean and notifies only external listeners')
it('markDocumentSaved cleans the buffer only if the text still matches')
it('readDocumentText includes edits a pending writer has not flushed yet')
```

`use-document-text.test.tsx` (fake timers): after `replaceDocumentText`, the hook's value updates within 150 ms; 20 replacements inside 150 ms render at most twice.

- [ ] **Step 2:** Run them. Expected: FAIL (module missing).
- [ ] **Step 3:** Implement on the store backing described above.
- [ ] **Step 4: Switch consumers.** Behaviour is unchanged except where noted:
  - Plate: initial value from `documentText(ref)`. Flush → `replaceDocumentText`. The `content` prop effect becomes an `onExternalText` subscription that keeps "never clobber unsaved local edits". The `flush-editor-content` window listener becomes `registerPendingWriter(ref, flush)`.
  - `buffer-save`: `const written = await readDocumentText(ref)`. Format applies with `applyExternalText(ref, formatted, { saved: false })` only if `documentText(ref) === written`. Then `markDocumentSaved(ref, written)`.
  - Only `buffer-save`, `reload-buffer` and `lsp-status-menu` dispatched the window event; after this step none of them does, because the seam does it.
- [ ] **Step 5:** Run the two new tests plus every modified test file; then `bun tsc --noEmit`, eslint and knip. `grep -rn "\.content\b" web/src/features/editor web/src/lib/persistence web/src/features/workspace/lib` may only hit `document-text.ts`, `use-document-text.ts`, `editor-surface.tsx`'s sink, and preview snapshots (`markdownPreview`/`htmlPreview` `content`).
- [ ] **Step 6: Live.** In rich markdown: type, Cmd+S immediately, `cat` shows the text. Reload from disk on a clean and on a dirty buffer. Format-on-save on a TS file. Markdown preview updates while typing.
- [ ] **Step 7:** Commit with `refactor(editor): route document text through one seam`.

**Acceptance:** Only the seam and the Monaco sink touch buffer `content`; all listed tests green.

---

## Task 6: Monaco models own document text

**Goal:** Implement §3 "Monaco owns buffer text" end to end.
- The model is created when a document opens and released with its buffer.
- The buffer store holds identity, dirty flag and view state; dirty is written only on transitions.
- Unsaved text persists as drafts.
- The content sink, the apply latch and the store→model reconcile are deleted.

**Files:**
- Create: `web/src/features/editor/documents/document-registry.ts` (replaces `features/editor/lib/model-registry.ts`), `draft-store.ts`, `restore-documents.ts`, `use-document-open.ts` (all in `features/editor/documents/`).
- Create tests: `web/src/__tests__/features/editor/documents/{document-registry,draft-store,restore-documents}.test.ts`
- Delete: `features/editor/lib/model-registry.ts` + test; `features/editor/lib/content-sink.ts` + test.
- Modify:
  - `features/editor/documents/document-text.ts`: new backing.
  - `features/editor/documents/use-document-text.ts`: registry subscription through `createRafCoalescer` with `minIntervalMs: 150`.
  - `features/editor/documents/open-document.ts`: open the model before the buffer; `openUntitledDocument` opens an empty model.
  - `features/editor/host/editor-host.ts`: `EditorHost = { manager, documents }`; wires the registry listener.
  - `features/editor/lib/editor-manager.ts`: `showBuffer` takes the model from the registry and returns `false` when it is not open; delete `held`, `acquire` and `applyExternalEdit`.
  - `features/editor/lib/monaco-adapters.ts`: `IModelLike` gains `getAlternativeVersionId`, `getValueLength`, `onDidChangeContent` and `replaceAll(text, preserveSelections)`; `BufferMeta` goes.
  - `features/editor/hooks/use-pane-editor-controller.ts`: no sink; `applySwitch` also runs on `documents.onDidOpen`.
  - `features/editor/hooks/use-pane-editor-satellites.ts`: delete the reconcile effect and `externalApplyRef`.
  - `features/editor/components/editor-surface.tsx`: delete the write seam and `onPromote`/`isPreview`/`isActiveSurface` refs used only by it.
  - `features/editor/lib/pane-editor-controller.ts`: delete `shouldReconcileModelFromStore`.
  - `features/editor/lib/buffer-save.ts`: delete `setBufferContent` and the `settings://` branch; add `setDocumentDirty(uri, dirty)` and `noteDocumentEdited(uri)`; autosave timers keyed by uri.
  - `features/panes/types/pane-content.ts`: drop `content`, `savedContent`, `tokens` and `TokenEntry` from `EditorContent`; the editor open spec drops `content`.
  - `features/panes/stores/slices/buffer-slice.ts`: add `rebindEditorBuffer`.
  - `features/panes/lib/buffer-release.ts`: release the document and clear its draft.
  - `features/panes/components/editor-pane.tsx`: rich markdown waits for `useDocumentOpen`.
  - `features/editor/markdown/plate/markdown-editor-pane.tsx`: pending writer through the registry.
  - `features/workspace/lib/external-buffer-sync.ts`: gains `reconcileWorkspaceDocuments(wsId)`, moved from hydrate.
  - `features/workspace/stores/hooks/use-workspace-file-tree.ts`
  - `lib/persistence/hydrate.ts`: delete `reconcileRestoredBuffers`, `restoreBufferDirtyState`, `setBufferFileMissing` and the `editor-state` read. If `hydrateWorkspace` then does nothing, delete it and its `hydrated` gate in `workspace-view.tsx` after checking with grep that nothing else feeds that gate. The upgrade calls `splitLegacyEditorContent`; hydrate calls `restoreDocuments()` after `setState`.
  - `features/panes/utils/persisted-layout.ts` + test: `splitLegacyEditorContent`.
  - `lib/persistence/{idb.ts,schemas.ts,workspace-layout.ts}`: the next DB version adds `editor-drafts` (keyPath `uri`) and deletes `editor-state`; `WINDOW_LAYOUT_VERSION` + 1; `EditorState` type goes.
  - `features/panes/stores/window-pane-store.ts`: the persist comparison is unchanged, but buffers no longer carry text.
  - `features/editor/lsp/peek-models.ts`: adoption moves into `DocumentRegistry.open`.
  - `web/eslint.config.js`: delete `use-pane-editor-controller.ts` from its baseline if the directive is gone.
- Update tests: `editor-manager.test.ts`, `monaco-adapters.test.ts`, `use-pane-editor-controller.test.ts`, `pane-editor-controller.test.ts`, `buffer-save.test.ts`, `reload-buffer.test.ts`, `external-buffer-sync.test.ts`, `hydrate.test.ts` (reconcile cases move to `restore-documents.test.ts`; the file shrinks), `idb-schema.test.ts`, `workspace-layout.test.ts`, `markdown-editor-pane*.test.tsx`, `editor-host.test.ts`, `editor-pane.test.tsx`, `open-file-real-monaco-flow.test.ts`, `document-text.test.ts` (unchanged and must pass).

**Interfaces — Consumes:** the T5 seam contract (unchanged), `documentUri`/`documentRefOf` (T3), `loadEditorHost`/`getEditorHost` (T2). **Produces:**

```ts
// document-registry.ts
export interface DocumentListener {
  dirtyChanged(uri: string, dirty: boolean): void
  contentChanged(uri: string): void
  opened(uri: string, ref: DocumentRef): void   // T7 attaches LSP here
  released(uri: string): void
}
export class DocumentRegistry {
  constructor(api: MonacoModelApi, listener: DocumentListener)
  open(ref: DocumentRef, languageId: string, text: string, savedText?: string): IModelLike  // idempotent per uri; adopts an existing peek model, overwriting its text
  release(uri: string): void          // dispose the model; listener.released
  has(uri: string): boolean
  get(uri: string): IModelLike | null
  text(uri: string): string | null    // runs pending writers first
  savedText(uri: string): string | null
  isDirty(uri: string): boolean
  markSaved(uri: string, text: string): void
  replace(uri: string, text: string): void                         // undoable full replace, user origin
  applyExternal(uri: string, text: string, saved: boolean): void   // undoable, keeps every showing editor's selections, fires onDidApplyExternal
  registerPendingWriter(uri: string, flush: () => void): () => void
  onDidOpen(listener: (uri: string) => void): () => void
  onDidApplyExternal(uri: string, listener: () => void): () => void
}
// draft-store.ts
export interface EditorDraft { uri: string; text: string; savedText: string; updatedAt: number }
export const DRAFT_PERSIST_DELAY_MS = 1000
export function noteDraftChanged(uri: string): void
export function clearDraft(uri: string): void
export function flushDrafts(): Promise<void>         // also bound to window 'pagehide'
export function loadDrafts(): Promise<Map<string, EditorDraft>>
export function putDrafts(drafts: readonly EditorDraft[]): Promise<void>
// restore-documents.ts
export function restoreDocuments(): Promise<void>
// use-document-open.ts
export function useDocumentOpen(uri: string | null): boolean
// persisted-layout.ts
export function splitLegacyEditorContent(layout: WorkspaceLayout): { layout: WorkspaceLayout; drafts: EditorDraft[] }
// buffer-slice BufferActions
rebindEditorBuffer(id: string, doc: DocumentRef, name: string): void   // Save As of an untitled buffer
```

Dirty rule (Ruling 6), evaluated in the registry's `onDidChangeContent` handler:

```ts
const dirty = model.getAlternativeVersionId() !== doc.savedVersionId &&
  !(model.getValueLength() === doc.savedText.length && model.getValue() === doc.savedText)
if (dirty !== doc.dirty) { doc.dirty = dirty; listener.dirtyChanged(uri, dirty) }
listener.contentChanged(uri)
```

`markSaved(uri, text)` sets `savedText = text`. It sets `savedVersionId = model.getAlternativeVersionId()` when `model.getValue() === text`, else `-1`, then recomputes `dirty`.

The host wires the listener:
- `dirtyChanged` → `setDocumentDirty`: patch `isDirty` on the buffer with that uri; on the first `true`, promote a preview buffer.
- `contentChanged` → `noteDocumentEdited`:
  - autosave if dirty, `settings.autoSave` and `canAutosave`;
  - `noteDraftChanged(uri)` if dirty;
  - `clearDraft(uri)` if clean.

- [ ] **Step 1: Failing tests.**

`document-registry.test.ts` (real Monaco through `realModelApi`; the listener is a recording fake):

```ts
it('a burst of edits reports one dirty transition and no other store write')  // 50 model.applyEdits → dirtyChanged called once (true); windowPaneStore subscriber fires once
it('undo back to the saved version marks the document clean')
it('typing a character and deleting it leaves the document clean')
it('a save marks clean only if the model still holds what was written')  // markSaved(uri,'v1') after an edit to 'v2' → still dirty
it('applyExternal keeps the caret of every editor showing the model')  // two monaco.editor.create on the same model; carets restored (clamped)
it('a document lives as long as its buffer, not its pane')  // closing the tab in one of two panes keeps the model; closing the buffer releases it
```

`draft-store.test.ts` (fake timers, fake-indexeddb):

```ts
it('a dirty document is written once after the quiet period, not per keystroke')  // 30 noteDraftChanged within 900 ms → 0 puts; +1000 ms → 1 put with the model's current text
it('pagehide flushes pending drafts')
it('clearing a draft removes it')
```

`restore-documents.test.ts` (mocks `@/lib/api/documents`):

```ts
it('restores a dirty draft as a dirty document')  // layout buffer + draft {text:'mine',savedText:'disk'}; readDocument → 'disk' → model 'mine', isDirty true, no toast
it('flags a dirty draft whose file changed on disk')  // readDocument → 'other' → hasExternalChange true + toast
it('opens a clean buffer from disk')  // no draft → model = disk text, isDirty false
it('marks a clean buffer whose file is gone as missing')  // 404 → fileMissing true, no model
it('restores an untitled buffer from its draft')  // virtual buffer + draft {text:'scratch',savedText:''} → model 'scratch', dirty; no disk read
```

`persisted-layout.test.ts`:

```ts
it('moves legacy unsaved buffer text into drafts')  // {content:'a',savedContent:'b'} → draft {text:'a',savedText:'b'}; buffer has no content/savedContent/tokens
it('drops clean legacy buffer text without a draft')
```

- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Registry and host.** Implement `DocumentRegistry` (≤ 250 lines) and the adapter additions:
  - `replaceAll(text, preserveSelections)`:
    1. collect `editor.getSelections()` for every `monaco.editor.getEditors()` whose model is this one;
    2. `pushStackElement()`;
    3. `pushEditOperations(null, [{ range: fullRange, text }], () => null)`;
    4. `pushStackElement()`;
    5. restore the selections.
  - In `editor-host.ts`, `EditorHost` holds `documents`; delete `models`. `EditorManager(editorApi, documents)`: `showBuffer` → `const model = documents.get(uri); if (!model) return false`, then save/restore view state as today. `closeBuffer` only drops that pane's view state.
- [ ] **Step 4: Open, restore, release.**
  - `openDocument`:
    1. `Promise.all([loadEditorHost(), readDocument(target)])`;
    2. re-check the existing buffer;
    3. `documents.open(ref, langForUri(uri), text)`;
    4. `openContent({ type:'editor', doc: ref, name, isPreview }, { paneId })`;
    5. release the document if `openContent` returned `''`.
  - `restoreDocuments`:
    1. `const [host, drafts] = await Promise.all([loadEditorHost(), loadDrafts()])`;
    2. for every editor buffer: `draft = drafts.get(uri)`;
    3. virtual buffer → open with `draft?.text ?? ''`;
    4. otherwise `readDocument(ref)` with the rules the tests pin;
    5. set `isDirty` from the registry.
  - `buffer-release.disposeBuffers` → `getEditorHost()?.documents.release(uri)`, then `clearDraft(uri)`.
  - Save As (untitled):
    1. write to the target;
    2. `documents.open(newRef, lang, text, text)`;
    3. `rebindEditorBuffer(id, newRef, name)`;
    4. release the old uri.

    Panes follow, because the controller sees the new uri.
- [ ] **Step 5: Seam backing.** `document-text.ts` resolves `uri = documentUri(ref)` and calls the host registry. `readDocumentText` awaits `loadEditorHost()` and, if the uri is not open yet, the first `onDidOpen(uri)`. `use-document-text` subscribes to the model's `onDidChangeContent` through `createRafCoalescer(..., { minIntervalMs: () => 150 })`. Delete the `flush-editor-content` event everywhere.
- [ ] **Step 6: Delete** the deletion-table rows "Content sink", "Apply latch" and "Store → model reconcile". The controller keeps only:
  - mount once per pane;
  - narrow `activeEditorTabId` subscription plus `documents.onDidOpen` → `applySwitch`;
  - cursor-sync listeners.
- [ ] **Step 7: Persistence.**
  - IDB upgrade (next version): create `editor-drafts`, delete `editor-state`.
  - `upgradeWindowPaneLayout` applies `splitLegacyEditorContent` when `version < WINDOW_LAYOUT_VERSION`, then `putDrafts`, then saves the layout.
  - Bind `flushDrafts` to `pagehide` once in `draft-store.ts`, at module evaluation guarded by `typeof window`.
- [ ] **Step 8: Verify.** Run every new and updated test file listed above (never the full suite). Then `bun tsc --noEmit`, `bunx eslint src --max-warnings 0`, `bun run knip`, `bun run lint:doctor`, `bunx prettier --check src`. Grep for no remaining occurrences of `ContentSink`, `externalApplyRef`, `setBufferContent`, `flush-editor-content`, `shouldReconcileModelFromStore`, `savedContent` or `.tokens` outside preview snapshot types.
- [ ] **Step 9: Live (`make dev-desktop`, user at the keyboard).**
  - Type in a split of the same file: both panes update; Cmd+Z works from either.
  - Close one pane: the other keeps text and undo.
  - Type then reload the webview: the text returns, dirty.
  - Rich markdown: type, toggle to source and back; reload from disk on a clean and a dirty buffer; the external-change toast still appears; Save As on an untitled buffer; format-on-save.
  - Measure K1–K3 (Appendix A §A.3).
- [ ] **Step 10:** Commit with `refactor(editor): Monaco models own document text`.

**Acceptance:** The keystroke path writes no store state after the dirty flip and no IndexedDB record during a burst (K2); every deletion-table row for T6 is gone; `document-text.test.ts` passes unchanged from T5.

---

## Task 7: LSP and diagnostics attach to the document

**Goal:** One didOpen, didChange stream, didClose and diagnostics subscription per document. Today they are per pane.

**Files:**
- Create: `web/src/features/editor/documents/document-lsp.ts`; test `web/src/__tests__/features/editor/documents/document-lsp.test.ts` (adapted from `use-lsp-document-sync.test.ts`).
- Delete: `features/editor/hooks/use-lsp-document-sync.ts` and `__tests__/features/editor/hooks/use-lsp-document-sync.test.ts`.
- Modify: `features/editor/hooks/use-pane-editor-satellites.ts` (stop calling the hook); `features/editor/host/editor-host.ts` (`opened`/`released` → attach/detach).

**Interfaces — Produces:** `attachDocumentLsp(ref: DocumentRef, model: Monaco.editor.ITextModel): () => void`. It is a no-op disposer for host refs, virtual buffers and project-home workspaces (`isHomeWorkspace`).

- [ ] **Step 1: Failing tests.** Mock `../lsp/lsp-client` (1 mock):

```ts
it('opens a document with the language server once, however many panes show it')
it('sends changes after the idle-deferred open and closes on release')
it('reopens with the new language when the model language changes')
it('never contacts the language server for a host document')
it('puts pushed diagnostics on the model and clears them on release')
```

- [ ] **Step 2:** Run it. Expected: FAIL.
- [ ] **Step 3: Implement.** Move the body of `useLspDocumentSync` into `attachDocumentLsp`: `scheduleIdleTask` didOpen, `scheduleChange` on content change, `onDiagnosticsUpdate` → `setModelMarkers`. Add `model.onDidChangeLanguage` → close and reopen. The host keeps `Map<uri, dispose>` and resolves the Monaco model for `opened(uri, ref)` with `editor.getModel(Uri.parse(uri))`.
- [ ] **Step 4:** Run the new test plus `monaco-lsp-providers.test.ts`, `lsp-client.test.ts` and `editor-host.test.ts`; then tsc, eslint and knip.
- [ ] **Step 5: Live (TS language server available in the dev env).** One file in two panes: diagnostics show in both. Rename across files works. Close both panes: the LSP status menu shows the document closed.
- [ ] **Step 6:** Commit with `refactor(editor): language-server sync per document, not per pane`.

**Acceptance:** `useLspDocumentSync` is gone; one didOpen per document.

---

## Task 8: Gate

**Goal:** Prove the S5 gate. Every budget passes live, §7 rows 1–2 are closed, every animation in the feature map behaves as before, and the static gates are clean.

**Files:** `docs/plans/2026-10-17-s5-editor-host-plan.md` (Appendix B "after" column and the verdict line). Code changes only to fix a failed check, each with its own failing test first.

- [ ] **Step 1:** `df -h`. `make dev-desktop`, `make seed`, fixtures (§A.1), reload the webview once (stale-HMR guard), `caffeinate -u -t 1800`, window foregrounded by the user.
- [ ] **Step 2:** Measure O1–O3 and K1–K3 (user at the keyboard for K and O3), then O4 on `make dev-bundle`. Fill Appendix B "after". Each metric must be within budget and ≤ baseline × 1.1.
- [ ] **Step 3: §7 row 1 checklist** (all on the desktop app):
  - Cmd+O, a Finder drop and a terminal link each open `/tmp/…/host-probe.ts`.
  - Edit and save writes the disk.
  - Reload the app: the host tab restores.
  - Run `make dev-web` + `make dev-api` in a browser and try a host open: the "Can't open this file here" toast appears, and no tab or Open File… item exists.
- [ ] **Step 4: Animation checklist** (live, by eye):
  - Tab close-button fade on hover.
  - Status-bar button colour transitions.
  - Markdown link transition.
  - Caret blink.
  - Find widget slide (Cmd+F).
  - Pane resize stays smooth with no snap.
  - Tab switch shows no blank frame.
- [ ] **Step 5: Static gates.**
  - Web: `cd web && bun tsc --noEmit && bunx eslint . --max-warnings 0 && bun run knip && bunx prettier --check . && bun run lint:doctor && bun run lint:baselines`.
  - Go: `cd api && make lint && make deadcode && make missing-tests`.
  - Targeted tests: every test file created or modified in T0–T7 (`git diff --name-only <T0 parent>..HEAD -- web/src/__tests__ api`), run explicitly by path.
- [ ] **Step 6:** Commit with `docs(plans): record the S5 gate measurements`.

**Acceptance:** Appendix B shows every metric passing; the checklists are ticked in the commit body.

---

## Appendix A: Live measurement procedure

Every number comes from the real app via the Tauri MCP (`mcp__tauri__driver_session`, `webview_execute_js`, `webview_interact`). `webview_execute_js` cannot await: install synchronously, stash results on `window.__s5`, read them back in a second call with `JSON.stringify(window.__s5)`. Do not reload between install and read.

**A.1 Fixtures** (bash; `WT` is the seed's `worktree`):

```bash
for n in $(seq 1 10); do
  { echo "export function s5Probe$n(): number {"; for i in $(seq 1 1998); do echo "  const v$i = $i + 1 // filler $n"; done; echo "  return 0"; echo "}"; } > "$WT/s5-open-$n.ts"
done
{ echo "export function s5Type(): void {"; for i in $(seq 1 4998); do echo "  console.log($i)"; done; echo "}"; } > "$WT/s5-type.ts"
HOSTDIR=$(mktemp -d)/s5-host; mkdir -p "$HOSTDIR"; echo 'export function s5HostProbe(): void {}' > "$HOSTDIR/host-probe.ts"; echo "$HOSTDIR/host-probe.ts"
```

Reload the webview once so the Files tree lists them. Before measuring O1/O2, warm the host and the TS grammar by opening one unrelated `.ts` file.

**A.2 Instrument** (one `webview_execute_js` call):

```js
(() => {
  if (window.__s5) return 'already'
  const s = (window.__s5 = { opens: [], keys: [], longFrames: 0, idb: {}, recording: false, expectHost: false })
  for (const m of ['put', 'add']) {
    const orig = IDBObjectStore.prototype[m]
    IDBObjectStore.prototype[m] = function (...a) { if (s.recording) s.idb[this.name] = (s.idb[this.name] || 0) + 1; return orig.apply(this, a) }
  }
  let last = performance.now()
  const tick = (t) => { if (s.recording && t - last > 33.4) s.longFrames++; last = t; requestAnimationFrame(tick) }
  requestAnimationFrame(tick)
  document.addEventListener('keydown', (e) => {
    if (!s.recording || !e.isTrusted || !(e.target instanceof Element) || !e.target.closest('.monaco-editor')) return
    const t0 = e.timeStamp
    requestAnimationFrame(() => { const ch = new MessageChannel(); ch.port1.onmessage = () => s.keys.push(performance.now() - t0); ch.port2.postMessage(0) })
  }, true)
  const watch = (rec, probe) => {
    let first = null
    const obs = new MutationObserver(() => {
      const span = [...document.querySelectorAll('.monaco-editor .view-line span span')].find((n) => n.textContent === probe)
      if (!span) return
      if (first === null) { first = span.className; requestAnimationFrame(() => { rec.painted = performance.now() - rec.t0 }) }
      else if (span.className !== first) { requestAnimationFrame(() => { rec.colored = performance.now() - rec.t0; obs.disconnect() }) }
    })
    obs.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ['class'] })
  }
  document.addEventListener('pointerdown', (e) => {
    const row = e.target instanceof Element && e.target.closest('.file-tree-row')
    const n = row && row.textContent.match(/s5-open-(\d+)\.ts/)
    if (!n) return
    const rec = { file: n[0], t0: e.timeStamp, painted: null, colored: null }
    s.opens.push(rec); watch(rec, 's5Probe' + n[1])
  }, true)
  window.addEventListener('focus', (e) => {
    if (!s.expectHost) return
    s.expectHost = false
    const rec = { file: 'host', t0: e.timeStamp, painted: null, colored: null }
    s.opens.push(rec); watch(rec, 's5HostProbe')
  }, true)
  return 'installed'
})()
```

**A.3 Gestures**
- **O1/O2:** single-click each `s5-open-<n>.ts` row (user or `webview_interact` click on the row; the open path is the row's JS handler either way). Wait until its record has `colored`, close the tab, next file.
- **O3:** set `window.__s5.expectHost = true`; the user presses Cmd+O and picks `host-probe.ts`. Repeat 5×, closing the tab each time.
- **K1–K3:** open `s5-type.ts`, split right so the same file shows twice, click into line 2,500, set `window.__s5.recording = true`. The user types continuously for ~30 s (≥ 200 keystrokes, no pause ≥ 1 s). Stop, wait 2 s, set `recording = false`, read back.
- **O4** (`make dev-bundle`, fresh launch): install the instrument, then click one `s5-open` row; one sample per launch, 3 launches.

**A.4 Readback:** `JSON.stringify(window.__s5)`. Compute p50/p95 from `opens[].painted`, `opens[].colored` and `keys[]` (drop `keys[0]`). Read `idb['workspace-layout']`, `idb['editor-drafts']` and `longFrames` directly. Reset with `window.__s5.opens.length = 0; window.__s5.keys.length = 0; window.__s5.idb = {}; window.__s5.longFrames = 0`.

## Appendix B: Measurements

| Id | Budget | Baseline (T0) | After (T8) | Pass |
|---|---|---|---|---|
| O1 p50 / p95 | 80 / 120 ms | | | |
| O2 p95 | 400 ms | | | |
| O3 p95 | 150 ms | | | |
| O4 painted / colored | 500 / 1,000 ms | | | |
| K1 p95 / max | 16 / 50 ms | | | |
| K2 layout / drafts during / drafts after | ≤ 1 / 0 / 1 per doc | | | |
| K3 frames > 33 ms | 0 | | | |

Verdict: _filled by T8._
