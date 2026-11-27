use crate::shell::chats::RowId;
use crate::shell::command::Command;
use crate::shell::command::clip_mode::ClipMode;
use crate::shell::command::new_entry::NewEntry;
use crate::shell::command::question::Question;
use crate::shell::switcher::ProjectId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Intent {
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
    ToggleChangedFold(String),
    Ask(Question),
    Send(Command),
}
