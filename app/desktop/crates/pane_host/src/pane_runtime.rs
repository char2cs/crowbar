use model::PaneInstance;
use protocol::{Lifecycle, PaneState, ShellFrame};

use crate::pane_phase::PanePhase;
use crate::slot_id::SlotId;
use crate::stage::Stage;

#[derive(Debug)]
pub(crate) struct PaneRuntime {
    pub(crate) instance: PaneInstance,
    pub(crate) phase: PanePhase,
    pub(crate) slot: Option<SlotId>,
    pub(crate) stage: Stage,
    pub(crate) discard_state: bool,
}

impl PaneRuntime {
    pub(crate) fn new(instance: PaneInstance) -> Self {
        Self {
            instance,
            phase: PanePhase::Parked,
            slot: None,
            stage: Stage::Hidden,
            discard_state: false,
        }
    }

    pub(crate) fn lifecycle_frame(&self, event: Lifecycle) -> ShellFrame {
        let state = match (event, self.discard_state) {
            (Lifecycle::Mount, true) => PaneState::default(),
            _ => self.instance.state.clone(),
        };
        ShellFrame::Lifecycle {
            pane_id: self.instance.pane_id.clone(),
            event,
            params: self.instance.params.clone(),
            state,
        }
    }

    pub(crate) fn reveal(&mut self) {
        if self.stage == Stage::Hidden {
            self.stage = Stage::Shown;
        }
    }

    pub(crate) fn is_visible(&self) -> bool {
        self.stage != Stage::Hidden
    }

    pub(crate) fn is_mounted(&self) -> bool {
        self.phase == PanePhase::Mounted
    }

    pub(crate) fn detach(&mut self) {
        self.slot = None;
        self.stage = Stage::Hidden;
        self.phase = PanePhase::Parked;
    }
}

#[cfg(test)]
#[path = "pane_runtime_tests.rs"]
mod tests;
