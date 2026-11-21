use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{AppContext as _, Context, Entity};
use layoutview::LayoutView;
use ptysession::TerminalSessions;
use sidebar::Preferences;
use sidebarview::Sidebar;
use startup::MonotonicClock;
use viewtabs::TabBar;

use super::built::Built;
use super::pane_wiring::PaneWiring;
use super::panes::Panes;
use super::sidebar_wiring::SidebarWiring;
use crate::browser_bars::BrowserBars;
use crate::chat_terminals::ChatTerminals;
use crate::daemon_link::DaemonLink;
use crate::native_panes::NativePanes;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_request::ShellRequest;
use crate::sidebar_requests::SidebarRequests;
use crate::startup_requests::StartupRequests;
use crate::terminal_panes::TerminalPanes;
use crate::workspace::Workspace;

pub(super) fn start_data(
    link: &DaemonLink,
    requests: &async_channel::Sender<ShellRequest>,
) -> (Rc<store::Store>, Arc<AtomicBool>) {
    let queued = Arc::new(AtomicBool::new(false));
    let (signal, outcome) = crate::data_signal::callbacks(requests, &queued);
    let wiring = store::Ports {
        desk: link.desk(),
        signal,
        outcome,
    };
    let data = store::Store::start(&link.handle(), &link.transport(), link.hosts(), wiring);
    (Rc::new(data), queued)
}

fn terminal_sessions(link: &DaemonLink) -> TerminalSessions {
    let (transport, host) = (link.transport(), link.local_host());
    TerminalSessions::new(transport, host, link.handle(), daemonapi::CORE_ORIGIN)
}

pub(super) fn build_panes<V: 'static>(
    wiring: &PaneWiring<'_>,
    cx: &mut Context<'_, V>,
) -> Panes {
    let PaneWiring {
        state,
        link,
        data,
        requests,
    } = wiring;
    let bars = BrowserBars::new(state.browsers(), (*requests).clone());
    let terminals = TerminalPanes::new(
        state.terminals(),
        (terminal_sessions(link), ChatTerminals::of(data)),
    );
    let new_tab = crate::new_tab_panes::NewTabPanes::new((*requests).clone());
    let panes = NativePanes::new(terminals, new_tab);
    let source = ShellPaneSource::new(state.titles(), state.serials(), bars, panes);
    let source = source.marking(state.dirty()).drawing(glyphs_of(data));
    let source = Rc::new(source);
    let view_source = Rc::clone(&source);
    let layout = cx.new(|_| LayoutView::new(view_source));
    let first_tab = state.serials().next_tab();
    let tabs = cx.new(|_| TabBar::new(first_tab));
    let workspace = cx.new(|_| Workspace::new(tabs.clone(), layout.clone()));
    Panes {
        layout,
        tabs,
        workspace,
        source,
    }
}

fn glyphs_of(data: &Rc<store::Store>) -> crate::shell_pane_source::ChatGlyphs {
    let data = Rc::clone(data);
    Rc::new(move |chat| data.chat_glyph(chat))
}

pub(super) fn build_sidebar<V: 'static>(
    wiring: &SidebarWiring<'_>,
    cx: &mut Context<'_, V>,
) -> Entity<Sidebar> {
    let view = Rc::new(wiring.data.view());
    let sink = Rc::new(SidebarRequests {
        requests: wiring.requests.clone(),
        data: wiring.data.commands(),
    });
    let preferences = preferences_or_default(Preferences::load(wiring.preferences_path));
    let workspace = wiring.workspace.clone();
    cx.new(|cx| {
        Sidebar::new(
            view as Rc<dyn sidebar::Source>,
            sink,
            workspace.into(),
            (preferences, cx),
        )
    })
}

pub(super) fn build_startup<V: 'static>(
    requests: &async_channel::Sender<ShellRequest>,
    cx: &mut Context<'_, V>,
) -> Entity<startup::View> {
    cx.new(|cx| {
        let actions = Rc::new(StartupRequests(requests.clone()));
        startup::View::new(
            startup::Timing::default(),
            Rc::new(MonotonicClock::start()),
            actions,
            cx,
        )
    })
}

fn preferences_or_default(loaded: Result<Preferences, sidebar::Error>) -> Preferences {
    loaded.unwrap_or_else(|error| {
        tracing::warn!(%error, "sidebar preferences unreadable, using defaults");
        Preferences::default()
    })
}

pub(super) fn build_children<V: 'static>(
    parts: &crate::shell_parts::ShellParts,
    requests: &async_channel::Sender<ShellRequest>,
    cx: &mut Context<'_, V>,
) -> Built {
    let link = &*parts.link;
    let (data, data_queued) = start_data(link, requests);
    let panes = build_panes(
        &PaneWiring {
            state: &parts.state,
            link,
            data: &data,
            requests,
        },
        cx,
    );
    let sidebar = build_sidebar(
        &SidebarWiring {
            requests,
            data: &data,
            preferences_path: parts.preferences_path.as_path(),
            workspace: &panes.workspace,
        },
        cx,
    );
    let startup = build_startup(requests, cx);
    let notices = sidebar.read(cx).toast_queue();
    Built {
        data,
        data_queued,
        layout: panes.layout,
        tabs: panes.tabs,
        workspace: panes.workspace,
        source: panes.source,
        sidebar,
        startup,
        notices,
        subscriptions: Vec::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn unreadable_preferences_fall_back_to_the_defaults() {
        let corrupt = serde_json::from_str::<Preferences>("{ not json").unwrap_err();
        let unreadable = sidebar::Error::ParsePreferences {
            path: "preferences.json".into(),
            source: corrupt,
        };
        let loaded = preferences_or_default(Err(unreadable));
        assert_eq!(loaded, Preferences::default(), "fell back");
    }

    #[test]
    fn readable_preferences_are_kept() {
        let stored = Preferences {
            side: sidebar::DockSide::Right,
            ..Preferences::default()
        };
        assert_eq!(preferences_or_default(Ok(stored.clone())), stored, "kept");
    }
}
