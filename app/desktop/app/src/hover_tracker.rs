use protocol::PaneId;

#[derive(Debug, Default)]
pub(crate) struct HoverTracker {
    over: Vec<PaneId>,
}

impl HoverTracker {
    pub(crate) fn entered(
        &mut self,
        pane: PaneId,
    ) -> Option<PaneId> {
        self.over.retain(|known| known != &pane);
        self.over.push(pane);
        self.over.last().cloned()
    }

    pub(crate) fn left(
        &mut self,
        pane: &PaneId,
    ) -> Option<PaneId> {
        self.over.retain(|known| known != pane);
        self.over.last().cloned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    enum Step {
        Enter(&'static str),
        Leave(&'static str),
    }

    type Case = (&'static str, &'static [Step], Option<&'static str>);

    const CASES: &[Case] = &[
        (
            "a late leave of the old pane keeps the new one hovered",
            &[Step::Enter("a"), Step::Enter("b"), Step::Leave("a")],
            Some("b"),
        ),
        (
            "leaving the last pane clears the hover",
            &[Step::Enter("a"), Step::Leave("a")],
            None,
        ),
        (
            "the leave of a pane never entered is harmless",
            &[Step::Leave("a")],
            None,
        ),
        (
            "leaving the newest pane falls back to the one still entered",
            &[Step::Enter("a"), Step::Enter("b"), Step::Leave("b")],
            Some("a"),
        ),
    ];

    fn pane(id: &str) -> PaneId {
        PaneId::new(id).unwrap()
    }

    fn replay(steps: &[Step]) -> Option<PaneId> {
        let mut tracker = HoverTracker::default();
        steps.iter().fold(None, |_, step| match step {
            Step::Enter(id) => tracker.entered(pane(id)),
            Step::Leave(id) => tracker.left(&pane(id)),
        })
    }

    #[test]
    fn the_hovered_pane_follows_the_reports_whatever_their_order() {
        for (name, steps, expected) in CASES {
            assert_eq!(replay(steps), expected.map(pane), "{name}");
        }
    }
}
