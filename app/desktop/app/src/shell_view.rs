use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{
    App, Context, Div, Entity, FocusHandle, InteractiveElement, IntoElement, ParentElement, Render,
    Styled, Subscription, Window, div,
};
use keymap::Keymap;
use layoutview::{ChromeContext, LayoutView};
use settingsview::SettingsModal;
use sidebarview::Sidebar;
use store::SettingsAdapter;
use ui::{Portal, Surface, Theme, ToastQueue, layer};
use viewtabs::{SidebarState, TabBar};

mod appearance_settings;
mod backend;
mod browsing;
mod browsing_data;
mod built;
mod chat_zoom;
mod child_events;
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
mod pane_toasts;
mod pane_wiring;
mod panes;
mod pick_effect;
mod picking;
mod pump;
mod readout;
mod remote_folder;
mod renaming;
mod review_viewed;
mod shell_commands;
mod sidebar_position;
mod sidebar_wiring;
mod tab_strip;
mod terminal_files;
mod terminal_life;
mod terminal_links;
#[cfg(test)]
mod test_support;
mod watches;

use self::watches::Watches;

use crate::daemon_link::DaemonLink;
use crate::focused_chat::focused_chat;
use crate::holes::visible_panes;
use crate::hover_tracker::HoverTracker;
use crate::ports::Ports;
use crate::shell_pane_source::ShellPaneSource;
use crate::shell_parts::ShellParts;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;

use self::children::build_children;

pub(crate) struct ShellView {
    state: ShellState,
    link: Rc<DaemonLink>,
    preferences_path: PathBuf,
    sidebar: Entity<Sidebar>,
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    startup: Option<Entity<startup::View>>,
    settings: Option<Entity<SettingsModal>>,
    branch_import: Option<Entity<branchimport::View>>,
    remote_folders: Option<Entity<folderpicker::View>>,
    file_prompt: Option<Entity<crate::file_prompt::FilePrompt>>,
    rename_prompt: Option<(protocol::PaneId, Entity<crate::rename_prompt::RenamePrompt>)>,
    pairing: Option<Entity<pairing::View>>,
    first_run: Option<(crate::landing::Landing, Entity<oobe::View>)>,
    window_title: String,
    unsaved: Option<Entity<crate::unsaved_dialog::UnsavedDialog>>,
    pending_close: Option<crate::pending_close::PendingClose>,
    source: Rc<ShellPaneSource>,
    focus: FocusHandle,
    keymap: Keymap,
    watches: Watches,
    family_names: crate::family_names::FamilyNames,
    system_fonts: store::SystemFonts,
    requests: async_channel::Sender<ShellRequest>,
    data: Rc<store::Store>,
    data_queued: Arc<AtomicBool>,
    settings_backend: Option<SettingsAdapter>,
    placement: Option<(webview::ParentWindow, webview::Bounds)>,
    hover: HoverTracker,
    focus_surface: Option<sidebar::Mode>,
    ports: Ports,
    readout: crate::readout_state::ReadoutState,
    notices: Entity<ToastQueue>,
    backend: crate::backend::BackendWatch,
    link_menu: Option<crate::link_menu::LinkMenu>,
    subscriptions: Vec<Subscription>,
}

