use crate::shell::chats::RowId;
use crate::shell::switcher::project_id::ProjectId;

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum IconTarget {
    Space(ProjectId),
    Repo(RowId),
}
