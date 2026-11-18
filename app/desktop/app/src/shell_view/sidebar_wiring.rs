use std::path::Path;

use gpui::Entity;

use crate::shell_request::ShellRequest;
use crate::workspace::Workspace;

pub(super) struct SidebarWiring<'a> {
    pub(super) requests: &'a async_channel::Sender<ShellRequest>,
    pub(super) data: &'a store::Store,
    pub(super) preferences_path: &'a Path,
    pub(super) workspace: &'a Entity<Workspace>,
}
