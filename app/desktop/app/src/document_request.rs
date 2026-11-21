#[derive(Debug, Clone, PartialEq)]
pub(crate) enum DocumentRequest {
    /// A file pane's document read answered.
    Read {
        pane: protocol::PaneId,
        read: Result<String, editor::OpenFailure>,
    },
    /// The user asked a file pane to read its document once more.
    Retry { pane: protocol::PaneId },
    /// A file pane's text began or stopped differing from its saved text.
    Dirty { pane: protocol::PaneId, dirty: bool },
    /// A file pane asked to save (ctrl+s, or the close prompt's Save).
    Save { pane: protocol::PaneId },
    /// A file pane's edits paused while it differs from its file: autosave's moment.
    Paused { pane: protocol::PaneId },
    /// A file pane's text reached its file.
    Saved {
        pane: protocol::PaneId,
        text: String,
    },
    /// A file pane's save failed, with the daemon's reason.
    SaveFailed {
        pane: protocol::PaneId,
        reason: String,
    },
}
