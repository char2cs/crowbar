use gpui::{Bounds, Pixels};

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum WorkspaceEvent {
    Measured(Bounds<Pixels>),
}