impl ShellView {
    pub(crate) fn new(
        parts: ShellParts,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let (requests, queue) = async_channel::unbounded();
        let mut built = build_children(&parts, &requests, cx);
        built.subscriptions = Self::subscribe_children(&built, window, cx);
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
            branch_import: None,
            remote_folders: None,
            file_prompt: None,
            rename_prompt: None,
            pairing: None,
            first_run: None,
            unsaved: None,
            window_title: crate::window_memory::WINDOW_TITLE.to_owned(),
            pending_close: None,
            focus,
            keymap: Keymap::defaults(),
            watches: Watches::default(),
            family_names: crate::family_names::FamilyNames::default(),
            system_fonts: store::SystemFonts::default(),
            requests,
            data: built.data,
            data_queued: built.data_queued,
            settings_backend: None,
            placement: None,
            hover: HoverTracker::default(),
            focus_surface: None,
            readout: crate::readout_state::ReadoutState::new(Arc::clone(&parts.ports.clock)),
            ports: parts.ports,
            notices: built.notices,
            backend: backend::watching(built.workspace),
            link_menu: None,
            subscriptions: built.subscriptions,
        }
    }

    fn begin(
        &mut self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        window.focus(&self.focus, cx);
        self.system_fonts = crate::installed_fonts::installed(cx);
        self.announce_start(cx);
        self.spawn_tasks(queue, window, cx);
        self.attach_agent_peer();
        self.refresh(cx);
        self.watch_keymap();
        self.forward_bindings(window, cx);
        self.watch_appearance(cx);
        self.watch_pane_settings();
        self.flush_settings_on_quit(cx);
    }

    fn modal_open(&self) -> bool {
        self.startup.is_some()
            || self.settings.is_some()
            || self.branch_import.is_some()
            || self.remote_folders.is_some()
            || self.file_prompt.is_some()
            || self.rename_prompt.is_some()
            || self.pairing.is_some()
            || self.first_run.is_some()
            || self.unsaved.is_some()
    }

    fn floating(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Portal> {
        let shown = self.modal_open()
            || self.readout.view.is_some()
            || self.floats_toasts(cx)
            || self.backend.crash.is_some();
        if !shown {
            return None;
        }
        let shell = cx.entity();
        let portal = Portal::cover("shell-layers", move |_, cx| shell.read(cx).layers(cx));
        Some(portal.priority(layer::MODAL))
    }

    fn layers(
        &self,
        cx: &App,
    ) -> Div {
        div()
            .relative()
            .size_full()
            .children(layer(self.settings.as_ref()))
            .children(layer(self.branch_import.as_ref()))
            .children(layer(self.remote_folders.as_ref()))
            .children(self.prompt_layer())
            .children(layer(self.rename_prompt.as_ref().map(|(_, prompt)| prompt)))
            .children(layer(self.first_run.as_ref().map(|(_, oobe)| oobe)))
            .children(layer(self.pairing.as_ref()))
            .children(layer(self.unsaved.as_ref()))
            .children(layer(self.startup.as_ref()))
            .children(self.readout.view.clone())
            .children(self.backend.crash.as_ref().map(|(panel, _)| panel.clone()))
            .children(self.floats_toasts(cx).then(|| self.notices.clone()))
    }

    fn chrome(
        &self,
        cx: &Context<'_, Self>,
    ) -> (ChromeContext, SidebarState) {
        let preferences = self.sidebar.read(cx).preferences();
        let (mut chrome, state) = crate::chrome::of(&preferences);
        chrome.tab_strip = self.tabs.read(cx).has_tabs();
        (chrome, state)
    }

    fn refresh(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let titles = self.state.titles();
        let layout = self.state.layout();
        self.source.native().follow(layout, &titles);
        let tab = layout.active_tab().cloned();
        self.data.focus_chat(focused_chat(layout).as_deref());
        let open = crate::open_chats::open_chats(layout)
            .into_iter()
            .map(sidebar::RowId::from);
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.set_open_chats(open, cx));
        self.follow_focus(cx);
        self.push_tabs(cx);
        let (chrome, sidebar_state) = self.chrome(cx);
        self.tabs
            .update(cx, |bar, cx| bar.set_sidebar(sidebar_state, cx));
        self.layout.update(cx, |view, cx| {
            view.set_chrome(chrome, cx);
            view.set_tab(tab, cx);
        });
        self.announce_evictions(cx);
        self.drop_toasts_of_closed_panes(cx);
        self.sync(cx);
        self.state.publish_tabs();
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
            .capture_key_down(cx.listener(Self::on_key_alongside))
            .on_key_down(cx.listener(Self::on_key_down))
            .capture_any_mouse_down(cx.listener(Self::on_shell_pressed))
            .relative()
            .bg(palette.background())
            .child(self.sidebar.clone())
            .children(self.link_layers(cx))
            .children(self.floating(cx))
    }
}

