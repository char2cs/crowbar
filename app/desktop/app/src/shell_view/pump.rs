use gpui::{Context, Window};
use panehost::IpcMessage;
use protocol::PaneId;
use viewtabs::{MemberKind, MemberView};

use super::ShellView;
use crate::shell_request::ShellRequest;

use crate::chat_label::NEW_CHAT;

const TERMINAL_LABEL: &str = "Terminal";

pub(super) fn member(
    (titles, layout): (&crate::titles::Titles, &layout::Layout),
    pane: &PaneId,
) -> MemberView {
    let title = titles.get(pane);
    let kind = kind_of(layout, pane);
    MemberView {
        pane: pane.clone(),
        label: title
            .as_ref()
            .map_or_else(|| untitled(kind).into(), |title| title.to_string().into()),
        provisional: title.is_none(),
        working: false,
        kind,
    }
}

fn kind_of(
    layout: &layout::Layout,
    pane: &PaneId,
) -> MemberKind {
    match layout.pane(pane).map(|instance| instance.type_id.as_str()) {
        Some(crate::core_panes::TERMINAL) => MemberKind::Terminal,
        Some(crate::core_panes::NEW_TAB) => MemberKind::NewTab,
        _ => MemberKind::Chat,
    }
}

fn untitled(kind: MemberKind) -> &'static str {
    if kind == MemberKind::Terminal {
        return TERMINAL_LABEL;
    }
    NEW_CHAT
}

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
            ShellRequest::Agent(command) => self.run_agent(&command, cx),
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
                let hosts = settings::PageId::from(crate::settings_modal::HOSTS_PAGE);
                self.show_settings_page(Some(hosts), window, cx);
            },
            Ok(SidebarAction::OpenFile(path)) => self.open_workspace_file(&path, cx),
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
    use super::*;
    use crate::core_panes::{CHAT, NEW_TAB, TERMINAL};
    use crate::titles::Titles;

    type Case = (
        &'static str,
        Option<&'static str>,
        (MemberKind, &'static str, bool),
    );

    const CASES: [Case; 5] = [
        (CHAT, Some("Review"), (MemberKind::Chat, "Review", false)),
        (TERMINAL, Some("zsh"), (MemberKind::Terminal, "zsh", false)),
        (CHAT, None, (MemberKind::Chat, NEW_CHAT, true)),
        (TERMINAL, None, (MemberKind::Terminal, "Terminal", true)),
        (NEW_TAB, None, (MemberKind::NewTab, NEW_CHAT, true)),
    ];

    fn layout_holding(
        pane: &PaneId,
        type_id: &str,
    ) -> layout::Layout {
        let mut layout = layout::Layout::default();
        let tab = layout::TabId(1);
        layout
            .apply(layout::Command::AddTab {
                id: tab,
                title: "t".into(),
            })
            .unwrap();
        let instance = layout::PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new(type_id).unwrap(),
            params: protocol::Params::default(),
            state: protocol::PaneState::default(),
        };
        layout
            .apply(layout::Command::OpenPane {
                tab,
                instance,
                placement: None,
            })
            .unwrap();
        layout
    }

    fn titled(
        pane: &PaneId,
        title: Option<&str>,
    ) -> Titles {
        let titles = Titles::default();
        if let Some(text) = title {
            titles.set(pane.clone(), protocol::PaneTitle::new(text).unwrap());
        }
        titles
    }

    #[test]
    fn a_member_carries_its_pane_and_a_title_or_the_kind_default_until_one_arrives() {
        for (type_id, title, (kind, label, provisional)) in CASES {
            let pane = PaneId::new("p7").unwrap();
            let titles = titled(&pane, title);
            let member = member((&titles, &layout_holding(&pane, type_id)), &pane);
            assert_eq!(
                (member.pane, member.label.as_ref(), member.kind),
                (pane, label, kind),
                "{type_id} {title:?}"
            );
            assert_eq!(member.provisional, provisional, "{type_id} {title:?}");
            assert!(!member.working, "{type_id} {title:?}");
        }
    }
}
