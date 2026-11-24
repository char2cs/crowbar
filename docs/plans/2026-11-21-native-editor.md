# Native editor: gpui-kit `Editor` in place of the Monaco pane

Status: APPROVED 2026-11-21 with the decisions in section 7. S0, S1 and S2 are being built on `lane-nated`.

## Goal
The shell's code editor becomes the native gpui-kit `Editor` (`gpui_kit::component::input::{Editor, EditorState}`, gpui-kit 0.10.0, Apache-2.0). It replaces Monaco, which today runs inside the `core.editor` web pane (`app/panes/src/entries/editor.tsx`). The review/diff view stays React. Parity is develop's editor behaviour and look (`origin/develop:web/src/features/editor`), not Monaco internals.

Sources read: develop's `web/src/features/editor/**`, `features/settings/config/default-settings.ts`, `features/keymaps/registry.ts`, `lib/api/documents.ts`, `features/workspace/lib/external-buffer-sync.ts`; the shell's `app/desktop/app/src/{core_panes,native_panes,terminal_panes,dirty_panes}.rs`, `shell_state/{open_requested,preview}.rs`, `pane_spec/file_specs.rs`, `app/panes/src/entries/*`; the crates.io sources of `gpui-kit`, `gpui-component` and `gpui-base` 0.10.0 (fetched, not vendored, not compiled); the ED rows of `docs/triage/2026-11-18-side-by-side-audit/13-ed.md` and the clusters in `~/.cache/crowbar-lanes/fail-clusters.md`.

## 1. What gpui-kit 0.10.0 actually is
- `gpui-kit` is a facade. The `Editor` element lives in `gpui-component` (`src/input/editor.rs`). The editing engine lives in `gpui-base` (`src/input`), where `EditorState = InputBaseState<EditorMode>`. All three are Apache-2.0, pin `gpui-pre =0.3.12`, and depend on `ropey =2.0.0-beta.1` and `lsp-types 0.97`.
- `gpui-component` always pulls in `gpui-kit-assets`, `notify`, `markdown`, `rust-i18n` and `similar`.
  - Highlighting is opt-in: the `tree-sitter` feature is incremental and has one grammar feature per language; `syntect` is the alternative.
  - `speech` (cpal) is optional and stays off.
- The engine has no LSP client. It takes host-implemented provider traits: `CompletionProvider`, `HoverProvider`, `DefinitionProvider`, `CodeActionProvider`, `DocumentColorProvider`, `DocumentRangeSemanticTokensProvider`, and a `DiagnosticSet`.
- Test support is `gpui-kit/test-support`: `TestWindowExt` (press, input, click, drag) and `TestInput` (commit and compose text).
- The engine reads its colours from gpui-component's global `Theme` (`cx.theme()`, `highlight_theme`). The styled `Editor` re-applies theme, read-only and disabled on every render, so the element is where they are set.
- Its own limit: `EditorState::new`'s doc says it handles "up to about 50K lines".

## 2. Feature inventory: develop vs gpui-kit
Key: **C** covered, **P** partly covered, **M** missing (needs a shell-side build or a gpui-kit patch). Cites are gpui-base `src/input` (B) and gpui-component `src` (G).

