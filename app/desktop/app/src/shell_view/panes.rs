use std::rc::Rc;

use gpui::Entity;
use layout_view::LayoutView;
use tab_bar::TabBar;

use crate::shell_pane_source::ShellPaneSource;
use crate::workspace::Workspace;

pub(super) struct Panes {
    pub(super) layout: Entity<LayoutView>,
    pub(super) tabs: Entity<TabBar>,
    pub(super) workspace: Entity<Workspace>,
    pub(super) source: Rc<ShellPaneSource>,
}
