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
use ui::{Portal, Theme, ToastQueue, layer};
use viewtabs::{SidebarState, TabBar};

pub(crate) mod assembly;
mod browser;
mod commands;
mod editing;
mod events;
mod native_alert;
mod settings;
mod sidebar;
mod spaces;
mod terminals;
#[cfg(test)]
mod test_support;

use crate::shell::shell_view::assembly::pane_area::PaneArea;
use crate::shell::shell_view::assembly::watches::Watches;

use crate::daemon::daemon_link::DaemonLink;
use crate::panes::content::holes::{overlays, visible_panes};
use crate::panes::content::shell_pane_source::ShellPaneSource;
use crate::panes::tracking::focused_chat::focused_chat;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::ports::Ports;
use crate::shell::shell_state::ShellState;
use crate::shell::shell_view::assembly::shell_parts::ShellParts;

use crate::panes::tracking::active_file;
use crate::panes::tracking::open_chats;
use crate::shell::dialogs::file_prompt;
use crate::shell::dialogs::name_prompt;
use crate::shell::dialogs::unsaved::pending_close;
use crate::shell::shell_view::assembly::children::build_children;
use crate::shell::window::chrome;
use crate::shell::window::readout::readout_state;

pub(crate) struct ShellView {
    state: ShellState,
    link: Rc<DaemonLink>,
    preferences_path: PathBuf,
    sidebar: Entity<Sidebar>,
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    startup: Entity<startup::View>,
    settings: Option<Entity<SettingsModal>>,
    branch_import: Option<Entity<branchimport::View>>,
    remote_folders: Option<Entity<folderpicker::View>>,
    folder_import: Option<Entity<folderimport::View>>,
    file_prompt: Option<Entity<file_prompt::FilePrompt>>,
    name_prompt: Option<(protocol::PaneId, Entity<name_prompt::NamePrompt>)>,
    pairing: Option<Entity<pairing::View>>,
    device_offer: Option<Entity<pairing::OfferView>>,
    first_run: Option<(events::first_run::landing::Landing, Entity<oobe::View>)>,
    pending_close: Option<pending_close::PendingClose>,
    source: Rc<ShellPaneSource>,
    focus: FocusHandle,
    keymap: Keymap,
    watches: Watches,
    family_names: settings::family_names::FamilyNames,
    system_fonts: store::SystemFonts,
    requests: async_channel::Sender<ShellRequest>,
    data: Rc<store::Store>,
    data_queued: Arc<AtomicBool>,
    settings_backend: Option<SettingsAdapter>,
    pane_area: PaneArea,
    focus_surface: Option<::sidebar::Mode>,
    ports: Ports,
    readout: readout_state::ReadoutState,
    notices: Entity<ToastQueue>,
    backend: crate::daemon::backend::BackendWatch,
    subscriptions: Vec<Subscription>,
}

impl ShellView {
    pub(crate) fn new(
        parts: ShellParts,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let (requests, queue) = (parts.requests.clone(), parts.queue.clone());
        let mut built = build_children(&parts, &requests, window, cx);
        built.subscriptions = Self::subscribe_children(&built, window, cx);
        let resync = cx.observe(&built.pane_area.cover, |this, _, cx| this.sync(cx));
        built.subscriptions.push(resync);
        let mut view = Self::assemble(parts, built, requests, cx.focus_handle());
        view.begin(queue, window, cx);
        view
    }

    fn assemble(
        parts: ShellParts,
        built: assembly::built::Built,
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
            startup: built.startup,
            settings: None,
            branch_import: None,
            remote_folders: None,
            folder_import: None,
            file_prompt: None,
            name_prompt: None,
            pairing: None,
            device_offer: None,
            first_run: None,
            pending_close: None,
            focus,
            keymap: Keymap::defaults(),
            watches: Watches::default(),
            family_names: settings::family_names::FamilyNames::default(),
            system_fonts: store::SystemFonts::default(),
            requests,
            data: built.data,
            data_queued: built.data_queued,
            settings_backend: None,
            focus_surface: None,
            readout: readout_state::ReadoutState::new(Arc::clone(&parts.ports.clock)),
            ports: parts.ports,
            notices: built.notices,
            backend: events::backend::watching(built.workspace),
            pane_area: built.pane_area,
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
        self.system_fonts = settings::installed_fonts::installed(cx);
        self.announce_start(cx);
        self.feed_known_phases(window, cx);
        self.spawn_tasks(queue, window, cx);
        self.attach_agent_peer();
        self.refresh(cx);
        self.watch_keymap();
        self.forward_bindings(window, cx);
        self.watch_appearance(cx);
        self.follow_system_appearance(window.appearance(), cx);
        self.watch_pane_settings();
        self.flush_settings_on_quit(cx);
    }

