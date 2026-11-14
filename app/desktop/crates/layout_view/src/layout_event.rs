use model::Command;

/// What a [`LayoutView`](crate::LayoutView) asks its owner to do.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum LayoutEvent {
    /// Apply this command to the layout, then give the view the new state.
    Apply(Command),
}
