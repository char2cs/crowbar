pub(super) mod scope_sync;

use crate::shell::switcher::project::Project;
use crate::shell::switcher::space_list::scope_sync::ScopeSync;

pub trait SpaceList {
    fn sync(&self) -> ScopeSync;
    fn projects(&self) -> Vec<Project>;
}