#[cfg(test)]
mod tests {
    use crate::preferences_store::PreferencesStore as _;
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

    pub(super) struct Opened {
        pub(super) shell: Entity<ShellView>,
        pub(super) window: gpui::VisualTestContext,
        top: gpui::VisualTestContext,
        pub(super) mock: Rc<webview::MockPlatform>,
        pub(super) pane: protocol::PaneId,
        preferences: Arc<crate::test_support::MemoryPreferences>,
    }

    pub(super) fn open_shell(cx: &mut TestAppContext) -> Opened {
        let opened = open_shell_unsettled(cx);
        opened.window.executor().advance_clock(backend::SHOW_DELAY);
        opened.window.run_until_parked();
        opened
    }

    pub(super) fn open_shell_unsettled(cx: &mut TestAppContext) -> Opened {
        cx.executor().allow_parking();
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let mut rig = crate::test_support::shell_rig();
        let pane = crate::test_support::synced_chat(&mut rig);
        let preferences = Arc::new(crate::test_support::MemoryPreferences::default());
        let ports = Ports {
            clock: Arc::new(crate::test_support::FixedClock::at(0)),
            bytes: Arc::new(crate::test_support::MemoryFiles::default()),
            preferences: Arc::<crate::test_support::MemoryPreferences>::clone(&preferences),
        };
        let parts = ShellParts {
            state: rig.shell,
            link: Rc::new(crate::test_support::link_without_daemon()),
            preferences_path: PathBuf::from("/state/sidebar.json"),
            ports,
        };
        let window = cx.add_window(move |window, cx| ShellView::new(parts, window, cx));
        let shell = window.entity(cx).expect("the shell view");
        let owner: gpui::AnyWindowHandle = window.into();
        let top = cx
            .update(|cx| ui::TopLayer::open(owner, cx))
            .expect("the top layer opens");
        let window = gpui::VisualTestContext::from_window(owner, cx);
        window.run_until_parked();
        Opened {
            shell,
            window,
            top: gpui::VisualTestContext::from_window(top, cx),
            mock: rig.mock,
            pane,
            preferences,
        }
    }

