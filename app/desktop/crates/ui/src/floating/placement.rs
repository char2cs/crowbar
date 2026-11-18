//! The side of its trigger a popover panel opens on.

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
