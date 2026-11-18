/// What the view tells its owner.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    /// The daemon serves and the view has faded out: remove it and show the layout.
    HandedOver,
}
