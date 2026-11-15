use protocol::{
    ExtensionStatus, Lifecycle, PaneFrame, PaneId, PaneState, PaneTypeId, ShellFrame, StatusKind,
};

use crate::error::Result;
use crate::host_event::HostEvent;
use crate::pane_host::PaneHost;
use crate::pane_phase::PanePhase;
use crate::pane_runtime::PaneRuntime;
use crate::stage::Stage;

fn wanted_stage(runtime: &PaneRuntime, id: &PaneId, focused: Option<&PaneId>) -> Stage {
    if runtime.is_mounted() && focused == Some(id) {
        Stage::Active
    } else {
        Stage::Shown
    }
}

impl PaneHost {
    pub(crate) fn route_activation(&mut self) -> Result<()> {
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
            .try_for_each(|(pane, stage)| self.set_activation(&pane, stage))
    }

    pub(crate) fn set_activation(&mut self, pane: &PaneId, stage: Stage) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.stage = stage;
        let event = if stage == Stage::Active {
            Lifecycle::Activate
        } else {
            Lifecycle::Deactivate
        };
        let frame = runtime.lifecycle_frame(event);
        let slot = runtime.slot;
        self.send(pane, &frame)?;
        match (stage, slot.and_then(|id| self.pool.get(id))) {
            (Stage::Active, Some(slot)) => Ok(slot.handle.focus()?),
            _ => Ok(()),
        }
    }

    fn remember_state(&mut self, pane: &PaneId, state: &PaneState) {
        if let Some(runtime) = self.panes.get_mut(pane) {
            runtime.instance.state = state.clone();
        }
    }

    pub(crate) fn on_frame(&mut self, pane: PaneId, frame: PaneFrame) -> Result<Vec<HostEvent>> {
        let event = match frame {
            PaneFrame::Register { type_id, .. } => {
                return self.on_register(&pane, &type_id).map(|()| Vec::new());
            },
            PaneFrame::SetTitle { title } => HostEvent::TitleChanged { pane, title },
            PaneFrame::SetState { state } => {
                self.remember_state(&pane, &state);
                HostEvent::StateChanged { pane, state }
            },
            PaneFrame::Open { type_id, params } => HostEvent::OpenRequested {
                from: pane,
                type_id,
                params,
            },
            PaneFrame::Request { id, method, params } => HostEvent::RequestMade {
                pane,
                id,
                method,
                params,
            },
            PaneFrame::Subscribe { id, event } => HostEvent::Subscribed { pane, id, event },
            PaneFrame::Unsubscribe { id } => HostEvent::Unsubscribed { pane, id },
            PaneFrame::RunCommand { command } => HostEvent::CommandRequested { pane, command },
            frame @ (PaneFrame::SettingsGet { .. }
            | PaneFrame::SettingsSubscribe { .. }
            | PaneFrame::ComposerAttach { .. }
            | PaneFrame::ComposerDraft { .. }
            | PaneFrame::AttachmentOpen { .. }) => HostEvent::ShellCall { pane, frame },
        };
        Ok(vec![event])
    }

    fn on_register(&mut self, pane: &PaneId, type_id: &PaneTypeId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if &runtime.instance.type_id != type_id {
            let message = format!(
                "page registered as {type_id} but was opened as {}",
                runtime.instance.type_id
            );
            return self.fail(pane, message);
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(Lifecycle::Mount);
        runtime.discard_state = false;
        self.send(pane, &mount)?;
        if !self.keybindings.is_empty() {
            self.send(
                pane,
                &ShellFrame::Keybindings {
                    bindings: self.keybindings.clone(),
                },
            )?;
        }
        self.route_activation()
    }

    pub(crate) fn fail(&mut self, pane: &PaneId, message: String) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        runtime.phase = PanePhase::Failed(message);
        runtime.stage = Stage::Shown;
        self.refresh_native_visibility(pane)
    }

    pub(crate) fn apply_status(&mut self, pane: &PaneId, status: &ExtensionStatus) -> Result<()> {
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
            StatusKind::Running | StatusKind::Back => self.recover(pane),
        }
    }

    fn recover(&mut self, pane: &PaneId) -> Result<()> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(());
        };
        if !matches!(runtime.phase, PanePhase::Failed(_)) || runtime.slot.is_none() {
            return Ok(());
        }
        runtime.phase = PanePhase::Mounted;
        let mount = runtime.lifecycle_frame(Lifecycle::Mount);
        self.send(pane, &mount)?;
        self.refresh_native_visibility(pane)
    }
}

#[cfg(test)]
#[path = "pane_host_frames_tests.rs"]
mod tests;
