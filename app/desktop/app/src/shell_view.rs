use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use command_palette::PalettePanel;
use gpui::{
    Context, Entity, FocusHandle, InteractiveElement, IntoElement, ParentElement, Render, Styled,
    Subscription, Window, div,
};
use layout_view::{ChromeContext, LayoutEvent, LayoutView};
use pane_host::{IpcMessage, Overlay};
use settings_ui::SettingsModal;
use shell_data::{SettingsAdapter, ShellData};
use shell_keys::Keymap;
use sidebar::{Sidebar, SidebarEvent};
use startup_view::{StartupEvent, StartupInput, StartupView};
use tab_bar::{SidebarState, TabBar, TabBarEvent, TabView};
use ui::{Surface, Theme, ToastKind};

mod appearance_settings;
mod browsing;
mod children;
mod data_events;
mod file_actions;
mod keymap_settings;
mod notices;
mod outcomes;
mod pane_commands;
mod pump;
mod remote_folder;
mod shell_commands;
mod sidebar_position;

use crate::daemon_link::DaemonLink;
use crate::focused_chat::focused_chat;
use crate::holes::visible_panes;
use crate::hover_tracker::HoverTracker;
use crate::notice_key::NoticeKey;
use crate::notices::Notices;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_parts::ShellParts;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;
use crate::workspace_event::WorkspaceEvent;

use self::children::{
    build_panes, build_sidebar, build_startup, overlay_channel, start_data, subscribe_all,
};
use self::pump::{drain, member};

const WINDOW_CANNOT_HOST: &str = "This window cannot show panes: it is not an X11 window. Restart Crowbar under X11 or XWayland.";

pub(crate) struct ShellView {
    state: ShellState,
    link: Rc<DaemonLink>,
    preferences_path: PathBuf,
    sidebar: Entity<Sidebar>,
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    startup: Option<Entity<StartupView>>,
    settings: Option<Entity<SettingsModal>>,
    palette: Option<Entity<PalettePanel>>,
    branch_import: Option<Entity<branch_import_view::BranchImportView>>,
    remote_folders: Option<Entity<remote_folder_picker::RemoteFolderPicker>>,
    file_prompt: Option<Entity<crate::file_prompt::FilePrompt>>,
    pairing: Option<Entity<pairing_view::PairingView>>,
    source: Rc<ShellPaneSource>,
    focus: FocusHandle,
    keymap: Keymap,
    keymap_watch: Option<settings_ui::Subscription>,
    appearance_watch: Option<settings_ui::Subscription>,
    family_names: crate::family_names::FamilyNames,
    system_fonts: shell_data::SystemFonts,
    requests: async_channel::Sender<ShellRequest>,
    data: ShellData,
    data_queued: Arc<AtomicBool>,
    settings_backend: Option<SettingsAdapter>,
    placement: Option<(webview::ParentWindow, webview::Bounds)>,
    overlays_open: bool,
    hover: HoverTracker,
    notices: Notices,
    subscriptions: Vec<Subscription>,
}

fn warn_unserved(command: &sidebar::Command) {
    tracing::warn!(
        ?command,
        "the sidebar command needs daemon data the shell does not read yet"
    );
}

impl ShellView {
    pub(crate) fn new(
        ShellParts {
            state,
            link,
            preferences_path,
        }: ShellParts,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let (requests, queue) = overlay_channel(cx);
        let (layout, tabs, workspace, source) = build_panes((&state, &link), &requests, cx);
        let (data, data_queued) = start_data(&link, &requests);
        let sidebar = build_sidebar((&requests, &data), &preferences_path, &workspace, cx);
        let startup = build_startup(&requests, cx);
        let subscriptions =
            subscribe_all((&workspace, &tabs, &layout, &sidebar, &startup), window, cx);
        let mut view = Self {
            state,
            link,
            preferences_path,
            sidebar,
            tabs,
            layout,
            source,
            startup: Some(startup),
            settings: None,
            palette: None,
            branch_import: None,
            remote_folders: None,
            file_prompt: None,
            pairing: None,
            focus: cx.focus_handle(),
            keymap: Keymap::defaults(),
            keymap_watch: None,
            appearance_watch: None,
            family_names: crate::family_names::FamilyNames::default(),
            system_fonts: shell_data::SystemFonts::default(),
            requests,
            data,
            data_queued,
            settings_backend: None,
            placement: None,
            overlays_open: false,
            hover: HoverTracker::default(),
            notices: Notices::default(),
            subscriptions,
        };
        view.begin(queue, window, cx);
        view
    }

