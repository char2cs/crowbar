# One buffer per file, shared by every pane that shows it

Status: PLAN. Code follows slice by slice, each with its red test first.

## Problem
Two file panes on the same document each build their own `editorview::EditorView`, and each view owns a
gpui-kit `EditorState` holding its own text, undo history, saved text, save mark and dirty flag. Typing
in one pane does not reach the other, each pane saves its own copy over the other's, and closing either
pane discards the document's draft (`EditorPanes::follow` forgets the drafts of every `gone` target,
whatever other pane still shows that URI). That is a second copy of the document's state. CLAUDE.md
asks for this plan before the fix.

## Develop's model (the spec)
- **One buffer per document.** `findOpenBuffer` (`web/src/features/panes/stores/slices/find-open-buffer.ts`)
  matches an editor spec by `sameDocument(documentRefOf(b), spec.doc)`: the document's authority,
  workspace and path. The chat that asked doesn't count. `openContent` (`buffer-slice.ts:104`) reuses that
  buffer. `open-document.ts:show` focuses a pane holding it in the same view, or else seats the **same
  buffer id** in the target pane (`addEditorTabToPane`). One buffer can be listed by several panes'
  `editorTabIds`.
- **Lifetime is derived from the panes.** Invariant C2 (`buffer-release.ts:releaseUnreferencedBuffers`):
  a buffer exists while some pane lists it. The write that unlists it from its last pane also releases
  it, and `disposeBuffers` then releases its document (`documents.release(uri)`) and its draft
  (`clearDraft(uri)`). There is no separate refcount: the pane lists are the count.
