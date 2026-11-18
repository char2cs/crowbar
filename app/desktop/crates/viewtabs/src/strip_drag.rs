use layout::{Command, TabId};

use crate::span::Span;

const HYSTERESIS: f32 = 6.0;
const BAND_MARGIN: f32 = 32.0;

/// A tab being dragged along the strip: it follows the pointer, the others slide aside, and
/// releasing drops it into the gap. Pure geometry; the strip view feeds it pointer positions.
#[derive(Debug, Clone, PartialEq)]
pub struct StripDrag {
    tabs: Vec<TabId>,
    spans: Vec<Span>,
    dragged: usize,
    grab: f32,
    target: usize,
}

impl StripDrag {
    /// Starts dragging the tab at `dragged` among `tabs` laid out in `spans`, grabbed at
    /// pointer `x`; the slots must be the strip's content coordinates, scroll included.
    #[must_use]
    pub fn begin(
        tabs: Vec<TabId>,
        spans: Vec<Span>,
        dragged: usize,
        x: f32,
    ) -> Option<Self> {
        let slot = spans.get(dragged)?;
        (tabs.len() == spans.len()).then_some(Self {
            grab: x - slot.x,
            tabs,
            spans,
            dragged,
            target: dragged,
        })
    }

    /// The index the tab would land on if released now.
    #[must_use]
    pub fn target(&self) -> usize {
        self.target
    }

    /// Left edge of the dragged tab for pointer `x`, held between the first and last slot.
    #[must_use]
    pub fn ghost_x(
        &self,
        x: f32,
    ) -> f32 {
        let (Some(first), Some(last)) = (self.spans.first(), self.spans.last()) else {
            return x;
        };
        let width = self.width();
        (x - self.grab).clamp(first.x, (last.end() - width).max(first.x))
    }

    /// Top of the ghost: pinned to the strip row while the pointer is near it, easing toward the
    /// free pointer across 32px so leaving the band has no jump.
    #[must_use]
    pub fn ghost_y(
        &self,
        pointer_y: f32,
        row_top: f32,
        grab_y: f32,
    ) -> f32 {
        let delta = pointer_y - grab_y - row_top;
        row_top + delta * (delta.abs() / BAND_MARGIN).clamp(0.0, 1.0)
    }

    /// Moves the pointer to `x`; the target changes hands once the ghost has passed a
    /// neighbour's middle by 6px.
    pub fn moved(
        &mut self,
        x: f32,
    ) {
        let ghost = self.ghost_x(x);
        let width = self.width();
        while self.step(ghost, width) {}
    }

    /// How far slot `index` is shifted to make room: the dragged tab's width for the tabs it has
    /// passed, zero for the rest and for the dragged tab itself.
    #[must_use]
    pub fn offset(
        &self,
        index: usize,
    ) -> f32 {
        let width = self.width();
        if index > self.dragged && index <= self.target {
            return -width;
        }
        if index < self.dragged && index >= self.target {
            return width;
        }
        0.0
    }

    /// The command that performs the drop, or `None` when the tab is back where it started.
    #[must_use]
    pub fn drop(&self) -> Option<Command> {
        let tab = *self.tabs.get(self.dragged)?;
        (self.target != self.dragged).then_some(Command::MoveTab {
            tab,
            to_index: self.target,
        })
    }

    /// The tab being dragged.
    #[must_use]
    pub fn tab(&self) -> Option<TabId> {
        self.tabs.get(self.dragged).copied()
    }

    fn width(&self) -> f32 {
        self.spans.get(self.dragged).map_or(0.0, |span| span.width)
    }

    fn step(
        &mut self,
        ghost: f32,
        width: f32,
    ) -> bool {
        let Some(home) = self.spans.get(self.target) else {
            return false;
        };
        let reach = width / 2.0 + HYSTERESIS;
        if ghost > home.x + reach && self.target + 1 < self.spans.len() {
            self.target += 1;
            return true;
        }
        if ghost < home.x - reach && self.target > 0 {
            self.target -= 1;
            return true;
        }
        false
    }
}

#[cfg(test)]
mod tests {
    use layout::{Command, TabId};

    use super::*;

    fn strip(
        count: u64,
        width: f32,
    ) -> (Vec<TabId>, Vec<Span>) {
        let tabs = (0..count).map(TabId).collect();
        let spans = (0..count)
            .map(|index| Span {
                x: f32::from(u16::try_from(index).expect("small")) * width,
                width,
            })
            .collect();
        (tabs, spans)
    }

    struct Case {
        name: &'static str,
        dragged: usize,
        grab: f32,
        moves: &'static [f32],
        target: usize,
        drop_to: Option<usize>,
        ghosts: &'static [(f32, f32)],
        offsets: Option<[f32; 4]>,
    }