    fn modal_open(
        &self,
        cx: &App,
    ) -> bool {
        self.startup.read(cx).covers()
            || self.settings.is_some()
            || self.branch_import.is_some()
            || self.remote_folders.is_some()
            || self.folder_import.is_some()
            || self.file_prompt.is_some()
            || self.name_prompt.is_some()
            || self.pairing.is_some()
            || self.device_offer.is_some()
            || self.first_run.is_some()
            || self.backend.crash.is_some()
    }

    fn floating(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Portal> {
        let modal = self.modal_open(cx);
        let shown = modal || self.readout.view.is_some() || self.floats_toasts(cx);
        if !shown {
            return None;
        }
        let shell = cx.entity();
        let portal = Portal::cover("shell-layers", move |_, cx| shell.read(cx).layers(cx));
        let portal = portal.priority(layer::MODAL);
        if !modal {
            return Some(portal);
        }
        Some(portal.float(ui::Float::Modal))
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
            .children(layer(self.name_prompt.as_ref().map(|(_, prompt)| prompt)))
            .children(layer(self.first_run.as_ref().map(|(_, oobe)| oobe)).map(solid))
            .children(layer(self.folder_import.as_ref()))
            .children(layer(self.pairing.as_ref()))
            .children(layer(self.device_offer.as_ref()))
            .children(layer(Some(&self.startup).filter(|gate| gate.read(cx).covers())).map(solid))
            .children(self.readout.view.clone().map(solid))
            .children(
                self.backend
                    .crash
                    .as_ref()
                    .map(|(panel, _)| solid(panel.clone())),
            )
            .children(self.floats_toasts(cx).then(|| self.notices.clone()))
    }

    fn chrome(
        &self,
        cx: &Context<'_, Self>,
    ) -> (ChromeContext, SidebarState) {
        let preferences = self.sidebar.read(cx).preferences();
        let (mut chrome, state) = chrome::of(&preferences);
        chrome.tab_strip = self.tabs.read(cx).has_tabs();
        (chrome, state)
    }

    fn refresh(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let titles = self.state.titles();
        let layout = self.state.layout();
        let space = self.state.space();
        self.source.native().follow((layout, &space), &titles);
        let tab = layout.showing(&space).cloned();
        self.data
            .focus_chat(focused_chat(layout, &space).as_deref());
        self.data.watch_chats(open_chats::tab_chats(layout));
        let open = open_chats::open_chats(layout)
            .into_iter()
            .map(::sidebar::RowId::from);
        let active = active_file::active_file(layout, &space);
        self.sidebar.update(cx, |sidebar, cx| {
            sidebar.set_open_chats(open, cx);
            sidebar.set_active_file(active, cx);
        });
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

    fn sync(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((parent, area)) = self.pane_area.placement else {
            return;
        };
        let (chrome, _) = self.chrome(cx);
        let visible = visible_panes(self.state.showing(), area, (chrome, self.source.as_ref()));
        self.state.set_theme(*Theme::of(cx));
        let handle = self.layout.read(cx).handle_hole();
        let overlays = overlays(self.pane_area.cover.read(cx), handle, area);
        let outcome = self
            .state
            .sync(parent, (visible, self.source.native()), overlays);
        self.apply_outcome(outcome, cx);
    }
}

fn solid(content: impl IntoElement) -> ui::Shaped {
    ui::Shaped::solid(content, gpui::px(0.0))
}

fn layer<V: Render>(view: Option<&Entity<V>>) -> Option<Div> {
    view.map(|view| div().absolute().inset_0().child(view.clone()))
}

impl Render for ShellView {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        self.readout.paint();
        div()
            .size_full()
            .track_focus(&self.focus)
            .capture_key_down(cx.listener(Self::on_key_alongside))
            .on_key_down(cx.listener(Self::on_key_down))
            .capture_any_mouse_down(cx.listener(Self::on_shell_pressed))
            .relative()
            .child(ui::hear())
            .child(self.sidebar.clone())
            .children(self.floating(cx))
    }
}

#[cfg(test)]
mod tests {
    use crate::shell::ports::preferences_store::PreferencesStore as _;
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
        pub(super) mock: Rc<webview::MockPlatform>,
        pub(super) pane: protocol::PaneId,
        preferences: Arc<crate::test_support::MemoryPreferences>,
        pub(super) space: Rc<crate::test_support::FakeSpace>,
    }