- **One model per document.** `DocumentRegistry` (`web/src/features/editor/documents/document-registry.ts`)
  keeps one Monaco model per document URI. `open` is idempotent per document, and adopts a model the
  references peek already made. The registry owns `savedText`, `savedVersionId`, `dirty` (derived
  from the model's alternative version id, and reported on transitions through `dirtyChanged(uri)`)
  and `saveMark`. `applyExternal` replaces the text as one undoable edit that keeps **every showing
  editor's carets** (`replaceAll(text, preserveSelections=true)`).
- **Views own only carets and scroll.** `EditorManager` (`web/src/features/editor/lib/editor-manager.ts`)
  keeps one retained Monaco widget per pane and swaps models into it. It saves view state (selections
  and scroll) per `${paneId} ${uri}`. Two panes over one model each keep their own selections and
  scroll. The undo stack belongs to the model, so ctrl+z in either pane undoes the last edit to the
  document. An edit in one pane shifts the other pane's carets, as Monaco does for every editor on a
  model.
- **LSP per document.** `document-lsp.ts` attaches once per model: one `didOpen` and one `didClose`
  per document, addressed to the document's own workspace, never per pane or chat.
- **Previews read the document.** `usePreviewSource` renders the open document's live text
  (`useDocumentText(uri)`), else the preview buffer's snapshot.
- **Drafts are per document URI** (`draft-store.ts`, keyPath `uri`), cleared when the buffer goes.

## Where the shell stands
- `EditorPanes` (`app/desktop/app/src/editor_panes.rs`) keeps one `Followed` per `PaneId`. Each holds
  a `PanePhase::Shown(Entity<EditorView>)`.
- `EditorView` (`crates/editorview/src/editor_view.rs`) holds document facts (`saved`, `save_mark`,
  `disk_conflict`, `reported_dirty`, the `LspLink`, the language) next to view facts (find, goto,
  occurrences, caret and scroll in its `EditorState`).
- A document already has an identity: `EditorPanes::uri_of(target)` gives the workspace URI that drafts
  are keyed by. `SharedDocuments` (`app/src/shared_documents.rs`) refcounts LSP opens per
  `(chat, path)`. That workaround goes away once the document owns its LSP link, and its key is wrong
  anyway: develop's key is the workspace.
- gpui-kit's `EditorState = InputBaseState<EditorMode>` (`vendor/gpui-base/src/input/base/state.rs:438`)
  fuses the text (`text: Rope`), the history (`undo_manager`), the revision and the highlighter with
  the view (selections, display map, scroll, layout, IME, hover). It has no notion of a model shared by
  several states.

## The owner design
### Text: a gpui-base `TextModel` shared by the states that show it (vendored patch `shared-text`)
gpui-base is split the way Monaco splits model from editor:

- **`TextModel` (an `Entity`)** owns the text (`Rope`), the undo and redo transactions (today's
  `UndoManager`), the `document_revision`, and the highlighter (today an
  `Rc<RefCell<Option<Box<dyn InputHighlighter>>>>`, which already allows sharing). It applies a batch
  of `Change`s for an origin `EntityId` and records them as one transaction, with that origin's
  selections before and after. It then emits `TextModelEvent::Edited { changes, origin, kind }`, where
  `kind` is `Typed`, `ReplacedKeepingPositions` or `Undo/Redo`.
- **`InputBaseState`** holds `model: Entity<TextModel>`. Every local edit path
  (`replace_text_in_range_silent`, `replace_all*`, undo and redo) goes through the model.
  - Its `text` field becomes the model's **snapshot**: `ropey::Rope` clones share their nodes in O(1),
    and the snapshot is replaced (never edited) from the model's text in the same synchronous event.
    It is a read handle on the owner, not a second copy. A test checks that
    `state.revision == model.revision` after every edit, from either side.
  - The state subscribes to its model. On an edit from another origin, it runs the same incremental
    path a local edit runs (display map, `adjust_annotations`, LSP reset) but **transforms** its own
    selections through the changes instead of placing them. On `ReplacedKeepingPositions` it keeps
    them by line and column, as `replace_all_keeping_selections` does today.
  - Undo and redo ask the model. The state that undoes takes the selections recorded with the
    transaction (Monaco's cursor state), and every other state transforms its own.
- **Unchanged API:** `EditorState::new` makes its own private model, so single-line inputs and text
  areas behave exactly as before.
- **New API:** `TextModel::new(text, language, cx)`, `EditorState::over(model, window, cx)`,
  `EditorState::model()`, `TextModel::{text, revision, replace_all_keeping_positions, on edits}`.
- Display map, wrap widths, folds, search session, decorations, hover and completion stay per state,
  as in Monaco: each pane has its own width and its own folds.
- The patch is recorded in `vendor/README.md` and `patches/gpui-base/NNNN-shared-text.patch`. It is
  dropped if upstream gains a shared buffer.

The alternative of mirroring each view's edits into the other views (one `EditorState` per pane, kept
equal through `InputEditsEvent`) is rejected. It is a resync of N copies, its undo stacks diverge (undo
in pane B would record a new edit in A instead of undoing it), and CLAUDE.md forbids it.

### Document facts: `editorview::Document` (one per open document)
- **A new `Document` entity** owns `Entity<TextModel>`, the saved text, the `SaveMark`, the disk
  conflict, `dirty` (with the transition reported once: `DocumentEvent::DirtyChanged`), the language
  (which drives the highlighter and the toolbar label), the `LspLink` (one didOpen/didChange/didSave/
  didClose per document) and the quiet period (`EditsPaused`, which drafts and autosave use).
- **`EditorView`** becomes `EditorView::new(document: Entity<Document>, ViewOpening { line, column,
  access }, ...)`. It keeps the toolbar, find, goto, occurrences, keyboard and its `EditorState` over
  `document.model`.
  - `mark_saved`, `take_disk_text`, `take_page_text`, `rebind` and `is_dirty` move to `Document`.
    The view's toolbar and language follow `Document` events.
  - LSP requests that need a caret (hover, completion, definition) stay on the view and go through
    the document's link.

### The registry: `OpenDocuments` inside `EditorPanes` (app crate)
- **Key:** `uri_of(target)`, the workspace URI drafts use already (authority, workspace and path),
  not `(chat, path)`. An untitled pane is its own document (its label is unique), as in develop.
- **Open:** a pane arriving on a URI that is already open does no read. It goes straight to `Shown`
  with a new view over the open document, at its own line and column. Otherwise the first read lands
  the document (`Landed` → `Document::new`).
- **Release, derived from the panes (C2):** after `follow` retains the panes the layout lists,
  `OpenDocuments` drops every document no `Followed` names. There is no counter: the pane map is the
  count. Drafts are forgotten only for released URIs, which fixes the draft lost when one of two panes
  closes.
- **Dirty:** `DocumentEvent::DirtyChanged` fans out to every pane whose target has that URI, through
  the existing `DocumentRequest::Dirty { pane }`. The set of panes is read from the pane map when the
  event arrives, so no list of subscribers is kept.
- **Save, save-as, reload, disk:**
  - Ctrl+s in any pane saves the document's text. `saved` marks the document, so every pane is clean.
  - `disk_changed` reads once per document, not once per pane, and `take_disk` applies to the
    document; each view keeps its carets through the model.
  - A reload replaces the document.
  - Save-as of an untitled pane re-keys that pane's document to the new URI. When the new URI is
    already open, the adoption rules (`Adoption::Replaces`/`Stays` in `saving_as.rs`) apply to the
    open document instead of to one holder view. They keep their current outcomes.
- **Previews:** `page_text` and `previews_of` look the source up by URI in `OpenDocuments`, not as "the
  first other pane that shows the target". `take_page_text` (the rich view) writes into the document,
  so every source pane and every preview of the file follow, as `usePreviewSource` does.
- **LSP:** the document owns the link, addressed to its workspace's server. `SharedDocuments` and its
  `(chat, path)` refcount are deleted.

### Open routing (after SPACES lands)
`ShellState::file_pane` matches by document (workspace and path), not by `(threadId, path)`, within
`showing(space)`. A holder in the showing space is focused (develop's `show`). Otherwise the file opens
in a new pane over the same document. This waits for SPACES' `showing(space)` and `AddTab { space }`.
Until then `file_pane` keeps its key, and the registry still shares the document between the panes the
current routing makes (two chats on one workspace, or two spaces).

## Slices (red test first in each)
1. **`shared-text` patch (gpui-base).**
   - Red, in `crates/editorview` (public API): `two_states_over_one_model_show_each_others_edits_and_keep_their_own_carets`.
     Typing in A appears in B; B's caret, placed after the edit point, shifts by the inserted length;
     A's does not move.
   - Red: `undo_in_either_state_undoes_the_documents_last_edit`.
   - Red: `a_replacement_keeping_positions_keeps_every_states_caret`.
2. **`editorview::Document`.** Move the saved text, save mark, dirty, conflict, language, quiet period
   and LSP link out of `EditorView`.
   - Red: `marking_the_document_saved_cleans_every_view_over_it`.
   - Red: `a_document_shown_twice_sends_one_did_open` (FakeServer).
   - Existing `EditorView` tests move to construct through a `Document`.
3. **`OpenDocuments` in `EditorPanes`.**
   - Red (`editor_panes.rs` tests, two chats `c1`/`c2` on `w1`, both on `src/pricing.ts`):
     `a_second_pane_on_an_open_file_shows_its_unsaved_text_without_reading`;
     `typing_in_one_pane_shows_in_the_other_and_marks_both_dirty`;
     `closing_one_of_two_panes_keeps_the_draft`;
     `saving_from_either_pane_cleans_both`;
     `a_change_on_disk_reads_the_file_once`.
4. **Previews and the rich view by URI.** Red: `a_preview_follows_the_open_documents_text_whichever_pane_typed`.
5. **Delete `SharedDocuments`** (made unused by slice 2). Its tests move to the document's
   one-did-open test.
6. **Open routing by document** (after SPACES lands). Red, in `shell_state/file_panes.rs`:
   `opening_a_file_open_under_another_chat_of_its_workspace_focuses_that_pane`.

Each slice runs the crate checks (`fmt`, `clippy -p editorview -p crowbar`, `test -p editorview -p crowbar`,
rules, standards) and makes a rig pair where something shows.

## Rows
- **ED-100** (same file in two panes, edits mirror): today `blocked` because develop's UI only seats one
  buffer in two panes across views. Re-drive it on the rig across two spaces once SPACES lands, or
  with two chats on one workspace. The pair is two panes typing into one document, both dirty.
- **ED-153** (preview follows source edits live): re-pair after slice 4.
- **ED-017 / UPG-087** (drafts per URI): re-check after slice 3. A closed second pane must keep the
  draft.
- **TST-103:** unaffected (rig cannot fail IndexedDB writes).

## Risks
- The patch touches gpui-base's edit path (state.rs: about 100 `self.text` reads). Reads keep working
  on the snapshot, so the change is confined to the write paths, the undo manager's home and the new
  follow path. Re-porting it on a gpui-kit update is the cost (see `vendor/update.sh`).
- One model is edited by two states in one frame only through gpui's entity updates. A follow that
  re-enters the origin state is skipped by origin id, so no update cycle is possible.
