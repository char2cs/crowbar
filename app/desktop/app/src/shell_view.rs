use std::path::PathBuf;
use std::rc::Rc;

use gpui::{
    AppContext as _, Context, Entity, IntoElement, ParentElement, Render, Styled, Subscription,
    Window, div,
};
use layout_view::{ChromeContext, LayoutEvent, LayoutView};
use pane_host::{IpcMessage, Overlay};
use settings_ui::SettingsModal;
use sidebar::{DemoData, Preferences, Sidebar, SidebarData, SidebarEvent};
use startup_view::{MonotonicClock, StartupEvent, StartupInput, StartupTiming, StartupView};
use tab_bar::{MemberView, SidebarState, TabBar, TabBarEvent, TabView};
use ui::{Overlays, Surface, Theme};

use crate::chat_label::NEW_CHAT;
use crate::daemon_link::DaemonLink;
use crate::holes::visible_panes;
use crate::pane_source::ShellPaneSource;
use crate::request_sinks::{OverlayRequests, SidebarRequests, StartupRequests};
use crate::settings_modal::open_settings;
use crate::shell_outcome::Outcome;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;
use crate::workspace::{Workspace, WorkspaceEvent};

/// Everything the root view is built from.
pub(crate) struct ShellParts {
    pub(crate) state: ShellState,
    pub(crate) link: Rc<DaemonLink>,
    pub(crate) preferences_path: PathBuf,
}

/// The window's root view: the sidebar around the workspace, with the startup view and the
/// settings modal layered over it.
pub(crate) struct ShellView {
    state: ShellState,
    link: Rc<DaemonLink>,
    preferences_path: PathBuf,
    sidebar: Entity<Sidebar>,
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    startup: Option<Entity<StartupView>>,
    settings: Option<Entity<SettingsModal>>,
    requests: async_channel::Sender<ShellRequest>,
    placement: Option<(webview::ParentWindow, webview::Bounds)>,
    overlays_open: bool,
    _subscriptions: Vec<Subscription>,
}

impl ShellView {
    pub(crate) fn new(parts: ShellParts, window: &mut Window, cx: &mut Context<'_, Self>) -> Self {
        let ShellParts {
            state,
            link,
            preferences_path,
        } = parts;
        let (requests, queue) = async_channel::unbounded();
        Overlays::install(Rc::new(OverlayRequests::new(requests.clone())), cx);
        let source = Rc::new(ShellPaneSource::new(state.titles(), state.serials()));
        let layout = cx.new(|_| LayoutView::new(source));
        let first_tab = state.serials().next_tab();
        let tabs = cx.new(|_| TabBar::new(first_tab));
        let workspace = cx.new(|_| Workspace::new(tabs.clone(), layout.clone()));
        let sidebar = build_sidebar(&requests, &preferences_path, &workspace, cx);
        let startup = build_startup(&requests, cx);
        let subscriptions = vec![
            cx.subscribe_in(&workspace, window, |this, _, event, window, cx| {
                this.on_workspace(event, window, cx);
            }),
            cx.subscribe(&tabs, Self::on_tabs),
            cx.subscribe(&layout, Self::on_layout),
            cx.subscribe(&sidebar, Self::on_sidebar),
            cx.subscribe(&startup, Self::on_startup),
        ];
        let mut view = Self {
            state,
            link,
            preferences_path,
            sidebar,
            tabs,
            layout,
            startup: Some(startup),
            settings: None,
            requests,
            placement: None,
            overlays_open: false,
            _subscriptions: subscriptions,
        };
        view.spawn_tasks(queue, window, cx);
        view.refresh(cx);
        view
    }

    fn spawn_tasks(
        &self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let inbox = self.state.host().inbox();
        let phases = self.link.phases();
        cx.spawn_in(window, async move |this, cx| {
            drain(inbox, this, cx, Self::on_ipc).await;
        })
        .detach();
        cx.spawn_in(window, async move |this, cx| {
            drain(phases, this, cx, Self::feed_startup).await;
        })
        .detach();
        cx.spawn_in(window, async move |this, cx| {
            drain(queue, this, cx, Self::handle).await;
        })
        .detach();
    }

