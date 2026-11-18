//! One frame's worth of data between layout and paint.

use terminal::Snapshot;

use crate::shaping::Shaping;
use crate::view_layout::ViewLayout;

pub(crate) struct Frame {
    pub(crate) snapshot: Snapshot,
    pub(crate) layout: ViewLayout,
    pub(crate) shaping: Shaping,
}
