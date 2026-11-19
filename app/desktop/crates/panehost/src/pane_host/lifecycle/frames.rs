use layout::Layout;
use protocol::{ExtensionStatus, Lifecycle, PaneFrame, PaneId, PaneTypeId, ShellFrame, StatusKind};

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::context::setting_event;
use crate::pane_host::lifecycle::pane_phase::PanePhase;
use crate::pane_host::lifecycle::pane_runtime::PaneRuntime;
use crate::pane_host::lifecycle::stage::Stage;
use crate::pane_host::reconcile::Keyboard;
use crate::slot::HostEvent;

fn wanted_stage(
    runtime: &PaneRuntime,
    id: &PaneId,
    focused: Option<&PaneId>,
) -> Stage {
    if runtime.is_mounted() && focused == Some(id) {
        return Stage::Active;
    }
    Stage::Shown
}

fn lifecycle_for(stage: Stage) -> Lifecycle {
    if stage == Stage::Active {
        return Lifecycle::Activate;
    }
    Lifecycle::Deactivate
}

impl PaneHost {
    pub(crate) fn route_activation(
        &mut self,
        layout: &Layout,
    ) -> Result<()> {
        let focused = self.focused.as_ref();
        let changes: Vec<(PaneId, Stage)> = self
            .panes
            .iter()
            .filter(|(_, runtime)| runtime.is_visible())
            .map(|(id, runtime)| (id.clone(), wanted_stage(runtime, id, focused)))
            .filter(|(id, wanted)| self.panes.get(id).is_some_and(|r| r.stage != *wanted))
            .collect();
        changes
            .into_iter()
            .try_for_each(|(pane, stage)| self.set_activation(&pane, stage, layout))
    }

    pub(crate) fn set_activation(
        &mut self,
        pane: &PaneId,
        stage: Stage,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.stage = stage;
        let frame = runtime.lifecycle_frame(pane, lifecycle_for(stage), layout.pane(pane));
        let slot = runtime.slot;
        self.send(pane, &frame)?;
        match (stage, slot.and_then(|id| self.pool.get(id))) {
            (Stage::Active, Some(slot)) if self.keyboard == Keyboard::Pane => {
                Ok(slot.handle.focus()?)
            },
            _ => Ok(()),
        }
    }

    pub(crate) fn on_frame(
        &mut self,
        pane: PaneId,
        frame: PaneFrame,
        layout: &Layout,
    ) -> Result<Vec<HostEvent>> {
        let event = match frame {
            PaneFrame::Register { type_id, .. } => {
                return self
                    .on_register(&pane, &type_id, layout)
                    .map(|()| Vec::new());
            },
            PaneFrame::SetTitle { title } => HostEvent::TitleChanged { pane, title },
            PaneFrame::SetState { state } => HostEvent::StateChanged { pane, state },
            PaneFrame::SetDirty { dirty } => HostEvent::DirtyChanged { pane, dirty },
            PaneFrame::Open { type_id, params } => HostEvent::OpenRequested {
                from: pane,
                type_id,
                params,
            },
            PaneFrame::Request { id, method, .. } => {
                return self.refuse_request(&pane, id, &method);
            },
            PaneFrame::Subscribe { event, .. } => {
                return self.refuse(&pane, format!("no extension serves the event {event}"));
            },
            PaneFrame::Unsubscribe { .. } => return Ok(Vec::new()),
            frame @ PaneFrame::SettingsSet { .. } => return Ok(setting_event(pane, frame)),
            PaneFrame::RunCommand { command } => HostEvent::CommandRequested { pane, command },
            PaneFrame::OpenExternal { url } => HostEvent::OpenExternal { url },
            PaneFrame::Reveal { path } => HostEvent::Reveal { path },
        };
        Ok(vec![event])
    }

