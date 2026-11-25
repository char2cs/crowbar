use gpui::{Context, MouseMoveEvent, PlatformInput, Window, point, px};
use panehost::IpcMessage;
use protocol::PaneId;

use super::ShellView;
use crate::daemon::startup::startup_command;
use crate::holes::visible_panes;
use crate::shell_request::ShellRequest;
use crate::single_precision::single_precision;

impl ShellView {
    pub(super) fn spawn_tasks(
        &self,
        queue: async_channel::Receiver<ShellRequest>,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let inbox = self.state.host().inbox();
        let menus = self.state.host().menu_asks();
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
            drain(menus, this, cx, Self::pop_pane_menu).await;
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

    pub(super) fn on_ipc(
        &mut self,
        message: IpcMessage,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let mut outcome = self.state.handle_ipc(message);
        let page_took_focus = !outcome.focused.is_empty();
        let pointer = outcome.pointer.take();
        self.show_pane_toasts(std::mem::take(&mut outcome.toasts), cx);
        self.apply_outcome(outcome, cx);
        self.replay_pointer(pointer, window, cx);
        if !page_took_focus {
            return;
        }
        window.focus(&self.focus, cx);
        self.state.apply_keyboard();
    }

    fn replay_pointer(
        &self,
        pointer: Option<(PaneId, f64, f64)>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((pane, x, y)) = pointer else {
            return;
        };
        let Some(position) = self.window_point(&pane, (x, y), cx) else {
            return;
        };
        let moved = MouseMoveEvent {
            position,
            pressed_button: None,
            modifiers: window.modifiers(),
        };
        window.dispatch_event(PlatformInput::MouseMove(moved), cx);
    }

    pub(super) fn window_point(
        &self,
        pane: &PaneId,
        (x, y): (f64, f64),
        cx: &mut Context<'_, Self>,
    ) -> Option<gpui::Point<gpui::Pixels>> {
        let (_, area) = self.placement?;
        let (chrome, _) = self.chrome(cx);
        let visible = visible_panes(self.state.showing(), area, (chrome, self.source.as_ref()));
        let origin = visible.iter().find(|shown| &shown.pane == pane)?;
        Some(point(
            px(single_precision(origin.bounds.x + x)),
            px(single_precision(origin.bounds.y + y)),
        ))
    }

    pub(super) fn feed_known_phases(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let phases = self.link.phases();
        while let Ok(input) = phases.try_recv() {
            self.feed_startup(input, window, cx);
        }
    }

    fn feed_startup(
        &mut self,
        input: startup::Input,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.track_daemon(&input, cx);
        self.startup.update(cx, |gate, cx| gate.apply(input, cx));
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
                startup_command::perform(command, &self.link, cx);
            },
            ShellRequest::DataChanged => self.on_data_changed(cx),
            ShellRequest::Outcome(outcome) => self.on_data_outcome(outcome, window, cx),
            ShellRequest::CloseSettings(back) => {
                self.settings = None;
                window.focus(back.as_ref().unwrap_or(&self.focus), cx);
                cx.notify();
            },
            ShellRequest::Closing(choice) => self.choose_closing(choice, window, cx),
            ShellRequest::ReloadDiscarding(pane) => {
                self.source.native().editors().reload(&pane, cx);
            },
            ShellRequest::SelectProject(slot) => self.select_project(slot),
            ShellRequest::ResumeClosing => self.resume_closing(window, cx),
            ShellRequest::KeymapChanged => self.on_keymap_changed(window, cx),
            ShellRequest::AppearanceChanged => self.apply_appearance(cx),
            ShellRequest::PaneSettingsChanged => self.push_pane_settings(),
            ShellRequest::NewTabChoice { pane, action } => {
                self.choose_in_new_tab((&pane, action), window, cx);
            },
            ShellRequest::Browse { pane, event } => self.browse(&pane, event, cx),
            ShellRequest::Agent(command) => self.run_agent(&command, cx),
            ShellRequest::Command(command) => self.run_shell_command(command, window, cx),
            ShellRequest::Terminal { pane, event } => self.on_terminal((&pane, event), window, cx),
            ShellRequest::TerminalSession { pane, report } => {
                self.on_terminal_session((&pane, report), cx);
            },
            ShellRequest::Link { pane, url, choice } => self.choose_link((&pane, url, choice), cx),
            ShellRequest::Contextual(command) => self.run_contextual(command, window, cx),
            ShellRequest::Document(request) => self.on_document(request, window, cx),
            ShellRequest::FirstRun(event) => self.on_oobe(event, window, cx),
        }
    }

    pub(super) fn on_sidebar_command(
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
                let hosts = settings::PageId::from(crate::settings_modal::HOSTS_PAGE);
                self.show_settings_page(Some(hosts), window, cx);
            },
            Ok(SidebarAction::OpenFile(path)) => self.open_workspace_file(&path, cx),
            Ok(SidebarAction::PreviewFile(path)) => self.preview_workspace_file(&path, cx),
            Ok(SidebarAction::OpenReview(path)) => self.open_review_at(&path, cx),
            Ok(SidebarAction::ImportRepo) => {
                self.show_folder_import(folderimport::Kind::Repo, window, cx);
            },
            Ok(SidebarAction::ChooseSpaceFolder(host)) => self.choose_space_folder(&host, cx),
            Ok(SidebarAction::ImportBranches(row)) => self.show_branch_import(&row, window, cx),
            Ok(SidebarAction::Reveal(path)) => self.reveal_workspace_path(&path),
            Ok(SidebarAction::SetFileTreeFlag { key, on }) => {
                self.set_file_tree_flag((key, settings::Value::Bool(on)), cx);
            },
            Ok(SidebarAction::Unserved) => self.perform_file_action(command, window, cx),
            Err(error) => tracing::warn!(%error, "a sidebar command was invalid"),
        }
    }

    pub(super) fn open_from_sidebar(
        &mut self,
        (type_id, params): (protocol::PaneTypeId, protocol::Params),
        cx: &mut Context<'_, Self>,
    ) {
        self.state.open_from_sidebar((type_id, params));
        self.refresh(cx);
    }
}

