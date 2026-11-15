//! The side of its trigger a popover panel opens on.

/// Where a popover panel sits relative to its trigger.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Placement {
    /// Under the trigger, left edges aligned.
    #[default]
    Below,
    /// Beside the trigger on its right, top edges aligned.
    Right,
}