    const CASES: &[Case] = &[
        Case {
            name: "pressing a tab does not move it",
            dragged: 1,
            grab: 150.0,
            moves: &[],
            target: 1,
            drop_to: None,
            ghosts: &[(150.0, 100.0)],
            offsets: None,
        },
        Case {
            name: "the ghost keeps its grab offset and stays over the tabs",
            dragged: 1,
            grab: 130.0,
            moves: &[],
            target: 1,
            drop_to: None,
            ghosts: &[(180.0, 150.0), (-500.0, 0.0), (900.0, 300.0)],
            offsets: None,
        },
        Case {
            name: "55px past a neighbour's middle is inside the hysteresis",
            dragged: 1,
            grab: 150.0,
            moves: &[205.0],
            target: 1,
            drop_to: None,
            ghosts: &[],
            offsets: None,
        },
        Case {
            name: "57px past a neighbour's middle swaps places",
            dragged: 1,
            grab: 150.0,
            moves: &[207.0],
            target: 2,
            drop_to: Some(2),
            ghosts: &[],
            offsets: None,
        },
        Case {
            name: "a long move crosses several tabs",
            dragged: 0,
            grab: 50.0,
            moves: &[390.0],
            target: 3,
            drop_to: Some(3),
            ghosts: &[],
            offsets: None,
        },
        Case {
            name: "coming back to the start drops nothing",
            dragged: 0,
            grab: 50.0,
            moves: &[390.0, 10.0],
            target: 0,
            drop_to: None,
            ghosts: &[],
            offsets: None,
        },
        Case {
            name: "neighbours slide left toward the gap when the tab moves right",
            dragged: 1,
            grab: 150.0,
            moves: &[350.0],
            target: 3,
            drop_to: Some(3),
            ghosts: &[],
            offsets: Some([0.0, 0.0, -100.0, -100.0]),
        },
        Case {
            name: "neighbours slide right toward the gap when the tab moves left",
            dragged: 3,
            grab: 350.0,
            moves: &[120.0],
            target: 1,
            drop_to: Some(1),
            ghosts: &[],
            offsets: Some([0.0, 100.0, 100.0, 0.0]),
        },
    ];

    fn drag(
        dragged: usize,
        x: f32,
    ) -> StripDrag {
        let (tabs, spans) = strip(4, 100.0);
        StripDrag::begin(tabs, spans, dragged, x).expect("a drag")
    }

    fn follow(case: &Case) -> StripDrag {
        let mut drag = drag(case.dragged, case.grab);
        for x in case.moves {
            drag.moved(*x);
        }
        drag
    }

    fn offsets_of(drag: &StripDrag) -> Vec<f32> {
        (0..4).map(|index| drag.offset(index)).collect()
    }

    #[test]
    fn a_drag_follows_the_pointer_to_the_slot_it_would_drop_into() {
        for case in CASES {
            let drag = follow(case);
            assert_eq!(drag.target(), case.target, "{}", case.name);
            assert_eq!(drag.drop(), expected_drop(case), "{}", case.name);
            assert_ghosts(&drag, case);
            assert_eq!(
                case.offsets.map(|_| offsets_of(&drag)),
                case.offsets.map(Vec::from),
                "{}",
                case.name
            );
        }
    }

    fn expected_drop(case: &Case) -> Option<Command> {
        let tab = TabId(u64::try_from(case.dragged).expect("small"));
        case.drop_to
            .map(|to_index| Command::MoveTab { tab, to_index })
    }

    fn assert_ghosts(
        drag: &StripDrag,
        case: &Case,
    ) {
        let actual: Vec<f32> = case.ghosts.iter().map(|(x, _)| drag.ghost_x(*x)).collect();
        let expected: Vec<f32> = case.ghosts.iter().map(|(_, ghost)| *ghost).collect();
        assert_eq!(actual, expected, "{}", case.name);
    }

    #[test]
    fn the_dragged_tab_is_reported() {
        let drag = drag(2, 250.0);
        assert_eq!(drag.tab(), Some(TabId(2)), "the dragged tab");
    }

    #[test]
    fn begin_refuses_mismatched_input() {
        let (tabs, spans) = strip(2, 100.0);
        assert!(
            StripDrag::begin(tabs.clone(), spans.clone(), 5, 0.0).is_none(),
            "index out of range"
        );
        assert!(
            StripDrag::begin(tabs, spans[..1].to_vec(), 0, 0.0).is_none(),
            "length mismatch"
        );
    }

    #[test]
    fn the_ghost_is_pinned_to_the_row_near_it_and_free_far_from_it() {
        let drag = drag(0, 10.0);
        assert_eq!(drag.ghost_y(26.0, 5.0, 21.0), 5.0, "level with the row");
        let eased = drag.ghost_y(26.0 + 16.0, 5.0, 21.0);
        assert!(eased > 5.0 && eased < 21.0, "halfway eased: {eased}");
        assert_eq!(
            drag.ghost_y(26.0 + 200.0, 5.0, 21.0),
            205.0,
            "free beyond the margin"
        );
    }
}
