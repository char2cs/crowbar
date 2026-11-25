use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{Entity, Subscription};
use layoutview::LayoutView;
use sidebarview::Sidebar;
use ui::ToastQueue;
use viewtabs::TabBar;

use crate::panes::content::shell_pane_source::ShellPaneSource;
use crate::workspace::Workspace;

pub(super) struct Built {
    pub(super) data: Rc<store::Store>,
    pub(super) data_queued: Arc<AtomicBool>,
    pub(super) layout: Entity<LayoutView>,
    pub(super) tabs: Entity<TabBar>,
    pub(super) workspace: Entity<Workspace>,
    pub(super) source: Rc<ShellPaneSource>,
    pub(super) sidebar: Entity<Sidebar>,
    pub(super) startup: Entity<startup::View>,
    pub(super) notices: Entity<ToastQueue>,
    pub(super) subscriptions: Vec<Subscription>,
}