### 2.1 Settings (`use-editor-appearance.ts`, `monaco-adapters.ts`, defaults from `default-settings.ts`)
| Develop | Default | gpui-kit | Plan |
|---|---|---|---|
| `fontFamily`, `fontSize` × editor zoom (`editorZoomLevel`; mod+0 resets) | mono font, code size | C: `Editor` is `Styled`, so `.font_family()` and `.text_size()` win over the theme mono font (G editor.rs:150-176) | Shell side |
| `editorLineHeight`: `ceil(fontSize × 1.4)` px | 1.4 | P: the private `EDITOR_LINE_HEIGHT = 1.5` is applied before `refine_style`, so `.line_height(px)` overrides it; there is no state setter | Set on the element |
| `tabSize`, insert spaces, no indent detection | 2, spaces | C: `set_tab_size(TabSize{tab_size, hard_tabs:false})` | Shell side |
| `wordWrap` on/off | off | C: `set_soft_wrap` (gpui-kit's default is **on**; the shell must turn it off) | Shell side |
| `lineNumbers` on/off | on | C: `set_line_number` | Shell side |
| `showMinimap` (also in the toolbar preferences menu) | off | **M**: no minimap | Dropped (D4): setting and menu item deleted in S7 |
| `renderWhitespace` none/boundary/trailing/all (the settings catalog has 4 values; Monaco also has `selection`) | none | P: only a bool, `set_show_whitespaces` | Patch: a mode enum |
| `renderIndentGuides` + active indent guide | on | P: `set_indent_guides`; no active-guide highlight | Patch (active guide) |
| `semanticHighlighting` (LSP semantic tokens) | on | C: `semantic_tokens_provider` | Slice 6 |
| `highlightOccurrences` (word under the caret, Monaco `occurrencesHighlight`) | off | **M** | Shell: range decorations from the caret word; no patch |
| Selection highlight of other occurrences (`selectionHighlight: true`, ED-029) | on | **M** | Shell: range decorations from the selection |
| `autoCompletion` (quick suggestions, trigger characters) | on | C: `CompletionProvider` + `is_completion_trigger` | Slices 5-6 |
| `parameterHints` (signature help) | on | **M**: no signature-help provider | Patch (popover + trait) |
| `formatOnSave` | off | **M**: no formatting provider, but `apply_lsp_edits` exists | Shell: LSP `formatting` → `apply_lsp_edits` before the write |
| `autoSave` (1 s quiet period, not untitled) | off | n/a | Shell: the document owner |
| `inlineBlame` | (setting) | **M**: no after-line virtual text | ED-098/099 show no inline blame in either app; keep it off and record that |
| Ligatures | Monaco default (off) | M: no font-features API | Nothing to do: develop does not turn them on |
| Theme from app CSS tokens; follows light/dark live; transparent background (ED-020/021) | | P: global `Theme` | Shell: one adapter writes gpui-component's `Theme` from the shell theme (section 3.6) |
| Cursor line style, blinking (ED-036) | | C (`CursorBlink`) | Check the look |
| No scroll beyond the last line; no overview ruler; scrollbars auto (ED-027/028) | | P: overview-ruler decorations are opt-in; scrollbar visibility unknown | Verify on the rig; patch if bars show at rest |
| No editor context menu (ED-053) | | C: `set_context_menu_enabled(false)` | Shell side |
| Bracket-pair colours and matching-bracket boxes: develop draws **none** (ED-030) | | M, which here is correct | Nothing to do |

### 2.2 Editing, find, navigation
| Develop (Monaco) | gpui-kit | Plan |
|---|---|---|
| Undo/redo ctrl+z / ctrl+shift+z (ED-048) | P: Undo/Redo actions only; Linux binds ctrl-y, not ctrl+shift+z; no public `undo()` and no "saved version" | Rebind; a patch exposes the history version (needed for dirty, 3.2) |
| Auto-closing brackets and quotes, auto-indent (ED-047) | C: `set_auto_close`, `LanguageConfig` | Per-language configs |
| Multi-cursor: alt+click, shift+alt+Up/Down (ED-042) | C on Linux | Done |
| Column selection shift+alt+drag (ED-044) | C | Done |
| mod+d add next occurrence; ctrl+shift+l select all occurrences (ED-043) | **M**: `add_selection` exists, but there is no action and no getter for all selections | Shell actions over `add_selection`; a patch adds a selections getter |
| Line ops: alt+Up/Down move, shift+alt+Down copy, ctrl+shift+k delete (ED-045) | **M** | Patch (engine actions) |
| Toggle line comment: develop leaves ctrl+/ to the app (ED-046) | none bound | Nothing to bind; the chord stays with the shell keymap |
| Folding: chevrons on hover, click folds with a … marker (ED-055) | C: gutter fold icons, a placeholder renderer, ranges from the tree-sitter highlighter | Look: use `fold_icon_renderer` with phosphor icons |
| Fold keys ctrl+shift+[ / ] and fold all ctrl+k ctrl+0 (ED-056) | **M**: `DisplayMap::set_folded` is not reachable mutably | Patch (actions) |
| Find widget ctrl+f, floating top-right over the text; "n of m" / "No results"; match case / whole word / regex (alt+c/w/r); in-selection; next/prev Enter, F3; replace ctrl+h, Replace All; Escape (ED-057..064) | P: a built-in `SearchPanel` that docks as a row, matches literal text only (AhoCorasick, case only), labels "1/3" | Shell: an `editorview` find widget drawn like develop's. Matching uses `regex`, highlights use range decorations, and moves use `set_selected_range`. Disable the built-in panel (`set_searchable(false)`). No patch. |
| Toolbar "Find in file" button (ED-058) | n/a | Toolbar opens the shell widget |
| Go to line ctrl+g: quick input "line:col", jumps and centres (ED-065) | P: `set_cursor_position` scrolls to the caret; no centring (`scroll_to` is `pub(crate)`) | Shell quick input; a patch adds a centred reveal |
| Quick outline ctrl+shift+o (document symbols, ED-066) | **M** | Shell picker over the LSP `documentSymbol` route |
| F1 / ctrl+shift+p Monaco palette (ED-054) | none | Dropped (D5) |
| mod+a selects all (ED-041) | C | Must win over the shell keymap in the `Input` context |
| Cursor chip "line:col", 10 Hz while dragging; the chip of a non-active split's buffer (ED-038/039) | C: `cursor_position()`, `InputEvent` | Toolbar chip; no throttle needed (a native redraw is cheap) |
| Jump list (back/forward over positions, `jump-list-store.ts`) | n/a | Shell: per pane, in `editorview` |
| Reveal at a line from a terminal link (`fresh` + `line` in the pane state, ED-003) | C: `set_cursor_position` | Slice 1 |
| IME (marked text) | C: `EntityInputHandler`, composition as one undo unit | Section 6 |

### 2.3 Language and highlighting
- Develop maps a path to a language id (`utils/language-id.ts`, `monaco/language.ts`). Highlighting has two tiers: a Monarch base plus a Shiki TextMate tier over identifiers (`monaco/shiki-tokens.ts`). The language picker in the toolbar overrides the path's language per buffer (ED-075/076). The chip says "Plain Text" when there is none.
- gpui-kit: tree-sitter grammars cover what develop's seeds show: ts, tsx, js, json, md, css, html, rust, go, python, yaml, toml, bash and more.
- **Colours** (ED-018: today Rust and Tauri differ in token colours):
  - Parity means develop's Shiki colours for each token class.
  - That needs a `HighlightTheme` built from the same theme tokens develop's `define-theme.ts` and the Shiki theme use, mapped from tree-sitter capture names.
  - Pixel-exact identity with TextMate scopes is not reachable for every token. Rows will be scored per class (keywords, types, strings, properties, comments).
- Decision D3 asks whether a close mapping is acceptable.

### 2.4 LSP (develop: `features/editor/lsp/*`, the daemon's `/v0/chats/:id/lsp/*` routes through `ownedChatBase(wsId,'lsp')`)
Develop sends the sync verbs `didOpen`, `didChange`, `didSave` and `didClose`, plus `status` and `restart`. It registers these Monaco providers:

| Provider | gpui-kit |
|---|---|
| completion (`*` word-based + server) | C |
| hover | C |
| signatureHelp | **M** (patch) |
| definition (opens the target tab; "Definition is outside this workspace") | C (`DefinitionProvider` + `ShowDocumentHandler`) |
| references (peek widget) | **M**: shell peek list, or a patch |
| rename (inline box, workspace edit) | **M**: shell rename box + `apply_lsp_edits` per document |
| codeAction (lightbulb, ctrl+.) | C |
| documentSymbol | M (shell picker) |
| codeLens | **M** (ED-110 is blocked on the rig anyway) |
| semantic tokens | C |
| formatting (shift+alt+f) | M: shell over `apply_lsp_edits` |
| diagnostics squiggles + hover | C (`DiagnosticSet`, diagnostic popover; fed from the stream signal) |

The toolbar's LSP chip and status menu (ED-078..081) are shell UI.

### 2.5 Documents: open, save, dirty, drafts, external change, read-only
- **Open** (`documents/open-document.ts`):
  - Refusals: 404 "File not found" + path + Try again (ED-009/010), 413 "File too large to open" toast with no tab (ED-011), 403 permission denied (ED-012), generic (ED-013), host-document refusals (ED-014).
  - Binary files open as Latin-1 text; images open as text (ED-015/016).
  - Already-open documents focus their tab (ED-005).
  - Untitled: "Untitled", `untitled:Untitled-1`, Plain Text, focused (ED-006).
- **Dirty and save** (`documents/document-registry.ts`, `lib/buffer-save.ts`):
  - The model owns the text. The registry keeps `savedText` and `savedVersionId` and derives dirty. A first edit keeps a preview tab.
  - mod+s saves and mod+shift+s saves all. A save is clean only if the buffer still holds what was written. Saves of one buffer are serialized.
  - Format on save applies to manual saves only.
  - Untitled save-as: native "Save as:" prompt, rebinds the tab, refuses onto an open dirty file (ED-087..089).
  - "Failed to save file" toast (ED-090). Locked workspace: "Workspace is read-only" toast and the editor read-only up front (ED-091/092).
  - After a save: git refresh and blame cache cleared.
  - Autosave after a 1 s quiet period (ED-093).
- **Drafts** (`documents/draft-store.ts`):
  - Each dirty document's text and saved text are written to IndexedDB `editor-drafts` once edits pause, and flushed on `pagehide`.
  - On restart, tabs, cursor and scroll come back, and drafts come back dirty (ED-017).
- **External change** (`workspace/lib/external-buffer-sync.ts`):
  - A clean buffer takes the disk text silently, as one undo step with selections kept (ED-095).
  - A dirty buffer keeps its edits and toasts "<name> changed on disk / Your unsaved changes are kept…" once (ED-096).
  - The echo of our own save is ignored (ED-097).
  - Reload from disk replaces the text as one undoable edit.
- **gpui-kit has none of this model.**
  - `set_value` clears undo history and resets the selection, the scroll and the LSP state.
  - `replace_all` is undoable but resets the caret and the scroll.
  - `InputEditsEvent` gives the edits.
  - A patch adds `replace_all_preserving_selections` and a monotonic text version that undo returns to (Monaco's `alternativeVersionId`), so "back to the saved text by undo" reads clean as develop does.

### 2.6 Toolbar (breadcrumb row) and preview
- **Toolbar layout:** the path breadcrumb on the left; on the right Preview, Find, the language chip, the cursor chip, the LSP chip, the preferences menu and extension actions (ED-067..083).
  - Each breadcrumb segment opens a dropdown of its directory: a folder drills in and shows "Go back"; a file opens as a preview and closes the dropdown; the same segment again closes it (ED-069..071).
  - Breadcrumbs can be turned off. A host document's breadcrumb is plain text.
- **Markdown (ED-114..151):**
  - A `.md` file opens in the rich Plate WYSIWYG view by default.
  - A rich/source toggle switches between them; the source view is Monaco with the toolbar.
  - Preview tabs: "<name> (Preview)" static render (ED-152), relative links open documents (ED-155), and an HTML preview in a sandboxed iframe (ED-156).
- **gpui-kit has no WYSIWYG markdown.** gpui-component's `text` module only renders markdown read-only.
  - Recommendation (decision D1): the rich markdown view, the markdown preview and the HTML preview stay React.
  - They live in a web pane type `core.markdown` (rich view + preview) and `core.htmlpreview`, or one `core.preview`, served by the existing `app/panes` Plate code.
  - The native editor is the source view. The rich/source toggle swaps the pane's presentation between the two through the layout, the way develop swaps `markdown-view-store`.

### 2.7 Keybindings
- **Shell keymap chords that act on the editor:** `editor.save` mod+s, `editor.saveAll` mod+shift+s, new file mod+shift+n, open file mod+o, mod+w and mod+shift+t on the editor tab, and chat zoom mod+= / mod+- (the editor does not zoom, ED-033).
- **Editor-local chords** (Monaco defaults that develop exposes, rows above): ctrl+f, ctrl+h, F3/shift+F3, alt+c/w/r in find, ctrl+g, ctrl+d, ctrl+shift+l, alt+Up/Down, shift+alt+Down, ctrl+shift+k, ctrl+shift+[ / ], ctrl+k ctrl+0, F12/ctrl+click, shift+F12, F2, ctrl+., shift+alt+f, ctrl+space, ctrl+shift+o, mod+a.
- **gpui-kit's own bindings** live in the `Input` context. On Linux: ctrl-f search, **ctrl-h replace** (matches), **ctrl-y redo** (develop: ctrl+shift+z), shift-alt-up/down add a cursor, and ctrl-. code actions.
- **What is missing:** ShowCompletions and GoToDefinition have no default binding.
- **Routing:**
  - The shell keymap must yield to the editor for chords the editor owns while it is focused (KEYROUTE note on ED-122: ctrl+b in the source view still toggles the sidebar in develop, so that row is about the rich view).
  - The editor binds its chords in a `NativeEditor` key context. The shell's `keymap` crate declares which shell chords it gives up inside that context, with one table and no per-chord flags.

## 3. Ownership and data flow
Today, one `core.editor` web view per editor tab runs develop's whole document stack against `ROOT_PANE_ID`:
- The text, saved text, dirty flag and drafts live in that page's `DocumentRegistry` and IndexedDB.
- The shell learns about dirty state only through `pane.setDirty` into `DirtyPanes` (`app/desktop/app/src/dirty_panes.rs`).
- The shell sends `pane.save` into the page.
- `pane.open` from the page (breadcrumb file click, preview links, definition) reaches `ShellState::open_requested`.
- The layout (`app/crates/layout/src/ide`) owns tabs, preview, pinned and order.

The native design keeps one owner per piece of state:

| State | Owner (one) | Who reads it |
|---|---|---|
| Tabs, order, preview/kept, pinned, active tab, which file a tab shows (`PaneInstance.state.path`/`absolute`/`untitled`) | `layout::Layout` (unchanged) | ideshell, editor panes |
| The text of an open document, its undo history, selections, scroll | the document's `Entity<EditorState>` (gpui-kit), as develop's Monaco model owns the text | `editorview`, drafts, save, LSP |
| Saved text and saved version, hence dirty; save mark (orders reads against saves); save chain; pending-save echo | `editor::Document`: a gpui-free record held next to the `EditorState` entity in one `Documents` registry in the binary (`app/desktop/app/src/editor_panes.rs`, the analogue of `terminal_panes.rs`) | close guard, tab dot, Save All, title |
| Dirty for **web** panes (extensions, any SDK page) | `DirtyPanes` as today, fed by `pane.setDirty` | close guard |
| Drafts on disk | `editor::Drafts`: a file per home under the shell's data dir, written with `atomicfile`; written on quiet, removed on clean | restore at startup |
| Editor settings | `settings` crate values (catalog `editor.rs` already lists every key) | `editorview::Settings`, pushed like `terminalview::Settings` |
| Workspace locked (read-only) | the store's chat lock state, already read through `ShellPaneSource::locking` (`ChatLocks`) | the tab lock glyph and `Editor::readonly`, from the same predicate |
| Per-pane view state (scroll, cursor, folds) across park and restore | the `EditorState` while the pane lives; persisted into the pane state on park | restore |

Rules that keep it single-owner:
1. **Dirty for native panes is derived, never stored.** `Document::is_dirty(&EditorState)` compares the version with the saved version. The close guard asks `EditorPanes::is_dirty(pane)` for native panes and `DirtyPanes::contains` for web panes. A pane is one kind or the other, so the two sets never describe the same pane.
2. **No `pane.*` frames inside the shell.** A native pane calls the shell directly through `ShellRequest`, as `TerminalPanes` does:
   - `pane.setTitle` becomes the tab title from the document name.
   - `pane.setDirty` disappears (derived).
   - `pane.save` becomes `EditorPanes::save(pane)`.
   - `pane.setState` becomes a layout `SetPaneState` command for the settled `fresh`/`line` (what `settled()` does in `buffer-pane-host.tsx`).
   - `pane.openRefused` becomes the existing refused-open path that closes a fresh tab and toasts.
   - The breadcrumb's file click and LSP definition become `ShellRequest::OpenDocument { from, file, mode: Preview|Kept }`, handled by the same `document_commands` that `open_requested` uses today, so ED-071's "opens as preview" and ED-005's "focus existing" go through one path.
3. **A first edit keeps a preview.** On the document's clean→dirty transition `EditorPanes` sends `ShellRequest::KeepIfPreview(pane)`, which runs `ShellState::keep_if_preview` (it exists). This is develop's `promotePreview`.
4. **Saving goes through the daemon API.** `PUT /v0/documents` carries `{workspaceId?, path, content}` (develop `writeDocument`) and `GET /v0/documents?workspace&path` reads. Both go through `daemonapi`: `Route::document_content` exists for reads; a document write route is added there. A save runs:
   1. format if asked (manual saves only);
   2. write;
   3. `markSaved` (clean only if the text still equals what was written);
   4. git refresh;
   5. send LSP `didSave`.

   No `api/` change is needed: the routes exist.
5. **External change.** The daemon's file-change stream signal reaches the store, which already holds the host stream. It is routed to `EditorPanes::disk_changed(ref)`, which implements `takeDiskText` (clean: replace keeping selections, as one undo step; dirty: flag once and toast; own echo: dropped by the pending-save mark). The stream signal name and subscription must be confirmed when slice 3 starts. If the store does not yet subscribe to file-change signals for open documents, that subscription is the slice's first change (shell side, no `api/` edit).
6. **The theme has one owner.** The shell theme stays the owner. gpui-component's global `Theme` (including `highlight_theme`) is a projection written by one adapter, `editorview::theme::install(tokens, cx)`, only when the shell theme changes, the way `terminalview::settings::theme_palette` projects the terminal palette. Nothing else writes `Theme`. The owner should confirm that this derived global is not a "second copy" (decision D6).

The document registry is keyed by `DocumentUri` (`workspace:<ws>/<path>`, `host:<path>`, `untitled:<n>`). It is the one place a save-as rebinds an untitled document to its file (ED-088) and refuses onto an open dirty one (ED-089).

## 4. Where it goes in the tree
Following `docs/plans/2026-11-18-rust-tree.md` (the `terminal`/`terminalview` split and the `<feature>view` naming rule):

| Crate | Layer | Kind | Holds | Deps |
|---|---|---|---|---|
| `editor` (new) | L1 | logic, gpui-free | `DocumentRef`, `DocumentUri`, `Document` (saved text and version, save mark, the dirty rule, the echo mark), `Drafts` (records + atomic file), `Language` (path → id, display name, picker list), open refusal reasons from problem codes, `TextVersion` | atomicfile, protocol |
| `editorview` (new) | L3 | gpui + gpui-kit | `EditorView` (the entry: one pane's editor around an `Entity<EditorState>`), `Settings` (from `settings` values), `theme` (Theme projection, highlight theme), `toolbar/` (breadcrumb + dropdown, language chip and picker, cursor chip, LSP chip and menu, preferences menu, Find and Preview buttons), `find/` (the find widget), `goto/` (go-to-line, outline), `lsp/` (gpui-kit provider impls over a consumer-defined `LspPort` trait, M28), `keys` (the `NativeEditor` context bindings), `bin/gallery` (`editorgallery`) | editor, ui, gpui, gpui-kit |
| `store` | L2 | logic | + `documents` module: read, write and LSP route calls through `daemonapi`, and the file-change signal | daemonapi (+ editor for `DocumentRef`) |
| `daemonapi` | L1 | logic | + the document write route and the LSP routes | protocol |
| `keymap` | L1 | logic | + the table of shell chords yielded inside `NativeEditor` | chord |
| `crowbar` (binary) | L4 | | `editor_panes.rs` (+ `editor_panes/`): the `Documents` registry, one `EditorView` per native pane, save chains, quiet-period timers (on the injected `clock.rs`), drafts restore, the `LspPort` implementation over `store` | all |

- `rust-layers.txt` gains `editor` (L1) and `editorview` (L3).
- `mapping.md` gains the two crates' files. No existing file moves; the files listed in section 5 are deleted.
- `core.editor` changes from `PaneKind::Web, entry "editor/index.html"` to `PaneKind::Native` in `core_panes.rs`. `NativePanes` gains `editors: EditorPanes` beside `terminals` and `new_tabs`.
- Pane params and state stay `{threadId, fileId}` + `{path, absolute?, untitled?, fresh?, line?}`, so saved layouts restore unchanged (no layout schema bump).

**Vendoring and licence.**
- gpui-kit, gpui-component, gpui-base and gpui-kit-assets are all Apache-2.0, which `app/deny.toml` allows. Their transitive deps must pass `cargo deny`: tree-sitter and its grammars are MIT, ropey MIT, lsp-types MIT, notify CC0/Artistic (check), html5ever MIT/Apache.
- **Recommendation: vendor `gpui-base` and `gpui-component`** exactly as gpui-pre is vendored. Each gets an `upstream.txt` pin with its sha256 (gpui-base `4f4e5aff…25d95`, gpui-component `607c90f6…3646`), `patches/<dir>/series`, `[patch.crates-io]`, and coverage by `update.sh --check`.
  - The plan needs about eight engine patches (section 6). One person publishes both crates, as with gpui-pre.
  - `gpui-kit` (the facade) and `gpui-kit-assets` stay unpatched crates.io pins in `Cargo.lock`.
  - Vendoring happens in slice 0, before the first patch, so every later patch is a reviewed file in `patches/`.
  - The vendored trees come to about 5.6 MB.
- **Features:**
  - `gpui-kit = { version = "=0.10.0", default-features = false, features = ["component", "tree-sitter-<lang>"...] }`, so `assets` and `speech` stay off.
  - gpui-component still pulls in `gpui-kit-assets` itself. Its icons must not show in the shell (CLAUDE.md: icons are phosphor). The fold chevrons go through `fold_icon_renderer` with `ui` phosphor icons, and the built-in search panel and context menu are disabled.
- `ui::TextInput`/`TextArea` stay for form fields. gpui-kit's `Input`/`Textarea` are not adopted, so no second text-field library is used for one job. Only `Editor` is used.

## 5. Slices (each lands green: fmt, workspace clippy, rules, standards, tests)
Every slice:
- starts with a red test through the public API (gpui test platform with `gpui-kit/test-support`, `TestWindowExt::press/input`);
- has a rig pair per cluster as the brief asks;
- flips the rows listed.

The Monaco pane stays in place until slice 7 retires it. No feature flag is added, because a flag would be a second path: `core.editor` switches to native for every file at slice 1. **Slices 1 and 2 must therefore land together, or slice 1 must ship read-only only on a branch** (decision D2).

### S0: vendor and wire, no behaviour
- `upstream.txt` + `update.sh` for `gpui-base` and `gpui-component`; the `editor` and `editorview` crates with empty public APIs; `rust-layers.txt`.
- `cargo deny`, `cargo machete`, and `cargo check -p crowbar --target x86_64-apple-darwin` (see the gpui-upgrade notes).
- Red test: `editorview` gallery smoke. An `EditorView` over "a\nb" renders two rows and `value()` round-trips. It fails before the crate exists.
- Rows: none.

### S1: read-only native view behind the same tab
- `core.editor` becomes native.
- `EditorPanes::open(pane)` reads through `store::documents`, creates `EditorState` (language from the path, tree-sitter highlight), sets read-only, and reveals `line`.
- Covered: the refusals (toast, no tab or unavailable state + Try again), the breadcrumb (static segments), the language and cursor chips, and restore after restart (view only).
- Red tests:
  - `opening_a_workspace_file_shows_its_text_in_the_tab` (fake daemon serving `src/pricing.ts`);
  - `a_413_read_closes_the_fresh_tab_and_toasts_too_large`;
  - `a_restored_tab_whose_file_is_gone_shows_file_not_found_with_try_again`.
- Rows: ED-001, 005, 009, 010, 011, 012, 013, 014, 015, 016, 022, 024, 025, 028, 031, 035 (EDSIZE: native text has no 12px root issue), 036, 038, 067, 068, 072, 073, 074, 082 (font).

### S2: editing and save
- Writable `EditorState`; the `Document` dirty rule (patch: the text version); save, save all, save chain, git refresh; untitled + save-as prompt + rebind; "Failed to save file"; locked read-only up front + "Workspace is read-only"; the preview kept on the first edit; the tab dirty dot and the close guard from the derived flag; autosave.
- Red tests:
  - `typing_marks_the_tab_dirty_and_undo_back_to_saved_clears_it`;
  - `ctrl_s_writes_the_text_and_clears_dirty_only_if_unchanged_since`;
  - `first_edit_keeps_a_preview_tab`;
  - `untitled_save_as_rebinds_the_tab_to_the_new_file`;
  - `a_locked_workspace_opens_read_only_and_save_toasts`.
- Rows: ED-006, 041, 047, 048, 084, 085, 086, 087, 088, 089, 090, 091, 092, 093, 097, plus close-model rows that need a dirty editor (DLG-023..026 are checked, not owned).

### S3: drafts, external change, reload
- `editor::Drafts` written on quiet, cleared on clean, restored dirty at startup.
- `disk_changed`: clean replace keeping selections (patch: `replace_all` keeping selections and scroll); dirty flag + one toast; the own echo dropped.
- Red tests:
  - `a_dirty_document_survives_a_restart_as_a_dirty_draft`;
  - `a_clean_document_takes_the_disk_text_in_one_undo_step`;
  - `a_dirty_document_keeps_its_edits_and_warns_once`.
- Rows: ED-017, 095, 096.
- Legacy: the drafts in the panes' IndexedDB are imported once into `editor::Drafts`, then deleted (decision D7).

### S4: settings and look
- `editorview::Settings` from every editor key.
- The theme projection and highlight theme built from develop's tokens and Shiki colours; a transparent background.
- Whitespace modes and the active indent guide (patches); selection-occurrence and caret-word highlights (shell decorations); scrollbars at rest; the toolbar preferences menu; the language picker; editor zoom.
- Red tests:
  - `settings_push_font_size_line_height_and_wrap_to_every_open_editor`;
  - `the_highlight_theme_follows_a_light_dark_switch`;
  - `selecting_a_word_highlights_its_other_occurrences`.
- Rows: ED-018, 020, 021, 023, 026, 027, 029, 030, 032, 033, 034, 075, 076, 077.

### S5: find, go to line, multi-cursor, folding, keys
- The shell find widget, floating, with regex, whole word, case and in-selection; replace / replace all; F3.
- Go-to-line with centring (patch); outline when LSP lands.
- ctrl+d / ctrl+shift+l; line ops (patch); fold keys (patch); `NativeEditor` key context and the keymap yield table; the ctrl+shift+z redo binding; context menu off; breadcrumb dropdowns.
- Red tests:
  - `ctrl_f_regex_counts_matches_and_enter_moves_to_the_next`;
  - `ctrl_g_42_puts_line_42_in_the_middle`;
  - `ctrl_d_adds_the_next_occurrence_as_a_second_selection`;
  - `ctrl_shift_bracket_folds_the_block_without_horizontal_scroll`;
  - `clicking_a_folder_segment_lists_its_entries_and_a_file_opens_as_preview`.
- Rows: ED-042, 043, 044, 045, 046, 053, 054, 055, 056, 057, 058, 059, 060, 061, 062, 063, 064, 065, 069, 070, 071.
- As built (rig, 2026-11-24): find, find next, replace, go to line, ctrl+d, alt+Down and ctrl+/ (left to the app's chat/terminal chord) behave as develop's. Known differences, each a row still failing:
  - no selection-occurrence highlight and no find-state strip after ctrl+d; the selection colour is not develop's `--editor-selection` (ED-043);
  - the replace row has no preserve-case toggle (ED-063);
  - the go-to-line hint row is not drawn as Monaco's focused list row and is not cut with an ellipsis; no sticky-scroll header (ED-065);
  - indent guides and bracket-pair highlight are absent (S4).

### S6: LSP
- `LspPort` over `store::documents`: didOpen, didChange, didSave and didClose; completion; hover; signature help (patch); definition (`ShowDocumentHandler` → `OpenDocument`, the outside-workspace toast); references and rename (shell UI + `apply_lsp_edits` per document, opening documents as needed through the registry); code actions; symbols; semantic tokens; formatting + format on save; diagnostics from the stream signal; the LSP chip and menu; the query failure toast.
- Red tests use a fake daemon LSP route:
  - `typing_a_dot_asks_the_server_and_shows_its_items`;
  - `f12_opens_the_definition_tab_at_its_line`;
  - `f2_renames_across_two_open_documents_in_one_edit_each`;
  - `format_on_save_writes_the_formatted_text`.
- Rows: ED-049, 050, 051, 052, 066, 078, 079, 080, 081, 094, 101..109, 111, 112 (110 and 113 stay blocked).

### S7: markdown and retiring Monaco
- With D1 as recommended: the rich view and the previews become their own web pane type served by `app/panes`; the rich/source toggle swaps between that pane and the native source pane, keeping one document owner. The rich view reads and writes the text through a `pane.document.*` frame pair, so the native `Documents` registry stays the one owner.
- Then the Monaco pane is retired.
- Red tests:
  - `toggling_a_markdown_tab_to_source_keeps_unsaved_rich_edits` (shell side);
  - `a_relative_link_in_the_preview_opens_the_document` (ED-155);
  - knip, tsc and eslint clean after the deletion.
- Rows: ED-114..152, 155, 156 (the font rows also need FONT fixed in the pane bundle), and ED-100/153/157 re-checked.

**Deleted from `app/panes` in S7.** CLAUDE.md requires deleting code made unused. knip is the gate; this list is the expected minimum:
- `src/entries/editor.tsx`, `entries/editor/index.html`, `open-editor-buffer.ts`, `editor-target.ts`, and the editor half of `buffer-pane-host.tsx` (review keeps its own host).
- `features/editor/{monaco,lsp,host,stores,styles,config,extensions,utils}/`, `features/editor/components/{editor-surface,stylesheet}.tsx` and `toolbar/*`, `features/editor/hooks/*`, `features/editor/lib/{editor-manager,monaco-adapters,pane-editor-controller,buffer-save,reload-buffer,reveal,selection-drag,raf-coalesce,idle-task,active-editor-context}.ts`, `features/editor/documents/*` (except what the markdown pane reads through its frames).
- `features/panes/components/{editor-pane,editor-host-registry,pane-editor-view-content}.tsx`, `features/panes/lib/editor-surface-loader.ts`, and the editor slices of `window-pane-store` (`editor-tab-actions.ts` and the buffers list) once chat stops reaching them.
- The `monaco-editor` dependency in `app/package.json`, and the editor tests under `src/__tests__/features/editor/**` that cover deleted units.
- Kept: Plate markdown (`features/editor/markdown/**`, shared with the chat composer), Shiki (`lib/shiki-grammars.ts`, used by the code-block node), and the review pane.

**As built (2026-11-24).** The list above cannot be deleted: `index.html` → `app-main.tsx` is the
mobile app's page (`app/mobile/src-tauri/tauri.conf.json` serves `panes/dist`), and its IDE screen
(`mobile-ide-screen.tsx` → `pane-editor-view-content.tsx` → `editor-pane.tsx`) is develop's Monaco
editor. So Monaco is retired from the shell's four pane bundles instead:
- The app installs its editor at boot (`features/editor/host/app-editor.ts`): the host build
  (`build-editor-host.ts`, the only module that loads Monaco) and its own preview opener. A shell
  pane installs neither, so `loadEditorHost` refuses there and no pane entry reaches Monaco.
- The review page no longer mounts the Monaco host registry; it shows only review buffers.
- The minimap setting (D4) is deleted from the settings catalog, the app's settings tab and the
  toolbar menu; ED-024 and ED-054 are marked dropped.

A reachability check run for this plan (imports from `entries/{chat,review,console}.tsx` with the review host cut off from `editor-pane.tsx`) shows 61 files that only the editor entry reaches. Most of the rest of `features/editor` (documents, LSP, buffer-save) is still reached from chat, through `window-pane-store`'s editor slices (`chat-lifecycle.ts` → `editor-tab-actions.ts` → `editor-host.ts` → `buffer-save.ts`). knip will flag it once those slices go.

## 6. Risks
- **Engine patches** to gpui-base/gpui-component, kept as `patches/` and dropped as upstream gains them (the minimap is no longer one, D4):
  1. Text version + saved-version undo (dirty);
  2. `replace_all` keeping selections and scroll;
  3. Centred reveal;
  4. Fold actions;
  5. Line move/copy/delete actions;
  6. All-selections getter;
  7. Signature-help provider and popover;
  8. Whitespace modes + active indent guide;

  Each needs a test in `editorview` that pins it (as the gpui-upgrade A1 rule). Upstreaming 1-6 to longbridge is preferred; they are small.
- **Performance:** the engine says "up to about 50K lines", and develop's daemon refuses only above its size limit (ED-011's 30 MB file).
  - A bench (`editorview/benches/open_large.rs`) opens 10K, 50K and 200K-line files and types at the end.
  - If 200K lines stalls, develop's behaviour for very large files (Monaco stays usable) is not met (decision D8: a line cap with a "too large for the editor" state, or an engine fix).
  - Tree-sitter on the main thread: injection parses have a 20 ms timeout, which needs measuring on lavapipe on the rig.
  - Per-keystroke `didChange` sends the full text as develop does; the cost is the same.
- **IME:** `EntityInputHandler` is implemented, and its comments name macOS cases (insertText without unmarkText, the Korean IME rewrite). Linux IME through gpui's X11/Wayland text input is unverified on the rig (XWayland + fcitx/ibus).
  - S2 adds a composition test with `TestInput::compose_text`.
  - A rig check with ibus is a pending row, not a pass.
- **Keyboard ownership:** gpui-kit binds in context `Input`, and the shell keymap binds globally. Wrong precedence either eats editor chords (ED-122) or lets the editor eat shell chords (mod+w, mod+s).
  - The yield table in `keymap` is the one decision point.
  - The gpui-kit ctrl-y redo and ctrl-h replace are kept only where develop has them.
- **Theme global:** gpui-component's `Theme` is global per `App`. Any other gpui-component widget that is pulled in would follow it too. Only `Editor` is used, which keeps the surface small.
- **Highlight parity:** tree-sitter captures are not Shiki TextMate scopes, so some token colours will differ (D3).
  - Known differences, accepted under D3 (2026-11-22):
    - Built-in types (`string`, `number`) take the type colour; develop's Monarch tokenizer paints them as keywords. gpui-kit's `SyntaxColors` has no `type.builtin` slot, so the capture falls back to `type`. Follow-up: a vendor patch to the TypeScript highlight query.
    - Properties take develop's property colour, which develop shows only once its language server runs. Tree-sitter uses one capture for TypeScript members and JSON keys.
- **macOS compile:**
  - gpui-base and gpui-component have `cfg(target_os = "macos")` code (objc2 text content type, keybindings, CoreText via `core-text` in gpui-component).
  - S0 runs `cargo check -p crowbar --target x86_64-apple-darwin` with the gpui-upgrade notes (no-op `CC`, `runtime_shaders`). No verification until macOS is in scope.
  - `PLATFORM:` notes go where mac chords differ (cmd-shift-f is replace on mac).
- **Dependencies and build:** tree-sitter grammars are C, so build time and binary size grow (estimate +10-20 MB for the grammar set).
  - Only develop's languages are enabled.
  - `notify` comes in through gpui-component and is unused by the shell. `cargo machete` sees it only as transitive, so it is acceptable.
- **gpui-pre moves:** gpui-kit pins `=0.3.12`. A later gpui-pre bump needs a matching gpui-kit release, or a re-port of our vendored gpui-component/base onto the new gpui (owner memory: "latest GPUI always; re-port patches").

## 7. Decisions (settled 2026-11-21)
- **D1** The markdown rich view and the markdown/HTML previews stay React in a web pane; the native editor is the source view. (Owner.)
- **D2** S1 and S2 land as one switch of `core.editor` to native; no feature flag. (Coordinator.)
- **D3** Highlight colours are matched per token class from develop's theme, not scope-exact Shiki. (Owner.)
- **D4** The minimap is dropped. Its setting (`showMinimap`) and its toolbar preferences item are deleted in S7, and ED-024 is marked "dropped by owner, 2026-11-21". No minimap patch. (Owner.)
- **D5** Monaco's F1 / ctrl+shift+p palette is dropped. ED-054 is marked "dropped by owner, 2026-11-21" in S7. (Owner.)
- **D6** gpui-component's global `Theme` is a derived projection of the shell theme, written only by `editorview::theme::install`. (Coordinator.)
- **D7** The old drafts are imported once, then deleted: S3 reads the pane IndexedDB `editor-drafts` store (and develop's, through the legacy-data-import path) into `editor::Drafts`, then removes the source. (Owner.)
- **D8** S1 adds the open-and-type bench at 10K, 50K and 200K lines. The cap-or-fix choice is made from its numbers. (Coordinator.)
  - Bench results (2026-11-21, release, gpui test platform with cosmic-text shaping, 50 keystrokes typed at the end):

    | Lines | Open | Keystroke, mean | Keystroke, worst |
    |---|---|---|---|
    | 10K | 446 ms | 0.7 ms | 1.3 ms |
    | 50K | 163 ms | 0.9 ms | 1.6 ms |
    | 200K | 252 ms | 0.6 ms | 1.2 ms |

  - The 10K open is the first run and includes loading the font. Neither opening nor typing grows with file size, so 200K lines does not stall and no line cap is needed.
- **D9** gpui-base and gpui-component (and gpui-kit, if a patch needs it) are vendored in S0 through `vendor/update.sh`, with `update.sh --check` passing. (Coordinator.)
