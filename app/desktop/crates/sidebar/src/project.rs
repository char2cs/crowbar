//! A space.

use crate::host_id::HostId;
use crate::mark::Mark;
use crate::project_id::ProjectId;

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
