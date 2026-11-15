use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{AppContext as _, Context, Entity, Subscription, Window};
use layout_view::LayoutView;
use shell_data::ShellData;
use sidebar::{Preferences, Sidebar, SidebarData};
use startup_view::{MonotonicClock, StartupTiming, StartupView};
use tab_bar::TabBar;
use ui::Overlays;

use super::ShellView;
use crate::browser_bars::BrowserBars;
use crate::daemon_link::DaemonLink;
use crate::overlay_requests::OverlayRequests;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;
use crate::sidebar_requests::SidebarRequests;
use crate::startup_requests::StartupRequests;
use crate::workspace::Workspace;

pub(super) fn overlay_channel(
    cx: &mut Context<'_, ShellView>
) -> (
    async_channel::Sender<ShellRequest>,
    async_channel::Receiver<ShellRequest>,
) {
    let (requests, queue) = async_channel::unbounded();
    Overlays::install(Rc::new(OverlayRequests::new(requests.clone())), cx);
    (requests, queue)
}

pub(super) fn start_data(
    link: &DaemonLink,
    requests: &async_channel::Sender<ShellRequest>,
) -> (ShellData, Arc<AtomicBool>) {
    let queued = Arc::new(AtomicBool::new(false));
    let callbacks = crate::data_signal::callbacks(requests, &queued);
    let data = ShellData::start(&link.handle(), &link.transport(), link.hosts(), callbacks);
    (data, queued)
}

pub(super) type Panes = (
    Entity<LayoutView>,
    Entity<TabBar>,
    Entity<Workspace>,
    Rc<ShellPaneSource>,
);

pub(super) fn build_panes(
    state: &ShellState,
    requests: &async_channel::Sender<ShellRequest>,
    cx: &mut Context<'_, ShellView>,
) -> Panes {
    let bars = BrowserBars::new(state.browsers(), requests.clone());
    let source = Rc::new(ShellPaneSource::new(state.titles(), state.serials(), bars));
    let view_source = Rc::clone(&source);
    let layout = cx.new(|_| LayoutView::new(view_source));
    let first_tab = state.serials().next_tab();
    let tabs = cx.new(|_| TabBar::new(first_tab));
    let workspace = cx.new(|_| Workspace::new(tabs.clone(), layout.clone()));
    (layout, tabs, workspace, source)
}

pub(super) fn build_sidebar(
    (requests, data): (&async_channel::Sender<ShellRequest>, &ShellData),
    preferences_path: &std::path::Path,
    workspace: &Entity<Workspace>,
    cx: &mut Context<'_, ShellView>,
) -> Entity<Sidebar> {
    let view = Rc::new(data.view());
    let sink = Rc::new(SidebarRequests {
        requests: requests.clone(),
        data: data.commands(),
    });
    let preferences = load_preferences(preferences_path);
    cx.new(|cx| {
        Sidebar::new(
            view as Rc<dyn SidebarData>,
            sink,
            workspace.clone().into(),
            (preferences, cx),
        )
    })
}

pub(super) fn build_startup(
    requests: &async_channel::Sender<ShellRequest>,
    cx: &mut Context<'_, ShellView>,
) -> Entity<StartupView> {
    cx.new(|cx| {
        let actions = Rc::new(StartupRequests(requests.clone()));
        StartupView::new(
            StartupTiming::default(),
            Rc::new(MonotonicClock::start()),
            actions,
            cx,
        )
    })
}

fn load_preferences(path: &std::path::Path) -> Preferences {
    Preferences::load(path).unwrap_or_else(|error| {
        tracing::warn!(%error, "sidebar preferences unreadable, using defaults");
        Preferences::default()
    })
}

type Children<'a> = (
    &'a Entity<Workspace>,
    &'a Entity<TabBar>,
    &'a Entity<LayoutView>,
    &'a Entity<Sidebar>,
    &'a Entity<StartupView>,
);

pub(super) fn subscribe_all(
    (workspace, tabs, layout, sidebar, startup): Children<'_>,
    window: &mut Window,
    cx: &mut Context<'_, ShellView>,
) -> Vec<Subscription> {
    vec![
        cx.subscribe_in(workspace, window, |this, _, event, window, cx| {
            this.on_workspace(event, window, cx);
        }),
        cx.subscribe(tabs, ShellView::on_tabs),
        cx.subscribe(layout, ShellView::on_layout),
        cx.subscribe(sidebar, ShellView::on_sidebar),
        cx.subscribe_in(startup, window, |this, _, event, window, cx| {
            this.on_startup(event, window, cx);
        }),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn unreadable_or_missing_preferences_fall_back_to_the_defaults() {
        let dir = tempfile::tempdir().unwrap();
        let corrupt = dir.path().join("corrupt.json");
        std::fs::write(&corrupt, "{ not json").unwrap();
        for path in [corrupt, dir.path().join("missing.json")] {
            assert_eq!(load_preferences(&path), Preferences::default(), "{path:?}");
        }
    }
}
