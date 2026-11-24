# Markdown rich view beside the native editor

The owner's decision (2026-11-24): the Markdown rich view and the previews stay React. The native
gpui-kit editor is the source view. The flip (0001f3a40) made `core.editor` native for every file,
so README.md now opens as source. In develop it opens rich (audit ED-114..117 and ED-082).

## Develop's behaviour (the spec)
- `web/src/features/panes/components/editor-pane.tsx:74`: a `.md`/`.markdown` buffer (not `.mdx`)
  shows Plate's `MarkdownEditorPane` while its view is `rich`, and Monaco otherwise.
- `markdown/plate/markdown-view-store.ts`: the view is per buffer, kept in memory only, `rich` by
  default and dropped when the buffer closes. Every open, including one after a restart, starts rich.
- `markdown-view-toggle.tsx`: a ghost button that shows a Code icon ("View markdown source") in
  rich view and an Eye icon ("View rich editor") in source view. It flushes pending rich edits
  before it switches (ED-117).
  - In rich view the toggle floats top-right over the reading column (`markdown-editor-pane.tsx:68`).
  - In source view it sits in the toolbar after the language chip
    (`editor-status-actions.tsx:286`).
- `toolbar/breadcrumb.tsx:108`: in source view, `.md`/`.markdown`/`.html`/`.htm` files get an Eye
  "Preview" button before Find. It opens a separate `markdownPreview`/`htmlPreview` tab. HTML has
  no rich mode: it opens as source.
- Toolbar order (ED-067): [Preview] [Find] | line:col, language, [md toggle], LSP status, sliders.

## Who owns the state
The shell owns the rich/source view of each `core.editor` pane: an in-memory map from pane to view
in `ShellState`. It is absent for files that aren't Markdown and is cleared when the pane closes.
It is not saved with the layout (develop doesn't save it either). Nothing else holds a copy:
- panehost asks the shell for the view;
- layoutview asks `NativePanes::is_native(pane)`;
- the editor page learns the view from the pane's params.

## The change
1. **panehost.** Decide native or web per pane, not per type. Today `PaneHost::is_native(type_id)`
   and `placement::show` read `PaneType.kind`. Add a per-pane hook on the host's pane source, for
   example `PaneKind` resolved by a `fn kind_of(&PaneId)` the shell supplies. `core.editor` keeps
   `entry: "editor/index.html"` for the rich page.
2. **app.**
   - `ShellState` holds `MarkdownView` per editor pane: `Rich` on open for `.md`/`.markdown`.
   - `NativePanes` creates the `EditorView` only for a source-view pane. A rich pane gets a web slot.
   - Toggling one way drops the native view after reading its text (the document store already
     owns the text), and toggling back gives the slot up. Dirty state and saving stay with the
     document owner, so nothing is copied.
3. **The page (`app/panes`).**
   - The editor page renders only Plate's `MarkdownEditorPane`.
   - Its toggle sends a frame (a new `PaneFrame::SetView`, or `RunCommand` with an editor command)
     that asks the shell to switch to source, after flushing (ED-117).
   - S7's "retire Monaco" must keep this page and Plate, and delete only the Monaco surface.
4. **The native toolbar** (`editorview/src/toolbar.rs`):
   - for `.md`/`.html`, the Eye "Preview" button before Find, which opens the preview pane through
     the shell;
   - for Markdown, the Eye "View rich editor" toggle after the language chip, which emits an
     `EditorEvent` that the shell turns into the view switch.

   The Rust shell has no preview pane type. Before the flip, develop's page drew the preview as a
   page-local `markdownPreview`/`htmlPreview` buffer. The preview needs a web pane: the editor page
   with a preview param, or a `core.preview` type on `editor/index.html`. That is an owner choice.

   LSP status and the sliders menu are separate rows (ED-067). They are not stubs: leave them out
   until they work.

## Tests (red first)
- **ShellState:** opening README.md gives a rich pane; opening pricing.ts gives none; toggling
  switches the pane between web and native; closing forgets it.
- **editorview:** the toolbar of a `.md` view shows the rich toggle and Preview; a `.ts` view shows
  neither; a click emits the event.
- **panehost:** a pane whose source says web gets a slot although its type is native, and the
  reverse.
- **Rig:** ED-114, ED-115, ED-116, ED-117 and ED-082 pairs against Tauri.

## As built (S1, 2026-11-24)
Coordinator decisions: the panehost change is approved; the preview is the editor page with a
preview parameter, not a new pane type (develop: a `preview` buffer in the same group); S7 keeps
the editor page and Plate; LSP status and the sliders menu stay out.
- **One owner of the view:** `Followed.view` (`editor_panes/pane_view.rs`) per file pane, in
  memory: `Rich` on open for `.md`/`.markdown`, `Preview` for a preview pane, `Source` otherwise.
  Closing the pane drops it. The page keeps no copy: it is mounted only while the pane is rich or a
  preview.
- **One owner of the text:** the pane's `EditorView`, made when the text lands even while it is not
  drawn. The rich page reads it (`document.text`, sent on mount, after a read and after a disk
  change is taken) and writes it back (`document.edit`, one undoable edit), so dirty state, drafts,
  autosave and save stay where they are. `document.showSource` and `document.save` send the
  page's pending edit first (frames are ordered).
- **Keyboard:** a rich or preview pane's `EditorView` is neither drawn nor focused, so the page
  keeps the keys; switching views focuses the pane so the keys follow the view that draws it.
- **panehost:** `LayoutSnapshot::kinds` (`PaneKinds`) answers per pane; `NativePanes` answers for
  file panes, the type decides for the rest.
- **Preview:** `PaneSpec::preview` = `core.editor` with `view=preview` in its params and
  `preview: markdown|html` in its state, titled "<name> (Preview)". It renders the live text of an
  open pane on the same file, else its own read; it follows edits when they pause, on save and
  when a disk change is taken.

Known differences from develop:
- A Markdown file whose text does not fit one 256 KB frame opens in the source view.
- A preview follows unsaved source edits when they pause (the autosave quiet period), not per
  keystroke.
- Rich-view undo is Plate's own; the native undo stack keeps each page edit as one step.
