use std::rc::Rc;

use gpui::Entity;
use layoutview::LayoutView;
use viewtabs::TabBar;

use crate::panes::content::shell_pane_source::ShellPaneSource;
use crate::shell::shell_view::assembly::workspace::Workspace;

pub(crate) struct Panes {
    pub(crate) layout: Entity<LayoutView>,
    pub(crate) tabs: Entity<TabBar>,
    pub(crate) workspace: Entity<Workspace>,
    pub(crate) source: Rc<ShellPaneSource>,
}
