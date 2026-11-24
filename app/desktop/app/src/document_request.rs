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
    /// An untitled pane's text reached the file it was saved as.
    SavedAs {
        pane: protocol::PaneId,
        saved: crate::saved_file::SavedFile,
    },
    /// A file pane's file changed on disk and was read again, under the save mark it had then.
    DiskRead {
        pane: protocol::PaneId,
        text: String,
        since: editorview::SaveMark,
    },
    /// A file pane's page holds `text` for the document the pane's editor owns.
    PageEdited {
        pane: protocol::PaneId,
        text: String,
    },
    /// The user asked to see a file pane's document in its native source editor.
    ShowSource { pane: protocol::PaneId },
    /// The user asked to see a Markdown pane's document in its page's rich view.
    ShowRich { pane: protocol::PaneId },
    /// The user asked to open a file pane's preview tab.
    Preview { pane: protocol::PaneId },
    /// A file pane's save failed, with the daemon's reason.
    SaveFailed {
        pane: protocol::PaneId,
        reason: String,
    },
}
