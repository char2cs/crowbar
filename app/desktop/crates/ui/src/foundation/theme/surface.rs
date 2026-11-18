//! Which palette region a control is painted in.

/// Region of the window a control sits in.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum Surface {
    /// The canvas, popups and dialogs: the palette of the active mode.
    #[default]
    Window,
    /// The sidebar and the view-tabs row (`.chrome-surface`): crowbar keeps them dark in light mode.
    Chrome,
}
