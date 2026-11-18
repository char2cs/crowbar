use std::collections::BTreeMap;
use std::time::Instant;

use protocol::PaneId;
use ui::{CubicBezier, Glide, Motion, timing};

#[derive(Debug, Default)]
pub(crate) struct PaneFades {
    fades: BTreeMap<PaneId, Glide>,
}

impl PaneFades {
    pub(crate) fn show(
        &mut self,
        pane: &PaneId,
        now: Instant,
        motion: Motion,
    ) {
        self.fade(pane).go_to(1.0, now, motion);
    }

    pub(crate) fn hide_all_but(
        &mut self,
        keep: Option<&PaneId>,
        now: Instant,
        motion: Motion,
    ) {
        for (_, fade) in self
            .fades
            .iter_mut()
            .filter(|(pane, _)| Some(*pane) != keep)
        {
            fade.go_to(0.0, now, motion);
        }
    }

    pub(crate) fn opacity(
        &self,
        pane: &PaneId,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.fades
            .get(pane)
            .map_or(0.0, |fade| fade.value(now, motion))
    }

    pub(crate) fn is_running(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.fades.values().any(|fade| fade.is_running(now, motion))
    }

    pub(crate) fn retain(
        &mut self,
        panes: &[PaneId],
    ) {
        self.fades.retain(|pane, _| panes.contains(pane));
    }

    fn fade(
        &mut self,
        pane: &PaneId,
    ) -> &mut Glide {
        self.fades
            .entry(pane.clone())
            .or_insert_with(|| Glide::resting(0.0, timing::DEFAULT, CubicBezier::DEFAULT))
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use ui::Motion;

    use super::*;
    use crate::test_support::pane_id;

    const FULL: Duration = Duration::from_millis(150);

    struct Case {
        name: &'static str,
        shown: &'static [&'static str],
        keep: Option<&'static str>,
        opacities: &'static [(&'static str, f32)],
    }

    const CASES: &[Case] = &[
        Case {
            name: "a pane fades out again when nothing is kept",
            shown: &["a"],
            keep: None,
            opacities: &[("a", 0.0)],
        },
        Case {
            name: "hide_all_but keeps only the named pane",
            shown: &["a", "b"],
            keep: Some("a"),
            opacities: &[("a", 1.0), ("b", 0.0)],
        },
        Case {
            name: "an unknown pane is invisible",
            shown: &[],
            keep: None,
            opacities: &[("x", 0.0)],
        },
    ];

    fn assert_opacities(
        fades: &PaneFades,
        case: &Case,
        at: Instant,
    ) {
        for (name, expected) in case.opacities {
            let actual = fades.opacity(&pane_id(name), at, Motion::default());
            assert_eq!(actual, *expected, "{}: {name}", case.name);
        }
    }

    fn hidden_but_kept(
        case: &Case,
        start: Instant,
    ) -> PaneFades {
        let mut fades = PaneFades::default();
        for name in case.shown {
            fades.show(&pane_id(name), start, Motion::default());
        }
        let keep = case.keep.map(pane_id);
        fades.hide_all_but(keep.as_ref(), start + FULL, Motion::default());
        fades
    }

    #[test]
    fn hiding_all_but_the_kept_pane_fades_the_others_out() {
        for case in CASES {
            let start = Instant::now();
            let fades = hidden_but_kept(case, start);
            assert_opacities(&fades, case, start + FULL * 2);
        }
    }

    #[test]
    fn a_pane_fades_in_over_150_ms() {
        let start = Instant::now();
        let mut fades = PaneFades::default();
        fades.show(&pane_id("a"), start, Motion::default());
        let at = |when| fades.opacity(&pane_id("a"), when, Motion::default());
        assert_eq!(at(start), 0.0, "starts clear");
        assert_eq!(at(start + FULL), 1.0, "ends shown");
    }

    #[test]
    fn retain_forgets_closed_panes() {
        let start = Instant::now();
        let mut fades = PaneFades::default();
        fades.show(&pane_id("a"), start, Motion::default());
        fades.retain(&[pane_id("b")]);
        assert!(
            !fades.is_running(start, Motion::default()),
            "nothing left to run"
        );
    }
}
