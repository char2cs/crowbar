//! A row drag as the pointer drives it: a press on the subject, then moves over a target row
//! laid out at a fixed height.

use gpui::{Bounds, Context, point, px, size};
use sidebar::RowId;

use crate::sidebar::{DragPoint, DragPress, Sidebar};

pub(crate) const ROW_TOP: f32 = 200.0;

pub(crate) fn over_at(
    rows: (&str, &str),
    top: f32,
    at: (f32, f32),
) -> DragPoint {
    DragPoint {
        subject: RowId::from(rows.0),
        target: RowId::from(rows.1),
        bounds: Bounds::new(point(px(0.0), px(top)), size(px(200.0), px(36.0))),
        at: point(px(at.0), px(top + at.1)),
    }
}

/// Presses `rows.0` and moves the pointer to `at` within `rows.1`.
pub(crate) fn drag(
    this: &mut Sidebar,
    rows: (&str, &str),
    at: (f32, f32),
    cx: &mut Context<'_, Sidebar>,
) {
    this.press_row(DragPress {
        row: RowId::from(rows.0),
        at: point(px(0.0), px(ROW_TOP)),
    });
    this.drag_over(&over_at(rows, ROW_TOP, at), cx);
}

/// Lifts the `working` chat over the `folder` row of the rich data.
pub(crate) fn lift_a_row(
    this: &mut Sidebar,
    cx: &mut Context<'_, Sidebar>,
) {
    drag(this, ("working", "folder"), (20.0, 18.0), cx);
}
