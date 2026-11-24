use protocol::PaneId;

use crate::hover_source::HoverSource;

#[derive(Debug, Default)]
pub(crate) struct PaneHover {
    over: Vec<(HoverSource, PaneId)>,
}

impl PaneHover {
    pub(crate) fn entered(
        &mut self,
        source: HoverSource,
        pane: &PaneId,
    ) {
        self.over.retain(|(known, _)| *known != source);
        self.over.push((source, pane.clone()));
    }

    pub(crate) fn left(
        &mut self,
        source: HoverSource,
        pane: &PaneId,
    ) {
        self.over
            .retain(|(known, over)| *known != source || over != pane);
    }

    pub(crate) fn pane(&self) -> Option<&PaneId> {
        self.over.last().map(|(_, pane)| pane)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::pane_id;

    enum Step {
        Enter(HoverSource, &'static str),
        Leave(HoverSource, &'static str),
    }

    type Case = (&'static str, &'static [Step], Option<&'static str>);

    const CASES: &[Case] = &[
        (
            "a pane stays hovered while the pointer is on its floating handle",
            &[
                Step::Enter(HoverSource::Cell, "a"),
                Step::Enter(HoverSource::Handle, "a"),
                Step::Leave(HoverSource::Cell, "a"),
            ],
            Some("a"),
        ),
        (
            "leaving the handle for nothing ends the hover",
            &[
                Step::Enter(HoverSource::Handle, "a"),
                Step::Leave(HoverSource::Handle, "a"),
            ],
            None,
        ),
        (
            "a late leave of the old pane keeps the new one hovered",
            &[
                Step::Enter(HoverSource::Page, "a"),
                Step::Enter(HoverSource::Page, "b"),
                Step::Leave(HoverSource::Page, "a"),
            ],
            Some("b"),
        ),
        (
            "a page entered later stands for the pointer, not one that never said it left",
            &[
                Step::Enter(HoverSource::Page, "a"),
                Step::Enter(HoverSource::Page, "b"),
                Step::Leave(HoverSource::Page, "b"),
            ],
            None,
        ),
        (
            "the shell's own leave of a pane does not end the page's hover of it",
            &[
                Step::Enter(HoverSource::Cell, "a"),
                Step::Enter(HoverSource::Page, "a"),
                Step::Leave(HoverSource::Cell, "a"),
            ],
            Some("a"),
        ),
    ];

    fn applied(steps: &[Step]) -> PaneHover {
        let mut hover = PaneHover::default();
        for step in steps {
            match step {
                Step::Enter(source, pane) => hover.entered(*source, &pane_id(pane)),
                Step::Leave(source, pane) => hover.left(*source, &pane_id(pane)),
            }
        }
        hover
    }

    #[test]
    fn the_last_report_still_standing_names_the_hovered_pane() {
        for (name, steps, expected) in CASES {
            let hover = applied(steps);
            assert_eq!(hover.pane(), expected.map(pane_id).as_ref(), "{name}");
        }
    }
}
