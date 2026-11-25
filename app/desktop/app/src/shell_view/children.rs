use std::rc::Rc;
use std::sync::Arc;

use gpui::{AppContext as _, Context, Entity};
use layoutview::LayoutView;
use ptysession::TerminalSessions;
use sidebar::{FilesSource as _, Preferences};
use sidebarview::Sidebar;
use viewtabs::TabBar;

use super::built::Built;
use super::pane_wiring::PaneWiring;
use super::panes::Panes;
use super::sidebar_wiring::SidebarWiring;
use crate::backend::ProjectsFailed;
use crate::browser_bars::BrowserBars;
use crate::chat_hosts::ChatHosts;
use crate::daemon_link::DaemonLink;
use crate::native_panes::NativePanes;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_request::ShellRequest;
use crate::sidebar_requests::SidebarRequests;
use crate::startup_requests::StartupRequests;
use crate::terminal_panes::TerminalPanes;
use crate::workspace::Workspace;

fn terminal_sessions(link: &DaemonLink) -> TerminalSessions {
    let (transport, host) = (link.transport(), link.local_host());
    TerminalSessions::new(transport, host, link.handle(), daemonapi::CORE_ORIGIN)
        .once(link.readiness())
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
        drafts,
    } = wiring;
    let bars = BrowserBars::new(state.browsers(), (*requests).clone());
    let terminals = TerminalPanes::new(
        state.terminals(),
        (terminal_sessions(link), ChatHosts::of(data)),
        (*requests).clone(),
    );
    let new_tab = crate::new_tab_panes::NewTabPanes::new((*requests).clone());
    let editors = editors_of((data, drafts), requests);
    let panes = NativePanes::new(terminals, new_tab, editors);
    let source = ShellPaneSource::new(state.titles(), state.serials(), bars, panes);
    let (reviews, locks) = (Rc::clone(data), Rc::clone(data));
    let source = source
        .marking(state.dirty())
        .drawing(glyphs_of(data))
        .naming(names_of(data))
        .reviewing(Rc::new(move |chat| reviews.chat_reviews_branch(chat)))
        .locking(Rc::new(move |chat| locks.chat_read_only(chat)))
        .iconing(icons_of(data));
    let first_tab = state.serials().next_tab();
    let tabs = cx.new(|_| TabBar::new(first_tab));
    let source = Rc::new(source.previewing(previews_of(&tabs)));
    let view_source = Rc::clone(&source);
    let layout = cx.new(|_| LayoutView::new(view_source));
    let workspace = workspace_of((&tabs, &layout), data, cx);
    Panes {
        layout,
        tabs,
        workspace,
        source,
    }
}

fn workspace_of<V: 'static>(
    (tabs, layout): (&Entity<TabBar>, &Entity<LayoutView>),
    data: &Rc<store::Store>,
    cx: &mut Context<'_, V>,
) -> Entity<Workspace> {
    let (retrying, reading) = (Rc::clone(data), Rc::clone(data));
    let landing = cx.new(|_| ProjectsFailed::new(Rc::new(move || retrying.retry_projects())));
    let failing = Rc::new(move || reading.projects_load() == store::ProjectsLoad::Failed);
    let panes = (tabs.clone(), layout.clone());
    cx.new(|_| Workspace::new(panes, landing, failing))
}

fn previews_of(tabs: &Entity<TabBar>) -> crate::shell_pane_source::PanePreviews {
    let bar = tabs.downgrade();
    Rc::new(move |pane, cx| bar.upgrade()?.read(cx).preview(pane))
}

fn editors_of(
    (data, drafts): (&Rc<store::Store>, &std::path::Path),
    requests: &async_channel::Sender<ShellRequest>,
) -> crate::editor_panes::EditorPanes {
    let documents: Rc<dyn crate::document_source::DocumentSource> = Rc::new(data.documents());
    let (locks, workspaces) = (Rc::clone(data), Rc::clone(data));
    let servers = data.language_servers();
    let chats = crate::editor_panes::ChatReads {
        locks: Rc::new(move |chat| locks.chat_read_only(chat)),
        workspaces: Rc::new(move |chat| workspaces.chat_workspace(chat)),
        servers: crate::pane_language_server::PaneLanguageServer::per_chat(servers),
    };
    let file = editor::DiskDraftFile::at(drafts.to_path_buf());
    let (drafts, unread) = editor::Drafts::open(Box::new(file));
    if let Some(error) = unread {
        tracing::warn!(%error, "the unsaved drafts of the last run could not be read");
    }
    crate::editor_panes::EditorPanes::new(documents, chats, drafts, requests.clone())
}

fn icons_of(data: &Rc<store::Store>) -> crate::shell_pane_source::FileIcons {
    let view = data.view();
    Rc::new(move || view.file_tree_view().icons)
}

fn names_of(data: &Rc<store::Store>) -> crate::shell_pane_source::ChatNames {
    let data = Rc::clone(data);
    Rc::new(move |chat| data.chat_row(chat).map(|row| row.label))
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
    let preferences = preferences_or_default(wiring.preferences.load());
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
    cx.new(|_| startup::View::new(Rc::new(StartupRequests(requests.clone()))))
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
    let data = Rc::clone(&parts.data);
    let drafts = parts
        .preferences_path
        .with_file_name(crate::editor_panes::DRAFTS_FILE);
    let panes = build_panes(
        &PaneWiring {
            state: &parts.state,
            link,
            data: &data,
            requests,
            drafts: &drafts,
        },
        cx,
    );
    let sidebar = build_sidebar(
        &SidebarWiring {
            requests,
            data: &data,
            preferences: parts.ports.preferences.as_ref(),
            workspace: &panes.workspace,
        },
        cx,
    );
    let startup = build_startup(requests, cx);
    let notices = sidebar.read(cx).toast_queue();
    Built {
        data,
        data_queued: Arc::clone(&parts.data_queued),
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
