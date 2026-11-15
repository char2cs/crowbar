//! What a click on a menu entry or row button means.

use crate::command::Command;
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
    Send(Command),
}
