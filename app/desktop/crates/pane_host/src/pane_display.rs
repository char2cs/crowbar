use protocol::PaneTypeId;

/// What the shell draws in a pane's rectangle.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PaneDisplay {
    /// The pane's web view covers the rectangle.
    Web,
    /// No web view right now (parked, hidden or still being assigned); draw the background.
    Empty,
    /// The pane's type is not registered any more; draw a placeholder. State is kept.
    Placeholder {
        /// The missing type.
        type_id: PaneTypeId,
    },
    /// The pane failed; draw the message inside its frame.
    Error {
        /// What went wrong.
        message: String,
    },
}
