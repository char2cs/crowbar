use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{Entity, Subscription};
use layout_view::LayoutView;
use shell_data::ShellData;
use sidebar::Sidebar;
use startup_view::StartupView;
use tab_bar::TabBar;
use ui::ToastQueue;

use crate::shell_pane_source::ShellPaneSource;
use crate::workspace::Workspace;

pub(super) struct Built {
    pub(super) data: Rc<ShellData>,
    pub(super) data_queued: Arc<AtomicBool>,
    pub(super) layout: Entity<LayoutView>,
    pub(super) tabs: Entity<TabBar>,
    pub(super) workspace: Entity<Workspace>,
    pub(super) source: Rc<ShellPaneSource>,
    pub(super) sidebar: Entity<Sidebar>,
    pub(super) startup: Entity<StartupView>,
    pub(super) notices: Entity<ToastQueue>,
    pub(super) subscriptions: Vec<Subscription>,
}