    fn on_register(
        &mut self,
        pane: &PaneId,
        type_id: &PaneTypeId,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if &runtime.type_id != type_id {
            let message = format!(
                "page registered as {type_id} but was opened as {}",
                runtime.type_id
            );
            return self.fail(pane, message);
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(pane, Lifecycle::Mount, layout.pane(pane));
        runtime.discard_state = false;
        self.send(pane, &mount)?;
        self.send_context(pane)?;
        self.route_activation(layout)
    }

    fn send_context(
        &self,
        pane: &PaneId,
    ) -> Result<()> {
        if !self.keybindings.is_empty() {
            let bindings = self.keybindings.clone();
            self.send(pane, &ShellFrame::Keybindings { bindings })?;
        }
        if let Some(tokens) = self.theme.clone() {
            self.send(pane, &ShellFrame::Theme { tokens })?;
        }
        match self.settings_context() {
            Some(frame) => self.send(pane, &frame),
            None => Ok(()),
        }
    }

    pub(crate) fn fail(
        &mut self,
        pane: &PaneId,
        message: String,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.phase = PanePhase::Failed(message);
        runtime.stage = Stage::Shown;
        self.refresh_native_visibility(pane)
    }

    pub(crate) fn apply_status(
        &mut self,
        pane: &PaneId,
        status: &ExtensionStatus,
        layout: &Layout,
    ) -> Result<()> {
        if self.panes.get(pane).is_some_and(PaneRuntime::is_mounted) {
            self.send(pane, &ShellFrame::ExtensionStatus(status.clone()))?;
        }
        match status.status() {
            StatusKind::Unavailable | StatusKind::Disabled => {
                let reason = status.reason().map_or_else(
                    || format!("extension {:?}", status.status()),
                    ToString::to_string,
                );
                self.fail(pane, reason)
            },
            StatusKind::Running | StatusKind::Back => self.recover(pane, layout),
        }
    }

    fn recover(
        &mut self,
        pane: &PaneId,
        layout: &Layout,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if !matches!(runtime.phase, PanePhase::Failed(_)) || runtime.slot.is_none() {
            return Ok(());
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(pane, Lifecycle::Mount, layout.pane(pane));
        self.send(pane, &mount)?;
        self.refresh_native_visibility(pane)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{CommandId, DisplayText, ExtensionId, PaneFrame, PaneState, PaneTitle, encode};
    use serde_json::json;
    use webview::{Call, Visibility};

    use super::*;
    use crate::pane_host::reconcile::PaneDisplay;
    use crate::slot::IpcMessage;
    use crate::test_support::{Fixture, fixture, instance, lifecycle_events, register};

    #[test]
    fn a_page_takes_native_focus_on_activation_only_while_the_keyboard_is_the_pages() {
        let cases = [
            ("the pane holds the keyboard", Keyboard::Pane, true, false),
            ("the shell holds the keyboard", Keyboard::Shell, false, true),
        ];
        for (name, keyboard, focuses_page, returns_to_shell) in cases {
            let mut f = fixture();
            let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
            f.sync(&panes, &["p1", "p2"], Some("p1"));
            register(&mut f, "p1", "core.chat");
            register(&mut f, "p2", "core.chat");
            let before = f.mock.calls().len();
            f.sync_with(&panes, &["p1", "p2"], (Some("p2"), keyboard));
            let calls = f.mock.calls();
            let after = calls.iter().skip(before);
            let focused = after.clone().any(|call| matches!(call, Call::Focus { .. }));
            let returned = after
                .clone()
                .any(|call| matches!(call, Call::FocusParent { .. }));
            assert_eq!(
                (focused, returned),
                (focuses_page, returns_to_shell),
                "{name}"
            );
        }
    }

    pub(crate) fn started() -> Fixture {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f
    }

    fn post(
        f: &mut Fixture,
        frame: &PaneFrame,
    ) -> Vec<HostEvent> {
        let text = encode(frame).unwrap();
        f.ipc(IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text,
        })
    }

    type Case = (&'static str, PaneFrame, HostEvent);

    fn assert_events(cases: Vec<Case>) {
        for (name, frame, expected) in cases {
            assert_eq!(post(&mut started(), &frame), [expected], "{name}");
        }
    }

    #[test]
    fn content_frames_become_their_events() {
        let pane = PaneId::new("p1").unwrap();
        let title = PaneTitle::new("Hi").unwrap();
        let state = PaneState::new(json!({"k": 1})).unwrap();
        let cases: Vec<Case> = vec![
            (
                "a title frame",
                PaneFrame::SetTitle {
                    title: title.clone(),
                },
                HostEvent::TitleChanged {
                    pane: pane.clone(),
                    title,
                },
            ),
            (
                "a state frame",
                PaneFrame::SetState {
                    state: state.clone(),
                },
                HostEvent::StateChanged { pane, state },
            ),
        ];
        assert_events(cases);
    }

    #[test]
    fn control_frames_become_their_events() {
        let pane = PaneId::new("p1").unwrap();
        let command = CommandId::new("palette.open").unwrap();
        let cases: Vec<Case> = vec![
            (
                "a dirty frame",
                PaneFrame::SetDirty { dirty: true },
                HostEvent::DirtyChanged {
                    pane: pane.clone(),
                    dirty: true,
                },
            ),
            (
                "a command frame",
                PaneFrame::RunCommand {
                    command: command.clone(),
                },
                HostEvent::CommandRequested { pane, command },
            ),
        ];
        assert_events(cases);
    }

    #[test]
    fn stale_messages_from_a_pane_without_a_web_view_are_ignored() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        let stale = IpcMessage {
            pane: PaneId::new("p2").unwrap(),
            text: "{}".into(),
        };
        assert!(f.ipc(stale).is_empty(), "p2 has no web view; ignored");
        assert_eq!(
            f.host.display(&PaneId::new("p2").unwrap()),
            PaneDisplay::Empty,
            "p2 untouched"
        );
    }

    #[test]
    fn a_page_that_registers_the_wrong_type_fails_inside_its_own_frame() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        register(&mut f, "p1", "core.term");
        assert!(
            matches!(
                f.host.display(&PaneId::new("p1").unwrap()),
                PaneDisplay::Error { .. }
            ),
            "error display"
        );
        assert!(
            matches!(
                f.mock.calls().last(),
                Some(Call::SetVisibility {
                    visibility: Visibility::Hidden,
                    ..
                })
            ),
            "web view hidden"
        );
        assert!(lifecycle_events(&f.mock).is_empty(), "never mounted");
    }

    #[test]
    fn extension_panes_get_their_own_web_view_kind_and_react_to_status() {
        let mut f = fixture();
        let review = instance("r", "ext.git.review");
        f.sync(&[review], &["r"], Some("r"));
        let git = ExtensionId::new("git").unwrap();
        assert!(
            matches!(&f.mock.calls()[0], Call::Create { kind: webview::Kind::Extension(e), .. } if e.as_str() == git.as_str()),
            "extension view"
        );
        register(&mut f, "r", "ext.git.review");
        let down = ExtensionStatus::new(
            git.clone(),
            StatusKind::Unavailable,
            Some(DisplayText::new("crashed").unwrap()),
        );
        f.host.extension_status(&down, &f.layout).unwrap();
        assert_eq!(
            f.host.display(&PaneId::new("r").unwrap()),
            PaneDisplay::Error {
                message: "crashed".into()
            },
            "error in frame"
        );
        let back = ExtensionStatus::new(git, StatusKind::Back, None);
        f.host.extension_status(&back, &f.layout).unwrap();
        assert_eq!(
            f.host.display(&PaneId::new("r").unwrap()),
            PaneDisplay::Web,
            "recovered"
        );
        let mounts = lifecycle_events(&f.mock)
            .iter()
            .filter(|e| **e == Lifecycle::Mount)
            .count();
        assert_eq!(mounts, 2, "mount re-sent with the saved state");
    }

    #[test]
    fn the_pointer_report_becomes_a_hover_event() {
        let mut f = started();
        let report = IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text: r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#.into(),
        };
        assert_eq!(
            f.ipc(report),
            [HostEvent::HoverChanged {
                pane: PaneId::new("p1").unwrap(),
                hovered: true
            }],
            "hover"
        );
    }
}
