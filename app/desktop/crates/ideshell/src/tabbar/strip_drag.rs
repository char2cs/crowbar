use gpui::{Bounds, Pixels, Point};
use protocol::PaneId;

const ACTIVATION: f32 = 6.0;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct StripDrag {
    pane: PaneId,
    from: usize,
    press: Point<Pixels>,
    armed: bool,
    target: usize,
    slots: Vec<Bounds<Pixels>>,
    origins: Vec<Pixels>,
}

impl StripDrag {
    pub(crate) fn new(
        (pane, from): (PaneId, usize),
        press: Point<Pixels>,
        slots: Vec<Bounds<Pixels>>,
    ) -> Self {
        let origins = slots.iter().map(Bounds::left).collect();
        Self {
            pane,
            from,
            press,
            armed: false,
            target: from,
            slots,
            origins,
        }
    }

    pub(crate) fn moved(
        &mut self,
        pointer: Point<Pixels>,
    ) -> bool {
        let before = (self.armed, self.target);
        self.armed |= (pointer - self.press).magnitude() >= f64::from(ACTIVATION);
        if self.armed {
            self.retarget(self.closest(pointer.x - self.press.x));
        }
        before != (self.armed, self.target)
    }

    fn retarget(
        &mut self,
        target: usize,
    ) {
        if target == self.target {
            return;
        }
        let before = self.lefts();
        self.target = target;
        let after = self.lefts();
        let moved = before
            .into_iter()
            .zip(after)
            .map(|(was, now)| (was != now).then_some(was));
        for (origin, was) in self.origins.iter_mut().zip(moved) {
            *origin = was.unwrap_or(*origin);
        }
    }

    fn lefts(&self) -> Vec<Pixels> {
        let (Some(first), gap) = (self.slots.first(), self.gap()) else {
            return Vec::new();
        };
        let advance = |index: usize| {
            self.slots
                .get(index)
                .map_or(Pixels::ZERO, |slot| slot.size.width + gap)
        };
        let mut placed: Vec<(usize, Pixels)> = self
            .order(self.slots.len())
            .into_iter()
            .scan(first.left(), |left, index| {
                let at = *left;
                *left += advance(index);
                Some((index, at))
            })
            .collect();
        placed.sort_by_key(|(index, _)| *index);
        placed.into_iter().map(|(_, left)| left).collect()
    }

    fn gap(&self) -> Pixels {
        match self.slots.as_slice() {
            [first, second, ..] => second.left() - first.right(),
            _ => Pixels::ZERO,
        }
    }

    pub(crate) fn slide(
        &self,
        index: usize,
    ) -> Option<(usize, Pixels)> {
        let order = self.order(self.slots.len());
        let seat = order.iter().position(|shown| *shown == index)?;
        let now = self.lefts().get(index).copied()?;
        let origin = self.origins.get(index).copied()?;
        Some((seat, origin - now))
    }

    fn closest(
        &self,
        shift: Pixels,
    ) -> usize {
        let Some(start) = self.slots.get(self.from) else {
            return self.target;
        };
        let centre = start.center().x + shift;
        let distance = |slot: &Bounds<Pixels>| f32::from((slot.center().x - centre).abs());
        self.slots
            .iter()
            .enumerate()
            .min_by(|a, b| distance(a.1).total_cmp(&distance(b.1)))
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
    fn a_shifted_tab_slides_from_the_slot_it_last_left_and_a_still_one_keeps_its_course() {
        let slides =
            |drag: &super::StripDrag| (0..3).map(|index| drag.slide(index)).collect::<Vec<_>>();
        let seat = |at: usize, offset: f32| Some((at, gpui::px(offset)));
        let past_one = dragged(0, &[6.0, 120.0]);
        assert_eq!(
            slides(&past_one),
            [seat(1, -100.0), seat(0, 100.0), seat(2, 0.0)],
            "the lifted tab and the passed one swap slots"
        );
        let past_both = dragged(0, &[6.0, 120.0, 260.0]);
        assert_eq!(
            slides(&past_both),
            [seat(2, -100.0), seat(0, 100.0), seat(1, 100.0)],
            "the first passed tab still slides from where it stood"
        );
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