    pub(super) fn open_shell(cx: &mut TestAppContext) -> Opened {
        let opened = open_shell_unsettled(cx);
        opened
            .window
            .executor()
            .advance_clock(events::backend::SHOW_DELAY);
        opened.window.run_until_parked();
        opened
    }

    struct Rig {
        parts: ShellParts,
        mock: Rc<webview::MockPlatform>,
        pane: protocol::PaneId,
        preferences: Arc<crate::test_support::MemoryPreferences>,
        space: Rc<crate::test_support::FakeSpace>,
    }

    pub(super) const SPACE: &str = "p1";

    fn shell_rig(cx: &mut TestAppContext) -> Rig {
        cx.executor().allow_parking();
        cx.update(|cx| {
            ui::init(cx).expect("ui installs");
            ui::testing::open_float_windows(cx);
        });
        cx.update(editorview::init);
        let mut rig = crate::test_support::shell_rig();
        rig.space.select(SPACE);
        let pane = crate::test_support::synced_chat(&mut rig);
        let preferences = Arc::new(crate::test_support::MemoryPreferences::default());
        let ports = Ports {
            clock: Arc::new(crate::test_support::FixedClock::at(0)),
            bytes: Arc::new(crate::test_support::MemoryFiles::default()),
            preferences: Arc::<crate::test_support::MemoryPreferences>::clone(&preferences),
        };
        let link = Rc::new(crate::test_support::link_without_daemon());
        let (requests, queue) = async_channel::unbounded();
        let (data, data_queued) = crate::shell::inbox::data_signal::start_data(&link, &requests);
        let parts = ShellParts {
            state: rig.shell,
            link,
            preferences_path: PathBuf::from("/state/sidebar.json"),
            ports,
            data,
            data_queued,
            requests,
            queue,
        };
        Rig {
            parts,
            mock: rig.mock,
            pane,
            preferences,
            space: rig.space,
        }
    }

    pub(super) fn open_shell_unsettled(cx: &mut TestAppContext) -> Opened {
        let Rig {
            parts,
            mock,
            pane,
            preferences,
            space,
        } = shell_rig(cx);
        let window = cx.add_window(move |window, cx| ShellView::new(parts, window, cx));
        let shell = window.entity(cx).expect("the shell view");
        let owner: gpui::AnyWindowHandle = window.into();
        let window = gpui::VisualTestContext::from_window(owner, cx);
        window.run_until_parked();
        Opened {
            shell,
            window,
            mock,
            pane,
            preferences,
            space,
        }
    }

    fn solid_fills_at(
        window: &mut Window,
        (x, y): (f32, f32),
    ) -> Vec<f32> {
        let scale = window.scale_factor();
        let at = gpui::point(gpui::px(x).scale(scale), gpui::px(y).scale(scale));
        window
            .painted_quads()
            .into_iter()
            .filter(|quad| {
                quad.bounds
                    .intersect(&quad.content_mask.bounds)
                    .contains(&at)
            })
            .filter_map(|quad| quad.background.as_solid())
            .map(|fill| fill.a)
            .filter(|alpha| *alpha > 0.0)
            .collect()
    }

