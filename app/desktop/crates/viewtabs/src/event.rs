use gpui::{Pixels, Point};
use layout::{Command, TabId};

use crate::gesture::TabDrop;

/// What the tab bar asks its owner to do.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Event {
    /// Apply these commands in order; each is one step of the model.
    Apply(Vec<Command>),
    /// Show or hide the sidebar.
    ToggleSidebar,
    /// A dragged tab is outside the strip, with the pointer at `position` in window
    /// coordinates: whatever is under it may preview a drop.
    DragOut {
        /// The dragged tab.
        tab: TabId,
        /// The pointer, in window coordinates.
        position: Point<Pixels>,
    },
    /// A dragged tab was released outside the strip at `position`; the strip does not reorder.
    DropOut {
        /// The dragged tab.
        tab: TabId,
        /// The pointer, in window coordinates.
        position: Point<Pixels>,
    },
    /// A tab that was dragged out of the strip came back to it, or its drag ended without a
    /// drop: drop any preview of it.
    DragCancel,
    /// A sidebar chat row was dropped on the strip: on a tab's middle the chat joins that tab, and
    /// between tabs the chat's own tab moves there.
    DropChat {
        /// The dropped chat.
        chat: String,
        /// Where on the strip it landed.
        at: TabDrop,
    },
}
