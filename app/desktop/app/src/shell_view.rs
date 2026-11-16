use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use command_palette::PalettePanel;
use gpui::{
    Context, Div, Entity, FocusHandle, InteractiveElement, IntoElement, ParentElement, Render,
    Styled, Subscription, Window, div,
};
use layout_view::{ChromeContext, LayoutEvent, LayoutView};
use pane_host::{IpcMessage, Overlay};
use settings_ui::SettingsModal;
use shell_data::{SettingsAdapter, ShellData};
use shell_keys::Keymap;
use sidebar::{Sidebar, SidebarEvent};
use startup_view::{StartupEvent, StartupInput, StartupView};
use tab_bar::{SidebarState, TabBar, TabBarEvent};
use ui::{Surface, Theme, ToastKind, ToastQueue};

mod appearance_settings;
mod browsing;
mod browsing_data;
mod built;
mod children;
mod closing;
mod data_events;
mod evictions;
mod file_actions;
mod file_tree_flag;
mod first_run;
mod focus_follow;
mod keymap_settings;
mod new_tab;
mod notices;
mod outcomes;
mod pane_commands;
mod pane_settings_watch;
mod pick_effect;
mod picking;
mod pump;
mod readout;
mod remote_folder;
mod shell_commands;
mod sidebar_position;
mod tab_strip;

use crate::daemon_link::DaemonLink;
use crate::focused_chat::focused_chat;
use crate::holes::visible_panes;
use crate::hover_tracker::HoverTracker;
use crate::notice_key::NoticeKey;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_parts::ShellParts;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;
use crate::workspace_event::WorkspaceEvent;

use self::children::{build_children, overlay_channel};
use self::pump::drain;

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
    first_run: Option<(crate::landing::Landing, Entity<oobe_view::OobeView>)>,
    window_title: String,
    unsaved: Option<Entity<crate::unsaved_dialog::UnsavedDialog>>,
    pending_close: Option<crate::pending_close::PendingClose>,
    source: Rc<ShellPaneSource>,
    focus: FocusHandle,
    keymap: Keymap,
    keymap_watch: Option<settings_ui::Subscription>,
    appearance_watch: Option<settings_ui::Subscription>,
    pane_settings_watch: Option<settings_ui::Subscription>,
    family_names: crate::family_names::FamilyNames,
    system_fonts: shell_data::SystemFonts,
    requests: async_channel::Sender<ShellRequest>,
    data: Rc<ShellData>,
    data_queued: Arc<AtomicBool>,
    settings_backend: Option<SettingsAdapter>,
    placement: Option<(webview::ParentWindow, webview::Bounds)>,
    overlays_open: bool,
    hover: HoverTracker,
    focus_surface: Option<sidebar::Mode>,
    readout: crate::readout_state::ReadoutState,
    notices: Entity<ToastQueue>,
    subscriptions: Vec<Subscription>,
}

impl ShellView {
    pub(crate) fn new(
        parts: ShellParts,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let (requests, queue) = overlay_channel(cx);
        let built = build_children(&parts, &requests, window, cx);
        let mut view = Self::assemble(parts, built, requests, cx.focus_handle());
        view.begin(queue, window, cx);
        view
    }

    fn assemble(
        parts: ShellParts,
        built: built::Built,
        requests: async_channel::Sender<ShellRequest>,
        focus: FocusHandle,
    ) -> Self {
        Self {
            state: parts.state,
            link: parts.link,
            preferences_path: parts.preferences_path,
            sidebar: built.sidebar,
            tabs: built.tabs,
            layout: built.layout,
            source: built.source,
            startup: Some(built.startup),
            settings: None,
            palette: None,
            branch_import: None,
            remote_folders: None,
            file_prompt: None,
            pairing: None,
            first_run: None,
            unsaved: None,
            window_title: crate::window_memory::WINDOW_TITLE.to_owned(),
            pending_close: None,
            focus,
            keymap: Keymap::defaults(),
            keymap_watch: None,
            appearance_watch: None,
            pane_settings_watch: None,
            family_names: crate::family_names::FamilyNames::default(),
            system_fonts: shell_data::SystemFonts::default(),
            requests,
            data: built.data,
            data_queued: built.data_queued,
            settings_backend: None,
            placement: None,
            overlays_open: false,
            hover: HoverTracker::default(),
            focus_surface: None,
            readout: crate::readout_state::ReadoutState::default(),
            notices: built.notices,
            subscriptions: built.subscriptions,
        }
    }

    fn begin(
        &mut self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        window.focus(&self.focus);
        self.system_fonts = crate::installed_fonts::installed(cx);
        self.announce_start(cx);
        self.spawn_tasks(queue, window, cx);
        self.refresh(cx);
        self.watch_keymap();
        self.watch_appearance(cx);
        self.watch_pane_settings();
    }

