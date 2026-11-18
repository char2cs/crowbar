//! A space.

use crate::shell::chats::Mark;
use crate::shell::daemons::HostId;
use crate::shell::switcher::project_id::ProjectId;

/// A space: a named group of repositories on one host.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Project {
    /// Identity.
    pub id: ProjectId,
    /// The name shown.
    pub name: String,
    /// The icon.
    pub mark: Mark,
    /// The host it lives on.
    pub host: HostId,
}
