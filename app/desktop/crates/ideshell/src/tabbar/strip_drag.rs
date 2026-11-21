use gpui::{Pixels, Point};
use protocol::PaneId;

const ACTIVATION: f32 = 6.0;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct StripDrag {
    pane: PaneId,
    from: usize,
    press: Point<Pixels>,
    armed: bool,
    target: usize,
    centres: Vec<Pixels>,
}

impl StripDrag {
    pub(crate) fn new(
        (pane, from): (PaneId, usize),
        press: Point<Pixels>,
        centres: Vec<Pixels>,
    ) -> Self {
        Self {
            pane,
            from,
            press,
            armed: false,
            target: from,
            centres,
        }
    }

    pub(crate) fn moved(
        &mut self,
        pointer: Point<Pixels>,
    ) -> bool {
        let before = (self.armed, self.target);
        self.armed |= (pointer - self.press).magnitude() >= f64::from(ACTIVATION);
        if self.armed {
            self.target = self.closest(pointer.x - self.press.x);
        }
        before != (self.armed, self.target)
    }

    fn closest(
        &self,
        shift: Pixels,
    ) -> usize {
        let Some(start) = self.centres.get(self.from) else {
            return self.target;
        };
        let centre = *start + shift;
        let distance = |slot: Pixels| f32::from((slot - centre).abs());
        self.centres
            .iter()
            .enumerate()
            .min_by(|a, b| distance(*a.1).total_cmp(&distance(*b.1)))
            .map_or(self.target, |(index, _)| index)
    }

    pub(crate) fn order(
        &self,
        tabs: usize,
    ) -> Vec<usize> {
        let mut order: Vec<usize> = (0..tabs).filter(|index| *index != self.from).collect();
        let at = self.shown_target().min(order.len());
        order.insert(at, self.from);
        order
    }

    fn shown_target(&self) -> usize {
        if self.armed {
            return self.target;
        }
        self.from
    }

    pub(crate) fn lifts(
        &self,
        index: usize,
    ) -> bool {
        self.armed && index == self.from
    }

    pub(crate) fn armed(&self) -> bool {
        self.armed
    }

    pub(crate) fn landing(&self) -> Option<(PaneId, usize)> {
        let moves = self.armed && self.target != self.from;
        moves.then(|| (self.pane.clone(), self.target))
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::dragged;

    struct Case {
        name: &'static str,
        from: usize,
        moves: &'static [f32],
        order: [usize; 3],
        landing: Option<usize>,
    }

    const CASES: &[Case] = &[
        Case {
            name: "4 px is still a press",
            from: 0,
            moves: &[4.0],
            order: [0, 1, 2],
            landing: None,
        },
        Case {
            name: "6 px lifts the tab in place",
            from: 0,
            moves: &[6.0],
            order: [0, 1, 2],
            landing: None,
        },
        Case {
            name: "past the second tab's centre it takes the second slot",
            from: 0,
            moves: &[6.0, 120.0],
            order: [1, 0, 2],
            landing: Some(1),
        },
        Case {
            name: "past the last tab it takes the last slot",
            from: 0,
            moves: &[6.0, 260.0],
            order: [1, 2, 0],
            landing: Some(2),
        },
        Case {
            name: "dragged left from the end",
            from: 2,
            moves: &[-6.0, -190.0],
            order: [2, 0, 1],
            landing: Some(0),
        },
        Case {
            name: "once lifted, coming back near the press puts it back",
            from: 1,
            moves: &[60.0, 2.0],
            order: [0, 1, 2],
            landing: None,
        },
    ];

    #[test]
    fn a_dragged_tab_lifts_after_six_pixels_and_lands_on_the_closest_centre() {
        for case in CASES {
            let drag = dragged(case.from, case.moves);
            assert_eq!(drag.order(3), case.order, "{}: order", case.name);
            let landing = drag.landing().map(|(_, index)| index);
            assert_eq!(landing, case.landing, "{}: landing", case.name);
        }
    }

    #[test]
    fn only_the_lifted_tab_fades() {
        let resting = dragged(0, &[4.0]);
        let lifted = dragged(0, &[8.0]);
        assert_eq!(
            [resting.lifts(0), lifted.lifts(0), lifted.lifts(1)],
            [false, true, false],
            "faded once lifted, and only itself"
        );
    }
}