    fn on_ipc(&mut self, message: IpcMessage, _: &mut Window, cx: &mut Context<'_, Self>) {
        let outcome = self.state.handle_ipc(message);
        self.apply_outcome(outcome, cx);
    }

    fn feed_startup(&mut self, input: StartupInput, _: &mut Window, cx: &mut Context<'_, Self>) {
        if let Some(startup) = &self.startup {
            startup.update(cx, |startup, cx| startup.apply(input, cx));
        }
    }

    fn handle(&mut self, request: ShellRequest, window: &mut Window, cx: &mut Context<'_, Self>) {
        match request {
            ShellRequest::Sidebar(command) => self.on_sidebar_command(&command, window, cx),
            ShellRequest::Startup(command) => {
                crate::startup_command::perform(command, &self.link, cx);
            },
            ShellRequest::CloseSettings => {
                self.settings = None;
                self.update_overlay(cx);
            },
            ShellRequest::Overlays { open } => {
                self.overlays_open = open;
                self.update_overlay(cx);
            },
        }
    }

    fn on_sidebar_command(
        &mut self,
        command: &sidebar::Command,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        use crate::sidebar_action::{SidebarAction, action_for};
        match action_for(command) {
            Ok(SidebarAction::Open(type_id, params)) => {
                self.state.open_pane(type_id, params);
                self.refresh(cx);
            },
            Ok(SidebarAction::OpenSettings) => self.show_settings(window, cx),
            Ok(SidebarAction::Unserved) => {
                tracing::warn!(
                    ?command,
                    "the sidebar command needs daemon data the shell does not read yet"
                );
            },
            Err(error) => tracing::warn!(%error, "a sidebar command was invalid"),
        }
    }

    fn show_settings(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) {
        match open_settings(self.requests.clone(), window, cx) {
            Ok(modal) => {
                self.settings = Some(modal);
                self.update_overlay(cx);
            },
            Err(error) => tracing::error!(%error, "could not open settings"),
        }
    }

    fn update_overlay(&mut self, cx: &mut Context<'_, Self>) {
        let covered = self.startup.is_some() || self.settings.is_some() || self.overlays_open;
        self.state.set_overlay(if covered {
            Overlay::Active
        } else {
            Overlay::Inactive
        });
        cx.notify();
    }

    fn on_workspace(
        &mut self,
        event: &WorkspaceEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let WorkspaceEvent::Measured(bounds) = event;
        match webview::ParentWindow::from_window(window) {
            Ok(parent) => {
                self.placement = Some((parent, crate::pixels::to_bounds(*bounds)));
                self.sync(cx);
            },
            Err(error) => tracing::error!(%error, "the window cannot host web views"),
        }
    }

    fn on_tabs(&mut self, _: Entity<TabBar>, event: &TabBarEvent, cx: &mut Context<'_, Self>) {
        match event {
            TabBarEvent::Apply(commands) => {
                self.state.apply(commands.clone());
                self.refresh(cx);
            },
            TabBarEvent::ToggleSidebar => self.sidebar.update(cx, Sidebar::toggle),
            _ => {},
        }
    }

    fn on_layout(
        &mut self,
        _: Entity<LayoutView>,
        event: &LayoutEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let LayoutEvent::Apply(command) = event else {
            return;
        };
        self.state.apply(vec![command.clone()]);
        self.refresh(cx);
    }

