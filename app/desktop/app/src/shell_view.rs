use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use gpui::{
    Context, Div, Entity, FocusHandle, InteractiveElement, IntoElement, ParentElement, Render,
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
mod browsing;
mod browsing_data;
mod built;
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
mod pane_wiring;
mod panes;
mod pick_effect;
mod picking;
mod pump;
mod readout;
mod remote_folder;
mod renaming;
mod shell_commands;
mod sidebar_position;
mod sidebar_wiring;
mod tab_strip;
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
        self.attach_agent_peer();
        self.refresh(cx);
        self.watch_keymap();
        self.watch_appearance(cx);
        self.watch_pane_settings();
    }

    /// Whether a shell-wide modal covers the window, which then holds the keyboard.
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

    /// What floats over the panes, painted in the window's top layer while any is shown.
    fn floating(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Portal> {
        let shown =
            self.modal_open() || self.readout.view.is_some() || !self.notices.read(cx).is_empty();
        if !shown {
            return None;
        }
        let shell = cx.entity();
        let portal = Portal::cover("shell-layers", move |_, cx| shell.read(cx).layers());
        Some(portal.priority(layer::MODAL))
    }

    fn layers(&self) -> Div {
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
            .child(self.notices.clone())
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
        self.source.native().follow(layout, &titles);
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
            .on_key_down(cx.listener(Self::on_key_down))
            .capture_any_mouse_down(cx.listener(Self::on_shell_pressed))
            .relative()
            .bg(palette.background())
            .child(self.sidebar.clone())
            .children(self.floating(cx))
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

    struct Opened {
        shell: Entity<ShellView>,
        window: gpui::VisualTestContext,
        top: gpui::VisualTestContext,
        mock: Rc<webview::MockPlatform>,
        pane: protocol::PaneId,
    }

    fn open_shell(cx: &mut TestAppContext) -> Opened {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let mut rig = crate::test_support::shell_rig();
        let pane = crate::test_support::synced_chat(&mut rig);
        let ports = Ports {
            clock: Arc::new(crate::test_support::FixedClock::at(0)),
            bytes: Arc::new(crate::test_support::MemoryFiles::default()),
        };
        let parts = ShellParts {
            state: rig.shell,
            link: Rc::new(crate::test_support::link_without_daemon()),
            preferences_path: PathBuf::from("/nonexistent/sidebar.json"),
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
        }
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
}
