use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{AppContext as _, Context, Entity, Subscription, Window};
use layout_view::LayoutView;
use shell_data::ShellData;
use sidebar::{Preferences, Sidebar, SidebarData};
use startup_view::{MonotonicClock, StartupTiming, StartupView};
use tab_bar::TabBar;
use terminal_session::TerminalSessions;
use ui::Overlays;

use super::ShellView;
use super::built::Built;
use crate::browser_bars::BrowserBars;
use crate::chat_terminals::ChatTerminals;
use crate::daemon_link::DaemonLink;
use crate::overlay_requests::OverlayRequests;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;
use crate::sidebar_requests::SidebarRequests;
use crate::startup_requests::StartupRequests;
use crate::terminal_panes::TerminalPanes;
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
) -> (Rc<ShellData>, Arc<AtomicBool>) {
    let queued = Arc::new(AtomicBool::new(false));
    let callbacks = crate::data_signal::callbacks(requests, &queued);
    let data = ShellData::start(
        &link.handle(),
        &link.transport(),
        (link.hosts(), link.desk()),
        callbacks,
    );
    (Rc::new(data), queued)
}

fn terminal_sessions(link: &DaemonLink) -> TerminalSessions {
    let (transport, host) = (link.transport(), link.local_host());
    TerminalSessions::new(transport, host, link.handle(), shell_data::CORE_ORIGIN)
}

pub(super) type Panes = (
    Entity<LayoutView>,
    Entity<TabBar>,
    Entity<Workspace>,
    Rc<ShellPaneSource>,
);

pub(super) fn build_panes(
    (state, link, data): (&ShellState, &DaemonLink, &Rc<ShellData>),
    requests: &async_channel::Sender<ShellRequest>,
    cx: &mut Context<'_, ShellView>,
) -> Panes {
    let bars = BrowserBars::new(state.browsers(), requests.clone());
    let terminals = TerminalPanes::new(
        state.terminals(),
        (terminal_sessions(link), ChatTerminals::of(data)),
    );
    let source = Rc::new(ShellPaneSource::new(
        state.titles(),
        state.serials(),
        bars,
        (
            terminals,
            crate::new_tab_panes::NewTabPanes::new(requests.clone()),
        ),
    ));
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

pub(super) fn build_children(
    (state, link, preferences_path): (&ShellState, &DaemonLink, &std::path::Path),
    requests: &async_channel::Sender<ShellRequest>,
    window: &mut Window,
    cx: &mut Context<'_, ShellView>,
) -> Built {
    let (data, data_queued) = start_data(link, requests);
    let (layout, tabs, workspace, source) = build_panes((state, link, &data), requests, cx);
    let sidebar = build_sidebar((requests, &data), preferences_path, &workspace, cx);
    let startup = build_startup(requests, cx);
    let notices = super::notices::build_notices(&sidebar, cx);
    let subscriptions = subscribe_all((&workspace, &tabs, &layout, &sidebar, &startup), window, cx);
    Built {
        data,
        data_queued,
        layout,
        tabs,
        source,
        sidebar,
        startup,
        notices,
        subscriptions,
    }
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
