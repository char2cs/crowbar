use gpui::Entity;

use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::ports::preferences_store::PreferencesStore;
use crate::shell::shell_view::assembly::workspace::Workspace;

pub(crate) struct SidebarWiring<'a> {
    pub(crate) requests: &'a async_channel::Sender<ShellRequest>,
    pub(crate) data: &'a store::Store,
    pub(crate) preferences: &'a dyn PreferencesStore,
    pub(crate) workspace: &'a Entity<Workspace>,
}