type Step<V, T> = fn(&mut V, T, &mut Window, &mut Context<'_, V>);

pub(super) async fn drain<V: 'static, T>(
    source: async_channel::Receiver<T>,
    this: gpui::WeakEntity<V>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<V, T>,
) {
    while stepped(&source, &this, cx, step).await {}
}

async fn stepped<V: 'static, T>(
    source: &async_channel::Receiver<T>,
    this: &gpui::WeakEntity<V>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<V, T>,
) -> bool {
    let Ok(item) = source.recv().await else {
        return false;
    };
    cx.update(|window, cx| this.update(cx, |view, cx| step(view, item, window, cx)))
        .flatten()
        .is_ok()
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, IntoElement, Render, TestAppContext};

    use super::*;

    #[derive(Default)]
    struct Collector {
        seen: Vec<u8>,
        windows: Vec<gpui::WindowId>,
    }

    struct Reader(gpui::Entity<Collector>);

    impl Render for Reader {
        fn render(
            &mut self,
            _: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            self.0.read(cx);
            gpui::Empty
        }
    }

    impl Render for Collector {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            gpui::Empty
        }
    }

    fn collect(
        view: &mut Collector,
        item: u8,
        window: &mut Window,
        _: &mut Context<'_, Collector>,
    ) {
        view.seen.push(item);
        view.windows.push(window.window_handle().window_id());
    }

    #[gpui::test]
    fn closing_settings_gives_the_keyboard_back_to_what_held_it(cx: &mut TestAppContext) {
        let mut opened = crate::shell_view::tests::open_shell(cx);
        let gear = opened.window.update(|_, cx| cx.focus_handle());
        let back = Some(gear.clone());
        let on_gear = opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.handle(ShellRequest::CloseSettings(back), window, cx);
                gear.is_focused(window)
            });
        assert!(on_gear, "the keyboard went back to the gear");
    }

    fn feed(
        opened: &mut crate::shell_view::tests::Opened,
        input: startup::Input,
    ) {
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.feed_startup(input, window, cx);
            });
    }

    #[gpui::test]
    fn the_startup_view_takes_only_failures_and_the_window_hears_of_the_rest(
        cx: &mut TestAppContext
    ) {
        let mut opened = crate::shell_view::tests::open_shell(cx);
        let shown = |opened: &mut crate::shell_view::tests::Opened| {
            opened.window.run_until_parked();
            opened
                .shell
                .read_with(&opened.window, |shell, cx| shell.notices.read(cx).len())
        };
        for input in [
            startup::Input::Restarted,
            startup::Input::Unavailable(startup::Outage::Unresponsive),
        ] {
            let before = shown(&mut opened);
            feed(&mut opened, input.clone());
            assert_eq!(shown(&mut opened), before + 1, "{input:?} is a toast");
        }
    }

    #[gpui::test]
    async fn drain_hands_every_item_to_the_view_in_order_and_ends_with_the_channel(
        cx: &mut TestAppContext
    ) {
        let (view, cx) = cx.add_window_view(|_, _| Collector::default());
        let (sender, receiver) = async_channel::unbounded();
        for item in [3, 1, 2] {
            sender.try_send(item).unwrap();
        }
        drop(sender);
        let weak = view.downgrade();
        let task = cx.update(|window, cx| {
            window.spawn(cx, async move |cx| drain(receiver, weak, cx, collect).await)
        });
        task.await;
        let seen = view.read_with(cx, |view, _| view.seen.clone());
        assert_eq!(seen, [3, 1, 2]);
    }

    fn open_reader(
        read: gpui::Entity<Collector>,
        cx: &mut gpui::App,
    ) -> gpui::WindowHandle<Reader> {
        let options = gpui::WindowOptions::default();
        cx.open_window(options, |_, cx| cx.new(|_| Reader(read)))
            .unwrap()
    }

    #[gpui::test]
    async fn drain_steps_in_the_window_it_runs_in_though_another_window_drew_the_view_last(
        cx: &mut TestAppContext
    ) {
        let (view, cx) = cx.add_window_view(|_, _| Collector::default());
        let own = cx.update(|window, _| window.window_handle().window_id());
        let read = view.clone();
        let other = cx.update(|_, cx| open_reader(read, cx));
        other.update(cx, |_, window, _| window.refresh()).unwrap();
        cx.run_until_parked();
        let (sender, receiver) = async_channel::unbounded();
        sender.try_send(1).unwrap();
        drop(sender);
        let weak = view.downgrade();
        let task = cx.update(|window, cx| {
            window.spawn(cx, async move |cx| drain(receiver, weak, cx, collect).await)
        });
        task.await;
        let windows = view.read_with(cx, |view, _| view.windows.clone());
        assert_eq!(
            windows,
            [own],
            "a float window drawing a view does not move its tasks"
        );
    }
}
