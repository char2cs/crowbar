use layout::Layout;
use protocol::{Lifecycle, PaneFrame, PaneId, PaneTypeId, ShellFrame};

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

fn chat_fills(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let group = layout.tab_of(pane).and_then(|tab| tab.group_of(pane));
    group.is_some_and(|group| {
        group.chat() == Some(pane) && group.tabs().is_empty() && !group.editor_open()
    })
}

fn toast_events(
    pane: PaneId,
    frame: PaneFrame,
) -> Vec<HostEvent> {
    let event = match frame {
        PaneFrame::Toast(toast) => HostEvent::ToastShown { pane, toast },
        PaneFrame::ToastClose { key } => HostEvent::ToastClosed { pane, key },
        PaneFrame::OpenRefused { title, message } => HostEvent::OpenRefused {
            pane,
            title,
            message,
        },
        _ => return Vec::new(),
    };
    vec![event]
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
                self.on_register(&pane, &type_id, layout)?;
                let mounted = self.panes.get(&pane).is_some_and(PaneRuntime::is_mounted);
                return Ok(mounted
                    .then_some(HostEvent::Mounted { pane })
                    .into_iter()
                    .collect());
            },
            PaneFrame::SetTitle { title } => HostEvent::TitleChanged { pane, title },
            PaneFrame::SetParams { params } => HostEvent::ParamsChanged { pane, params },
            PaneFrame::SetState { state } => HostEvent::StateChanged { pane, state },
            PaneFrame::SetDirty { dirty } => HostEvent::DirtyChanged { pane, dirty },
            PaneFrame::Open(open) => HostEvent::OpenRequested { from: pane, open },
            PaneFrame::Request { id, method, .. } => {
                return self.refuse_request(&pane, id, &method);
            },
            PaneFrame::Subscribe { event, .. } => {
                return self.refuse(&pane, &format!("no extension serves the event {event}"));
            },
            PaneFrame::Unsubscribe { .. } => return Ok(Vec::new()),
            frame @ PaneFrame::SettingsSet { .. } => return Ok(setting_event(pane, frame)),
            PaneFrame::RunCommand { command } => HostEvent::CommandRequested { pane, command },
            PaneFrame::OpenExternal { url } => HostEvent::OpenExternal { url },
            PaneFrame::Reveal { path } => HostEvent::Reveal { path },
            PaneFrame::ReviewSetViewed { files, viewed } => HostEvent::ReviewViewedSet {
                pane,
                files,
                viewed,
            },
            PaneFrame::DocumentEdit { text } => HostEvent::DocumentEdited { pane, text },
            PaneFrame::DocumentShowSource => HostEvent::SourceRequested { pane },
            PaneFrame::DocumentSave => HostEvent::DocumentSaveRequested { pane },
            frame @ (PaneFrame::Toast(_)
            | PaneFrame::ToastClose { .. }
            | PaneFrame::OpenRefused { .. }) => return Ok(toast_events(pane, frame)),
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
            return self.fail(pane, &message);
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(pane, Lifecycle::Mount, layout.pane(pane));
        runtime.discard_state = false;
        runtime.chat_fills = None;
        self.send(pane, &mount)?;
        self.send_context(pane)?;
        self.send_groups(layout)?;
        self.route_activation(layout)
    }

    pub(crate) fn fail(
        &mut self,
        pane: &PaneId,
        message: &str,
    ) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        tracing::warn!(%pane, %message, "the pane failed");
        runtime.phase = PanePhase::Failed;
        runtime.stage = Stage::Shown;
        self.refresh_native_visibility(pane)
    }

    pub(crate) fn send_groups(
        &mut self,
        layout: &Layout,
    ) -> Result<()> {
        let changed: Vec<(PaneId, bool)> = self
            .panes
            .iter_mut()
            .filter(|(_, runtime)| runtime.is_mounted() && runtime.type_id.is_chat())
            .filter_map(|(id, runtime)| {
                let fills = chat_fills(layout, id);
                let told = runtime.chat_fills.replace(fills);
                (told != Some(fills)).then(|| (id.clone(), fills))
            })
            .collect();
        for (pane, chat_fills) in changed {
            self.send(&pane, &ShellFrame::Group { chat_fills })?;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use protocol::{CommandId, DisplayText, PaneState, PaneTitle, Params, ViewedFile, encode};
    use serde_json::json;

    use super::*;
    use crate::slot::IpcMessage;
    use crate::test_support::{
        Fixture, fixture, frames_sent, instance, lifecycle_events, register,
    };

    fn p1() -> PaneId {
        PaneId::new("p1").unwrap()
    }

    fn started() -> Fixture {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f
    }

    fn post(
        f: &mut Fixture,
        text: &str,
    ) -> Vec<HostEvent> {
        let text = text.to_owned();
        f.ipc(IpcMessage { pane: p1(), text })
    }

    type Case = (PaneFrame, HostEvent);

    fn content_cases(pane: &PaneId) -> [Case; 4] {
        let title = PaneTitle::new("Hi").unwrap();
        let state = PaneState::new(json!({"k": 1})).unwrap();
        let params = Params::default().with("threadId", "c1").unwrap();
        let (pane, dirty) = (pane.clone(), true);
        [
            (
                PaneFrame::SetTitle {
                    title: title.clone(),
                },
                HostEvent::TitleChanged {
                    pane: pane.clone(),
                    title,
                },
            ),
            (
                PaneFrame::SetState {
                    state: state.clone(),
                },
                HostEvent::StateChanged {
                    pane: pane.clone(),
                    state,
                },
            ),
            (
                PaneFrame::SetParams {
                    params: params.clone(),
                },
                HostEvent::ParamsChanged {
                    pane: pane.clone(),
                    params,
                },
            ),
            (
                PaneFrame::SetDirty { dirty },
                HostEvent::DirtyChanged { pane, dirty },
            ),
        ]
    }

    fn command_cases(pane: &PaneId) -> [Case; 2] {
        let command = CommandId::new("palette.open").unwrap();
        let files = vec![ViewedFile {
            path: "src/a.ts".into(),
            signature: "M||1|0".into(),
        }];
        let (pane, viewed) = (pane.clone(), true);
        [
            (
                PaneFrame::RunCommand {
                    command: command.clone(),
                },
                HostEvent::CommandRequested {
                    pane: pane.clone(),
                    command,
                },
            ),
            (
                PaneFrame::ReviewSetViewed {
                    files: files.clone(),
                    viewed,
                },
                HostEvent::ReviewViewedSet {
                    pane: pane.clone(),
                    files,
                    viewed,
                },
            ),
        ]
    }

    fn document_cases(pane: &PaneId) -> [Case; 3] {
        let (pane, text) = (pane.clone(), String::from("Title\n"));
        [
            (
                PaneFrame::DocumentEdit { text: text.clone() },
                HostEvent::DocumentEdited {
                    pane: pane.clone(),
                    text,
                },
            ),
            (
                PaneFrame::DocumentShowSource,
                HostEvent::SourceRequested { pane: pane.clone() },
            ),
            (
                PaneFrame::DocumentSave,
                HostEvent::DocumentSaveRequested { pane },
            ),
        ]
    }

    fn toast_cases(pane: &PaneId) -> [Case; 3] {
        let key = DisplayText::new("save").unwrap();
        let toast = protocol::PaneToast {
            key: key.clone(),
            tone: protocol::ToastTone::Error,
            title: DisplayText::new("Save the file").unwrap(),
            description: None,
            pinned: false,
        };
        let (pane, message) = (pane.clone(), key.clone());
        let title = Some(DisplayText::new("Not found").unwrap());
        [
            (
                PaneFrame::Toast(toast.clone()),
                HostEvent::ToastShown {
                    pane: pane.clone(),
                    toast,
                },
            ),
            (
                PaneFrame::ToastClose { key: key.clone() },
                HostEvent::ToastClosed {
                    pane: pane.clone(),
                    key,
                },
            ),
            (
                PaneFrame::OpenRefused {
                    title: title.clone(),
                    message: message.clone(),
                },
                HostEvent::OpenRefused {
                    pane,
                    title,
                    message,
                },
            ),
        ]
    }

    #[test]
    fn each_frame_a_page_posts_becomes_its_event() {
        let pane = p1();
        let cases = content_cases(&pane).into_iter().chain(command_cases(&pane));
        let cases = cases.chain(document_cases(&pane));
        for (frame, expected) in cases.chain(toast_cases(&pane)) {
            assert_eq!(
                post(&mut started(), &encode(&frame).unwrap()),
                [expected],
                "{frame:?}"
            );
        }
        let hover = post(
            &mut started(),
            r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#,
        );
        let pane = p1();
        assert_eq!(
            hover,
            [HostEvent::HoverChanged {
                pane,
                hovered: true
            }],
            "hover"
        );
    }

    #[test]
    fn a_page_that_registers_is_mounted_and_one_of_the_wrong_type_fails_hidden() {
        let mut f = started();
        assert!(!f.host.is_mounted(&p1()), "not before it registers");
        assert_eq!(
            register(&mut f, "p1", "core.chat"),
            [HostEvent::Mounted { pane: p1() }]
        );
        assert!(f.host.is_mounted(&p1()), "mounted");
        let mut f = started();
        let before = f.calls();
        assert!(
            register(&mut f, "p1", "core.term").is_empty(),
            "the wrong type"
        );
        assert_eq!(f.view_calls(before), ["hide"], "web view hidden");
        assert!(
            lifecycle_events(&f.mock).is_empty() && !f.host.is_mounted(&p1()),
            "never mounted"
        );
    }

    #[test]
    fn stale_messages_from_a_pane_without_a_web_view_are_ignored() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        let text = encode(&PaneFrame::SetDirty { dirty: true }).unwrap();
        let stale = IpcMessage {
            pane: PaneId::new("p2").unwrap(),
            text,
        };
        assert!(f.ipc(stale).is_empty(), "p2 has no web view; ignored");
    }

    #[test]
    fn a_page_takes_native_focus_on_activation_only_while_the_keyboard_is_the_pages() {
        let cases = [
            (Keyboard::Pane, ["focus"]),
            (Keyboard::Shell, ["focus_parent"]),
        ];
        for (keyboard, wanted) in cases {
            let mut f = fixture();
            let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
            f.mount(&panes, Some("p1"));
            let before = f.calls();
            f.sync_with(&panes, &["p1", "p2"], (Some("p2"), keyboard));
            assert_eq!(f.focus_calls(before), wanted, "{keyboard:?}");
        }
    }

    #[test]
    fn a_chat_learns_whether_it_fills_its_group_on_mount_and_on_each_change_only() {
        let groups = |f: &Fixture| -> Vec<bool> {
            let frames = frames_sent(&f.mock).into_iter();
            frames
                .filter_map(|frame| match frame {
                    ShellFrame::Group { chat_fills } => Some(chat_fills),
                    _ => None,
                })
                .collect()
        };
        let mut f = fixture();
        let (chat, tab) = (instance("p1", "core.chat"), instance("t1", "core.term"));
        f.mount(std::slice::from_ref(&chat), Some("p1"));
        assert_eq!(groups(&f), [true], "alone, on mount");
        f.sync(std::slice::from_ref(&chat), &["p1"], Some("p1"));
        assert_eq!(groups(&f), [true], "nothing changed, nothing sent");
        f.sync(&[chat, tab], &["p1", "t1"], Some("t1"));
        assert_eq!(groups(&f), [true, false], "an editor tab joined");
    }
}
