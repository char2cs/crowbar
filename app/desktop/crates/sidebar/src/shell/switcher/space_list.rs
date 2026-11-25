//! The spaces the sidebar lists.

use crate::shell::switcher::project::Project;
use crate::shell::switcher::scope_sync::ScopeSync;

/// The spaces the sidebar lists.
pub trait SpaceList {
    /// Whether the projects scope has synced.
    fn sync(&self) -> ScopeSync;
    /// The spaces of the selected host, in switcher order.
    fn projects(&self) -> Vec<Project>;
}