    #[gpui::test]
    fn a_see_through_window_paints_nothing_solid_behind_its_sidebar(cx: &mut TestAppContext) {
        let mut opened = open_started_shell(cx);
        let see_through = Theme::default()
            .with_mode(ui::ThemeMode::Dark)
            .with_vibrancy(ui::Vibrancy::Translucent);
        opened.window.update(|_, cx| see_through.install(cx));
        opened.window.run_until_parked();
        let fills = opened.window.update(|window, _| {
            let bottom = f32::from(window.viewport_size().height) - 4.0;
            solid_fills_at(window, (4.0, bottom))
        });
        assert!(!fills.is_empty(), "the sidebar's wash is there");
        assert!(fills.iter().all(|alpha| *alpha < 1.0), "{fills:?}");
    }

    #[gpui::test]
    fn a_halt_known_before_the_window_opens_covers_its_first_frame(cx: &mut TestAppContext) {
        let parts = shell_rig(cx).parts;
        let window = cx.add_window(move |window, cx| ShellView::new(parts, window, cx));
        let covers = window.update(cx, |shell, _, cx| shell.startup.read(cx).covers());
        assert_eq!(
            covers.ok(),
            Some(true),
            "the launch failed before the window"
        );
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
        assert_eq!(kept, ::sidebar::Visibility::Closed);
    }

    fn views_set(
        mock: &webview::MockPlatform,
        wanted: webview::Visibility,
    ) -> usize {
        let set = |call: &&webview::Call| matches!(call, webview::Call::SetVisibility { visibility, .. } if *visibility == wanted);
        mock.calls().iter().filter(set).count()
    }

