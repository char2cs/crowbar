use std::path::Path;

use browser::PageUrl;
use panehost::{HostEvent, IpcMessage, LayoutSnapshot, Overlays, PaneKinds, VisiblePane};
use protocol::PaneId;
use webview::ParentWindow;

use crate::shell::shell_state::ShellState;
use crate::shell::shell_state::agent::page_document::page_document;
use layout::Command;

use crate::panes::content::pane_toast_change::PaneToastChange;
use crate::shell::shell_state::host::event_commands::commands_for;
use crate::shell::shell_state::host::outcome::Outcome;

impl ShellState {
    pub(crate) fn sync(
        &mut self,
        parent: ParentWindow,
        (visible, kinds): (Vec<VisiblePane>, &dyn PaneKinds),
        overlays: Overlays,
    ) -> Outcome {
        let focus = self.showing().and_then(layout::Tab::focused).cloned();
        let snapshot = LayoutSnapshot::from_layout(self.history.layout(), parent, visible)
            .with_focus(focus)
            .with_keyboard(self.keyboard)
            .with_overlays(overlays)
            .with_kinds(kinds);
        let report = self.host.sync(&snapshot);
        if let Some(error) = report.failure {
            tracing::error!(%error, "a pane could not be driven while syncing with the layout");
        }
        self.react(report.events)
    }

