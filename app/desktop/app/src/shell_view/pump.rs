use gpui::{Context, MouseMoveEvent, PlatformInput, Window, point, px};
use panehost::IpcMessage;
use protocol::PaneId;

use super::ShellView;
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
        let Some((_, area)) = self.placement else {
            return;
        };
        let (chrome, _) = self.chrome(cx);
        let visible = visible_panes(
            self.state.layout().active_tab(),
            area,
            (chrome, self.source.as_ref()),
        );
        let Some(origin) = visible.iter().find(|shown| shown.pane == pane) else {
            return;
        };
        let position = point(
            px(single_precision(origin.bounds.x + x)),
            px(single_precision(origin.bounds.y + y)),
        );
        let moved = MouseMoveEvent {
            position,
            pressed_button: None,
            modifiers: window.modifiers(),
        };
        window.dispatch_event(PlatformInput::MouseMove(moved), cx);
    }

    fn feed_startup(
        &mut self,
        input: startup::Input,
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
                window.focus(&self.focus, cx);
                cx.notify();
            },
            ShellRequest::Closing(choice) => self.choose_closing(choice, window, cx),
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
                let hosts = settings::PageId::from(crate::settings_modal::HOSTS_PAGE);
                self.show_settings_page(Some(hosts), window, cx);
            },
            Ok(SidebarAction::OpenFile(path)) => self.open_workspace_file(&path, cx),
            Ok(SidebarAction::PreviewFile(path)) => self.preview_workspace_file(&path, cx),
            Ok(SidebarAction::OpenReview(path)) => self.open_review_at(&path, cx),
            Ok(SidebarAction::ImportRepo) => self.choose_repo_folder(window, cx),
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
        let chat = params
            .get("threadId")
            .and_then(|chat| self.data.chat_terminal(chat));
        let (type_id, params) = crate::chat_route::route((type_id, params), chat);
        self.state.open_view(type_id, params);
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
    this.update_in(cx, |view, window, cx| step(view, item, window, cx))
        .is_ok()
}

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, Render, TestAppContext};

    use super::*;

    #[derive(Default)]
    struct Collector {
        seen: Vec<u8>,
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
        _: &mut Window,
        _: &mut Context<'_, Collector>,
    ) {
        view.seen.push(item);
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
}
