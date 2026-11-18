//! Emitted by a nested menu that wants to hand focus back to its parent.

#[derive(Debug, Clone, Copy)]
pub(crate) enum MenuNavigation {
    Back,
}