    #[gpui::test]
    fn hiding_the_sidebar_is_kept_by_the_shells_own_preferences_store(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell
                    .sidebar
                    .update(cx, |sidebar, cx| sidebar.toggle(window, cx));
            });
        opened.window.run_until_parked();
        let kept = opened.preferences.load().unwrap().panel().visibility();
        assert_eq!(kept, sidebar::Visibility::Closed);
    }

    fn hidden_views(mock: &webview::MockPlatform) -> usize {
        mock.calls().iter().filter(|call| hides(call)).count()
    }

    fn hides(call: &webview::Call) -> bool {
        let webview::Call::SetVisibility { visibility, .. } = call else {
            return false;
        };
        *visibility == webview::Visibility::Hidden
    }

    #[gpui::test]
    fn a_prompt_floats_above_the_panes_and_leaves_them_shown(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let before = hidden_views(&opened.mock);
        let pane = opened.pane.clone();
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.ask_rename((&pane, "Chat"), window, cx);
            });
        opened.window.run_until_parked();
        assert_eq!(
            opened.window.debug_bounds("ui-dialog"),
            None,
            "nothing in the window"
        );
        assert!(opened.top.debug_bounds("ui-dialog").is_some());
        assert_eq!(hidden_views(&opened.mock), before, "no web view was hidden");
    }

    fn page_focuses(mock: &webview::MockPlatform) -> usize {
        let focuses = |call: &&webview::Call| matches!(call, webview::Call::Focus { .. });
        mock.calls().iter().filter(focuses).count()
    }

    pub(super) const PLACED: webview::Bounds = webview::Bounds {
        x: 300.0,
        y: 40.0,
        width: 900.0,
        height: 700.0,
    };

    #[gpui::test]
    fn a_pointer_move_over_a_page_moves_the_window_pointer_over_the_same_spot(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_shell(cx);
        opened.window.run_until_parked();
        let pane = opened.pane.clone();
        let text = r#"{"sdk":1,"kind":"shell.pointer","x":3.5,"y":120.0}"#.to_owned();
        let message = panehost::IpcMessage { pane, text };
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.placement = Some((webview::ParentWindow::X11(9), PLACED));
                shell.on_ipc(message, window, cx);
            });
        let position = opened.window.update(|window, _| window.mouse_position());
        assert!(
            position.x > gpui::px(303.5) && position.y > gpui::px(160.0),
            "page point plus the page's origin: {position:?}"
        );
    }

    fn register_chat(opened: &mut Opened) {
        let register = protocol::PaneFrame::Register {
            type_id: crate::core_panes::chat_type().unwrap_or_else(|error| panic!("{error}")),
            sdk: protocol::SDK_VERSION,
        };
        let text = protocol::encode(&register).unwrap_or_else(|error| panic!("{error}"));
        let pane = opened.pane.clone();
        let message = panehost::IpcMessage { pane, text };
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.handle_ipc(message)
        });
    }

    fn open_tab_menu(cx: &mut TestAppContext) -> Opened {
        let mut opened = open_shell(cx);
        register_chat(&mut opened);
        opened
            .shell
            .update(&mut opened.window, |shell, _| shell.startup = None);
        opened.window.run_until_parked();
        let tab = opened
            .window
            .debug_bounds("tab-label")
            .unwrap_or_default()
            .center();
        let none = gpui::Modifiers::none();
        opened
            .window
            .simulate_mouse_down(tab, gpui::MouseButton::Right, none);
        opened
            .window
            .simulate_mouse_up(tab, gpui::MouseButton::Right, none);
        opened.window.run_until_parked();
        assert!(
            opened.top.update(|window, _| window.takes_keys()),
            "the menu took the keys"
        );
        opened
    }

    #[gpui::test]
    fn activating_the_window_leaves_the_keyboard_with_an_open_menu(cx: &mut TestAppContext) {
        let mut opened = open_tab_menu(cx);
        let before = page_focuses(&opened.mock);
        opened.window.deactivate_window();
        opened.window.update(|window, _| window.activate_window());
        opened.window.run_until_parked();
        assert_eq!(
            page_focuses(&opened.mock),
            before,
            "no page took the keyboard"
        );
    }

    fn own_keyboard_by_page(opened: &mut Opened) {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.state.apply(Vec::new());
            shell.sync(cx);
        });
        opened.window.update(|window, _| window.activate_window());
        opened.window.run_until_parked();
    }

    #[gpui::test]
    fn a_press_in_the_top_layer_leaves_the_keyboard_with_the_page(cx: &mut TestAppContext) {
        let mut opened = open_tab_menu(cx);
        own_keyboard_by_page(&mut opened);
        let before = page_focuses(&opened.mock);
        let away = gpui::point(gpui::px(600.0), gpui::px(400.0));
        let none = gpui::Modifiers::none();
        opened
            .top
            .simulate_mouse_down(away, gpui::MouseButton::Left, none);
        opened
            .top
            .simulate_mouse_up(away, gpui::MouseButton::Left, none);
        opened.window.run_until_parked();
        assert!(
            !opened.top.update(|window, _| window.takes_keys()),
            "the click away closed the menu"
        );
        assert!(
            page_focuses(&opened.mock) > before,
            "the shell did not claim the keyboard"
        );
    }

    #[gpui::test]
    fn a_menu_closed_with_escape_gives_the_keyboard_back_to_the_page(cx: &mut TestAppContext) {
        let mut opened = open_tab_menu(cx);
        own_keyboard_by_page(&mut opened);
        let before = page_focuses(&opened.mock);
        opened.top.simulate_keystrokes("escape");
        opened.window.run_until_parked();
        assert!(
            !opened.top.update(|window, _| window.takes_keys()),
            "the menu closed"
        );
        assert!(
            page_focuses(&opened.mock) > before,
            "the page that owns the keyboard has it again"
        );
    }
}