    pub(crate) fn apply_keyboard(&self) {
        if let Err(error) = self.host.apply_keyboard() {
            tracing::warn!(%error, "giving the keyboard back to its owner failed");
        }
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
        self.run(commands_for(&events));
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
                self.mark_dirty(pane);
                outcome.dirty_changed = true;
            },
            HostEvent::DirtyChanged { pane, dirty: false } => {
                self.clear_dirty(&pane);
                outcome.dirty_changed = true;
            },
            HostEvent::OpenRequested { from, open } => {
                outcome.opens.extend(self.requested_open(&from, open));
            },
            HostEvent::CommandRequested { command, .. } => outcome.commands.push(command),
            HostEvent::HoverChanged { pane, hovered } => outcome.hover.push((pane, hovered)),
            HostEvent::PageFocused { pane } => outcome.focused.push(pane),
            HostEvent::SettingRequested { key, value, .. } => outcome.settings.push((key, value)),
            HostEvent::OpenExternal { url } => log_failure(platform::open_external(url.as_str())),
            HostEvent::Reveal { path } => {
                log_failure(platform::reveal_in_file_manager(Path::new(path.as_str())));
            },
            HostEvent::ParamsChanged { .. } => outcome.titles_changed = true,
            HostEvent::StateChanged { .. } | HostEvent::StateDiscarded { .. } => {},
            HostEvent::BrowserPage { pane, update } => self.note_page(pane, update, outcome),
            HostEvent::PointerMoved { pane, x, y } => outcome.pointer = Some((pane, x, y)),
            report => note_report(report, outcome),
        }
    }

    pub(crate) fn note_page(
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
        if !PageUrl::is_blank(url) {
            self.replies.loading(pane.as_str());
        }
    }

    fn page_loaded(
        &self,
        pane: &PaneId,
        url: &str,
        outcome: &mut Outcome,
    ) {
        if PageUrl::is_blank(url) {
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

    pub(crate) fn save_address(
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

fn note_report(
    event: HostEvent,
    outcome: &mut Outcome,
) {
    if let HostEvent::Mounted { pane } = event {
        outcome.mounted.push(pane);
        return;
    }
    if let Some(request) = page_document(&event) {
        outcome.documents.push(request);
        return;
    }
    if !matches!(event, HostEvent::ReviewViewedSet { .. }) {
        return outcome.toasts.extend(PaneToastChange::of(event));
    }
    let HostEvent::ReviewViewedSet {
        pane,
        files,
        viewed,
    } = event
    else {
        return;
    };
    let marks = files
        .into_iter()
        .map(|f| (f.path, viewed.then_some(f.signature)));
    outcome.viewed.push((pane, marks.collect()));
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
    use crate::panes::kinds::core_panes::chat_type;
    use crate::shell::shell_state::host::outcome::Outcome;
    use crate::test_support::{ShellRig, shell_rig, synced_chat};

    fn told(
        rig: &mut ShellRig,
        (pane, text): (&PaneId, &str),
    ) -> Outcome {
        let message = IpcMessage {
            pane: pane.clone(),
            text: text.to_owned(),
        };
        rig.shell.handle_ipc(message)
    }

    fn registered() -> String {
        let register = PaneFrame::Register {
            type_id: chat_type().unwrap(),
            sdk: protocol::SDK_VERSION,
        };
        encode(&register).unwrap()
    }

    #[test]
    fn a_chat_takes_the_keyboard_again_when_the_window_is_activated() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        assert!(crate::test_support::page_takes_keys(&mut rig, pane));
    }

    #[test]
    fn a_pointer_move_over_a_page_is_reported_with_the_pane_and_point() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let text = r#"{"sdk":1,"kind":"shell.pointer","x":3.5,"y":120.0}"#;
        let outcome = told(&mut rig, (&pane, text));
        assert_eq!(outcome.pointer, Some((pane, 3.5, 120.0)), "the latest move");
    }

    #[test]
    fn a_pane_toast_and_its_close_reach_the_toast_host() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let frame_cases = [
            r#"{"sdk":1,"kind":"shell.toast","key":"t1","tone":"error","title":"Save the file"}"#,
            r#"{"sdk":1,"kind":"shell.toastClose","key":"t1"}"#,
        ];
        let mut toasts = Vec::new();
        for text in frame_cases {
            toasts.extend(told(&mut rig, (&pane, text)).toasts);
        }
        let key = protocol::DisplayText::new("t1").unwrap();
        let shown = protocol::PaneToast {
            key: key.clone(),
            tone: protocol::ToastTone::Error,
            title: protocol::DisplayText::new("Save the file").unwrap(),
            description: None,
            pinned: false,
        };
        assert_eq!(
            toasts,
            [
                PaneToastChange::Shown(pane.clone(), shown),
                PaneToastChange::Closed(pane, key)
            ]
        );
    }

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

    #[test]
    fn pane_state_flows_into_the_layout() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let state = PaneState::new(json!({ "draft": "hi" })).unwrap();
        let sent = PaneFrame::SetState {
            state: state.clone(),
        };
        told(&mut rig, (&pane, &encode(&sent).unwrap()));
        let stored = rig.shell.layout().pane(&pane).map(|p| p.state.clone());
        assert_eq!(stored, Some(state), "the model owns the new state");
    }

    #[test]
    fn a_page_title_is_kept_until_its_pane_closes() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let title = PaneFrame::SetTitle {
            title: PaneTitle::new("Fix the build").unwrap(),
        };
        let outcome = told(&mut rig, (&pane, &encode(&title).unwrap()));
        assert!(outcome.titles_changed, "the tab strip must refresh");
        let kept = rig.shell.titles().get(&pane).map(|t| t.to_string());
        assert_eq!(kept.as_deref(), Some("Fix the build"), "kept");
        rig.shell
            .apply(vec![Command::ClosePane { pane: pane.clone() }]);
        assert!(rig.shell.titles().get(&pane).is_none(), "forgotten");
    }

    #[test]
    fn a_mounted_page_and_a_viewed_mark_reach_the_outcome_with_their_pane() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let mounted = told(&mut rig, (&pane, &registered()));
        let text = r#"{"sdk":1,"kind":"review.setViewed","files":[{"path":"a.ts","signature":"M||1|0"},{"path":"b.ts","signature":"A||2|0"}],"viewed":false}"#;
        let marked = told(&mut rig, (&pane, text));
        assert_eq!(mounted.mounted, std::slice::from_ref(&pane));
        let cleared = vec![("a.ts".to_owned(), None), ("b.ts".to_owned(), None)];
        assert_eq!(marked.viewed, [(pane, cleared)]);
    }
}
