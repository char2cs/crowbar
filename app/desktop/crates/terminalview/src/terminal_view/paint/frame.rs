//! One frame's worth of data between layout and paint.

use terminal::Snapshot;

use crate::terminal_view::paint::shaping::Shaping;
use crate::terminal_view::paint::view_layout::ViewLayout;

pub(crate) struct Frame {
    pub(crate) snapshot: Snapshot,
    pub(crate) layout: ViewLayout,
    pub(crate) shaping: Shaping,
}
