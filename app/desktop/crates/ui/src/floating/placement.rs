//! The side of its trigger a popover panel opens on.

use gpui::{Bounds, Pixels, Point, point};

use crate::floating::menu::MenuEdge;

/// Where a popover panel sits relative to its trigger.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Placement {
    /// Under the trigger, left edges aligned.
    #[default]
    Below,
    /// Under the trigger, right edges aligned.
    BelowEnd,
    /// Under the trigger, the panel's left edge at the trigger's horizontal centre.
    BelowCentre,
    /// Beside the trigger on its right, top edges aligned.
    Right,
}

impl Placement {
    pub(crate) fn beside(
        self,
        trigger: Bounds<Pixels>,
        gap: Pixels,
    ) -> (Point<Pixels>, MenuEdge) {
        let below = trigger.bottom() + gap;
        match self {
            Self::Below => (point(trigger.left(), below), MenuEdge::Start),
            Self::BelowEnd => (point(trigger.right(), below), MenuEdge::End),
            Self::BelowCentre => (point(trigger.center().x, below), MenuEdge::Start),
            Self::Right => (point(trigger.right() + gap, trigger.top()), MenuEdge::Start),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};

    use super::*;

    const GAP: f32 = 4.0;

    #[test]
    fn a_native_menu_opens_on_the_side_of_its_trigger_that_the_placement_names() {
        let trigger = Bounds::new(point(px(10.0), px(20.0)), size(px(100.0), px(30.0)));
        let cases = [
            (
                Placement::Below,
                (point(px(10.0), px(54.0)), MenuEdge::Start),
            ),
            (
                Placement::BelowEnd,
                (point(px(110.0), px(54.0)), MenuEdge::End),
            ),
            (
                Placement::BelowCentre,
                (point(px(60.0), px(54.0)), MenuEdge::Start),
            ),
            (
                Placement::Right,
                (point(px(114.0), px(20.0)), MenuEdge::Start),
            ),
        ];
        for (placement, want) in cases {
            assert_eq!(placement.beside(trigger, px(GAP)), want, "{placement:?}");
        }
    }
}
