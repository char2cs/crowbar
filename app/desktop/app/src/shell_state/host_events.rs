use std::path::Path;

use browser::BrowserUrl;
use pane_host::{HostEvent, IpcMessage, LayoutSnapshot, VisiblePane};
use protocol::PaneId;
use webview::ParentWindow;

use super::ShellState;
use model::Command;

use crate::dropped_files;
use crate::event_commands::commands_for;
use crate::outcome::Outcome;

impl ShellState {
    pub(crate) fn sync(
        &mut self,
        parent: ParentWindow,
        visible: Vec<VisiblePane>,
    ) -> Outcome {
        self.frozen.set_holes(&visible);
        let snapshot = LayoutSnapshot::from_layout(self.history.layout(), parent, visible)
            .with_keyboard(self.keyboard);
        let report = self.host.sync(&snapshot);
        if let Some(error) = report.failure {
            tracing::error!(%error, "a pane could not be driven while syncing with the layout");
        }
        self.react(report.events)
    }

    pub(crate) fn handle_ipc(
        &mut self,
        message: IpcMessage,
    ) -> Outcome {
        match self.host.handle_ipc(message, self.history.layout()) {
            Ok(events) => self.react(events),
            Err(error) => {
                tracing::warn!(%error, "handling a pane message failed");
                Outcome::default()
            },
        }
    }

    fn react(
        &mut self,
        events: Vec<HostEvent>,
    ) -> Outcome {
        self.apply(commands_for(&events));
        let mut outcome = Outcome::default();
        for event in events {
            self.note(event, &mut outcome);
        }
        outcome
    }

    fn note(
        &mut self,
        event: HostEvent,
        outcome: &mut Outcome,
    ) {
        match event {
            HostEvent::TitleChanged { pane, title } => {
                self.titles.set(pane, title);
                outcome.titles_changed = true;
            },
            HostEvent::DirtyChanged { pane, dirty: true } => {
                self.dirty.mark(pane);
                outcome.dirty_changed = true;
            },
            HostEvent::DirtyChanged { pane, dirty: false } => {
                self.dirty.clear(&pane);
                outcome.dirty_changed = true;
            },
            HostEvent::OpenRequested {
                type_id, params, ..
            } => self.open_pane(type_id, params),
            HostEvent::CommandRequested { command, .. } => outcome.commands.push(command),
            HostEvent::HoverChanged { pane, hovered } => outcome.hover.push((pane, hovered)),
            HostEvent::PageFocused { pane } => outcome.focused.push(pane),
            HostEvent::SettingRequested { key, value, .. } => outcome.settings.push((key, value)),
            HostEvent::OpenExternal { url } => {
                log_failure(platform::open_external(url.as_str()).map_err(|e| e.to_string()));
            },
            HostEvent::Reveal { path } => log_failure(
                platform::reveal_in_file_manager(Path::new(path.as_str()))
                    .map_err(|e| e.to_string()),
            ),
            HostEvent::FilesDropped { pane, paths }
                if dropped_files::opens_files(self.history.layout(), &pane) =>
            {
                outcome.dropped.extend(paths);
            },
            HostEvent::StateChanged { .. }
            | HostEvent::StateDiscarded { .. }
            | HostEvent::FilesDropped { .. } => {},
            HostEvent::BrowserPage { pane, update } => self.note_page(pane, update, outcome),
        }
    }

    pub(super) fn note_page(
        &mut self,
        pane: PaneId,
        update: browser::PageUpdate,
        outcome: &mut Outcome,
    ) {
        self.pages.apply(pane.clone(), update.clone());
        match update {
            browser::PageUpdate::Loading { url } => {
                self.page_loading(&pane, &url);
                outcome.pages.push((pane, url));
            },
            browser::PageUpdate::Loaded { url } => {
                self.page_loaded(&pane, &url, outcome);
                outcome.pages.push((pane.clone(), url.clone()));
                self.save_address(pane, &url);
            },
            browser::PageUpdate::Title { title } => self.set_page_title(pane, title, outcome),
            browser::PageUpdate::AgentReply { id, answer } => {
                self.answer_from_page(&pane, &id, answer);
            },
            pick => outcome.picks.push((pane, pick)),
        }
    }