    fn begin(
        &mut self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        window.focus(&self.focus);
        self.system_fonts = crate::installed_fonts::installed(cx);
        self.announce_start();
        self.spawn_tasks(queue, window, cx);
        self.refresh(cx);
        self.watch_keymap();
        self.watch_appearance(cx);
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

    fn on_ipc(
        &mut self,
        message: IpcMessage,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let outcome = self.state.handle_ipc(message);
        self.apply_outcome(outcome, cx);
    }

    fn feed_startup(
        &mut self,
        input: StartupInput,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match &self.startup {
            Some(startup) => startup.update(cx, |startup, cx| startup.apply(input, cx)),
            None => self.track_daemon(&input, cx),
        }
    }

    fn handle(
        &mut self,
        request: ShellRequest,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match request {
            ShellRequest::Sidebar(command) => self.on_sidebar_command(&command, window, cx),
            ShellRequest::Startup(command) => {
                crate::startup_command::perform(command, &self.link, cx);
            },
            ShellRequest::DataChanged => self.on_data_changed(cx),
            ShellRequest::Outcome(outcome) => self.on_data_outcome(outcome, window, cx),
            ShellRequest::CloseSettings => {
                self.settings = None;
                window.focus(&self.focus);
                self.update_overlay(cx);
            },
            ShellRequest::KeymapChanged => self.on_keymap_changed(cx),
            ShellRequest::AppearanceChanged => self.apply_appearance(cx),
            ShellRequest::Browse { pane, event } => self.browse(&pane, event, cx),
            ShellRequest::Command(command) => self.run_shell_command(command, window, cx),
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
                self.open_from_sidebar((type_id, params), cx);
            },
            Ok(SidebarAction::OpenSettings) => self.show_settings(window, cx),
            Ok(SidebarAction::OpenFile(path)) => self.open_workspace_file(&path, cx),
            Ok(SidebarAction::ImportRepo) => self.choose_repo_folder(window, cx),
            Ok(SidebarAction::ImportBranches(row)) => self.show_branch_import(&row, window, cx),
            Ok(SidebarAction::Reveal(path)) => self.reveal_workspace_path(&path),
            Ok(SidebarAction::Unserved) => self.perform_file_action(command, window, cx),
            Err(error) => tracing::warn!(%error, "a sidebar command was invalid"),
        }
    }

    fn open_from_sidebar(
        &mut self,
        (type_id, params): (protocol::PaneTypeId, protocol::Params),
        cx: &mut Context<'_, Self>,
    ) {
        self.state.open_pane(type_id, params);
        self.refresh(cx);
    }

    fn overlay(&self) -> Overlay {
        let covered = self.startup.is_some()
            || self.settings.is_some()
            || self.palette.is_some()
            || self.branch_import.is_some()
            || self.remote_folders.is_some()
            || self.file_prompt.is_some()
            || self.pairing.is_some()
            || self.overlays_open;
        if covered {
            return Overlay::Active;
        }
        Overlay::Inactive
    }

    fn update_overlay(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.state.set_overlay(self.overlay());
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
            Err(error) => {
                tracing::error!(%error, "the window cannot host web views");
                self.show_notice(
                    NoticeKey::Window,
                    (ToastKind::Error, WINDOW_CANNOT_HOST.into()),
                    cx,
                );
            },
        }
    }

    fn on_tabs(
        &mut self,
        _: Entity<TabBar>,
        event: &TabBarEvent,
        cx: &mut Context<'_, Self>,
    ) {
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
    fn on_sidebar(
        &mut self,
        _: Entity<Sidebar>,
        event: &SidebarEvent,
        cx: &mut Context<'_, Self>,
    ) {
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
        event: &StartupEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !matches!(event, StartupEvent::HandedOver) {
            return;
        }
        window.set_window_title(crate::window_memory::WINDOW_TITLE);
        self.startup = None;
        self.update_overlay(cx);
    }

    fn chrome(
        &self,
        cx: &Context<'_, Self>,
    ) -> (ChromeContext, SidebarState) {
        let preferences = self.sidebar.read(cx).preferences();
        crate::chrome::of(&preferences)
    }

    fn refresh(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let (chrome, sidebar_state) = self.chrome(cx);
        let titles = self.state.titles();
        let layout = self.state.layout();
        let tabs: Vec<TabView> = layout
            .tabs()
            .iter()
            .map(|tab| TabView::from_tab(tab, |pane| member((&titles, layout), pane)))
            .collect();
        let active = layout.active_tab().map(model::Tab::id);
        let tab = layout.active_tab().cloned();
        self.data.focus_chat(focused_chat(layout).as_deref());
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

    fn sync(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((parent, area)) = self.placement else {
            return;
        };
        let (chrome, _) = self.chrome(cx);
        let visible = visible_panes(
            self.state.layout().active_tab(),
            area,
            (chrome, self.source.as_ref()),
        );
        self.state.set_theme(*Theme::of(cx));
        let outcome = self.state.sync(parent, visible);
        self.apply_outcome(outcome, cx);
    }
}

impl Render for ShellView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .size_full()
            .track_focus(&self.focus)
            .on_key_down(cx.listener(Self::on_key_down))
            .relative()
            .bg(palette.background())
            .child(self.sidebar.clone())
            .children(
                self.settings
                    .clone()
                    .map(|modal| div().absolute().size_full().child(modal)),
            )
            .children(
                self.palette
                    .clone()
                    .map(|panel| div().absolute().size_full().child(panel)),
            )
            .children(
                self.branch_import
                    .clone()
                    .map(|dialog| div().absolute().size_full().child(dialog)),
            )
            .children(
                self.remote_folders
                    .clone()
                    .map(|dialog| div().absolute().size_full().child(dialog)),
            )
            .children(self.prompt_layer())
            .children(
                self.pairing
                    .clone()
                    .map(|dialog| div().absolute().size_full().child(dialog)),
            )
            .children(
                self.startup
                    .clone()
                    .map(|startup| div().absolute().size_full().child(startup)),
            )
            .children(self.notices_element(cx))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chat_label::NEW_CHAT;

    #[test]
    fn member_labels_fall_back_to_new_chat_until_a_title_arrives() {
        let titles = crate::titles::Titles::default();
        let pane = protocol::PaneId::new("p1").unwrap();
        let before = member((&titles, &model::Layout::default()), &pane);
        assert_eq!(
            (before.label.as_ref(), before.provisional),
            (NEW_CHAT, true),
            "unnamed"
        );
        titles.set(pane.clone(), protocol::PaneTitle::new("Fix").unwrap());
        let after = member((&titles, &model::Layout::default()), &pane);
        assert_eq!(
            (after.label.as_ref(), after.provisional),
            ("Fix", false),
            "named"
        );
    }
}