    #[gpui::test]
    fn a_prompt_is_drawn_in_the_window_and_covers_the_web_views_until_it_closes(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_started_shell(cx);
        let [covered, shown] = [webview::Visibility::Covered, webview::Visibility::Shown]
            .map(|visibility| views_set(&opened.mock, visibility));
        let pane = opened.pane.clone();
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.ask_rename((&pane, "Chat"), window, cx);
            });
        opened.window.run_until_parked();
        assert!(
            opened.window.debug_bounds("ui-dialog").is_some(),
            "drawn in the window"
        );
        assert_eq!(
            views_set(&opened.mock, webview::Visibility::Covered),
            covered + 1,
            "the chat is covered under it"
        );
        opened.window.simulate_keystrokes("escape");
        opened.window.run_until_parked();
        assert_eq!(opened.window.debug_bounds("ui-dialog"), None, "closed");
        assert_eq!(
            views_set(&opened.mock, webview::Visibility::Shown),
            shown + 1,
            "the chat shows again"
        );
    }

    pub(super) fn page_focuses(mock: &webview::MockPlatform) -> usize {
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
        let shell = opened.shell.clone();
        opened
            .window
            .update(|window, cx| tell_placed(&shell, message, window, cx));
        let position = opened.window.update(|window, _| window.mouse_position());
        assert!(
            position.x > gpui::px(303.5) && position.y > gpui::px(160.0),
            "page point plus the page's origin: {position:?}"
        );
    }

    fn tell_placed(
        shell: &Entity<ShellView>,
        message: panehost::IpcMessage,
        window: &mut Window,
        cx: &mut App,
    ) {
        shell.update(cx, |shell, cx| {
            shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
            shell.on_ipc(message, window, cx);
        });
    }

    pub(super) fn register_chat(opened: &mut Opened) {
        let register = protocol::PaneFrame::Register {
            type_id: crate::panes::kinds::core_panes::chat_type()
                .unwrap_or_else(|error| panic!("{error}")),
            sdk: protocol::SDK_VERSION,
        };
        let text = protocol::encode(&register).unwrap_or_else(|error| panic!("{error}"));
        let pane = opened.pane.clone();
        let message = panehost::IpcMessage { pane, text };
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.handle_ipc(message)
        });
    }

    pub(super) fn open_started_shell(cx: &mut TestAppContext) -> Opened {
        let mut opened = open_shell(cx);
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
            let restarted = startup::Input::Restarted;
            shell
                .startup
                .update(cx, |gate, cx| gate.apply(restarted, cx));
        });
        opened.window.run_until_parked();
        opened
    }

    fn click(
        opened: &mut Opened,
        at: gpui::Point<gpui::Pixels>,
    ) {
        opened.window.simulate_click(at, gpui::Modifiers::none());
        opened.window.run_until_parked();
    }

    fn popover_open(opened: &mut Opened) -> bool {
        !ui::testing::covered(&mut opened.window).1.is_empty()
    }

    fn open_popover(cx: &mut TestAppContext) -> Opened {
        let mut opened = open_started_shell(cx);
        register_chat(&mut opened);
        opened.window.run_until_parked();
        let switcher = opened
            .window
            .debug_bounds("sidebar-switcher")
            .expect("the switcher");
        let add = gpui::point(switcher.right() - gpui::px(18.0), switcher.center().y);
        click(&mut opened, add);
        let icon = opened
            .window
            .debug_bounds("space-draft-icon")
            .expect("the new space's icon");
        opened
            .window
            .simulate_click(icon.center(), gpui::Modifiers::none());
        assert!(popover_open(&mut opened), "open");
        opened
    }

    pub(super) fn own_keyboard_by_page(opened: &mut Opened) {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.state.apply(Vec::new());
            shell.sync(cx);
        });
        opened.window.update(|window, _| window.activate_window());
        opened.window.run_until_parked();
    }

    fn cuts(mock: &webview::MockPlatform) -> Vec<Option<webview::Bounds>> {
        let cut = |call: &webview::Call| match call {
            webview::Call::SetCutout { hole, .. } => Some(*hole),
            _ => None,
        };
        mock.calls().iter().filter_map(cut).collect()
    }

    #[gpui::test]
    fn hovering_a_split_pane_cuts_its_handle_out_of_the_web_view_under_it(cx: &mut TestAppContext) {
        let mut opened = open_started_shell(cx);
        let pane = opened.pane.clone();
        let layout = opened
            .shell
            .read_with(&opened.window, |shell, _| shell.layout.clone());
        let tab = layout
            .read_with(&opened.window, |view, _| view.shown_tab())
            .expect("a tab");
        let placement = layout::Placement {
            target: pane.clone(),
            edge: layout::Edge::Right,
        };
        let chat = "t2".into();
        let open = layoutview::LayoutEvent::OpenChat {
            chat,
            tab,
            placement,
        };
        layout.update(&mut opened.window, |_, cx| cx.emit(open));
        opened.window.run_until_parked();
        let before = cuts(&opened.mock).len();
        layout.update(&mut opened.window, |view, cx| view.page_entered(&pane, cx));
        opened.window.run_until_parked();
        let hole = cuts(&opened.mock).get(before).copied().flatten();
        assert!(
            hole.is_some(),
            "the handle's hole is cut from the page under it"
        );
        layout.update(&mut opened.window, |view, cx| view.page_left(&pane, cx));
        opened.window.run_until_parked();
        assert_eq!(cuts(&opened.mock).last(), Some(&None), "whole again");
    }

    #[gpui::test]
    fn a_press_on_a_page_closes_a_popover_and_the_page_takes_the_keyboard(cx: &mut TestAppContext) {
        let mut opened = open_popover(cx);
        own_keyboard_by_page(&mut opened);
        let before = page_focuses(&opened.mock);
        let pane = opened.pane.clone();
        let text = r#"{"sdk":1,"kind":"shell.focus"}"#.to_owned();
        let pressed = panehost::IpcMessage { pane, text };
        let shell = opened.shell.clone();
        opened.window.update(|window, cx| {
            shell.update(cx, |shell, cx| shell.on_ipc(pressed, window, cx));
        });
        opened.window.run_until_parked();
        assert!(!popover_open(&mut opened), "closed");
        assert!(
            page_focuses(&opened.mock) > before,
            "the page has the keyboard"
        );
    }

    #[gpui::test]
    fn a_prompt_closed_with_escape_gives_the_keyboard_back_to_the_page(cx: &mut TestAppContext) {
        let mut opened = open_started_shell(cx);
        register_chat(&mut opened);
        let pane = opened.pane.clone();
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.ask_rename((&pane, "Chat"), window, cx);
            });
        opened.window.run_until_parked();
        own_keyboard_by_page(&mut opened);
        let before = page_focuses(&opened.mock);
        opened.window.simulate_keystrokes("escape");
        opened.window.run_until_parked();
        assert!(
            page_focuses(&opened.mock) > before,
            "the page that owns the keyboard has it again"
        );
    }
}