    fn spawn_tasks(
        &self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let inbox = self.state.host().inbox();
        let phases = self.link.phases();
        let saves = self.state.save_reports();
        cx.spawn_in(window, async move |this, cx| {
            drain(inbox, this, cx, Self::on_ipc).await;
        })
        .detach();
        cx.spawn_in(window, async move |this, cx| {
            drain(phases, this, cx, Self::feed_startup).await;
        })
        .detach();
        cx.spawn_in(window, async move |this, cx| {
            drain(saves, this, cx, Self::on_save_report).await;
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
            ShellRequest::Closing(choice) => self.choose_closing(choice, window, cx),
            ShellRequest::SelectProject(slot) => self.select_project(slot),
            ShellRequest::ResumeClosing => self.resume_closing(window, cx),
            ShellRequest::KeymapChanged => self.on_keymap_changed(cx),
            ShellRequest::AppearanceChanged => self.apply_appearance(cx),
            ShellRequest::PaneSettingsChanged => self.push_pane_settings(),
            ShellRequest::NewTabChoice { pane, action } => {
                self.choose_in_new_tab((&pane, action), window, cx);
            },
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
            Ok(SidebarAction::OpenHosts) => {
                let hosts = settings_ui::PageId::from(crate::settings_modal::HOSTS_PAGE);
                self.show_settings_page(Some(hosts), window, cx);
            },
            Ok(SidebarAction::OpenFile(path)) => self.open_workspace_file(&path, cx),
            Ok(SidebarAction::ImportRepo) => self.choose_repo_folder(window, cx),
            Ok(SidebarAction::ChooseSpaceFolder(host)) => self.choose_space_folder(&host, cx),
            Ok(SidebarAction::ImportBranches(row)) => self.show_branch_import(&row, window, cx),
            Ok(SidebarAction::Reveal(path)) => self.reveal_workspace_path(&path),
            Ok(SidebarAction::SetFileTreeFlag { key, on }) => {
                self.set_file_tree_flag((key, on), cx);
            },
            Ok(SidebarAction::Unserved) => self.perform_file_action(command, window, cx),
            Err(error) => tracing::warn!(%error, "a sidebar command was invalid"),
        }
    }

    fn open_from_sidebar(
        &mut self,
        (type_id, params): (protocol::PaneTypeId, protocol::Params),
        cx: &mut Context<'_, Self>,
    ) {
        let chat = params
            .get("threadId")
            .and_then(|chat| self.data.chat_terminal(chat));
        let (type_id, params) = crate::chat_route::route((type_id, params), chat);
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
            || self.first_run.is_some()
            || self.unsaved.is_some()
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
            TabBarEvent::Apply(commands) => self.apply_guarded(commands.clone(), cx),
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
        match event {
            LayoutEvent::Apply(command) => return self.apply_guarded(vec![command.clone()], cx),
            LayoutEvent::OpenChat {
                chat,
                tab,
                placement,
            } => self.state.open_chat_at(chat, (*tab, placement.clone())),
            _ => return,
        }
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
        match preferences.save(&self.preferences_path) {
            Ok(()) => self.clear_notice(NoticeKey::Preferences, cx),
            Err(error) => {
                tracing::warn!(%error, "saving the sidebar preferences failed");
                let text = format!("Sidebar preferences could not be saved: {error}");
                self.show_notice(NoticeKey::Preferences, (ToastKind::Error, text), cx);
            },
        }
        self.place_notices(cx);
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
        window.set_window_title(&self.window_title);
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
        self.source.follow_new_tabs(layout, &titles);
        let tab = layout.active_tab().cloned();
        self.data.focus_chat(focused_chat(layout).as_deref());
        self.follow_focus(cx);
        self.tabs
            .update(cx, |bar, cx| bar.set_sidebar(sidebar_state, cx));
        self.push_tabs(cx);
        self.layout.update(cx, |view, cx| {
            view.set_chrome(chrome, cx);
            view.set_tab(tab, cx);
        });
        self.announce_evictions(cx);
        self.sync(cx);
        cx.notify();
    }

    fn follow_window_title(
        &mut self,
        window: &mut Window,
    ) {
        let title = crate::window_title::window_title(self.state.layout(), &self.state.titles());
        if title == self.window_title {
            return;
        }
        window.set_window_title(&title);
        self.window_title = title;
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

fn layer<V: Render>(view: Option<&Entity<V>>) -> Option<Div> {
    view.map(|view| div().absolute().inset_0().child(view.clone()))
}

impl Render for ShellView {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        self.follow_window_title(window);
        let palette = Theme::of(cx).palette(Surface::Window);
        self.readout.paint();
        div()
            .size_full()
            .track_focus(&self.focus)
            .on_key_down(cx.listener(Self::on_key_down))
            .relative()
            .bg(palette.background())
            .child(self.sidebar.clone())
            .children(self.state.frozen_elements())
            .children(self.layout.read(cx).drop_preview(cx))
            .children(layer(self.settings.as_ref()))
            .children(layer(self.palette.as_ref()))
            .children(layer(self.branch_import.as_ref()))
            .children(layer(self.remote_folders.as_ref()))
            .children(self.prompt_layer())
            .children(layer(self.first_run.as_ref().map(|(_, oobe)| oobe)))
            .children(layer(self.pairing.as_ref()))
            .children(layer(self.unsaved.as_ref()))
            .children(layer(self.startup.as_ref()))
            .children(self.readout.view.clone())
            .child(self.notices.clone())
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    struct Probe;

    impl Render for Probe {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
        }
    }

    #[gpui::test]
    fn an_open_overlay_gets_a_full_window_layer_and_a_closed_one_none(cx: &mut TestAppContext) {
        let open = cx.new(|_| Probe);
        assert!(layer(Some(&open)).is_some(), "open");
        assert!(layer::<Probe>(None).is_none(), "closed");
    }

    #[gpui::test]
    fn a_layer_is_pinned_to_every_window_edge_because_a_percent_size_collapses_in_the_live_window(
        cx: &mut TestAppContext
    ) {
        let open = cx.new(|_| Probe);
        let mut shown = layer(Some(&open)).expect("an open overlay has a layer");
        let inset = &shown.style().inset;
        let pinned = [inset.top, inset.left, inset.right, inset.bottom]
            .iter()
            .all(|edge| *edge == Some(gpui::px(0.0).into()));
        assert!(pinned, "{inset:?}");
    }
}
