use gpui::Entity;

pub(crate) enum PanePhase {
    /// Its chat is not loaded yet, so there is no daemon to read from.
    Waiting,
    /// The read is out.
    Reading,
    /// The text arrived; the view is made when the pane is first drawn.
    Read(super::landed::Landed),
    /// The document shows.
    Shown(Entity<editorview::EditorView>),
    /// The document could not be read; the pane says why and offers to try again.
    Unavailable(editor::OpenFailure),
    /// It could not be read, and a Try again is reading it once more.
    Retrying(editor::OpenFailure),
}
