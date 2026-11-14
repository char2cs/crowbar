use std::collections::BTreeMap;
use std::time::Instant;

use protocol::PaneId;
use ui::{CubicBezier, Glide, Motion, timing};

#[derive(Debug, Default)]
pub(crate) struct PaneFades {
    fades: BTreeMap<PaneId, Glide>,
}

impl PaneFades {
    pub(crate) fn show(&mut self, pane: &PaneId, now: Instant, motion: Motion) {
        self.fade(pane).go_to(1.0, now, motion);
    }

    pub(crate) fn hide_all_but(&mut self, keep: Option<&PaneId>, now: Instant, motion: Motion) {
        for (_, fade) in self
            .fades
            .iter_mut()
            .filter(|(pane, _)| Some(*pane) != keep)
        {
            fade.go_to(0.0, now, motion);
        }
    }

    pub(crate) fn opacity(&self, pane: &PaneId, now: Instant, motion: Motion) -> f32 {
        self.fades
            .get(pane)
            .map_or(0.0, |fade| fade.value(now, motion))
    }

    pub(crate) fn is_running(&self, now: Instant, motion: Motion) -> bool {
        self.fades.values().any(|fade| fade.is_running(now, motion))
    }

    pub(crate) fn retain(&mut self, panes: &[PaneId]) {
        self.fades.retain(|pane, _| panes.contains(pane));
    }

    fn fade(&mut self, pane: &PaneId) -> &mut Glide {
        self.fades
            .entry(pane.clone())
            .or_insert_with(|| Glide::resting(0.0, timing::DEFAULT, CubicBezier::DEFAULT))
    }
}

#[cfg(test)]
#[path = "pane_fades_tests.rs"]
mod tests;