    fn page_loading(
        &self,
        pane: &PaneId,
        url: &str,
    ) {
        if !BrowserUrl::is_blank(url) {
            self.replies.loading(pane.as_str());
        }
    }

    fn page_loaded(
        &self,
        pane: &PaneId,
        url: &str,
        outcome: &mut Outcome,
    ) {
        if BrowserUrl::is_blank(url) {
            return;
        }
        self.replies.loaded(pane.as_str(), &self.page_shown(pane));
        outcome.loaded.push(pane.clone());
    }

    fn set_page_title(
        &mut self,
        pane: PaneId,
        title: String,
        outcome: &mut Outcome,
    ) {
        let Ok(title) = protocol::PaneTitle::new(title) else {
            return;
        };
        self.titles.set(pane, title);
        outcome.titles_changed = true;
    }

    fn save_address(
        &mut self,
        pane: PaneId,
        url: &str,
    ) {
        let saved = serde_json::json!({ "url": url });
        if let Ok(state) = protocol::PaneState::new(saved) {
            self.apply(vec![Command::SetPaneState { pane, state }]);
        }
    }
}

fn log_failure<E: std::fmt::Display>(served: Result<(), E>) {
    if let Err(error) = served {
        tracing::warn!(%error, "a pane's request could not be served");
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneFrame, PaneState, PaneTitle, encode};
    use serde_json::json;
    use webview::Call;

    use super::*;
    use crate::test_support::{shell_rig, synced_chat};

    #[test]
    fn an_opened_chat_gets_a_web_view_in_the_window() {
        let mut rig = shell_rig();
        synced_chat(&mut rig);
        let calls = rig.mock.calls();
        let created = matches!(
            &calls[0],
            Call::Create {
                parent: ParentWindow::X11(9),
                ..
            }
        );
        assert!(created, "{calls:?}");
        let navigated = matches!(&calls[1], Call::Navigate { url, .. }
            if url.starts_with("crowbar://local/panes/core/chat/index.html"));
        assert!(navigated, "{calls:?}");
    }

    fn stored_after_sending(draft: &str) -> (PaneState, Option<PaneState>) {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let state = PaneState::new(json!({ "draft": draft })).unwrap();
        let sent = PaneFrame::SetState {
            state: state.clone(),
        };
        let text = encode(&sent).unwrap();
        rig.shell.handle_ipc(IpcMessage {
            pane: pane.clone(),
            text,
        });
        let stored = rig.shell.layout().pane(&pane).map(|p| p.state.clone());
        (state, stored)
    }

    #[test]
    fn pane_state_flows_into_the_layout() {
        let cases = ["hi", "hello again"];
        for draft in cases {
            let (sent, stored) = stored_after_sending(draft);
            assert_eq!(stored, Some(sent), "{draft}: the model owns the new state");
        }
    }

    #[test]
    fn a_page_title_is_kept_until_its_pane_closes() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let text = encode(&PaneFrame::SetTitle {
            title: PaneTitle::new("Fix the build").unwrap(),
        })
        .unwrap();
        let outcome = rig.shell.handle_ipc(IpcMessage {
            pane: pane.clone(),
            text,
        });
        assert!(outcome.titles_changed, "the tab strip must refresh");
        let kept = rig.shell.titles().get(&pane).map(|t| t.to_string());
        assert_eq!(kept.as_deref(), Some("Fix the build"), "kept");
        rig.shell
            .apply(vec![Command::ClosePane { pane: pane.clone() }]);
        assert!(rig.shell.titles().get(&pane).is_none(), "forgotten");
    }
}
