use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{Entity, Subscription};
use layoutview::LayoutView;
use sidebarview::Sidebar;
use ui::ToastQueue;
use viewtabs::TabBar;

use crate::panes::content::shell_pane_source::ShellPaneSource;
use crate::shell::shell_view::assembly::pane_area::PaneArea;
use crate::shell::shell_view::assembly::workspace::Workspace;

pub(crate) struct Built {
    pub(crate) data: Rc<store::Store>,
    pub(crate) data_queued: Arc<AtomicBool>,
    pub(crate) layout: Entity<LayoutView>,
    pub(crate) tabs: Entity<TabBar>,
    pub(crate) workspace: Entity<Workspace>,
    pub(crate) source: Rc<ShellPaneSource>,
    pub(crate) sidebar: Entity<Sidebar>,
    pub(crate) startup: Entity<startup::View>,
    pub(crate) notices: Entity<ToastQueue>,
    pub(crate) pane_area: PaneArea,
    pub(crate) subscriptions: Vec<Subscription>,
}
