//! What an icon popover edits.

use crate::shell::chats::RowId;
use crate::shell::switcher::project_id::ProjectId;

/// The owner of an icon a popover edits: a space, or a repository through its header row.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum IconTarget {
    /// The icon of a space.
    Space(ProjectId),
    /// The icon of the repository a header row shows.
    Repo(RowId),
}
