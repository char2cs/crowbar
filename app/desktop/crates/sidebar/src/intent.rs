//! What a click on a menu entry or row button means.

use crate::clip_mode::ClipMode;
use crate::command::Command;
use crate::new_entry::NewEntry;
use crate::project_id::ProjectId;
use crate::row_id::RowId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Intent {
    StartRename(RowId),
    StartBranching(RowId),
    StartDraft(RowId),
    RequestRemoval(RowId),
    RequestProjectRemoval(ProjectId),
    Keep(RowId),
    ToggleFold(RowId),
    StartCreateSpace,
    StartFileRename(String),
    StartFileCreate { dir: String, entry: NewEntry },
    RequestFileDelete { path: String, is_dir: bool },
    ShowProperties(String),
    StageFile { path: String, mode: ClipMode },
    PasteFiles { dir: String },
    SelectFile(String),
    Send(Command),
}
