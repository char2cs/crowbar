use gpui::{FocusHandle, SharedString};

use crate::modal::pages::shortcuts::chord_look::ChordLook;

pub(crate) struct ChordFace {
    pub(crate) text: SharedString,
    pub(crate) look: ChordLook,
    pub(crate) focus: FocusHandle,
}
