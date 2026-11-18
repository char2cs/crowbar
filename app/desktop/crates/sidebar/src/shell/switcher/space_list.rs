//! The spaces and hosts the sidebar lists.

use crate::shell::daemons::Host;
use crate::shell::switcher::project::Project;
use crate::shell::switcher::scope_sync::ScopeSync;

/// The spaces and hosts the sidebar lists.
pub trait SpaceList {
    /// Whether the projects scope has synced.
    fn sync(&self) -> ScopeSync;
    /// Every known host.
    fn hosts(&self) -> Vec<Host>;
    /// The spaces of the selected host, in switcher order.
    fn projects(&self) -> Vec<Project>;
}