    #[expect(
        clippy::trivially_copy_pass_by_ref,
        reason = "gpui subscription handlers receive the event by reference"
    )]
    fn on_sidebar(&mut self, _: Entity<Sidebar>, event: &SidebarEvent, cx: &mut Context<'_, Self>) {
        if !matches!(event, SidebarEvent::PreferencesChanged) {
            return;
        }
        let preferences = self.sidebar.read(cx).preferences();
        if let Err(error) = preferences.save(&self.preferences_path) {
            tracing::warn!(%error, "saving the sidebar preferences failed");
        }
        self.refresh(cx);
    }

    #[expect(
        clippy::trivially_copy_pass_by_ref,
        reason = "gpui subscription handlers receive the event by reference"
    )]
    fn on_startup(
        &mut self,
        _: Entity<StartupView>,
        event: &StartupEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if !matches!(event, StartupEvent::HandedOver) {
            return;
        }
        self.startup = None;
        self.update_overlay(cx);
    }

    fn chrome(&self, cx: &Context<'_, Self>) -> (ChromeContext, SidebarState) {
        let preferences = self.sidebar.read(cx).preferences();
        crate::chrome::of(&preferences)
    }

    /// Pushes the layout into the tab strip and the layout view, then follows it with the
    /// web views.
    fn refresh(&mut self, cx: &mut Context<'_, Self>) {
        let (chrome, sidebar_state) = self.chrome(cx);
        let titles = self.state.titles();
        let layout = self.state.layout();
        let tabs: Vec<TabView> = layout
            .tabs()
            .iter()
            .map(|tab| TabView::from_tab(tab, |pane| member(&titles, pane)))
            .collect();
        let active = layout.active_tab().map(model::Tab::id);
        let tab = layout.active_tab().cloned();
        self.tabs.update(cx, |bar, cx| {
            bar.set_tabs(tabs, active, cx);
            bar.set_sidebar(sidebar_state, cx);
        });
        self.layout.update(cx, |view, cx| {
            view.set_chrome(chrome, cx);
            view.set_tab(tab, cx);
        });
        self.sync(cx);
    }

    fn sync(&mut self, cx: &mut Context<'_, Self>) {
        let Some((parent, area)) = self.placement else {
            return;
        };
        let (chrome, _) = self.chrome(cx);
        let visible = visible_panes(self.state.layout().active_tab(), area, chrome);
        let outcome = self.state.sync(parent, visible);
        self.apply_outcome(outcome, cx);
    }

    fn apply_outcome(&mut self, outcome: Outcome, cx: &mut Context<'_, Self>) {
        let Outcome {
            titles_changed,
            hover,
            commands,
        } = outcome;
        for (pane, hovered) in hover {
            let hovered = hovered.then_some(pane);
            self.layout
                .update(cx, |view, cx| view.set_hovered_pane(hovered, cx));
        }
        for command in commands {
            tracing::debug!(%command, "a pane ran a shell command the shell does not serve yet");
        }
        if titles_changed {
            self.refresh(cx);
        }
    }
}

fn build_sidebar(
    requests: &async_channel::Sender<ShellRequest>,
    preferences_path: &std::path::Path,
    workspace: &Entity<Workspace>,
    cx: &mut Context<'_, ShellView>,
) -> Entity<Sidebar> {
    cx.new(|cx| {
        let data = Rc::new(DemoData::reference());
        let sink = Rc::new(SidebarRequests(requests.clone()));
        let preferences = load_preferences(preferences_path);
        Sidebar::new(
            data as Rc<dyn SidebarData>,
            sink,
            workspace.clone().into(),
            (preferences, cx),
        )
    })
}

fn build_startup(
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

fn member(titles: &crate::titles::Titles, pane: &protocol::PaneId) -> MemberView {
    let title = titles.get(pane);
    MemberView {
        pane: pane.clone(),
        label: title
            .as_ref()
            .map_or_else(|| NEW_CHAT.into(), |title| title.to_string().into()),
        provisional: title.is_none(),
        working: false,
    }
}

type Step<T> = fn(&mut ShellView, T, &mut Window, &mut Context<'_, ShellView>);

/// Feeds every item of `source` to `step` on the main thread until either side closes.
async fn drain<T>(
    source: async_channel::Receiver<T>,
    this: gpui::WeakEntity<ShellView>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<T>,
) {
    while let Ok(item) = source.recv().await {
        if this
            .update_in(cx, |view, window, cx| step(view, item, window, cx))
            .is_err()
        {
            return;
        }
    }
}

fn load_preferences(path: &std::path::Path) -> Preferences {
    Preferences::load(path).unwrap_or_else(|error| {
        tracing::warn!(%error, "sidebar preferences unreadable, using defaults");
        Preferences::default()
    })
}

impl Render for ShellView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .size_full()
            .relative()
            .bg(palette.background())
            .child(self.sidebar.clone())
            .children(
                self.settings
                    .clone()
                    .map(|modal| div().absolute().size_full().child(modal)),
            )
            .children(
                self.startup
                    .clone()
                    .map(|startup| div().absolute().size_full().child(startup)),
            )
    }
}

#[cfg(test)]
#[path = "shell_view_tests.rs"]
mod tests;
