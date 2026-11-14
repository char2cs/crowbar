//! Which edge of the trigger a tooltip opens from.

/// Placement of a tooltip relative to its trigger.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TooltipSide {
    /// Below the trigger, centred; the sidebar header uses this.
    #[default]
    Bottom,
    /// Above the trigger, centred.
    Top,
}
