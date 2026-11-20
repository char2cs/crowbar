//! What a chord button reads: its text and its look.

use gpui::SharedString;

use crate::modal::pages::shortcuts::chord_look::ChordLook;

pub(crate) struct ChordFace {
    pub(crate) text: SharedString,
    pub(crate) look: ChordLook,
}
