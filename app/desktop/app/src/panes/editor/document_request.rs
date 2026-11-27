use crate::panes::editor::saved_file;
#[derive(Debug, Clone, PartialEq)]
pub(crate) enum DocumentRequest {
    Read {
        pane: protocol::PaneId,
        read: Result<String, editor::OpenFailure>,
    },
    Retry {
        pane: protocol::PaneId,
    },
    Dirty {
        pane: protocol::PaneId,
        dirty: bool,
    },
    Save {
        pane: protocol::PaneId,
    },
    Paused {
        pane: protocol::PaneId,
    },
    Saved {
        pane: protocol::PaneId,
        text: String,
    },
    SavedAs {
        pane: protocol::PaneId,
        saved: saved_file::SavedFile,
    },
    DiskRead {
        pane: protocol::PaneId,
        text: String,
        since: editorview::SaveMark,
    },
    Reloaded {
        pane: protocol::PaneId,
        text: String,
        since: editorview::SaveMark,
    },
    DiskMissing {
        pane: protocol::PaneId,
        failure: editor::OpenFailure,
        since: editorview::SaveMark,
    },
    PageEdited {
        pane: protocol::PaneId,
        text: String,
    },
    ShowSource {
        pane: protocol::PaneId,
    },
    ShowRich {
        pane: protocol::PaneId,
    },
    Preview {
        pane: protocol::PaneId,
    },
    Definition {
        pane: protocol::PaneId,
        path: String,
        at: (u32, u32),
    },
    DefinitionOutside {
        pane: protocol::PaneId,
        path: String,
    },
    SaveFailed {
        pane: protocol::PaneId,
        reason: String,
    },
}
